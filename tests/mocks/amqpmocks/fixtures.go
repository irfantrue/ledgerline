package amqpmocks

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// Delivery returns a delivery that reports its acknowledgements to the given
// acknowledger.
func Delivery(ack amqp.Acknowledger, body string) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Body:         []byte(body),
	}
}
