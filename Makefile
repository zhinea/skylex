.PHONY: build build-server build-agent test lint clean run-server run-agent proto dev dev-server

BINARY_SERVER ?= skylex-server
BINARY_AGENT ?= skylex-agent

GO ?= go
GOFLAGS ?= -ldflags="-s -w"

build: build-server build-agent build-bench

build-server:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_SERVER) ./cmd/server

build-agent:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT) ./cmd/agent

build-bench:
	$(GO) build $(GOFLAGS) -o bin/skylex-bench ./cmd/bench

run-server:
	$(GO) run ./cmd/server $(ARGS)

run-agent:
	$(GO) run ./cmd/agent $(ARGS)

test:
	$(GO) test ./...

lint:
	golangci-lint run ./...

proto:
	buf lint
	buf generate

clean:
	rm -rf bin/ gen/

dev:
	./scripts/dev.sh

dev-server:
	$(GO) run ./cmd/server config.example.yaml

docker-up:
	docker compose -f deploy/docker-compose/docker-compose.yaml up --build -d

docker-down:
	docker compose -f deploy/docker-compose/docker-compose.yaml down

docker-logs:
	docker compose -f deploy/docker-compose/docker-compose.yaml logs -f