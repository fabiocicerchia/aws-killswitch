BINARY  := aws-killswitch
BIN_DIR := bin
PKG     := ./cmd/aws-killswitch

.PHONY: all build test lint tidy clean

all: build

## build: compile the binary into ./bin
build:
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

## test: run tests
test:
	go test ./...

## lint: vet and formatting check
lint:
	go vet ./...
	@test -z "$$(gofmt -l . )" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

## tidy: tidy modules
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)
