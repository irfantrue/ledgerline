// Package brokertest holds the tests for internal/shared/broker.
//
// The tests live outside internal/ so the production package stays free of
// test-only helpers and fixtures.
//
// The tests need no RabbitMQ: the broker is exercised against a mocked
// connection and a mocked channel.
package brokertest
