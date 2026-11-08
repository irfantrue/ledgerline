package broker

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Confirmation reports the broker's answer to a published message.
type Confirmation interface {
	// WaitContext waits for the broker's answer. It reports whether the
	// message was accepted.
	WaitContext(ctx context.Context) (bool, error)
}

// Channel is the subset of the AMQP channel API the broker uses.
//
// It is exported so the broker can be exercised with a fake channel instead
// of a running RabbitMQ. The methods mirror the amqp091-go ones, so
// *amqp.Channel does not satisfy the interface directly: amqpChan adapts it.
type Channel interface {
	// Confirm enables publisher confirms on the channel.
	Confirm(noWait bool) error

	// Qos sets the prefetch limits of the channel.
	Qos(prefetchCount, prefetchSize int, global bool) error

	// ExchangeDeclare declares an exchange.
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error

	// QueueDeclare declares a queue.
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)

	// QueueBind binds a queue to an exchange.
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error

	// NotifyReturn registers a listener for messages the broker could not
	// route. It returns the listener channel.
	NotifyReturn(receiver chan amqp.Return) chan amqp.Return

	// Publish publishes a message and returns the confirmation of it.
	Publish(ctx context.Context, exchange, routingKey string, mandatory bool, msg amqp.Publishing) (Confirmation, error)

	// Consume registers a consumer for a queue.
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)

	// IsClosed reports whether the channel is closed.
	IsClosed() bool

	// Close closes the channel.
	Close() error
}

// Connection is the subset of the AMQP connection API the broker uses.
//
// It is exported so the broker can be exercised with a fake connection
// instead of a running RabbitMQ.
type Connection interface {
	// Channel opens a new channel on the connection.
	Channel() (Channel, error)

	// Close closes the connection.
	Close() error
}

// amqpConn adapts *amqp.Connection to the Connection interface.
type amqpConn struct {
	conn *amqp.Connection
}

// Channel opens a new channel on the connection.
func (c *amqpConn) Channel() (Channel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}
	return &amqpChan{ch: ch}, nil
}

// Close closes the connection.
func (c *amqpConn) Close() error {
	return c.conn.Close()
}

// amqpChan adapts *amqp.Channel to the Channel interface.
type amqpChan struct {
	ch *amqp.Channel
}

// Confirm enables publisher confirms on the channel.
func (c *amqpChan) Confirm(noWait bool) error {
	return c.ch.Confirm(noWait)
}

// Qos sets the prefetch limits of the channel.
func (c *amqpChan) Qos(prefetchCount, prefetchSize int, global bool) error {
	return c.ch.Qos(prefetchCount, prefetchSize, global)
}

// ExchangeDeclare declares an exchange.
func (c *amqpChan) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	return c.ch.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
}

// QueueDeclare declares a queue.
func (c *amqpChan) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	return c.ch.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

// QueueBind binds a queue to an exchange.
func (c *amqpChan) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	return c.ch.QueueBind(name, key, exchange, noWait, args)
}

// NotifyReturn registers a listener for messages the broker could not route.
func (c *amqpChan) NotifyReturn(receiver chan amqp.Return) chan amqp.Return {
	return c.ch.NotifyReturn(receiver)
}

// Publish publishes a message and returns the confirmation of it.
func (c *amqpChan) Publish(ctx context.Context, exchange, routingKey string, mandatory bool, msg amqp.Publishing) (Confirmation, error) {
	conf, err := c.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, mandatory, false, msg)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// Consume registers a consumer for a queue.
func (c *amqpChan) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	return c.ch.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

// IsClosed reports whether the channel is closed.
func (c *amqpChan) IsClosed() bool {
	return c.ch.IsClosed()
}

// Close closes the channel.
func (c *amqpChan) Close() error {
	return c.ch.Close()
}
