package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	connStr := "amqp://guest:guest@localhost:5672/"
	log.Println("Starting Peril server...")
	conn, err := amqp.Dial(connStr)
	if err != nil {
		log.Fatalf("Invalid connection! Err: %v \n", err)
	}
	defer conn.Close()
	log.Printf("Succesfull connection!")
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Could not create channel! Err: %v \n", err)
	}
	_, _, err = pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilTopic,
		routing.GameLogSlug,
		routing.GameLogSlug+".*",
		pubsub.Durable,
	)
	if err != nil {
		log.Fatalf("Could not bind! -> %v \n", err)
	}

	gamelogic.PrintServerHelp()

	for true {
		words := gamelogic.GetInput()
		if len(words) == 0 {
			continue
		}
		if words[0] == "pause" {
			log.Println("Sending Pause message!")
			if err := pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: true}); err != nil {
				log.Printf("Cannot publish json: %v \n", err)
				continue
			}
			continue
		}

		if words[0] == "resume" {
			log.Println("Sending resume message!")
			if err := pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: false}); err != nil {
				log.Printf("Cannot publish json: %v \n", err)
				continue
			}
			continue
		}

		if words[0] == "quit" {
			log.Println("Quiting the cli")
			break
		}
		log.Printf("I don't understand the command %s \n", words[0])
		continue
	}

	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	if os.Interrupt == <-signalChan {
		log.Fatalf("Program interrupted, closing!")
	}
}
