.PHONY: run build test

run:
	go run ./cmd/durstworld

build:
	CGO_ENABLED=0 go build -o durstworld ./cmd/durstworld

test:
	go test ./...
