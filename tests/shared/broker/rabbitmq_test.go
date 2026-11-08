package brokertest

import (
	"context"
	"errors"
	"testing"
	"time"

	"ledgerline/internal/shared/broker"
	"ledgerline/internal/shared/config"
	"ledgerline/tests/mocks/amqpmocks"
	"ledgerline/tests/mocks/brokermocks"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newBroker returns a broker whose connection is the given one.
//
// The topology is declared on the publish channel, so the channel mock has to
// accept the declaration calls.
func newBroker(t *testing.T, conn broker.Connection) broker.MessageBroker {
	t.Helper()

	b, err := broker.NewRabbitMQWithDialer(config.Config{
		RabbitMQ: config.RabbitMQConfig{URL: "amqp://mock"},
	}, func(string) (broker.Connection, error) {
		return conn, nil
	})
	require.NoError(t, err, "the broker should accept the mocked connection")

	t.Cleanup(func() { _ = b.Close() })

	return b
}

// returnsHolder holds the channel the broker registers for unroutable
// messages.
//
// The channel is created by the broker, so the mock only learns about it when
// the broker calls NotifyReturn. The holder is a pointer because the value
// is filled in after the helper returned.
type returnsHolder struct {
	ch chan amqp.Return
}

// topologyChannel returns a channel mock that accepts the topology
// declarations the broker makes when it connects.
//
// The holder gives access to the channel the broker registers for unroutable
// messages, so a test can push a return into it to simulate a message the
// broker could not route.
func topologyChannel(t *testing.T) (*brokermocks.Channel, *returnsHolder) {
	t.Helper()

	ch := &brokermocks.Channel{}
	ch.On("Confirm", mock.Anything).Return(nil).Maybe()
	ch.On("Qos", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	ch.On("ExchangeDeclare", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	ch.On("QueueDeclare", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(amqp.Queue{Name: "dlx.queue"}, nil).
		Maybe()
	ch.On("QueueBind", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	ch.On("Close").Return(nil).Maybe()

	holder := &returnsHolder{}
	ch.On("NotifyReturn", mock.Anything).
		Run(func(args mock.Arguments) { holder.ch = args.Get(0).(chan amqp.Return) }).
		Return(nil).
		Maybe()

	return ch, holder
}

// connectedBroker returns a broker whose connection hands out the given
// publish and consume channels.
func connectedBroker(t *testing.T, publish, consume *brokermocks.Channel) broker.MessageBroker {
	t.Helper()

	conn := &brokermocks.Connection{}
	conn.On("Channel").Return(publish, nil).Once()
	conn.On("Channel").Return(consume, nil).Once()
	conn.On("Close").Return(nil).Maybe()

	return newBroker(t, conn)
}

func TestNewRabbitMQ_InvalidURL(t *testing.T) {
	_, err := broker.NewRabbitMQ(config.Config{
		RabbitMQ: config.RabbitMQConfig{URL: "amqp://127.0.0.1:1/"},
	})

	require.Error(t, err, "a broker that cannot be reached should be reported")
}

// TestNewRabbitMQ_DialError checks that a failing dial is reported instead of
// a broker that cannot publish.
func TestNewRabbitMQ_DialError(t *testing.T) {
	_, err := broker.NewRabbitMQWithDialer(config.Config{}, func(string) (broker.Connection, error) {
		return nil, errors.New("dial failed")
	})

	require.Error(t, err)
}

// TestPublish checks that a published message reaches the broker with the
// exchange and the routing key the caller gave.
func TestPublish(t *testing.T) {
	conf := &brokermocks.Confirmation{}
	conf.On("WaitContext", mock.Anything).Return(true, nil).Once()

	publish, _ := topologyChannel(t)
	publish.On("Publish", mock.Anything, "ex", "rk", true, mock.Anything).
		Return(conf, nil).
		Once()
	publish.On("IsClosed").Return(false).Maybe()

	consume, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	require.NoError(t, b.Publish(context.Background(), "ex", "rk", []byte(`{"hello":"world"}`)))
	publish.AssertExpectations(t)
}

// TestPublish_Nack checks that a message the broker refuses is reported.
func TestPublish_Nack(t *testing.T) {
	conf := &brokermocks.Confirmation{}
	conf.On("WaitContext", mock.Anything).Return(false, nil).Once()

	publish, _ := topologyChannel(t)
	publish.On("Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(conf, nil).
		Once()
	publish.On("IsClosed").Return(false).Maybe()

	consume, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	err := b.Publish(context.Background(), "ex", "rk", []byte("body"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "nacked")
}

// TestPublish_UnroutableMessageReturns checks that a message the broker
// cannot route is reported instead of being dropped silently. The message is
// published as mandatory, so the broker returns it and Publish has to surface
// that as an error.
func TestPublish_UnroutableMessageReturns(t *testing.T) {
	conf := &brokermocks.Confirmation{}
	conf.On("WaitContext", mock.Anything).Return(true, nil).Once()

	publish, returns := topologyChannel(t)
	publish.On("IsClosed").Return(false).Maybe()

	// The publish call pushes a return into the channel the broker
	// registered, the way the broker does when it cannot route a message.
	publish.On("Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(conf, nil).
		Run(func(args mock.Arguments) {
			msg := args.Get(4).(amqp.Publishing)
			returns.ch <- amqp.Return{
				ReplyCode:  312,
				ReplyText:  "NO_ROUTE",
				Exchange:   "ex",
				RoutingKey: "rk",
				MessageId:  msg.MessageId,
			}
		}).
		Once()

	consume, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	err := b.Publish(context.Background(), "ex", "rk", []byte("payload"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "message returned")
}

// TestPublish_AfterChannelClosed checks that the broker reconnects when the
// publish channel went away, instead of deadlocking on its own mutex.
func TestPublish_AfterChannelClosed(t *testing.T) {
	conf := &brokermocks.Confirmation{}
	conf.On("WaitContext", mock.Anything).Return(true, nil).Once()

	closed, _ := topologyChannel(t)
	closed.On("IsClosed").Return(true).Maybe()

	reopened, _ := topologyChannel(t)
	reopened.On("Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(conf, nil).
		Once()
	reopened.On("IsClosed").Return(false).Maybe()

	conn := &brokermocks.Connection{}
	conn.On("Channel").Return(closed, nil).Once()
	conn.On("Channel").Return(mustChannel(t), nil).Once()
	conn.On("Channel").Return(reopened, nil).Once()
	conn.On("Channel").Return(mustChannel(t), nil).Once()
	conn.On("Close").Return(nil).Maybe()

	b, err := broker.NewRabbitMQWithDialer(config.Config{}, func(string) (broker.Connection, error) {
		return conn, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })

	require.NoError(t, b.Publish(context.Background(), "ex", "rk", []byte("payload")))
	reopened.AssertExpectations(t)
}

// TestConsume checks that a message the broker delivers reaches the handler.
func TestConsume(t *testing.T) {
	ack := &amqpmocks.Acknowledger{}
	settled := make(chan struct{})
	ack.On("Ack", mock.Anything, false).
		Run(func(mock.Arguments) { close(settled) }).
		Return(nil).
		Once()

	deliveries := make(chan amqp.Delivery, 1)

	consume, _ := topologyChannel(t)
	consume.On("Consume", "queue", "", false, false, false, false, mock.Anything).
		Return((<-chan amqp.Delivery)(deliveries), nil).
		Once()
	consume.On("IsClosed").Return(false).Maybe()

	publish, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	received := make(chan string, 1)
	require.NoError(t, b.Consume(context.Background(), "queue", func(_ context.Context, body []byte) error {
		received <- string(body)
		return nil
	}, 5*time.Second))

	deliveries <- amqpmocks.Delivery(ack, `{"hello":"world"}`)

	select {
	case got := <-received:
		require.Equal(t, `{"hello":"world"}`, got)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the message")
	}

	waitForSettlement(t, settled)
	ack.AssertExpectations(t)
}

// TestConsume_HandlerErrorNacks checks that a failing handler does not
// acknowledge the message.
func TestConsume_HandlerErrorNacks(t *testing.T) {
	ack := &amqpmocks.Acknowledger{}
	settled := make(chan struct{})
	ack.On("Nack", mock.Anything, false, false).
		Run(func(mock.Arguments) { close(settled) }).
		Return(nil).
		Once()

	deliveries := make(chan amqp.Delivery, 1)

	consume, _ := topologyChannel(t)
	consume.On("Consume", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan amqp.Delivery)(deliveries), nil).
		Once()
	consume.On("IsClosed").Return(false).Maybe()

	publish, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	called := make(chan struct{}, 1)
	require.NoError(t, b.Consume(context.Background(), "queue", func(context.Context, []byte) error {
		called <- struct{}{}
		return errors.New("boom")
	}, 5*time.Second))

	deliveries <- amqpmocks.Delivery(ack, "boom")

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the handler to run")
	}

	waitForSettlement(t, settled)
	ack.AssertExpectations(t)
}

// TestConsume_TimeoutNacks checks that a handler that runs past the timeout
// is cut off and the message is nacked.
func TestConsume_TimeoutNacks(t *testing.T) {
	ack := &amqpmocks.Acknowledger{}
	settled := make(chan struct{})
	ack.On("Nack", mock.Anything, false, false).
		Run(func(mock.Arguments) { close(settled) }).
		Return(nil).
		Once()

	deliveries := make(chan amqp.Delivery, 1)

	consume, _ := topologyChannel(t)
	consume.On("Consume", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((<-chan amqp.Delivery)(deliveries), nil).
		Once()
	consume.On("IsClosed").Return(false).Maybe()

	publish, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	started := make(chan struct{}, 1)
	require.NoError(t, b.Consume(context.Background(), "queue", func(ctx context.Context, _ []byte) error {
		started <- struct{}{}
		<-ctx.Done() // block until the timeout cancels the context
		return ctx.Err()
	}, 300*time.Millisecond))

	deliveries <- amqpmocks.Delivery(ack, "slow")

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the handler to start")
	}

	waitForSettlement(t, settled)
	ack.AssertExpectations(t)
}

// TestConsume_UnknownQueue checks that a queue the broker refuses is
// reported.
func TestConsume_UnknownQueue(t *testing.T) {
	consume, _ := topologyChannel(t)
	consume.On("IsClosed").Return(false).Maybe()
	consume.On("Consume", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("no such queue")).
		Once()

	publish, _ := topologyChannel(t)
	b := connectedBroker(t, publish, consume)

	err := b.Consume(context.Background(), "queue.that.does.not.exist", func(context.Context, []byte) error {
		return nil
	}, time.Second)

	require.Error(t, err)
}

func TestClose(t *testing.T) {
	publish, _ := topologyChannel(t)
	consume, _ := topologyChannel(t)

	conn := &brokermocks.Connection{}
	conn.On("Channel").Return(publish, nil).Once()
	conn.On("Channel").Return(consume, nil).Once()
	conn.On("Close").Return(nil).Once()

	b, err := broker.NewRabbitMQWithDialer(config.Config{}, func(string) (broker.Connection, error) {
		return conn, nil
	})
	require.NoError(t, err)

	require.NoError(t, b.Close())

	// Closing twice must be a no-op.
	require.NoError(t, b.Close())

	conn.AssertExpectations(t)
}

// mustChannel returns a channel mock that accepts the topology declarations.
func mustChannel(t *testing.T) *brokermocks.Channel {
	t.Helper()

	ch, _ := topologyChannel(t)
	return ch
}

// waitForSettlement waits until the consumer settled the delivery. The
// settlement happens in the consumer goroutine, so the test has to wait for
// it before it asserts on it.
func waitForSettlement(t *testing.T, settled chan struct{}) {
	t.Helper()

	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the delivery to be settled")
	}
}
