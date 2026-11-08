APP_NAME := app
AIR_DIR := air
AIR_PATH := $(AIR_DIR)/$(APP_NAME)
CMD_PATH := ./cmd/api/

# Coverage settings.
# COVERPKG instruments every package, because the tests live in tests/ and
# would otherwise only measure themselves.
COVERPKG ?= ./...
# Minimum total coverage in percent. `make coverage` fails below it.
COVERAGE_MIN ?= 80
# Packages left out of the measurement: cmd/api only wires the application
# together, the sqlcgen package is generated code, and tests/mocks holds
# the generated test doubles, which are scaffolding rather than production
# code. The pattern is a prefix, so it covers every mock package under
# tests/mocks/.
# The pattern is a basic regular expression, so the alternatives are separated
# with \|.
COVERAGE_EXCLUDE ?= ledgerline/cmd/api/\|ledgerline/internal/ledger/infrastructure/sqlcgen/\|ledgerline/tests/mocks/

.PHONY: all build build-air run watch tidy vendor lint mocks test test-coverage coverage clean

all: tidy vendor lint test build

run:
	go run ./cmd/api

tidy:
	go mod tidy
	go mod verify

vendor:
	go mod vendor

watch:
	air -c .air.toml

build-air:
	@mkdir -p $(AIR_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(AIR_PATH) $(CMD_PATH)
	@echo "Built: $(AIR_PATH)"

build:
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -mod=vendor -o bin/$(APP_NAME) $(CMD_PATH)
	@echo "Successfully built bin/$(APP_NAME)"

lint:
	@echo "Running linters..."
	golangci-lint run

# Regenerates the test doubles from the interfaces they stand in for.
#
# The generated files are committed, so a fresh clone can build and test
# without mockery installed. Run this after changing an interface.
mocks:
	@echo "Generating mocks..."
	mockery

test:
	go test -v -race ./...

# Runs the tests and writes an HTML coverage report to coverage.html.
test-coverage:
	go test -v -race -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Runs the tests and fails when the total coverage is below COVERAGE_MIN.
coverage:
	go test -race -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./...
	@./scripts/coverage.sh coverage.out $(COVERAGE_MIN) "$(COVERAGE_EXCLUDE)"

clean:
	@rm -rf bin/
	@rm -rf $(AIR_DIR)/
	@rm -f coverage.out coverage.html coverage.filtered.out
	@echo "Cleaned up successfully."
