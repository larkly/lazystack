test:
	cd src && go test ./...

build:
	cd src && go build -ldflags "-X main.version=dev" -o ../bin/lazystack ./cmd/lazystack
