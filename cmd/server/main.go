package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	connStr := "amqp://guest:guest@localhost:5672/"
	fmt.Println("Starting Peril server...")
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
	if err := pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{IsPaused: true}); err != nil {
		log.Printf("Cannot publish json: %v \n", err)
		return
	}

	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	if os.Interrupt == <-signalChan {
		log.Fatalf("Program interrupted, closing!")
	}
}
