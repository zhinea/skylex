.PHONY: build build-server build-agent build-bench build-agent-linux-amd64 build-agent-linux-arm64 assets ui-build test lint clean run-server run-agent proto dev dev-server

BINARY_SERVER ?= skylex-server
BINARY_AGENT ?= skylex-agent

GO ?= go
GOFLAGS ?= -ldflags="-s -w"

ASSETS_DIR = internal/server/assets

assets:
	@mkdir -p $(ASSETS_DIR)
	@if ! cmp -s scripts/install-agent.sh $(ASSETS_DIR)/install-agent.sh; then cp scripts/install-agent.sh $(ASSETS_DIR)/install-agent.sh; fi
	@if ! cmp -s version.txt $(ASSETS_DIR)/version.txt; then cp version.txt $(ASSETS_DIR)/version.txt; fi

build: assets ui-build build-server build-agent build-bench

ui-build:
	cd ui && npm run build

build-server: assets ui-build
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_SERVER) ./cmd/server

build-agent:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT) ./cmd/agent

build-bench:
	$(GO) build $(GOFLAGS) -o bin/skylex-bench ./cmd/bench

build-agent-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT)-linux-amd64 ./cmd/agent

build-agent-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT)-linux-arm64 ./cmd/agent

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

setup:
	mv ./config.example.yaml ./config.yaml

dev:
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_AGENT) ./cmd/agent
	./scripts/dev.sh

dev-server:
	$(GO) run ./cmd/server config.yaml

docker-up:
	docker compose -f deploy/docker-compose/docker-compose.yaml up --build -d

docker-down:
	docker compose -f deploy/docker-compose/docker-compose.yaml down

docker-logs:
	docker compose -f deploy/docker-compose/docker-compose.yaml logs -f
