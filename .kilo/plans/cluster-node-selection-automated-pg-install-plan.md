# Cluster Node Selection & Automated PostgreSQL Installation — Phased Plan

## Goal
Remove the requirement for users to manually install PostgreSQL on agent hosts. After linking an agent, the user selects which nodes belong to a cluster, chooses whether PostgreSQL runs **native** or **Dockerized**, and Skylex provisions it automatically. Per-node installation logs and conflicts (e.g. existing native PostgreSQL) are surfaced in the UI.

## Constraints
- Every phase must leave the repository in a **buildable, working state**.
- Phases are implemented in separate sessions; only one phase is merged at a time.
- Reuse existing components/services/utilities; avoid duplication.
- Follow Go best practices, keep queries efficient, and prioritize security.

## Current Baseline Observations
- `CreateClusterRequest` has no `node_ids`; the server auto-assigns idle nodes via `NodeRepository.ListUnassigned`.
- `NodeService` and command-log infrastructure already exist and will be reused.
- `AgentCommand` queue/result/log flow already exists.
- The `nodes` table tracks `postgres_installed`, `postgres_version`, `postgres_data_initialized`, and `status_detail`.
- There is a migration-version collision: both `000004_add_command_logs.sql` and `000004_add_node_pg_status.sql` exist. One will be skipped depending on lexical order, which is unsafe.

## Assumptions to Confirm
1. **Node roles:** The first selected node becomes the primary and the remaining selected nodes become replicas automatically. Is this acceptable, or do you want explicit role assignment in the UI?
2. **Docker hosting:** Should the agent install Docker Engine if it is missing, or should it assume Docker is already available on the host?
3. **Native conflict resolution:** If native PostgreSQL already exists and the user chooses "remove it," should the agent purge OS packages **and** delete the data directory, or only stop/disable the service? Should "proceed otherwise" mean adopting the existing PostgreSQL instance into the cluster?
4. **Native installation source:** Should the agent install PostgreSQL via the host's package manager (apt/dnf/apk) matching the cluster version, or via a specific repository/channel?

## Phase 0 — Repair Migration Baseline
**Purpose:** Fix the existing migration-version collision before adding new schema changes.

- Rename/consolidate the duplicate `000004` migrations so the sequence is strictly ordered.
  - Option A: fold `add_node_pg_status` into `000004` and `add_command_logs` into `000005` (rename `000005_add_cluster_settings.sql` → `000006...`, `000006_add_node_status_detail.sql` → `000007...`).
  - Option B: keep existing files as-is but add an idempotent `000007_repair_duplicate_migrations.sql` that adds any missing columns/indexes.
  - **Recommended:** Option B for existing dev databases, but rename files in the repo for clarity.
- Add defensive `IF NOT EXISTS` to affected columns/indexes.
- Run `make dev-server` and `make test` on a fresh SQLite database to verify.

**Deliverables:** No application behavior change; clean migration baseline.

## Phase 1 — Explicit Node Selection in Cluster Creation
**Purpose:** Let the user pick exactly which idle nodes form a cluster.

### Backend
1. **Proto** (`proto/skylex/v1/cluster.proto`):
   - Add `repeated string node_ids = 3;` to `CreateClusterRequest`.
2. **Repository** (`internal/db/node_repo.go`):
   - Add `GetByIDs(ctx, ids []string) ([]*models.Node, error)`.
   - Keep `ListUnassigned` for now (display only).
3. **Service** (`internal/server/cluster_service.go`):
   - Validate `len(node_ids) == replica_count + 1`.
   - Verify all nodes exist and are unassigned (`cluster_id` empty).
   - Replace auto-assignment with explicit assignment.
   - Keep the current preflight check that requires PostgreSQL to be installed (removed in Phase 3).
   - Wrap cluster creation + node assignment in a database transaction through the repository layer.
4. **Proto generation:** Run `make proto`.

### UI
1. `ui/app/routes/clusters.create.tsx`:
   - Replace the read-only "Verify Nodes" table with a multi-select checklist of idle nodes.
   - Disable nodes already assigned to clusters.
   - Show `1 primary + N replicas` count requirement.
   - Pass `nodeIds` to `createCluster.mutateAsync`.
2. `ui/app/hooks/useClusters.ts`:
   - Update `useCreateCluster` input type to include `nodeIds: string[]`.
3. After a successful create, `navigate(`/clusters/${cluster.id}`)`.

**Deliverables:** User can select nodes, create a cluster, and be redirected to the detail page.

## Phase 2 — Service Location Model
**Purpose:** Persist the decision to run PostgreSQL natively or inside Docker.

### Backend
1. **Proto**:
   - Add `enum ServiceLocation { SERVICE_LOCATION_UNSPECIFIED = 0; SERVICE_LOCATION_NATIVE = 1; SERVICE_LOCATION_DOCKER = 2; }` to `cluster.proto`.
   - Add `ServiceLocation service_location = 8;` to `ClusterConfig`.
   - Add `ServiceLocation service_location = 16;` to `Cluster` and `Node`.
   - Add `bool docker_available = 5;` to `NodeCapabilities` in `agent.proto`.
2. **Migrations**:
   - Add `service_location` columns to `clusters` and `nodes` (default `native`).
3. **Models** (`internal/models/cluster.go`, add a node model file or extend existing):
   - Add `ServiceLocation` type/constants.
4. **Agent** (`internal/agent/agent.go`):
   - Detect Docker availability (e.g. `docker version` command) and report it in `RegisterAgent` + `ReportStatus`.
5. **Server**:
   - Store service location on cluster and propagate to assigned nodes.
   - Validate: if `docker` is selected, warn when a node lacks `docker_available`.
   - Include service location in `clusterToProto` and `nodeToProto`.
6. **Proto generation:** `make proto`.

### UI
1. `clusters.create.tsx`:
   - Add a select for **Service Location** (Native / Dockerized).
   - Show an icon/warning for Docker-unavailable nodes when Docker is selected.
2. `clusters.$id.tsx`:
   - Display service location in the Configuration card.

**Deliverables:** Service location is a first-class cluster/node attribute and is visible in the UI.

## Phase 3 — Automated PostgreSQL Installation / Docker Provisioning
**Purpose:** The agent installs PostgreSQL automatically based on the chosen service location.

### Backend
1. **New package** `internal/agent/installer`:
   - Define `type Installer interface { Install(ctx context.Context, cfg InstallConfig, log LogSink) error; Purge(ctx context.Context, cfg InstallConfig, log LogSink) error; }`.
   - Implement `NativeInstaller`:
     - Detect package manager (apt, dnf, apk, zypper).
     - Install the requested PostgreSQL version.
     - Update `postgres.Instance.BinDir`/`Version` appropriately.
   - Implement `DockerInstaller`:
     - Pull/run the official Postgres container with a persistent host volume mapped to `cfg.DataDir`.
     - Use `exec.Command` with argument slices (no shell strings).
   - Reuse `postgres.Instance` for runtime commands once native install is complete.
2. **Agent commands** (`internal/agent/agent.go`):
   - `pg_preflight` — check existing PG / Docker readiness (Phase 4 foundation; can be a no-op here).
   - `pg_install_native` — call `NativeInstaller.Install`.
   - `pg_install_docker` — call `DockerInstaller.Install`.
   - `pg_purge_native` — for conflict resolution later.
3. **Server** (`internal/server/cluster_service.go`):
   - In `CreateCluster`, before queuing `pg_init`, queue the appropriate install command for nodes that are not already installed.
   - Do **not** fail if a native node already has PostgreSQL; adopt it (full conflict handling is Phase 4).
   - Transition cluster status from `CREATING` → `HEALTHY` only when primaries/replicas complete successfully.

### UI
1. `clusters.$id.tsx`:
   - Add a card "Installation Progress" showing per-node status and a tail of command logs.
   - Reuse existing `useCommandLogs` hook, filtered by `clusterId`.

**Deliverables:** A Docker-selected cluster provisions without host PostgreSQL; native installation runs automatically on supported OS; logs stream to the detail page.

## Phase 4 — Native Conflict Detection & Resolution
**Purpose:** When native PostgreSQL already exists, surface a choice to the user instead of failing silently or overwriting data.

### Backend
1. **Schema:**
   - Add `installation_state` to `nodes` (e.g. `pending_preflight`, `conflict`, `installing`, `installed`, `failed`, `adopted`).
2. **Proto** (`agent.proto`):
   - Extend `NodeStatusReport` with `InstallationState installation_state` and `string conflict_details`.
3. **Agent**:
   - `pg_preflight`: detect existing native PostgreSQL and existing data directory. Report:
     - `NOTHING_FOUND` → safe to install.
     - `PG_EXISTS` → existing installation; include version, data-dir path, and whether data exists.
   - `pg_purge_native`: stop service, remove packages, remove data directory (after user confirmation).
   - `pg_adopt_native`: set `PostgresInstalled=true` and continue without install.
4. **Server**:
   - On `CreateCluster`, queue `pg_preflight` for all selected native nodes.
   - Mark nodes with `installation_state=conflict` when `PG_EXISTS` is reported.
   - Add `NodeService.ResolveInstallationConflict` RPC accepting `node_id` + action (`ADOPT`, `PURGE`, `ABORT`).
   - On `PURGE`, queue `pg_purge_native` then `pg_install_native`. On `ADOPT`, queue normal init commands. On `ABORT`, mark cluster `FAILED`.
5. **Proto generation:** `make proto`.

### UI
1. `clusters.$id.tsx`:
   - When any node has `installation_state=conflict`, render a modal per node explaining the risk.
   - Buttons: **Use Existing (Adopt)**, **Remove & Reinstall (data loss)**, **Abort Cluster Creation**.
   - Poll node status/refetch logs until resolved.

**Deliverables:** Native PostgreSQL conflicts are detected, displayed, and resolved through the UI without unplanned data loss.

## Phase 5 — Security, Performance, Cleanup & Tests
**Purpose:** Harden and clean up before considering the feature complete.

1. **Security:**
   - Never pass user input to shell strings; use `exec.Command` with slice arguments.
   - Validate all IDs in `GetByIDs` and conflict resolution.
   - Redact secrets in command output (reuse `RedactSecrets`).
   - Audit-log cluster creation and conflict resolutions.
2. **Performance/database:**
   - Add indexes:
     - `idx_nodes_cluster_id_role` for primary lookup.
     - `idx_nodes_installation_state` for conflict polling.
     - `idx_agent_commands_node_id_status` for fetch/result lookups.
   - Keep `ListByCluster` query bounded.
3. **Code cleanup:**
   - Remove dead code paths from the old auto-assignment logic (`ListUnassigned` if no longer used).
   - Centralize node status-detail computation.
4. **Testing:**
   - Unit tests for `ClusterService.CreateCluster` validation.
   - Unit tests for native/Docker installer argument builders.
   - Integration test for migration applicability.
   - Run `make test` and `make lint`.
5. **Documentation:**
   - Update `AGENTS.md` with the new cluster-creation workflow and service-location behavior.

**Deliverables:** All tests and lint pass; documentation updated; no redundant code.

## Cross-Phase Rules
- Each phase ends with `make proto`, `make build`, `make test`, and a smoke run of `make dev-server`.
- New migrations are added to **both** `internal/db/migrations/sqlite/` and `internal/db/migrations/postgres/`.
- No hand-editing of files under `gen/`; all proto changes go through `make proto`.
- UI hooks and components are reused/extended (`useNodes`, `useCommandLogs`, `Card`, `Badge`, `ConfirmDialog`).
