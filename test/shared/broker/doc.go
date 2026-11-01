// Package brokertest holds the tests for internal/shared/broker.
//
// The tests live outside internal/ so the production package stays free of
// test-only helpers and fixtures.
//
// The integration tests need a real RabbitMQ and are skipped unless
// RABBITMQ_URL is set.
package brokertest
