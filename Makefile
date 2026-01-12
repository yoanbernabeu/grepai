.PHONY: build install test clean lint run docs docs-generate docs-build docs-dev fmt pre-commit

BINARY_NAME=grepai
VERSION?=0.1.0
BUILD_DIR=bin
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/grepai

install:
	go install $(LDFLAGS) ./cmd/grepai

test:
	go test -v -race ./...

test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

lint:
	docker run --rm -v $(PWD):/app -w /app golangci/golangci-lint:v1.64.2 golangci-lint run ./...

lint-local:
	golangci-lint run ./...

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Cross-compilation
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/grepai
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/grepai

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/grepai
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/grepai

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/grepai

# Documentation
docs: docs-build

docs-generate:
	go run cmd/gendocs/main.go

docs-build: docs-generate
	cd docs && npm ci && npm run build

docs-dev: docs-generate
	cd docs && npm install && npm run dev

# Code formatting
fmt:
	gofmt -w .

# Pre-commit checks: format, vet, lint, and test
pre-commit: fmt
	go vet ./...
	docker run --rm -v $(PWD):/app -w /app golangci/golangci-lint:v1.64.2 golangci-lint run ./...
	go test -race ./...
	@echo "✓ All checks passed! Ready to commit."
