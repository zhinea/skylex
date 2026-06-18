# Skylex — Agent Notes

Skylex is a self-hosted database control plane MVP. The Go backend has two binaries (`skylex-server` and `skylex-agent`) that talk over gRPC/Connect-RPC, plus a Vite + React Router 7 UI in `ui/`.


## Quick start

```bash
# Backend — build both binaries and run the server locally
make build
make dev                                    # starts etcd + minio via docker compose, then runs cmd/server with config.example.yaml
make dev-server                             # runs cmd/server with config.example.yaml (no extra services)

# Full reference stack in Docker Compose
make docker-up                              # builds images and starts server + 3 agents + etcd + minio
make docker-down
```

## Project layout

- `cmd/server` → `skylex-server` binary (control plane)
- `cmd/agent` → `skylex-agent` binary (runs on each DB node)
- `cmd/cli` → empty placeholder for a future `skylexctl`
- `internal/server`, `internal/agent`, `internal/backup`, `internal/db`, `internal/postgres`, `internal/dcs` → internal packages
- `pkg/` → empty; public packages go here if needed
- `proto/skylex/v1/` → protobuf service definitions
- `gen/` → generated Go code from `buf generate` (do not hand-edit)
- `ui/` → Vite + React Router 7 + Tailwind CSS v4 frontend
- `deploy/docker-compose/` → reference deployment and Dockerfiles

## Developer commands

| What | Command |
|------|---------|
| Build both binaries | `make build` |
| Build server only | `make build-server` |
| Build agent only | `make build-agent` |
| Run server locally with dev deps | `make dev` |
| Run server locally (no extra services) | `make run-server ARGS=path/to/config.yaml` or `make dev-server` |
| Run agent locally | `make run-agent` |
| Run all Go tests | `make test` |
| Lint Go code | `make lint` (uses `golangci-lint`; no repo-level config file) |
| Regenerate protobuf | `make proto` (runs `buf lint && buf generate`) |
| Clean build artifacts | `make clean` |
| UI dev server | `cd ui && npm run dev` → `http://localhost:5173` |
| UI typecheck | `cd ui && npm run typecheck` |
| UI production build | `cd ui && npm run build` |

## Configuration

- Server config is YAML; pass the path as the first argument (`./skylex-server config.yaml`). `make dev` uses `config.example.yaml`.
- Settings are merged with `koanf`: YAML file + env vars. Env vars use the prefix `SKYLEX_` and nested keys become `_` (e.g. `SKYLEX_DATABASE_DSN`, `SKYLEX_AUTH_JWT_SECRET`).
- `config.example.yaml` is committed and works as-is for local development.
- Defaults exist for most values; see `internal/server/config.go`. `auth.jwt_secret` defaults to `change-me-in-production` in `config.example.yaml` so dev sessions survive restarts. If `auth.jwt_secret` is left empty, a random secret is generated on startup and a warning is logged; existing JWTs will not validate after a restart.
- Agent settings are currently hard-coded defaults in `internal/agent/config.go`, but the agent binary also reads `SKYLEX_AGENT_TOKEN` and `SKYLEX_SERVER_ADDR` from the environment.

## Database and migrations

- Server uses embedded SQLite by default via `modernc.org/sqlite`.
- Migrations are embedded in `internal/db/migrations/*.sql` and applied automatically when the server starts. The migration table is `schema_migrations` and versioning is based on the first 14 characters of the filename.
- SQLite connection is intentionally limited to `SetMaxOpenConns(1)`.

## Protocol buffers and generated code

- Buf v2 is configured in `buf.yaml` (modules under `proto/`, lint + breaking rules) and `buf.gen.yaml` (generates Go gRPC + Connect-RPC into `gen/`).
- Regenerate with `make proto` after changing `.proto` files.
- `go_package_prefix` is `github.com/zhinea/skylex/gen`.

## Testing

- `make test` runs `go test ./...`. There are currently no test files in the repo; this is the canonical command when tests are added.
- The project has no CI workflows, no pre-commit hooks, and no `golangci-lint` config file yet.

## Docker Compose reference stack

- `make docker-up` starts: `skylex-server`, three `skylex-agent` instances, etcd, and MinIO.
- Server exposes `8080` (HTTP), `9090` (gRPC), and `9091` (metrics).
- MinIO console is on `9001`; S3 API on `9000`.
- Agent containers mount dedicated PostgreSQL data volumes (`pg-data-1`, etc.).
- Requires `docker compose` v2 and the env vars `SKYLEX_JWT_SECRET` / `SKYLEX_AGENT_TOKEN` only if you want to override their defaults (`change-me-in-production`, `dev-token`).

## Important conventions

- Go module: `github.com/zhinea/skylex`
- Requires **Go 1.26.1** per `go.mod`.
- UI is React Router 7 with **SSR enabled** (`ssr: true` in `react-router.config.ts`).
- Tailwind is v4 and loaded via `@tailwindcss/vite` in `ui/vite.config.ts`.
- `bin/`, `.vite/`, and `dist/` are gitignored; `make clean` removes `bin/` and `gen/`.
- `.kilo/plans/skylex-db-control-plane-plan.md` is a detailed design/plan document that predates much of the code; use it for architectural intent, but trust executable sources (Makefile, configs, source) for current behavior.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, invoke the `skill` tool with `skill: "graphify"` before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
