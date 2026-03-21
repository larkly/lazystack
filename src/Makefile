VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY  := lazystack
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build clean test vet

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/lazystack

clean:
	rm -f $(BINARY)

test:
	go test ./...

vet:
	go vet ./...

all: vet test build
