package broker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ledgerline/internal/shared/config"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ implements MessageBroker using AMQP.
type RabbitMQ struct {
	conn           Connection
	publishChannel Channel
	consumeChannel Channel

	// returns carries the messages the broker could not route. The broker
	// sends a return before the publisher confirm of the same message, so a
	// publish can look here right after its confirm.
	returns chan amqp.Return

	// mu guards the connection and the channels above.
	mu sync.RWMutex
	// reconnectMu serialises reconnects, so two goroutines never dial at the
	// same time.
	reconnectMu sync.Mutex
	// publishMu serialises publishes, so a returned message can be matched
	// to the publish that caused it.
	publishMu sync.Mutex

	done chan struct{}
	conf config.Config
	url  string
	// dial opens the connection. It is a field so a test can supply a
	// connection that never touches the network.
	dial func(url string) (Connection, error)
}

// NewRabbitMQ creates a new RabbitMQ connection with publisher confirms enabled.
func NewRabbitMQ(conf config.Config) (MessageBroker, error) {
	return NewRabbitMQWithDialer(conf, dialAMQP)
}

// NewRabbitMQWithDialer creates a broker that opens its connections with the
// given dialer.
//
// The dialer is a parameter so the broker can be tested without a running
// RabbitMQ: a test supplies a dialer that hands out a fake connection.
func NewRabbitMQWithDialer(conf config.Config, dial func(url string) (Connection, error)) (MessageBroker, error) {
	m := &RabbitMQ{
		conf: conf,
		url:  conf.RabbitMQ.URL,
		done: make(chan struct{}),
		dial: dial,
	}

	if err := m.connect(); err != nil {
		return nil, err
	}

	slog.Info("successfully connected to the RabbitMQ. DLX topology ready.")
	return m, nil
}

// dialAMQP opens a real AMQP connection.
func dialAMQP(url string) (Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to dial RabbitMQ: %w", err)
	}
	return &amqpConn{conn: conn}, nil
}

// connect opens the connection, its channels and the DLX topology.
func (m *RabbitMQ) connect() error {
	conn, publishChannel, consumeChannel, returns, err := m.dialChannels()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.conn = conn
	m.publishChannel = publishChannel
	m.consumeChannel = consumeChannel
	m.returns = returns

	return nil
}

// dialChannels opens a connection and its channels. It deliberately runs
// without holding mu: amqp.Dial blocks on the network and must not freeze
// every other operation on the broker.
func (m *RabbitMQ) dialChannels() (Connection, Channel, Channel, chan amqp.Return, error) {
	conn, err := m.dial(m.url)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	publishChannel, err := conn.Channel()
	if err != nil {
		_ = conn.Close() // best effort: the connection is unusable anyway
		return nil, nil, nil, nil, fmt.Errorf("failed to open publish channel: %w", err)
	}

	if err := publishChannel.Confirm(false); err != nil {
		_ = publishChannel.Close() // best effort
		_ = conn.Close()           // best effort
		return nil, nil, nil, nil, fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	consumeChannel, err := conn.Channel()
	if err != nil {
		_ = publishChannel.Close() // best effort
		_ = conn.Close()           // best effort
		return nil, nil, nil, nil, fmt.Errorf("failed to open consume channel: %w", err)
	}

	if err := consumeChannel.Qos(1, 0, false); err != nil {
		_ = consumeChannel.Close() // best effort
		_ = publishChannel.Close() // best effort
		_ = conn.Close()           // best effort
		return nil, nil, nil, nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	if err := declareTopology(publishChannel); err != nil {
		_ = consumeChannel.Close() // best effort
		_ = publishChannel.Close() // best effort
		_ = conn.Close()           // best effort
		return nil, nil, nil, nil, err
	}

	// The buffer holds the returns of the publishes that are in flight plus
	// the ones a failed publish left behind. The library never blocks the
	// connection reader for longer than 5 seconds on a full channel, so a
	// stuck return cannot wedge the connection.
	returns := make(chan amqp.Return, 8)
	publishChannel.NotifyReturn(returns)

	return conn, publishChannel, consumeChannel, returns, nil
}

// declareTopology declares the Dead Letter Exchange (DLX) and all required queues
// with their DLX bindings.
func declareTopology(ch Channel) error {
	if err := ch.ExchangeDeclare("dlx.exchange", "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("failed to declare dlx exchange: %w", err)
	}

	if _, err := ch.QueueDeclare("dlx.queue", true, false, false, false, nil); err != nil {
		return fmt.Errorf("failed to declare dlx queue: %w", err)
	}

	if err := ch.QueueBind("dlx.queue", "dlx.routingKey", "dlx.exchange", false, nil); err != nil {
		return fmt.Errorf("failed to bind dlx queue: %w", err)
	}

	dlxArgs := amqp.Table{
		"x-dead-letter-exchange":    "dlx.exchange",
		"x-dead-letter-routing-key": "dlx.routingKey",
	}

	// Declare all queues that use DLX.
	// Queue naming convention: "module.action" (e.g., "notification.reset_password").
	queues := []string{}

	for _, queue := range queues {
		if _, err := ch.QueueDeclare(queue, true, false, false, false, dlxArgs); err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", queue, err)
		}
	}

	return nil
}

// Publish sends a message to the specified exchange and routing key with publisher confirms.
//
// The message is published as mandatory. When the broker cannot route it, it
// returns the message instead of dropping it silently, and Publish reports
// that as an error.
func (m *RabbitMQ) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	ch, returns, err := m.getPublishChannel()
	if err != nil {
		return err
	}

	// Publishes are serialised: a returned message can only be matched to its
	// publish when no other publish is in flight.
	m.publishMu.Lock()
	defer m.publishMu.Unlock()

	messageID := uuid.NewString()

	conf, err := ch.Publish(ctx, exchange, routingKey, true, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Body:         body,
		Timestamp:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	if ok, err := conf.WaitContext(ctx); err != nil {
		return fmt.Errorf("wait context: %w", err)
	} else if !ok {
		return fmt.Errorf("message nacked by broker")
	}

	// The broker sends the return before the confirm, so it is already waiting.
	select {
	case ret := <-returns:
		if ret.MessageId == messageID {
			return fmt.Errorf("publish failed: message returned (%d %s): exchange=%q routing_key=%q",
				ret.ReplyCode, ret.ReplyText, ret.Exchange, ret.RoutingKey)
		}
		// The return belongs to an earlier publish that gave up on it.
		slog.Error("[RABBITMQ] Message returned for an earlier publish.",
			"message_id", ret.MessageId,
			"exchange", ret.Exchange,
			"routing_key", ret.RoutingKey,
			"reason", ret.ReplyText)
	default:
	}

	return nil
}

// getPublishChannel returns the current publish channel, reconnecting when the
// broker closed it.
func (m *RabbitMQ) getPublishChannel() (Channel, chan amqp.Return, error) {
	m.mu.RLock()
	ch, returns := m.publishChannel, m.returns
	m.mu.RUnlock()

	if ch != nil && !ch.IsClosed() {
		return ch, returns, nil
	}

	// Reconnect outside the lock: amqp.Dial blocks on the network and must
	// not freeze every other operation on the broker.
	m.reconnectMu.Lock()
	defer m.reconnectMu.Unlock()

	m.mu.RLock()
	ch, returns = m.publishChannel, m.returns
	m.mu.RUnlock()

	if ch != nil && !ch.IsClosed() {
		return ch, returns, nil // another goroutine reconnected in the meantime
	}

	if err := m.reconnect(); err != nil {
		slog.Error("[RABBITMQ] failed to reconnect for publish", "error", err)
		return nil, nil, fmt.Errorf("publish failed: %w", err)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.publishChannel, m.returns, nil
}

// reconnect replaces the connection and its channels.
func (m *RabbitMQ) reconnect() error {
	conn, publishChannel, consumeChannel, returns, err := m.dialChannels()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Release the previous connection. Overwriting the pointers without
	// closing them leaks a TCP connection and its reader goroutine on every
	// reconnect. Consumers that ran on the old channel stop and have to be
	// started again.
	if err := m.closeLocked(); err != nil {
		slog.Error("[RABBITMQ] Failed to close the previous connection.", slog.Any("error", err))
	}

	m.conn = conn
	m.publishChannel = publishChannel
	m.consumeChannel = consumeChannel
	m.returns = returns

	return nil
}

// Consume starts a consumer for the given queue. Messages are processed with the provided
// function. If processing exceeds the timeout, the message is Nacked and sent to DLX.
//
// Consume returns as soon as the consumer runs. The consumer stops when the
// context is cancelled, when the broker is closed or when the connection is
// replaced by a reconnect; call Consume again to resume consuming.
func (m *RabbitMQ) Consume(
	ctx context.Context,
	queue string,
	fn func(ctx context.Context, body []byte) error,
	timeout time.Duration,
) error {
	m.mu.RLock()
	ch := m.consumeChannel
	m.mu.RUnlock()

	if ch == nil || ch.IsClosed() {
		return fmt.Errorf("consume channel is not available")
	}

	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}

	go m.consumeLoop(ctx, msgs, fn, timeout)

	return nil
}

// consumeLoop reads deliveries until the consumer is stopped.
func (m *RabbitMQ) consumeLoop(
	ctx context.Context,
	msgs <-chan amqp.Delivery,
	fn func(ctx context.Context, body []byte) error,
	timeout time.Duration,
) {
	defer slog.Info("[RABBITMQ] Consumer goroutine exited.")

	for {
		select {
		case <-m.done:
			slog.Info("[RABBITMQ] Consumer stopped via done channel.")
			return
		case <-ctx.Done():
			slog.Info("[RABBITMQ] Consumer stopped via context.")
			return
		case msg, ok := <-msgs:
			if !ok {
				slog.Error("[RABBITMQ] Message channel closed. The consumer stopped and has to be started again.")
				return
			}
			m.handleDelivery(ctx, msg, fn, timeout)
		}
	}
}

// handleDelivery runs the handler for one delivery and settles the message:
// Ack on success, Nack to the DLX on error or on timeout.
func (m *RabbitMQ) handleDelivery(
	ctx context.Context,
	msg amqp.Delivery,
	fn func(ctx context.Context, body []byte) error,
	timeout time.Duration,
) {
	msgCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The handler runs in its own goroutine so that a handler that ignores
	// its context cannot block the consumer forever. The buffered channel
	// keeps that goroutine from blocking on the send.
	errCh := make(chan error, 1)
	go func(body []byte) {
		errCh <- fn(msgCtx, body)
	}(msg.Body)

	select {
	case <-m.done:
		return
	case <-msgCtx.Done():
		slog.Info("[RABBITMQ] Timeout. Nack → DLX.", slog.String("timeout", timeout.String()))
		if err := msg.Nack(false, false); err != nil {
			slog.Error("[RABBITMQ] Failed to Nack message on timeout.", slog.Any("error", err))
		}
	case err := <-errCh:
		if err != nil {
			slog.Error("[RABBITMQ] Error. Nack → DLX.", slog.Any("error", err))
			if err := msg.Nack(false, false); err != nil {
				slog.Error("[RABBITMQ] Failed to Nack message on error.", slog.Any("error", err))
			}
			return
		}
		if err := msg.Ack(false); err != nil {
			slog.Error("[RABBITMQ] Failed to Ack message.", slog.Any("error", err))
		}
	}
}

// Close closes all RabbitMQ channels and the connection.
func (m *RabbitMQ) Close() error {
	m.mu.Lock()
	select {
	case <-m.done:
		m.mu.Unlock()
		return nil // Already closed
	default:
		close(m.done)
	}
	m.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.closeLocked()
}

// closeLocked closes the channels and the connection. The caller must hold mu
// for writing.
func (m *RabbitMQ) closeLocked() error {
	var errs []error

	if m.consumeChannel != nil {
		if err := m.consumeChannel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close consume channel: %w", err))
		}
		m.consumeChannel = nil
	}

	if m.publishChannel != nil {
		if err := m.publishChannel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close publish channel: %w", err))
		}
		m.publishChannel = nil
	}

	if m.conn != nil {
		if err := m.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection: %w", err))
		}
		m.conn = nil
	}

	return errors.Join(errs...)
}
