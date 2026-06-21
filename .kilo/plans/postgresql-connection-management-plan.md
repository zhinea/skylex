# PostgreSQL Connection Management Plan

## Purpose

Skylex currently provisions PostgreSQL on service agents and shows installation progress, but it does not clearly answer how an operator or application connects to the newly installed PostgreSQL cluster. This plan defines a phased path from read-only connection visibility to full PostgreSQL connection management: endpoints, roles, databases, passwords, access controls, TLS, connection testing, and failover-safe connection guidance.

## Current State

- Provisioning queues PostgreSQL install/init/start/replication commands from `internal/server/cluster_service.go`.
- Agents execute PostgreSQL lifecycle commands in `internal/agent/agent.go` and helper logic in `internal/postgres/postgres.go`.
- Command results update node installation/readiness state in `internal/server/agent_service.go`.
- Nodes already store address, port, role, Docker/native service location, install status, data initialization status, agent latency, and latest metrics.
- Cluster detail UI shows installation progress, conflicts, diagnostics, node actions, command logs, and curated PostgreSQL settings in `ui/app/routes/clusters.$id.tsx`.
- The UI does not show a dedicated connection section with host, port, database, URI templates, psql examples, replica endpoints, or failover warnings.
- Existing secret encryption patterns are available in `internal/crypto/crypto.go` and `internal/db/storage_config_repo.go`.
- Current generated PostgreSQL config is permissive (`listen_addresses = '*'` and broad `pg_hba.conf` entries); this is acceptable only as an interim bootstrap behavior, not as the final connection security model.

## Architecture Decisions

- Connection visibility is cluster-scoped, because applications connect to a cluster role, not to an arbitrary node.
- Static connection configuration belongs in dedicated connection/profile tables, not in `clusters` or `nodes`.
- Managed roles, databases, and credentials belong in dedicated tables with encrypted secrets, status, and timestamps.
- Repeatedly emitted connection checks and operational history belong in operation/check-result tables, not on entity tables.
- PostgreSQL management actions should reuse the existing server-to-agent command queue instead of adding a second remote execution path.
- Mutating PostgreSQL DDL should run on the current primary unless the action is explicitly node-local, such as applying `pg_hba.conf`.
- Secrets must not be stored in plaintext in command payloads, logs, audit details, or UI state beyond a one-time reveal after creation/rotation.
- Phase 1 intentionally does not introduce new schema or credentials; it computes safe read-only connection information from existing cluster/node state.

## Assumptions

- Phase 1 exposes direct PostgreSQL node endpoints, not a built-in proxy or virtual IP.
- The first selected node is the primary during provisioning; after failover, the current primary is determined from node role metadata.
- The default database shown in connection examples is `postgres`.
- Full role/database management is required in later phases, but password generation and access management must wait until command-secret infrastructure exists.
- Stable HA endpoints require either user-provided DNS/VIP/proxy metadata or future Skylex-managed proxy support.

## Non-Goals For Phase 1

- Do not create PostgreSQL users/roles.
- Do not generate or store application passwords.
- Do not expose the replication password.
- Do not add new migrations.
- Do not modify PostgreSQL `pg_hba.conf` or TLS behavior.
- Do not claim a stable HA endpoint exists when only direct node endpoints are available.

## Phased Rollout

### Phase 1: Read-Only Connection Info

Goal: once PostgreSQL is installed and initialized, show operators how to connect without introducing credentials or schema changes.

#### Backend/API

- Prefer no new backend API if existing `GetCluster` + `ListNodes(cluster_id)` returns enough data.
- Compute in the UI from existing fields:
  - primary node: node with `role === "NODE_ROLE_PRIMARY"`
  - primary host: node `address` if present, else `hostname`
  - primary port: node `port` or `5432`
  - replicas: nodes with `role === "NODE_ROLE_REPLICA"`
  - readiness: `postgresInstalled && postgresDataInitialized`
- If existing enum/string mapping makes this brittle, add a small frontend normalization helper rather than a new backend abstraction.

#### UI

- Add a `PostgreSQL Connection` card to `ui/app/routes/clusters.$id.tsx` near the installation progress section.
- Show the card only when at least one node is assigned to the cluster.
- If the cluster has no ready primary, show a disabled/pending state explaining that connection details appear after the primary is ready.
- When ready, show:
  - primary endpoint: `<host>:<port>`
  - default database: `postgres`
  - service location: Native or Dockerized
  - `psql` example: `psql "host=<host> port=<port> dbname=postgres user=<user> sslmode=prefer"`
  - URI template: `postgresql://<user>:<password>@<host>:<port>/postgres?sslmode=prefer`
  - replica endpoints, if present
  - direct-node warning: the endpoint can change after failover until a stable endpoint/proxy is configured
  - network warning: firewall/security group access to the PostgreSQL port must be opened outside Skylex
- Include copy buttons for endpoint, psql command, and URI template if this matches existing UI patterns; otherwise keep the first pass simple and visible.
- Do not show or infer passwords.

#### Acceptance Criteria

- A healthy cluster detail page clearly answers how to connect to PostgreSQL.
- An in-progress cluster shows a useful pending message instead of blank connection information.
- Replica endpoints are visible when replicas exist.
- The card clearly states that direct primary endpoint is not stable across failover.
- `cd ui && npm run typecheck` passes.

### Phase 2: Connection Profile API

Goal: formalize connection metadata before adding credentials.

#### Proto/API

- Add a dedicated `PostgresManagementService` or `ConnectionService` instead of overloading cluster CRUD.
- Proposed RPCs:
  - `GetConnectionInfo(cluster_id)`
  - `UpdateConnectionProfile(cluster_id, public_host, port, ssl_mode, allowed_cidrs, endpoint_mode)`
  - `TestConnection(cluster_id, database, role_id)` once roles exist
- Return sanitized connection info only:
  - endpoint mode: `DIRECT_PRIMARY`, `MANUAL_STABLE_ENDPOINT`, future `SKYLEX_PROXY`
  - primary endpoint
  - replica endpoints
  - public/stable endpoint if configured
  - TLS/SSL mode
  - allowed CIDRs
  - warnings

#### Schema

- Add `cluster_connection_profiles`:
  - `cluster_id` primary key / foreign key
  - `endpoint_mode` text not null default `direct_primary`
  - `public_host` text not null default empty
  - `public_port` integer not null default `5432`
  - `ssl_mode` text not null default `prefer`
  - `allowed_cidrs` text/json not null default `[]`
  - `created_at`, `updated_at`
- Keep this separate from `clusters` because it is mutable access metadata, not cluster identity.

#### Validation/Security

- Validate CIDRs with `net/netip`.
- Validate hostnames conservatively; reject control characters and whitespace.
- Validate port range `1..65535`.
- Use `go-playground/validator` where request structs are introduced.
- Viewers may read sanitized profile info; operators/admins may update it.

### Phase 3: Managed Roles And Credentials

Goal: let operators create application users safely.

#### Schema

- Add `postgres_roles`:
  - `id` primary key
  - `cluster_id` foreign key
  - `role_name` text not null
  - `role_kind` text not null (`admin`, `read_write`, `read_only`, `custom`)
  - `encrypted_password` blob/text not null
  - `password_version` integer not null default `1`
  - `expires_at` timestamp nullable
  - `status` text not null (`pending`, `ready`, `failed`, `deleting`)
  - `created_at`, `updated_at`
  - unique `(cluster_id, role_name)`
- Add `postgres_operations`:
  - `id` primary key
  - `cluster_id` foreign key
  - `node_id` foreign key nullable
  - `operation_type` text not null
  - `status` text not null (`pending`, `running`, `succeeded`, `failed`)
  - `error` text not null default empty
  - `created_at`, `updated_at`, `completed_at`

#### Secret Handling

- Reuse AES-GCM encryption helpers from `internal/crypto/crypto.go`.
- Derive an encryption key from server secret material, following the storage config repository pattern.
- Do not store plaintext passwords in `agent_commands.payload`.
- Add command secret references before sending credentials to agents:
  - `agent_command_secrets(id, command_id, key, ciphertext, created_at, expires_at)`
  - payload contains secret identifiers, not secret values
  - `FetchCommand` resolves/decrypts only for the owning agent and command
- Expand redaction in `internal/agent/log.go` for:
  - PostgreSQL URLs
  - `PASSWORD` clauses
  - `PGPASSWORD`
  - JSON fields named `password`, `secret`, or `token`

#### Agent Commands

- Add idempotent commands:
  - `pg_ensure_role`
  - `pg_rotate_role_password`
  - `pg_drop_role`
- Prefer `pgx` SQL execution from Go over shelling out to `psql`.
- Use parameterized values and safe identifier quoting for role names.
- Every command must use context timeouts and return redacted errors.

#### API/UI

- Add role list/create/rotate/delete UI.
- Show generated password exactly once after create/rotate.
- Never display stored passwords by default.
- Provide connection string generation with `<password>` placeholder unless a one-time password was just generated.

### Phase 4: Managed Databases And Grants

Goal: manage application databases and attach them to managed roles.

#### Schema

- Add `postgres_databases`:
  - `id` primary key
  - `cluster_id` foreign key
  - `database_name` text not null
  - `owner_role_id` foreign key nullable
  - `status` text not null (`pending`, `ready`, `failed`, `deleting`)
  - `created_at`, `updated_at`
  - unique `(cluster_id, database_name)`

#### Agent Commands

- Add idempotent commands:
  - `pg_ensure_database`
  - `pg_drop_database`
  - `pg_grant_database_privileges`
- Run database/role DDL on the current primary only.
- Re-resolve primary before enqueueing if there has been a recent failover.

#### API/UI

- Add database list/create/delete UI.
- Allow selecting an owner role or leaving default ownership to a Skylex-managed admin role.
- Generate database-specific psql/URI templates.

### Phase 5: Managed Network Access And HBA

Goal: replace permissive bootstrap access with explicit allowlists.

#### Backend/UI

- Add an Access section to the connection UI:
  - allowed application CIDRs
  - allowed admin CIDRs
  - internal replication CIDRs
  - current HBA apply status
- Validate all CIDRs at the API boundary.
- Audit every access change.

#### Agent Commands

- Add `pg_apply_hba`.
- Generate a Skylex-managed `pg_hba.conf` block or include file.
- Include:
  - local admin access
  - replication traffic between cluster nodes
  - application role/database access from allowed CIDRs
- Reload PostgreSQL after successful HBA write.
- Roll back the written file if reload fails and preserve previous config.

### Phase 6: TLS Support

Goal: make external PostgreSQL connectivity production-safe.

#### Backend/UI

- Track TLS mode:
  - `disabled`
  - `prefer`
  - `required`
- Surface TLS status and warnings in connection info.
- Add CA download endpoint if Skylex manages certificates.

#### Agent Commands

- Add TLS config apply command that writes certificate/key paths and sets:
  - `ssl = on`
  - `ssl_cert_file`
  - `ssl_key_file`
  - optional `ssl_ca_file`
- Reload/restart as required.
- Do not mark TLS `required` until PostgreSQL confirms the config is active.

### Phase 7: Stable Endpoint / Proxy

Goal: prevent application connection strings from changing after failover.

#### Options

- Manual stable endpoint:
  - user provides DNS/VIP/proxy host
  - Skylex renders that host but does not manage failover routing
- Skylex-managed proxy:
  - future HAProxy/pgBouncer deployment
  - endpoint follows current primary
  - read/write and read-only endpoints may be separate
- External integration:
  - update DNS record or load balancer target during failover

#### Failover Behavior

- Direct mode always computes current primary endpoint from node role.
- Stable endpoint mode uses configured public host.
- UI must show whether Skylex manages endpoint failover or the operator must maintain it externally.

## Operation Flows

### Create Role

1. Validate cluster exists and is healthy enough for management.
2. Validate role name and role kind.
3. Generate password with crypto-secure randomness.
4. Encrypt password at rest.
5. Insert `postgres_roles` with `pending` status.
6. Insert `postgres_operations` row.
7. Resolve current primary.
8. Queue `pg_ensure_role` with command-secret reference.
9. Return role metadata and one-time password.
10. Agent completes command; server marks role `ready` or `failed`.

### Rotate Role Password

1. Validate role exists and is not deleting.
2. Generate new password.
3. Encrypt new password and increment `password_version` in a transaction.
4. Insert operation row.
5. Queue `pg_rotate_role_password` on current primary with secret reference.
6. Return one-time password.
7. Mark operation result from command completion.

### Create Database

1. Validate database name.
2. Validate owner role belongs to the same cluster, if provided.
3. Insert `postgres_databases` with `pending` status.
4. Insert operation row.
5. Queue `pg_ensure_database` on current primary.
6. Queue grants if required.
7. Mark database `ready` or `failed` from command results.

### Test Connection

1. Validate role/database references.
2. Resolve current endpoint and primary node.
3. Queue or execute `pg_connection_check` with a short timeout.
4. Return redacted result, latency, and actionable error.
5. Optionally store check results in a separate table with retention.

## Concurrency And Failure Handling

- Add unique constraints on `(cluster_id, role_name)` and `(cluster_id, database_name)`.
- Serialize mutating PostgreSQL management operations per cluster to avoid DDL races.
- Make agent commands idempotent so retries are safe.
- Use context timeouts for agent-side SQL execution.
- Do not spawn unbounded goroutines from user-controlled inputs.
- If primary changes while an operation is pending, either re-resolve primary before dispatch or fail with a retryable error.
- Do not automatically roll back already-applied PostgreSQL DDL unless the inverse operation is safe and explicit.
- Redact all errors before storing command logs and operation errors.

## Security Requirements

- Use mature validation (`go-playground/validator`) at API boundaries for structured requests.
- Use parameterized SQL for values and safe identifier quoting for database/role identifiers.
- Never string-concatenate user input into SQL commands.
- Never return stored passwords except as a one-time value immediately after create/rotation.
- Audit create/delete/rotate/reveal/access-profile changes.
- Keep viewer access read-only and sanitized.
- Require operator/admin privileges for all write operations.
- Ensure connection strings with passwords are redacted in logs.
- Do not rely on TTL alone for secret cleanup; delete command secrets once consumed/completed where possible.

## Test Plan

### Backend Migration Tests

- SQLite and PostgreSQL migrations remain sequential.
- New tables and indexes are created.
- Unique constraints prevent duplicate roles/databases per cluster.

### Repository Tests

- Encrypted credentials are not stored as plaintext.
- Role/database CRUD works across SQLite/PostgreSQL dialects.
- Operation state transitions are persisted correctly.
- Command secret references can be resolved only for the matching command/agent.

### Service Tests

- Viewer cannot create/delete/rotate credentials or update access profiles.
- Operator/admin can manage roles/databases.
- Invalid CIDR, host, port, role name, database name, and SSL mode are rejected.
- Connection info returns the current primary endpoint.
- Command payloads never contain plaintext passwords.

### Agent Tests

- Role commands are idempotent.
- Database commands are idempotent.
- Grants converge to the requested state.
- SQL execution handles cancellation/timeouts.
- Logs redact passwords and URLs.

### UI Tests / Typecheck

- Connection card renders pending state for clusters without a ready primary.
- Connection card renders primary and replica endpoints for ready clusters.
- URI examples use placeholders unless a one-time generated password exists.
- Mutations invalidate relevant role/database/query caches.
- `cd ui && npm run typecheck` passes.

## Verification Commands

- `make proto` after proto changes.
- `make test` after backend/schema/agent changes.
- `cd ui && npm run typecheck` after UI changes.
- Optionally `cd ui && npm run build` before release.

## Recommended Implementation Order

1. Phase 1: add read-only connection info card using existing cluster/node data.
2. Phase 2: add formal connection profile API/schema.
3. Phase 3: add command-secret infrastructure and managed roles/passwords.
4. Phase 4: add managed databases and grants.
5. Phase 5: replace broad HBA access with explicit allowlists.
6. Phase 6: add TLS management.
7. Phase 7: add stable endpoint/proxy support.
