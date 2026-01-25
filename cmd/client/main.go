package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril client...")
	usr, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatalf("error trying to process input %v", err)
	}
	connStr := "amqp://guest:guest@localhost:5672/"
	fmt.Println("Starting Peril client...")
	conn, err := amqp.Dial(connStr)
	if err != nil {
		log.Fatalf("Invalid connection! -> %v \n", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Could not create channel! Err: %v \n", err)
	}
	defer conn.Close()
	log.Printf("Succesfull connection!")

	gameState := gamelogic.NewGameState(usr)
	err = pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilDirect,
		routing.PauseKey+"."+usr,
		routing.PauseKey,
		pubsub.Transient,
		pubsub.HandlerPause(gameState),
	)
	if err != nil {
		log.Fatalf("Could not subscibe to exchange! -> %v \n", err)
	}

	err = pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilTopic,
		routing.ArmyMovesPrefix+"."+usr,
		routing.ArmyMovesPrefix+".*",
		pubsub.Transient,
		pubsub.HandlerMove(gameState, ch),
	)
	if err != nil {
		log.Fatalf("Could not bind to army moves exchange! -> %v \n", err)
	}

	err = pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilTopic,
		"war",
		routing.WarRecognitionsPrefix+".*",
		pubsub.Durable,
		pubsub.HandlerWar(gameState, ch),
	)
	if err != nil {
		log.Fatalf("Could not bind to army exchange! -> %v \n", err)
	}

	for true {
		words := gamelogic.GetInput()
		if len(words) == 0 {
			continue
		}
		cmd := words[0]
		switch cmd {
		case "spawn":
			if err := gameState.CommandSpawn(words); err != nil {
				fmt.Printf("Cannot spawn err:  %v\n", err)
				continue
			}
			fmt.Println("Pieces spawned to location!")
			continue
		case "move":
			move, err := gameState.CommandMove(words)
			if err != nil {
				fmt.Printf("Could not move -> %v \n", err)
				continue
			}
			err = pubsub.PublishJSON(
				ch,
				routing.ExchangePerilTopic,
				routing.ArmyMovesPrefix+"."+move.Player.Username,
				move,
			)
			if err != nil {
				fmt.Printf("Could not publish the move -> %v \n", err)
				continue
			}
			fmt.Printf("Pieces moved by %s: %v \n", move.Player.Username, move.Player.Units)
			continue
		case "status":
			gameState.CommandStatus()
			continue
		case "help":
			gamelogic.PrintClientHelp()
			continue
		case "quit":
			gamelogic.PrintQuit()
			return
		case "spam":
			if len(words) <= 1 {
				fmt.Println("Need another argument ie: spam 100")
				continue
			}
			times, err := strconv.Atoi(words[1])
			if err != nil {
				fmt.Printf("Invalid Number -> %v \n", err)
				continue
			}
			for range times {
				err := pubsub.PublishGameLog(ch,
					gameState.GetUsername(),
					gamelogic.GetMaliciousLog(),
				)
				if err != nil {
					fmt.Printf("Could not publish spam log -> %v \n", err)
					continue
				}
			}
			continue
		default:
			fmt.Println("That's not an actual command")
			continue
		}
	}
	if err != nil {
		log.Printf("Cannot bind queue err: %v \n", err)
		return
	}
	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	if os.Interrupt == <-signalChan {
		log.Fatalf("Program interrupted, closing!")
	}

}
