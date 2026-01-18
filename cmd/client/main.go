package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

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
		log.Fatalf("Invalid connection! Err: %v \n", err)
	}
	defer conn.Close()
	log.Printf("Succesfull connection!")
	_, _, err = pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilDirect,
		routing.PauseKey+"."+usr,
		routing.PauseKey,
		pubsub.Transient,
	)
	gameState := gamelogic.NewGameState(usr)
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
			fmt.Printf("Pieces moved: %v \n", move.Player.Units)
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
			fmt.Println("Spamming not allowed yet!")
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
