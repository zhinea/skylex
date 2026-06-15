# Skylex

Skylex is a self-hosted database control plane. It automates provisioning,
replication, backups, and high availability for PostgreSQL clusters -- without
handing your data to a cloud vendor.

Two binaries make up the system:

- **skylex-server** -- the control plane: REST/gRPC API, job scheduler, metadata
  store (embedded SQLite), and cluster state management.
- **skylex-agent** -- runs on each database node, executes commands from the
  server, and reports health/status back over gRPC.

Architecture:

```
Web UI + REST/gRPC API
       |
skylex-server (SQLite metadata, scheduler)
       | gRPC
skylex-agent-1    skylex-agent-2    skylex-agent-3
(PostgreSQL 16)   (PostgreSQL 16)   (PostgreSQL 16)
       |               |               |
       +--- etcd (HA/leader election) ---+
       +--- MinIO (S3 backups via pgBackRest) ---+
```

PostgreSQL is the only engine in the MVP. MySQL, MongoDB, and Redis are
planned for later.

## Minimum requirements

- **Go 1.26.1** -- build the server and agent
- **Node.js 20+** -- build and run the UI
- **Docker Compose v2** -- run the full reference stack (server, 3 agents, etcd, MinIO)
- **PostgreSQL 16** -- on agent nodes (Docker Compose handles this)

## Setup

### Local development (server only)

```bash
# clone
git clone https://github.com/zhinea/skylex.git
cd skylex

# build both binaries
make build

# run the server with the example config
make dev
```

The server starts on:

| Port  | Service    |
|-------|------------|
| 8080  | HTTP API   |
| 9090  | gRPC API   |
| 9091  | Metrics    |

### UI development

```bash
cd ui
npm install
npm run dev
```

The UI dev server runs on `http://localhost:5173`.

### Full stack with Docker Compose

```bash
make docker-up
```

This starts: `skylex-server`, 3 `skylex-agent` instances with PostgreSQL 16,
etcd, and MinIO (S3-compatible storage for backups).

```bash
make docker-down   # tear down
make docker-logs   # follow logs
```

MinIO console: `http://localhost:9001` (credentials: minioadmin / minioadmin)

### Regenerate protobuf code

```bash
make proto
```

### Run tests and lint

```bash
make test
make lint
```

## Configuration

The server reads a YAML config file. `config.example.yaml` is committed and
works as-is for local development. Settings can be overridden with environment
variables prefixed with `SKYLEX_` (e.g. `SKYLEX_AUTH_JWT_SECRET`).

| Env var                   | Use in Docker Compose       |
|---------------------------|-----------------------------|
| `SKYLEX_JWT_SECRET`       | JWT signing secret          |
| `SKYLEX_AGENT_TOKEN`      | Agent authentication token  |

If `auth.jwt_secret` is left empty, a random secret is generated at startup.

## Project layout

```
cmd/server        skylex-server binary
cmd/agent         skylex-agent binary
cmd/cli           placeholder for skylexctl
internal/server   API, auth, scheduler, state machines
internal/agent    agent daemon logic
internal/backup   pgBackRest integration
internal/db       SQLite metadata store and migrations
internal/dcs      etcd client wrappers
internal/postgres PG lifecycle management
internal/crypto   encryption and key management
proto/skylex/v1   protobuf service definitions
gen/              generated Go code (buf, do not hand-edit)
ui/               Vite + React Router 7 + Tailwind CSS v4
deploy/docker-compose  reference Docker Compose deployment
```

## Contributing

1. Fork the repository.
2. Create a branch for your change.
3. Make your changes. Follow the existing Go and TypeScript conventions in the
   codebase.
4. Run `make lint` and `make test` to verify nothing is broken.
5. If you changed protobuf files, run `make proto` and commit the generated
   code.
6. Open a pull request with a clear description of the change and why.

Before starting on a large feature, open an issue to discuss the design. The
project is in early MVP stage and the architecture is settling.

## License

MIT