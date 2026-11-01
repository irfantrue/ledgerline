package brokertest

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"ledgerline/internal/shared/broker"
	"ledgerline/internal/shared/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

// rabbitURL returns the broker URL for the tests, or skips the test.
func rabbitURL(t *testing.T) string {
	t.Helper()

	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set; skipping integration test")
	}
	return url
}

// newBroker connects to RabbitMQ and returns a broker that is closed when the
// test ends.
func newBroker(t *testing.T) broker.MessageBroker {
	t.Helper()

	url := rabbitURL(t)

	b, err := broker.NewRabbitMQ(config.Config{
		RabbitMQ: config.RabbitMQConfig{URL: url},
	})
	if err != nil {
		t.Fatalf("NewRabbitMQ() error = %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	return b
}

func TestNewRabbitMQ(t *testing.T) {
	b := newBroker(t)

	if b == nil {
		t.Fatal("NewRabbitMQ() = nil")
	}
}

func TestNewRabbitMQ_InvalidURL(t *testing.T) {
	_, err := broker.NewRabbitMQ(config.Config{
		RabbitMQ: config.RabbitMQConfig{URL: "amqp://127.0.0.1:1/"},
	})
	if err == nil {
		t.Fatal("NewRabbitMQ() error = nil, want a connection error")
	}
}

// TestPublishAndConsume covers the happy path: a published message reaches
// the consumer and is acknowledged.
func TestPublishAndConsume(t *testing.T) {
	b := newBroker(t)
	ctx := context.Background()

	const queue = "test.roundtrip"
	declareQueue(t, queue)

	received := make(chan string, 1)

	if err := b.Consume(ctx, queue, func(_ context.Context, body []byte) error {
		received <- string(body)
		return nil
	}, 5*time.Second); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	// Give the consumer time to register before publishing.
	time.Sleep(200 * time.Millisecond)

	if err := b.Publish(ctx, "", queue, []byte(`{"hello":"world"}`)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case got := <-received:
		if got != `{"hello":"world"}` {
			t.Errorf("body = %q, want %q", got, `{"hello":"world"}`)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the message")
	}
}

// TestConsume_HandlerErrorNacks checks that a failing handler does not
// acknowledge the message.
func TestConsume_HandlerErrorNacks(t *testing.T) {
	b := newBroker(t)
	ctx := context.Background()

	const queue = "test.handlererror"
	declareQueue(t, queue)

	called := make(chan struct{}, 1)

	if err := b.Consume(ctx, queue, func(_ context.Context, _ []byte) error {
		called <- struct{}{}
		return os.ErrInvalid
	}, 5*time.Second); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := b.Publish(ctx, "", queue, []byte("boom")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the handler to run")
	}
}

// TestConsume_TimeoutNacks checks that a handler that runs past the timeout
// is cut off and the message is nacked.
func TestConsume_TimeoutNacks(t *testing.T) {
	b := newBroker(t)
	ctx := context.Background()

	const queue = "test.timeout"
	declareQueue(t, queue)

	started := make(chan struct{}, 1)

	if err := b.Consume(ctx, queue, func(ctx context.Context, _ []byte) error {
		started <- struct{}{}
		<-ctx.Done() // block until the timeout cancels the context
		return ctx.Err()
	}, 300*time.Millisecond); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := b.Publish(ctx, "", queue, []byte("slow")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the handler to start")
	}

	// The consumer must survive the timeout and keep running.
	time.Sleep(500 * time.Millisecond)
}

// TestPublish_AfterChannelClosed checks that the broker reconnects when the
// publish channel went away, instead of deadlocking on its own mutex.
func TestPublish_AfterChannelClosed(t *testing.T) {
	b := newBroker(t)
	ctx := context.Background()

	const queue = "test.reconnect"
	declareQueue(t, queue)

	// Drop the connection so the next publish has to reconnect.
	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := b.Publish(publishCtx, "", queue, []byte("payload")); err != nil {
		t.Errorf("Publish() error = %v", err)
	}
}

func TestConsume_UnknownQueue(t *testing.T) {
	b := newBroker(t)

	err := b.Consume(context.Background(), "queue.that.does.not.exist", func(context.Context, []byte) error {
		return nil
	}, time.Second)
	if err == nil {
		t.Fatal("Consume() error = nil, want an error for an unknown queue")
	}
}

// TestPublish_UnroutableMessageReturns checks that a message the broker cannot
// route is reported instead of being dropped silently. The message is
// published as mandatory, so the broker returns it and Publish has to surface
// that as an error.
func TestPublish_UnroutableMessageReturns(t *testing.T) {
	b := newBroker(t)

	const queue = "test.queue.that.does.not.exist"

	publishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := b.Publish(publishCtx, "", queue, []byte("payload"))
	if err == nil {
		t.Fatal("Publish() error = nil, want an error for an unroutable message")
	}
	if !strings.Contains(err.Error(), "message returned") {
		t.Errorf("Publish() error = %v, want it to report that the message was returned", err)
	}
}

func TestClose(t *testing.T) {
	url := rabbitURL(t)

	b, err := broker.NewRabbitMQ(config.Config{
		RabbitMQ: config.RabbitMQConfig{URL: url},
	})
	if err != nil {
		t.Fatalf("NewRabbitMQ() error = %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Closing twice must be a no-op.
	if err := b.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

// declareQueue declares a queue the tests can publish to and consume from.
//
// The queue is durable and auto-delete. RabbitMQ 4 refuses to declare a
// transient, non-exclusive queue (the transient_nonexcl_queues feature is
// deprecated), and an exclusive queue would be invisible to the broker
// connection that the tests consume from.
func declareQueue(t *testing.T, name string) {
	t.Helper()

	conn, err := amqp.Dial(rabbitURL(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })

	if _, err := ch.QueueDeclare(name, true, true, false, false, nil); err != nil {
		t.Fatalf("declare queue %s: %v", name, err)
	}
	// Cleanups run last in, first out, so the queue goes before the channel
	// that declared it is closed.
	t.Cleanup(func() { _, _ = ch.QueueDelete(name, false, false, false) })
}
