.PHONY: build build-server build-agent build-bench build-agent-linux-amd64 build-agent-linux-arm64 test lint clean run-server run-agent proto dev dev-server release-agent

BINARY_SERVER ?= skylex-server
BINARY_AGENT ?= skylex-agent

GO ?= go
GOFLAGS ?= -ldflags="-s -w"

build: assets build-server build-agent build-bench

assets:
	@mkdir -p internal/server/assets
	@cp scripts/install-agent.sh internal/server/assets/install-agent.sh
	@cp version.txt internal/server/assets/version.txt

build-server: assets
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_SERVER) ./cmd/server

build-agent:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT) ./cmd/agent

build-bench:
	$(GO) build $(GOFLAGS) -o bin/skylex-bench ./cmd/bench

build-agent-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT)-linux-amd64 ./cmd/agent

build-agent-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT)-linux-arm64 ./cmd/agent

build-agent-linux-amd64:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(DIST)/$(BINARY_AGENT)-linux-amd64 ./cmd/agent

build-agent-linux-arm64:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(DIST)/$(BINARY_AGENT)-linux-arm64 ./cmd/agent

build-release-binaries: build-agent-linux-amd64 build-agent-linux-arm64

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