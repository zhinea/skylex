# Skylex PostgreSQL Agent Visibility & Management — Phased Plan

## Context

The current agent logs only command start events in JSON format. There is no visibility into:
- command progress, output, or failures in real time
- whether PostgreSQL binaries are installed and healthy on a node
- PostgreSQL configuration/settings from the UI
- novice-friendly guidance when provisioning fails

This plan delivers production-grade PostgreSQL management in **small, independent, mergeable phases**. Each phase is designed to be implemented in one session and leaves the repo in a working state.

---

## Architecture Principles

- **Reuse existing services** — extend `AgentService` / `NodeService` / `ClusterService` rather than creating new services.
- **Efficient queries** — every new DB table gets targeted indexes; list endpoints are paginated.
- **Security first** — agents authenticate via existing tokens; logs contain no secrets (passwords are redacted before transmission).
- **Low latency / high throughput** — agents batch log lines; UI polls at a reasonable cadence (5s default).
- **No duplication** — reuse the existing `postgres.Instance`, migration framework, and UI components (Card, Badge, Table, hooks).

---

## Phase 1 — Live Agent Command Logs in the UI

**Goal:** Surface every step an agent performs while executing a command so a user can watch PostgreSQL provisioning like a build log.

### Server / API
1. **Proto changes** (`proto/skylex/v1/agent.proto`)
   - Add `CommandLogEntry { string command_id; string level; string message; int64 timestamp_ms; }`
   - Add `ReportCommandLog(ReportCommandLogRequest) returns (ReportCommandLogResponse)` to `AgentService`.
   - Keep it unary so it works reliably with the existing gRPC polling model.

2. **Database**
   - Add migration `000004_add_command_logs.sql` for both SQLite and PostgreSQL.
   - Table `agent_command_logs`:
     ```sql
     id TEXT PRIMARY KEY,
     command_id TEXT NOT NULL,
     agent_id TEXT NOT NULL,
     level TEXT NOT NULL,
     message TEXT NOT NULL,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
     ```
   - Indexes: `idx_command_logs_command_id_created_at`, `idx_command_logs_agent_id_created_at`.

3. **Repository** (`internal/db/agent_command_log_repo.go`)
   - `Create(ctx, ...)` — insert a single log line.
   - `ListByCommandID(ctx, commandID, limit)` — newest-first, default limit 500.
   - `ListByNodeID(ctx, nodeID, limit)` — join `agent_commands` to find logs for the latest commands.

4. **AgentService** (`internal/server/agent_service.go`)
   - Implement `ReportCommandLog`: validate agent_id, batch-insert entries, redact any accidental secrets.

5. **NodeService** (`internal/server/node_service.go`)
   - Add `ListNodeCommandLogs(ListNodeCommandLogsRequest) returns (ListNodeCommandLogsResponse)` RPC.
   - Allow filter by `node_id` or `command_id`, paginated.
   - Wire through `connect.go` (no new service needed; NodeService is already exposed over HTTP/Connect).

### Agent
1. **Log capture** (`internal/agent/agent.go`)
   - Add `commandLogger` helper backed by a small ring/channel buffer.
   - Refactor `executeCommand` to run external commands with `StdoutPipe`/`StderrPipe` and stream each line as a log entry.
   - Continue capturing `CombinedOutput` for the final result but also emit per-line logs.
2. **Report loop**
   - During command execution, flush batches of log entries to `ReportCommandLog` every 250ms or when buffer reaches 50 lines.
   - On completion, flush any remaining lines and then report the final result via existing `ReportCommandResult`.

### UI
1. **Hook** (`ui/app/hooks/useCommandLogs.ts`)
   - `useCommandLogs(nodeId?, commandId?, pageSize=50)` using `react-query`, `refetchInterval: 5000`.
2. **Cluster detail page** (`ui/app/routes/clusters.$id.tsx`)
   - Add a **Logs** card showing the most recent command logs for all nodes in the cluster.
   - Display: timestamp, node hostname, level badge, message.
   - Auto-scroll to the newest entry when the user is at the bottom.
3. **Optional** — add a small inline log viewer to the Nodes list when a node is currently in `syncing`/`creating` state.

### Acceptance Criteria
- When creating a cluster, the UI shows log lines like `initdb completed`, `postgresql started`, `replication user created`.
- Failed commands show the exact stderr in the UI without exposing passwords.
- Existing agent behavior remains unchanged if the new log RPC is temporarily unavailable.

---

## Phase 2 — PostgreSQL Installation & Health Visibility

**Goal:** Tell the user, per node, whether PostgreSQL is installed, initialized, running, and what version it is.

### Server / API
1. **Proto changes** (`proto/skylex/v1/agent.proto`, `proto/skylex/v1/cluster.proto`)
   - Extend `NodeStatusReport` with:
     - `bool postgres_installed`
     - `string postgres_bin_version`
     - `bool postgres_data_initialized`
   - Add `NodeCapabilities` message to `RegisterAgentRequest`:
     - `bool postgres_available`
     - `string postgres_version`
     - `string postgres_bin_dir`
     - `string data_dir`

2. **Database**
   - Migration `000005_add_node_pg_status.sql`:
     - `ALTER TABLE nodes ADD COLUMN postgres_installed INTEGER DEFAULT 0;`
     - `ALTER TABLE nodes ADD COLUMN postgres_version TEXT DEFAULT '';`
     - `ALTER TABLE nodes ADD COLUMN postgres_data_initialized INTEGER DEFAULT 0;`

3. **Repositories**
   - `NodeRepository.UpdatePostgresStatus(ctx, nodeID, installed, version, initialized)`.

4. **AgentService**
   - Update `RegisterAgent` to store agent-reported capabilities.
   - Update `ReportStatus` to store node-reported PostgreSQL status.

### Agent
1. **Capability detection** (`internal/postgres/postgres.go`)
   - Add `DetectInstallation(ctx) (installed bool, version string, binDir string, err error)` using `pg_config --version` / `postgres --version` fallback.
   - Add `IsDataDirInitialized()` helper.
2. **Reporting**
   - Send capabilities in `RegisterAgent`.
   - Include `postgres_installed`, `postgres_bin_version`, `postgres_data_initialized` in every `NodeStatusReport`.

### UI
1. **Nodes page** (`ui/app/routes/nodes.tsx`) and **Cluster detail** (`ui/app/routes/clusters.$id.tsx`)
   - Add columns / badges:
     - PostgreSQL installed? (Yes / No)
     - Version (e.g., `16.4`)
     - Data initialized? (Yes / No)
     - Service running? (already partially shown via status, make explicit)
2. **Empty-state guidance**
   - If a node has `postgres_installed=false`, show a callout:
     > “PostgreSQL binaries were not detected. Install PostgreSQL {cluster version} on this host and the agent will detect it automatically.”

### Acceptance Criteria
- A node with no PostgreSQL shows a clear “PostgreSQL not installed” indicator.
- After installing PostgreSQL on the host, the badge updates on the next heartbeat/status report.
- The cluster creation flow can warn before creating if any target node lacks PostgreSQL.

---

## Phase 3 — PostgreSQL Settings Management

**Goal:** Allow users to view and edit a curated, validated set of PostgreSQL parameters without touching configuration files.

### Server / API
1. **Proto changes** (`proto/skylex/v1/cluster.proto`)
   - Add messages:
     ```protobuf
     message ClusterSettings { map<string, string> parameters = 1; }
     message GetClusterSettingsRequest { string cluster_id = 1; }
     message GetClusterSettingsResponse { ClusterSettings settings = 1; }
     message UpdateClusterSettingsRequest { string cluster_id = 1; ClusterSettings settings = 2; }
     message UpdateClusterSettingsResponse { Cluster cluster = 1; }
     ```
   - Add RPCs to `ClusterService`.

2. **Database**
   - Migration `000006_add_cluster_settings.sql`:
     ```sql
     CREATE TABLE cluster_settings (
       id TEXT PRIMARY KEY,
       cluster_id TEXT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
       key TEXT NOT NULL,
       value TEXT NOT NULL,
       created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
       updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
       UNIQUE(cluster_id, key)
     );
     CREATE INDEX idx_cluster_settings_cluster_id ON cluster_settings(cluster_id);
     ```

3. **Repository** (`internal/db/cluster_settings_repo.go`)
   - `GetByClusterID`, `Set`, `Delete`, `ListKeys`.

4. **ClusterService**
   - Implement `GetClusterSettings` / `UpdateClusterSettings`.
   - Validate allowed keys and value ranges/syntax server-side (e.g., `max_connections`, `shared_buffers`, `wal_level`, `max_wal_senders`, `work_mem`).
   - Queue `pg_apply_settings` commands to all nodes in the cluster.

5. **Agent command** (`internal/agent/agent.go`)
   - Add new action `pg_apply_settings`:
     - Write merged settings into a `skylex.conf.include` file inside the data directory.
     - Include it from `postgresql.conf` (Phase 3 will need a one-time conf update; safe because Phase 1 already writes a base conf).
     - Issue `pg_ctl reload` if possible; otherwise safe restart via existing `pg_stop` + `pg_start` commands.

### UI
1. **Cluster detail page** (`ui/app/routes/clusters.$id.tsx`)
   - Add a **Settings** tab/card.
   - Show key/value editor for curated parameters with inline validation hints.
   - Submitting changes queues the apply command and shows a toast + live command logs from Phase 1.
2. **Reusable components**
   - Add `SettingInput.tsx` for typed inputs (number, memory string, enum).
   - Reuse existing `Card`, `Badge`, and `ConfirmDialog`.

### Acceptance Criteria
- User can change `max_connections` and `shared_buffers` from the UI.
- Invalid values are rejected before any command is queued.
- Settings are persisted per cluster and applied to all nodes.
- A command log entry shows whether reload or restart was used.

---

## Phase 4 — Novice-Friendly Operations & Error Recovery

**Goal:** Make the first-run and day-2 operations obvious even for users who have never operated PostgreSQL.

### Server / API
1. **Cluster creation preflight**
   - In `ClusterService.CreateCluster`, before assigning nodes:
     - Verify each candidate node has `postgres_installed=true`.
     - Return a clear `FailedPrecondition` with the offending hostnames if not.
2. **Better status messages**
   - Extend `NodeStatus` (or add a `status_detail` text field) to store human-readable state:
     - `waiting_for_postgres`
     - `initializing_data_directory`
     - `starting_service`
     - `syncing_replica`
     - `healthy`
   - Drive status from the latest command logs / statuses.

### Agent
1. **Command outcomes**
   - Send richer `output` strings (e.g., `PostgreSQL 16.4 is running on port 5432`) instead of terse messages.
2. **Self-healing hints**
   - On `pg_init`/`pg_start` failure, include a short, actionable hint in the result:
     - e.g., permission denied → check `data_dir` ownership; port in use → stop existing PostgreSQL.

### UI
1. **Cluster creation wizard** (`ui/app/routes/clusters.create.tsx`)
   - Step 1: Verify agents / PostgreSQL availability.
   - Step 2: Choose cluster name, version, replicas.
   - Step 3: Review and create.
   - Show inline validation when not enough idle nodes or PostgreSQL is missing.
2. **Cluster detail page**
   - Add a “Diagnostics” card with:
     - Current operation / overall progress bar.
     - Last error + suggested fix.
     - One-click actions: **Restart Node**, **Re-sync Replica**, **View Logs**.
3. **Nodes page**
   - Add **Rejoin** action for offline nodes.
   - Add a tooltip-style status detail on hover.

### Acceptance Criteria
- A first-time user creating a cluster sees step-by-step guidance and clear errors.
- PostgreSQL missing on an agent is surfaced before cluster creation instead of silently failing commands.
- Common errors show suggested next steps in the UI.

---

## Cross-Cutting Concerns

- **Migrations** — maintain parity between `internal/db/migrations/sqlite/` and `internal/db/migrations/postgres/`. Each phase adds one migration pair.
- **Protobuf** — run `make proto` after each phase that changes `.proto` files.
- **Tests** — add Go unit tests for repositories and service methods; add at least one happy-path UI typecheck after major UI changes.
- **Secrets** — the agent must never transmit `PGPASSWORD`, replication passwords, or `primary_conninfo` passwords in logs. Add a small redaction helper in `internal/agent/log.go`.
- **Backward compatibility** — older agents that do not call `ReportCommandLog` continue to work; the UI simply shows no live logs for their commands.

---

## Suggested Session Order

1. **Session A** — Phase 1 (command logs). This is the highest-impact, lowest-risk change.
2. **Session B** — Phase 2 (PostgreSQL installation/health status).
3. **Session C** — Phase 3 (PostgreSQL settings editor).
4. **Session D** — Phase 4 (novice-friendly wizard and diagnostics).

Each session should end with the project building and tests passing.
