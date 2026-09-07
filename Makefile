BINARY  := aws-killswitch
BIN_DIR := bin
PKG     := ./cmd/aws-killswitch
LAMBDA_PKG := ./cmd/killswitch-lambda

.PHONY: all build lambda test lint tidy clean help setup install run format analyze

.DEFAULT_GOAL := help

## help: show this help
help:
	@awk '/^## [a-zA-Z0-9_-]+:/ { l=$$0; sub(/^## /,"",l); i=index(l,":"); \
		printf "  %-14s %s\n", substr(l,1,i-1), substr(l,i+2); next } \
		/^[a-zA-Z0-9_-]+:.*## / { i=index($$0,":"); j=index($$0,"## "); \
		printf "  %-14s %s\n", substr($$0,1,i-1), substr($$0,j+3) }' $(MAKEFILE_LIST)

all: build

## build: compile the binary into ./bin
build:
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

## lambda: build the Budgets-action handler and zip it for upload
lambda:
	@# provided.al2023 execs a file called exactly "bootstrap", and nothing else.
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
		go build -tags lambda.norpc -trimpath -ldflags="-s -w" \
		-o $(BIN_DIR)/bootstrap $(LAMBDA_PKG)
	cd $(BIN_DIR) && zip -q -X killswitch-lambda.zip bootstrap
	@echo "built $(BIN_DIR)/killswitch-lambda.zip  (runtime provided.al2023, arch arm64)"

## test: run tests
test:
	go test -race -count=1 ./...

## lint: vet and formatting check
lint: ## Run the whole gate — every hook, every file
	pre-commit run --all-files

## tidy: tidy modules
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)

setup: ## Install the pre-commit hook
	pre-commit install

install: ## Install the binary into GOBIN
	go install ./...

run: ## Run the binary
	go run ./cmd/aws-killswitch $(ARGS)

format: ## Rewrite the sources to gofmt form
	gofmt -w .

analyze: ## Lint with the house rule set
	golangci-lint run ./...
