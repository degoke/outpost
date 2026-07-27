.PHONY: build test install fmt fmt-check lint vet ci

BINARY := bin/outpost

build:
	go build -o $(BINARY) ./cmd/outpost

install:
	go install ./cmd/outpost

test:
	go test -race -count=1 ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

vet:
	go vet ./...

lint:
	golangci-lint run ./...

ci: fmt-check vet lint test build
