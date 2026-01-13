.PHONY: build test lint security clean run docker-build docker-push help

VERSION := $(shell cat VERSION)
BINARY := hostus
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

## build: Build the binary
build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/hostus

## test: Run all tests
test:
	go test -v -race -cover ./...

## test-short: Run tests without race detector
test-short:
	go test -v -cover ./...

## lint: Run golangci-lint
lint:
	golangci-lint run --timeout=5m ./...

## security: Run security checks (govulncheck + staticcheck)
security:
	govulncheck ./...
	staticcheck ./...

## fmt: Format code
fmt:
	gofmt -s -w .
	goimports -w -local github.com/jobrunner/hostus .

## vet: Run go vet
vet:
	go vet ./...

## tidy: Tidy go modules
tidy:
	go mod tidy

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)
	rm -rf dist/

## run: Run the service locally
run: build
	./$(BINARY)

## docker-build: Build Docker image
docker-build:
	docker build -t ghcr.io/jobrunner/hostus:$(VERSION) -t ghcr.io/jobrunner/hostus:latest .

## docker-push: Push Docker image to registry
docker-push:
	docker push ghcr.io/jobrunner/hostus:$(VERSION)
	docker push ghcr.io/jobrunner/hostus:latest

## docker-run: Run Docker container locally
docker-run:
	docker run --rm -p 8080:8080 ghcr.io/jobrunner/hostus:latest

## all: Run all checks and build
all: fmt lint vet security test build

## help: Show this help message
help:
	@echo "Available targets:"
	@echo ""
	@grep -E '^## [a-zA-Z_-]+:' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ": "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' | \
		sed 's/## //'
