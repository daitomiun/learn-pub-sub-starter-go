package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	jsonData, err := json.Marshal(val)
	if err != nil {
		log.Printf("Could not marshall value err: %s \n", err)
		return err
	}

	if err := ch.PublishWithContext(
		context.Background(),
		exchange,
		key,
		false,
		false,
		amqp.Publishing{ContentType: "application/json", Body: jsonData},
	); err != nil {

		log.Printf("Could not publish err: %s \n", err)
		return err
	}

	return nil
}

func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	gobData, err := EncodeToGob(val)
	if err != nil {
		log.Printf("Could not marshall value err: %s \n", err)
		return err
	}

	if err := ch.PublishWithContext(
		context.Background(),
		exchange,
		key,
		false,
		false,
		amqp.Publishing{ContentType: "application/gob", Body: gobData},
	); err != nil {

		log.Printf("Could not publish gob -> %s \n", err)
		return err
	}

	return nil
}

func EncodeToGob(data any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	err := encoder.Encode(data)
	if err != nil {
		return []byte{}, err
	}
	return buffer.Bytes(), err
}

func PublishGameLog(ch *amqp.Channel, username, msg string) error {
	return PublishGob(
		ch,
		routing.ExchangePerilTopic,
		routing.GameLogSlug+"."+username,
		routing.GameLog{
			Username:    username,
			CurrentTime: time.Now(),
			Message:     msg,
		},
	)
}

type SimpleQueueType int // an enum to represent "durable" or "transient"

const (
	Durable SimpleQueueType = iota
	Transient
)

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	var durable bool
	var autoDelete bool
	var exclusive bool
	switch queueType {
	case Durable:
		durable = true
	case Transient:
		durable = false
		autoDelete = true
		exclusive = true
	default:
		return nil, amqp.Queue{}, errors.New("Invalid queue type!")
	}

	queue, err := ch.QueueDeclare(
		queueName,
		durable,
		autoDelete,
		exclusive,
		false,
		amqp.Table{"x-dead-letter-exchange": "peril_dlx"},
	)
	if err != nil {
		return nil, amqp.Queue{}, err
	}
	if err = ch.QueueBind(queueName, key, exchange, false, nil); err != nil {
		return nil, amqp.Queue{}, err
	}
	return ch, queue, nil
}

type ActType int

const (
	Ack ActType = iota
	NackRequeue
	NackDiscard
)

func subscribe[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) ActType,
	unmarshaller func([]byte) (T, error),
) error {
	ch, queue, err := DeclareAndBind(
		conn,
		exchange,
		queueName,
		key,
		queueType,
	)
	if err != nil {
		return err
	}
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	chDelivery, err := ch.Consume(
		queue.Name,
		"",
		false,
		false,
		false,
		false,
		amqp.Table{},
	)
	if err != nil {
		return err
	}

	go func() {
		for item := range chDelivery {
			message, err := unmarshaller(item.Body)
			if err != nil {
				fmt.Printf("could not unmarshal message %v\n", err)
				continue
			}
			actType := handler(message)
			switch actType {
			case Ack:
				fmt.Println("Handler acknoledged")
				item.Ack(false)
			case NackDiscard:
				fmt.Println("Handler Not acknoledged and discarded")
				item.Nack(false, false)
			case NackRequeue:
				fmt.Println("Handler Not acknoledged and requeued")
				item.Nack(false, true)
			default:
				fmt.Printf("Could not infer act type -> %v \n", actType)
			}
		}
	}()

	return nil
}

func HandlerPause(gs *gamelogic.GameState) func(routing.PlayingState) ActType {
	return func(ps routing.PlayingState) ActType {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return Ack
	}
}

func HandlerMove(gs *gamelogic.GameState, publishCh *amqp.Channel) func(gamelogic.ArmyMove) ActType {
	return func(am gamelogic.ArmyMove) ActType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(am)
		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return Ack
		case gamelogic.MoveOutcomeMakeWar:
			err := PublishJSON(
				publishCh,
				routing.ExchangePerilTopic,
				routing.WarRecognitionsPrefix+"."+gs.GetUsername(),
				gamelogic.RecognitionOfWar{
					Attacker: am.Player,
					Defender: gs.GetPlayerSnap(),
				})
			if err != nil {
				fmt.Printf("Could not process the request -> %v \n", err)
				return NackRequeue
			}
			return Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return NackDiscard
		default:
			return NackDiscard
		}
	}
}

func HandlerLogs() func(routing.GameLog) ActType {
	return func(gl routing.GameLog) ActType {
		defer fmt.Print("> ")
		err := gamelogic.WriteLog(gl)
		if err != nil {
			return NackRequeue
		}
		return Ack
	}
}

func HandlerWar(gs *gamelogic.GameState, publishCh *amqp.Channel) func(gamelogic.RecognitionOfWar) ActType {
	return func(rw gamelogic.RecognitionOfWar) ActType {
		defer fmt.Print("> ")
		warOutcome, winner, loser := gs.HandleWar(rw)
		switch warOutcome {
		case gamelogic.WarOutcomeNotInvolved:
			return NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return NackDiscard
		case gamelogic.WarOutcomeYouWon:
			err := PublishGameLog(
				publishCh,
				gs.GetUsername(),
				fmt.Sprintf("%s won a war against %s \n", winner, loser),
			)
			if err != nil {
				return NackRequeue
			}
			return Ack
		case gamelogic.WarOutcomeOpponentWon:
			err := PublishGameLog(
				publishCh,
				gs.GetUsername(),
				fmt.Sprintf("%s won a war against %s \n", winner, loser),
			)

			if err != nil {
				return NackRequeue
			}
			return Ack
		case gamelogic.WarOutcomeDraw:
			err := PublishGameLog(
				publishCh,
				gs.GetUsername(),
				fmt.Sprintf("A war between %s and %s resulted in a draw \n", winner, loser),
			)
			if err != nil {
				return NackRequeue
			}
			return Ack
		default:
			fmt.Println("Invalid war outcome!")
			return NackDiscard
		}
	}

}

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) ActType,
) error {
	return subscribe(
		conn,
		exchange,
		queueName,
		key,
		queueType,
		handler,
		unmarshallJson,
	)
}

func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
	handler func(T) ActType,
) error {
	return subscribe(
		conn,
		exchange,
		queueName,
		key,
		queueType,
		handler,
		unmarshallGob,
	)
}

func unmarshallJson[T any](data []byte) (T, error) {
	var message T
	if err := json.Unmarshal(data, &message); err != nil {
		fmt.Printf("could not unmarshal message: %v\n", err)
		return message, err
	}
	return message, nil
}

func unmarshallGob[T any](data []byte) (T, error) {
	decoder := gob.NewDecoder(bytes.NewBuffer(data))

	var message T
	err := decoder.Decode(&message)
	if err != nil {
		return message, err
	}

	return message, nil
}
