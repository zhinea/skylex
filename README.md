# Skylex

Skylex is a self-hosted database control plane. It automates provisioning,
replication, backups, and high availability for PostgreSQL clusters — without
handing your data to a cloud vendor.

Two binaries make up the system:

- **skylex-server** — the control plane: REST/gRPC API, job scheduler, metadata
  store (embedded SQLite default, PostgreSQL for HA), and cluster state management.
- **skylex-agent** — runs on each database node, executes commands from the
  server, and reports health/status back over gRPC.

Architecture:

```
Web UI + REST/gRPC API
       |
skylex-server (SQLite or PostgreSQL metadata, scheduler)
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

- **Go 1.26.1** — build the server and agent
- **Node.js 20+** — build and run the UI
- **Docker Compose v2** — run the full reference stack (server, 3 agents, etcd, MinIO)
- **PostgreSQL 16** — on agent nodes (Docker Compose handles this)

## Quick start

### 1. Clone and build

```bash
git clone https://github.com/zhinea/skylex.git
cd skylex
make build
```

### 2. Run the server

```bash
# SQLite metadata (default, no external DB needed)
make dev
```

The server starts on:

| Port  | Service    |
|-------|------------|
| 8080  | HTTP API   |
| 9090  | gRPC API   |
| 9091  | Metrics + health |

Health check: `curl http://localhost:9091/health`

### 3. UI development

```bash
cd ui
npm install
npm run dev
```

The UI dev server runs on `http://localhost:5173`.

### 4. Full stack with Docker Compose

```bash
make docker-up        # server + 3 agents + etcd + MinIO
make docker-logs      # follow logs
make docker-down      # tear down
```

MinIO console: `http://localhost:9001` (minioadmin / minioadmin)

## Deployment modes

### Production HA (PostgreSQL metadata)

For production deployments, run the control plane metadata on PostgreSQL:

```bash
# Start PostgreSQL metadata container
docker compose -f deploy/docker-compose/docker-compose.yaml \
               -f deploy/docker-compose/docker-compose.postgres.yaml up -d

# Configure the server for PostgreSQL
export SKYLEX_DATABASE_DRIVER=postgres
export SKYLEX_DATABASE_DSN="postgres://skylex:skylex@localhost:5432/skylex?sslmode=disable"
export SKYLEX_DATABASE_MAX_OPEN_CONNS=25
export SKYLEX_DATABASE_MAX_IDLE_CONNS=10
export SKYLEX_DATABASE_CONN_MAX_LIFETIME=30m

make dev
```

### Systemd (bare metal)

```bash
# Install binaries
cp bin/skylex-server /usr/local/bin/
cp bin/skylex-agent /usr/local/bin/

# Install unit files
cp deploy/systemd/skylex-server.service /etc/systemd/system/
cp deploy/systemd/skylex-agent.service /etc/systemd/system/

# Create service user
useradd -r -s /bin/false skylex
mkdir -p /var/lib/skylex /var/log/skylex
chown -R skylex:skylex /var/lib/skylex /var/log/skylex

# Place config at /etc/skylex/config.yaml

systemctl daemon-reload
systemctl enable --now skylex-server
```

### Kubernetes (Helm)

```bash
helm install skylex deploy/helm/skylex \
  --set postgresMetadata.enabled=true \
  --set postgresMetadata.password=$(openssl rand -hex 16)
```

## Configuration

The server reads a YAML config file. `config.example.yaml` is committed and
works as-is for local development. Settings can be overridden with environment
variables prefixed with `SKYLEX_` (e.g. `SKYLEX_AUTH_JWT_SECRET`).

### Key settings

| Config path                  | Env var                         | Default              |
|------------------------------|---------------------------------|----------------------|
| `database.driver`            | `SKYLEX_DATABASE_DRIVER`         | `sqlite3`            |
| `database.dsn`               | `SKYLEX_DATABASE_DSN`            | SQLite WAL           |
| `database.max_open_conns`    | `SKYLEX_DATABASE_MAX_OPEN_CONNS` | 1 (SQLite), 25 (PG)  |
| `database.max_idle_conns`    | `SKYLEX_DATABASE_MAX_IDLE_CONNS` | 1 (SQLite), 10 (PG)  |
| `auth.jwt_secret`            | `SKYLEX_AUTH_JWT_SECRET`         | auto-generated       |
| `tls.enabled`                | `SKYLEX_TLS_ENABLED`             | `false`              |

### Docker Compose env vars

| Env var              | Use                          |
|----------------------|------------------------------|
| `SKYLEX_JWT_SECRET`  | JWT signing secret           |
| `SKYLEX_AGENT_TOKEN` | Agent authentication token   |
| `MINIO_ACCESS_KEY`   | MinIO access key             |
| `MINIO_SECRET_KEY`   | MinIO secret key             |

## Performance benchmarking

```bash
# Login to get a JWT token
grpcurl -plaintext -d '{"email":"admin@skylex.local","password":"admin"}' \
  localhost:9090 skylex.v1.AuthService/Login

# Run benchmark with token
bin/skylex-bench -addr localhost:9090 \
  -token "YOUR_JWT_TOKEN" \
  -n 100 -c 50
```

Options: `-n` total clusters, `-c` concurrency, `-addr` gRPC address, `-token` JWT.

## Project layout

```
cmd/server              skylex-server binary
cmd/agent               skylex-agent binary
cmd/bench               performance benchmarking tool
internal/server         API, auth, scheduler, state machines
internal/agent          agent daemon logic
internal/backup         pgBackRest integration
internal/db             metadata store (SQLite + PostgreSQL) and migrations
internal/dcs            etcd client wrappers
internal/postgres       PG lifecycle management
internal/crypto         encryption and key management
proto/skylex/v1         protobuf service definitions
gen/                    generated Go code (buf, do not hand-edit)
ui/                     Vite + React Router 7 + Tailwind CSS v4
deploy/docker-compose   reference Docker Compose deployment
deploy/systemd          systemd unit files
deploy/helm/skylex      Helm chart scaffold
```

## Migrations

Migrations are embedded in `internal/db/migrations/` with separate directories
for `sqlite` and `postgres`. They are applied automatically on server startup.
The migration table is `schema_migrations`.

## Contributing

1. Fork the repository.
2. Create a branch for your change.
3. Make your changes. Follow existing Go and TypeScript conventions.
4. Run `make lint` and `make test` to verify nothing is broken.
5. If you changed protobuf files, run `make proto` and commit the generated code.
6. Open a pull request with a clear description.

Before starting on a large feature, open an issue to discuss the design.

## License

MIT