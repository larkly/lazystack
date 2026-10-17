VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY  := lazystack
LDFLAGS := -X main.version=$(VERSION)
STATICCHECK ?= honnef.co/go/tools/cmd/staticcheck@latest

.PHONY: build clean test vet lint

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/lazystack

clean:
	rm -f $(BINARY)

test:
	go test ./...

vet:
	go vet ./...

# lint fails on unformatted files, vet findings or staticcheck findings.
lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt -l reports unformatted files:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go run $(STATICCHECK) ./...

all: vet test build
