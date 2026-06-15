# Skylex Database Control Plane — Development Plan

> **Status:** Finalized — based on confirmed decisions: PostgreSQL-first, Docker Compose primary target, single-organization, pgBackRest + etcd, embedded SQLite first, product name **Skylex**.

---

## 1. Vision

Build an open, self-hosted **database control plane** called **Skylex** that makes running production-grade databases as simple as using a managed DBaaS, while keeping the data on the user’s own infrastructure.

Defaults are opinionated for production:
- **PITR (Point-in-Time Recovery)** enabled by default.
- **Automated backups** with encryption, compression, and retention.
- **High availability** with automatic failover and consensus-backed leader election.
- **Multi-server / multi-cluster** management from a single pane.
- **Declarative, simple configuration** (YAML + UI), with agents that converge nodes to the desired state.

### MVP engine
The first version supports **PostgreSQL only**. MySQL, MongoDB, Redis, and others are planned for later phases.

### Target user
A **single organization** managing its own database fleet. Multi-tenancy (SaaS-style isolation, quotas, billing hooks) is explicitly out of scope for the MVP.

---

## 2. Problem Statement

Existing tools each solve a slice of the problem:
- Patroni / repmgr → HA only.
- pgBackRest / WAL-G / Barman → backup only.
- Ansible/Pulumi/Terraform → deployment only, not runtime operations.
- Managed cloud DBaaS → easy, but locks data into a vendor.

There is no single, cohesive, self-hosted product that combines HA, PITR, backup/restore, and multi-server fleet management with a modern UI and simple configuration.

---

## 3. Goals & Non-Goals

### Goals
- Single control plane managing many PostgreSQL clusters across many servers.
- Docker Compose first: a working reference stack within minutes.
- One-command or UI-driven provisioning of a highly-available PostgreSQL cluster.
- Continuous WAL archiving and scheduled base backups out of the box.
- PITR restore to a specific point in time / LSN / transaction ID.
- Automatic failover with no data loss in synchronous mode, minimal RPO in async mode.
- mTLS-secured agent/server communication.
- Web UI for operators and read-only/auditor users.
- S3-compatible object storage as the default backup target.

### Non-Goals (MVP)
- Multi-tenant SaaS billing/quota isolation.
- General-purpose SQL editor / BI tooling.
- Support for non-PostgreSQL engines.
- Replacing enterprise monitoring stacks (we integrate with Prometheus, not rebuild it).

---

## 4. Confirmed Design Decisions

| Topic | Decision | Rationale |
|-------|----------|-----------|
| **Engine** | PostgreSQL first | Richest WAL/PITR ecosystem; clear MVP scope. |
| **Deployment first** | Docker Compose | Fastest onboarding for single-org users. |
| **Tenancy** | Single organization | Simpler auth model; multi-tenant features added later. |
| **Backup engine** | pgBackRest | Mature, feature-complete, proven for PITR. |
| **DCS / HA** | etcd | Widely adopted, stable consensus for leader election. |
| **Control-plane metadata** | Embedded SQLite (default) | Keeps the server a single binary; external Postgres HA mode added later. |
| **Frontend** | Vite + React Router 7 + Tailwind CSS | Already scaffolded in `ui/`. |
| **Backend** | Go 1.26 | Matches existing `go.mod`. |
| **Product name** | Skylex | Matches existing module `github.com/zhinea/skylex`. |

---

## 5. High-Level Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                     Skylex Control Plane                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │   Web UI     │  │  REST/gRPC   │  │  Job Scheduler   │  │
│  │  (Vite+React)│  │    API       │  │ (backup/restore) │  │
│  └──────────────┘  └──────┬───────┘  └──────────────────┘  │
│                           │                                  │
│  ┌────────────────────────┴──────────────────────────────┐  │
│  │              Skylex Server (Go binary)                 │  │
│  │   - metadata store (SQLite default, Postgres optional) │  │
│  │   - cluster/node desired-state model                 │  │
│  │   - worker queue for long-running operations         │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                            │ gRPC (mTLS)
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│ Skylex Agent │   │ Skylex Agent │   │ Skylex Agent │
│   (node-1)   │   │   (node-2)   │   │   (node-3)   │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │                  │                  │
   ┌───▼────┐         ┌───▼────┐         ┌───▼────┐
│ PostgreSQL │   │ PostgreSQL │   │ PostgreSQL │
│  primary   │◄──│  replica   │   │  replica   │
└────────────┘   └────────────┘   └────────────┘
       │                  │                  │
       └──────────────────┴──────────────────┘
                         │ WAL archive + base backups
                         ▼
              ┌──────────────────────┐
              │ S3-compatible object │
              │   storage (encrypted)│
              └──────────────────────┘

        ┌─────────────────────────────────────────┐
        │ etcd (Distributed Config Store)         │
        │   leader election + membership          │
        └─────────────────────────────────────────┘
```

### Components

| Component | Tech | Responsibility |
|-----------|------|----------------|
| **Skylex Server** | Go 1.26+ | Control plane: API, scheduler, metadata, state machine, metrics. |
| **Skylex Agent** | Go 1.26+ | Runs on every DB node: executes local operations, reports state, manages PostgreSQL process. |
| **Metadata Store** | SQLite (default), Postgres (future HA control-plane mode) | Cluster/node/backup/job state and users. |
| **DCS** | etcd | Leader election, membership, distributed locks. |
| **Backup Engine** | pgBackRest | Base backups, WAL archiving, restore, PITR. |
| **Proxy Layer** | PgBouncer / HAProxy / built-in service discovery | Routing applications to the current primary. |
| **Frontend** | Vite + React Router 7 + Tailwind CSS | Dashboard, cluster management, backup/restore flows. |
| **Observability** | Prometheus `/metrics`, structured slog logs, optional OpenTelemetry | Alerting, dashboards, audit trails. |

---

## 6. Technology Stack

### Backend
- **Language:** Go 1.26 (matches existing `go.mod`).
- **API:** gRPC for agents; REST/JSON + gRPC-Web (or Connect-RPC) for UI.
- **Persistence:** SQLite via `modernc.org/sqlite` for embedded deployments; PostgreSQL driver support for future external HA metadata.
- **Schema/Migrations:** `pressly/goose` or `golang-migrate`.
- **Configuration:** `koanf` (YAML/env) or `viper`.
- **Job Queue:** In-house persistent worker (SQLite-backed) for MVP; later support Postgres-backed queue (e.g. `riverqueue/river`).
- **DCS Client:** `go.etcd.io/etcd/client/v3`.
- **Postgres Driver:** `jackc/pgx/v5` for health/lag queries.
- **Object Storage:** `minio/minio-go` (S3-compatible).
- **Encryption:** `x/crypto` for secrets at rest; AES-256-GCM/SSE-S3/SSE-KMS for backups.
- **Build:** standard `go build`, multi-platform CI, container images.

### Frontend
- **Framework:** React Router 7 (already configured in `ui/`).
- **Bundler:** Vite (already configured).
- **Styling:** Tailwind CSS v4 (already configured).
- **Data Fetching:** TanStack Query (React Query) + generated TypeScript client from OpenAPI spec.
- **Components:** Start with Tailwind primitives; optionally adopt Radix + shadcn/ui if complexity grows.
- **State Management:** Server-first via React Query; local UI state with React context or Zustand.

### DevOps & Packaging
- **Container Runtime:** Docker / Podman.
- **Deployment Modes:**
  1. Docker Compose (primary reference deployment).
  2. Single-node binary + systemd.
  3. Kubernetes Helm chart (future).
- **CI/CD:** GitHub Actions (lint, test, build, container push).
- **E2E Tests:** Playwright for UI; Testcontainers or ephemeral VMs for backend.

---

## 7. Repository Layout

```text
/home/zynos/projects/skylex
├── cmd/
│   ├── server/          # skylex-server
│   ├── agent/           # skylex-agent
│   └── cli/             # skylexctl (optional)
├── internal/
│   ├── server/          # API, auth, scheduler, state machines
│   ├── agent/           # agent logic
│   ├── db/              # metadata store abstractions & migrations
│   ├── dcs/             # etcd wrappers
│   ├── backup/          # backup engine abstraction + pgBackRest wrapper
│   ├── postgres/        # Postgres lifecycle (init, config, replication)
│   ├── crypto/          # encryption/key management
│   └── models/          # shared domain models
├── pkg/                 # reusable public packages
├── proto/               # protobuf definitions (agent ↔ server)
├── ui/                  # existing Vite + React Router frontend
├── deploy/
│   ├── docker-compose/
│   ├── systemd/
│   └── helm/            # future
├── config.example.yaml
├── Makefile
└── README.md
```

The existing `module/` directory should be renamed to `internal/` to follow Go conventions.

---

## 8. Core Feature Specification

### 8.1 Cluster & Node Lifecycle
- Declarative cluster spec (YAML or UI):
  - engine: `postgresql`
  - version, instance sizing, data directory
  - replication mode: synchronous / asynchronous
  - number of replicas
  - storage backend reference
- Agent registers nodes with the server using a one-time token.
- Server converges nodes to desired state:
  - initialize primary,
  - clone replicas via `pg_basebackup`,
  - configure `postgresql.conf`, `pg_hba.conf`, replication slots.
- Node health checks every 10s (configurable).
- API/UI actions: start, stop, restart, reconfigure, scale replicas up/down, delete cluster.

### 8.2 Backup & Restore
- Continuous WAL archiving to object storage.
- Scheduled base backups: full, incremental, differential (policy per cluster).
- Encryption: AES-256-GCM with a cluster-level key or KMS integration.
- Retention policies: count-based and time-based.
- On-demand backups from UI/API.
- Restore modes:
  - Full restore to latest backup.
  - Point-in-Time Recovery (PITR) to a chosen timestamp / LSN / xid.
  - Clone to a new cluster without disrupting the source.
- Restore flow spins up a new node/cluster, replays WAL, and optionally reattaches replicas.

### 8.3 High Availability
- etcd-backed leader election.
- Agent-side watchdog / token lease to detect unresponsive primaries.
- Automatic promotion of the most caught-up replica on primary failure.
- Rewind/rejoin failed former primary as replica (`pg_rewind`).
- Replication lag monitoring and alerts.
- Application routing via service discovery or an optional PgBouncer/HAProxy config pushed by the agent.
- Synchronous replication option for RPO=0 (with performance trade-off).
- Fencing hooks (IPMI, cloud provider APIs) in later phases to reduce split-brain risk.

### 8.4 Multi-Server Management
- Single server can manage many clusters across many hosts/data centers.
- Agent installed per server via Docker Compose or package with a bootstrap token.
- Tagging/labeling of nodes (region, rack, env).
- Anti-affinity rules for replica placement.
- Global dashboard showing fleet status, storage usage, backup health, alarms.

### 8.5 Security
- mTLS between agent and server (server signs agent certs or validates pinned tokens).
- TLS for Postgres client and replication connections.
- Encrypted secrets in control-plane DB (keys via environment or KMS).
- RBAC: Admin, Operator, Viewer.
- Audit log for all state-changing API calls and agent commands.
- Optional SSO (OIDC / SAML) in later phases.

### 8.6 Observability
- Prometheus metrics endpoint on server and agents:
  - cluster health, node role, replication lag,
  - backup success/failure counts + duration,
  - storage bytes, WAL archive lag,
  - API request latency/errors.
- Structured JSON logs via Go `slog`.
- Alerting webhooks (Slack, PagerDuty) for backup failures, failover events, high lag.
- UI dashboard with recent events and health cards.

---

## 9. API Design

Use **Connect-RPC** so one set of `.proto` files serves both agents and UI.

Service surface:

- `ClusterService`
  - `CreateCluster`, `UpdateCluster`, `DeleteCluster`, `GetCluster`, `ListClusters`
  - `FailoverCluster`, `RestartNode`, `ScaleCluster`
- `NodeService`
  - `ListNodes`, `GetNode`, `DrainNode`, `RejoinNode`
- `BackupService`
  - `CreateBackup`, `ListBackups`, `DeleteBackup`, `GetBackup`
  - `CreateRestoreJob`, `ListRestoreJobs`
- `ScheduleService`
  - `CreateSchedule`, `UpdateSchedule`, `ListSchedules`, `DeleteSchedule`
- `StorageService`
  - `CreateStorageConfig`, `ListStorageConfigs`, `ValidateStorageConfig`
- `AgentService` (agent-facing)
  - `RegisterAgent`, `Heartbeat`, `ReportStatus`, `FetchCommand`, `ReportCommandResult`
- `AuthService`
  - `Login`, `RefreshToken`, `ListUsers`, `CreateAPIKey`

Generated OpenAPI/Typescript client will drive the frontend.

---

## 10. Data Model (Metadata Store)

Key entities in the control-plane database:

- `users`, `roles`, `permissions`
- `clusters` — desired spec, engine, version, status
- `nodes` — cluster_id, hostname, role, address, labels, agent_version, last_seen
- `storage_configs` — type, bucket/endpoint, encrypted credentials, region
- `backups` — cluster_id, node_id, type, storage_path, wal_start/stop, lsn, size, status, created_at
- `restore_jobs` — cluster_id, target_time/lsn, target_node, status, created_at, completed_at
- `schedules` — cluster_id, cron, retention, storage_config_id, enabled
- `audit_logs` — user, action, resource, timestamp, ip
- `agent_tokens` — token hash, role, expiration

SQLite is sufficient for small-to-medium fleets (hundreds of clusters). A PostgreSQL-backed mode will be added when the server itself must be HA.

---

## 11. UI / Frontend Plan

Pages:

1. **Login** — email/password or API key (OIDC in v2).
2. **Dashboard** — fleet summary, clusters health, latest backups, active alerts.
3. **Clusters**
   - List view with status badges.
   - Create wizard (engine, version, nodes, storage, HA toggle, PITR toggle).
   - Detail view: topology graph, node status, metrics sparklines, actions.
4. **Nodes** — list of all nodes, labels, lag, last seen.
5. **Backups** — backups per cluster, restore button, schedule editor.
6. **Restore** — wizard: pick source cluster/backup, target time, destination cluster, run/schedule.
7. **Storage** — manage S3-compatible storage backends.
8. **Settings** — users, roles, agent tokens, notifications, global defaults.
9. **Audit Logs** — read-only event stream.

Frontend conventions:
- Use server-first data loading (React Router loaders + TanStack Query).
- Keep forms wizard-driven to enforce correctness (e.g., PITR cannot be enabled without WAL archiving + storage).
- Dark/light mode optional.

---

## 12. Implementation Phases

### Phase 0 — Foundation (4 weeks)
- Finalize repository layout, CI, linting, protobuf tooling.
- Metadata schema + migrations (SQLite).
- Server bootstrap: config, logging, basic HTTP/gRPC server.
- Agent bootstrap: registration + heartbeat over gRPC (mTLS placeholders).
- Frontend shell: layout, routing, auth stubs, empty pages.
- Docker Compose skeleton for server + agent + etcd + object storage.

**Deliverable:** Server and agent compile and can register; UI shell renders; `docker compose up` starts the control plane stack.

### Phase 1 — PostgreSQL Lifecycle (5 weeks)
- Agent can initialize/start/stop/configure PostgreSQL inside the Compose stack.
- Primary bootstrap and replica cloning via `pg_basebackup`.
- Cluster CRUD API + UI.
- Health checks and basic metrics.
- Node status reporting.

**Deliverable:** User can create a 1-primary + N-replica PostgreSQL cluster from the UI inside Docker Compose.

### Phase 2 — Backup & PITR (6 weeks)
- Storage config CRUD (S3-compatible default).
- WAL archiving integration with pgBackRest.
- Scheduled full/incremental/differential backups.
- On-demand backup UI/API.
- Restore job worker: full and PITR to a new cluster.
- Backup encryption and retention policies.

**Deliverable:** PITR-enabled backups work out of the box; restore wizard functional.

### Phase 3 — High Availability (5 weeks)
- etcd DCS integration.
- Leader election and failover decision engine.
- Replica promotion + former-primary rejoin.
- Synchronous replication option.
- PgBouncer/HAProxy config push or service-discovery metadata.
- Failover simulation tests.

**Deliverable:** Automated failover with documented RPO/RTO in the Docker Compose environment.

### Phase 4 — Security & Operations (4 weeks)
- Full mTLS agent-server.
- RBAC, users, API keys.
- Encrypted secrets at rest.
- Audit logging.
- Webhook alerting.
- Backup/restore of control-plane metadata.

**Deliverable:** Production-hardened authentication, authorization, and audit.

### Phase 5 — Packaging & Scale (4 weeks)
- Polished Docker Compose reference deployment.
- Systemd unit packaging.
- Helm chart + Kubernetes operator (future-facing scaffold).
- Server HA mode (Postgres-backed metadata).
- Performance benchmarking (≥100 clusters).
- Documentation, runbooks, and quick-start guide.

**Deliverable:** GA release with installable packages.

### Phase 6 — Additional Database Engines (future)
- MySQL Group Replication + Percona XtraDB Cluster.
- MongoDB replica sets + oplog-based PITR.
- Redis (AOF/RDB backups).

---

## 13. Milestones & Release Targets

| Milestone | Target | Definition of Done |
|-----------|--------|--------------------|
| **M1 Alpha** | End of Phase 1 | Single PostgreSQL cluster up/reachable with replicas; basic UI. |
| **M2 Beta** | End of Phase 3 | HA failover + PITR backups/restore; multi-node setup works. |
| **M3 GA** | End of Phase 5 | Packaged, documented, security-reviewed, supports 100+ clusters. |

---

## 14. Risk Analysis

| Risk | Impact | Mitigation |
|------|--------|------------|
| Split brain during failover | Data loss / corruption | etcd consensus, leader leases, fencing hooks, synchronous option. |
| Metadata store failure | Loss of cluster knowledge | Regular metadata backups; optional Postgres HA mode in Phase 5. |
| pgBackRest dependency mismatch | Restore failures | Pin versions; plugin abstraction; extensive restore integration tests. |
| Network partitions | False failovers | Conservative failure detectors, configurable quorum rules. |
| Scope creep | Slipping schedule | Strict MVP boundary: PostgreSQL first, other engines later. |

---

## 15. Next Steps

1. Review and approve this plan.
2. Begin **Phase 0: Foundation**:
   - reorganize repository layout (`module/` → `internal/`),
   - set up protobuf tooling and CI,
   - implement agent registration/heartbeat,
   - create the Docker Compose reference stack.
