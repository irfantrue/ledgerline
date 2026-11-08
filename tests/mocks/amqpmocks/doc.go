// Package amqpmocks holds the test doubles for the acknowledgements the broker sends for a delivery.
//
// The interfaces are mocked by mockery from github.com/rabbitmq/amqp091-go; see .mockery.yaml and
// `make mocks`. The generated files are committed, so a fresh clone can
// build and test without mockery installed.
//
// The package lives in the test tree on purpose: the doubles are test
// scaffolding, not production code.
package amqpmocks
