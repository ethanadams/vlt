.PHONY: build test test-unit test-go-integration test-e2e test-all clean vet lint install help

# Binary name
BINARY := vlt

# Build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go settings
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

## build: Build the binary
build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

## install: Install to GOPATH/bin
install:
	go install $(GOFLAGS) -ldflags "$(LDFLAGS)" .

## test-unit: Run Go unit tests (no Vault/Docker required)
test-unit:
	go test -v -race ./pkg/...

## test-go-integration: Run Go integration tests (requires Docker)
test-go-integration:
	go test -v -race -tags=integration ./pkg/...

## test-e2e: Run CLI end-to-end tests (requires running Vault)
test-e2e: build
	./test_e2e.sh

## test: Run unit + Go integration tests (requires Docker only)
test: test-unit test-go-integration

## test-all: Run all tests including e2e (requires Docker + running Vault)
test-all: test test-e2e

## vet: Run go vet
vet:
	go vet ./...

## lint: Run static analysis (requires golangci-lint)
lint:
	@which golangci-lint > /dev/null || (echo "Install golangci-lint: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)
	rm -f coverage.out

## coverage: Generate test coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## docker-up: Start test Vault server
docker-up:
	docker compose up -d

## docker-down: Stop test Vault server
docker-down:
	docker compose down

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
