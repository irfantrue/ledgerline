package broker

import (
	"context"
	"time"
)

// MessageBroker defines the interface for interacting with a message broker service.
type MessageBroker interface {
	// Publish sends a message to the specified exchange with a routing key.
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error

	// Consume listens to a queue, processes messages using the handler, and handles timeouts.
	Consume(
		ctx context.Context,
		queue string,
		handler func(ctx context.Context, body []byte) error,
		timeout time.Duration,
	) error

	// Close terminates the connection to the message broker.
	Close() error
}
