.PHONY: build test test-cli install fmt fmt-check lint vet ci ui-install ui-run ui-build ui-format ui-format-check ui-lint ui-ci

BINARY := bin/outpost
PNPM := corepack pnpm@10.29.3

build:
	go build -o $(BINARY) ./cmd/outpost

install:
	go install ./cmd/outpost

test:
	go test -race -count=1 ./...

test-cli:
	go test -race -count=1 -v ./internal/cli/...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

vet:
	go vet ./...

lint:
	golangci-lint run ./...

ci: fmt-check vet lint test build ui-ci

ui-install:
	$(PNPM) --dir ui install --frozen-lockfile

ui-run:
	$(PNPM) --dir ui dev

ui-build:
	$(PNPM) --dir ui build

ui-format:
	$(PNPM) --dir ui run format

ui-format-check:
	$(PNPM) --dir ui run format:check

ui-lint:
	$(PNPM) --dir ui run lint

ui-ci: ui-format-check ui-lint ui-build
