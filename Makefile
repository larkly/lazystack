test:
	go test -race -coverprofile=coverage.out ./...

build:
	go build -o bin/lazystack src/main.go
