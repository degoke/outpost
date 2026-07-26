.PHONY: build test install

build:
	go build -o bin/outpost ./cmd/outpost

install:
	go install ./cmd/outpost

test:
	go test ./...
