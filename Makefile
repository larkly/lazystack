.PHONY: test build lint

test:
	cd src && go test -race ./...

build:
	cd src && go build -ldflags "-X main.version=dev" -o ../bin/lazystack ./cmd/lazystack

lint:
	$(MAKE) -C src lint
