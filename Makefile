.PHONY: build build-e2e test test-cli test-e2e test-e2e-aws e2e e2e-aws install fmt fmt-check lint vet ci ui-install ui-run ui-build ui-format ui-format-check ui-lint ui-ci

BINARY := bin/outpost
E2E_BINARY := bin/outpost-e2e
PNPM := corepack pnpm@10.29.3

# Optional overrides for AWS e2e:
#   make e2e-aws PROFILE=my-aws-profile
#   make e2e-aws PROFILE=my-aws-profile REGION=eu-west-1
PROFILE ?=
REGION ?=

build:
	go build -o $(BINARY) ./cmd/outpost

build-e2e:
	go build -o $(E2E_BINARY) ./cmd/outpost

install:
	go install ./cmd/outpost

test:
	go test -race -count=1 ./...

test-e2e: build-e2e
	OUTPOST_E2E=1 OUTPOST_E2E_HOST_MODE=loopback \
		go test -tags=e2e -timeout=45m -count=1 -p 1 -v ./e2e/...

test-e2e-aws: build-e2e
	OUTPOST_E2E=1 \
	OUTPOST_E2E_HOST_MODE=aws \
	$(if $(PROFILE),OUTPOST_E2E_AWS_PROFILE=$(PROFILE) AWS_PROFILE=$(PROFILE),) \
	$(if $(REGION),OUTPOST_E2E_AWS_REGION=$(REGION) AWS_REGION=$(REGION),) \
	go test -tags="e2e aws" -timeout=60m -count=1 -p 1 -v ./e2e/...

e2e: test-e2e

e2e-aws: test-e2e-aws

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
