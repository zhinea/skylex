# Engine & Modules Mechanism

> Audience: future AI/LLM agents and human developers working on Skylex.
> Read this before adding a new database engine (MariaDB, MySQL, ...) or before
> moving/renaming the management repos. It explains *why* things are laid out the
> way they are so you don't "fix" something that is intentional.

## 1. The one-paragraph summary

Skylex is multi-engine. Anything that **differs per engine** (capability list,
agent command action strings, identifier validation, default port) lives behind
a small Go interface `engine.Provider` in `internal/engine`. Anything that is
**the same for every engine** (row storage, SQL, transactions) lives in
`internal/db` with engine-neutral table and type names. The engine of a cluster
is always derived from `clusters.engine` via its `cluster_id` — there is no
per-row engine column. Adding an engine = add one `Provider` implementation +
register it. You should not need to touch `internal/db`, the migrations, or the
UI menu logic.

## 2. Layer map (where things live and why)

| Concern | Package | Per-engine? |
|---|---|---|
| Capability/module set, action strings, name validation, default port | `internal/engine` + `internal/engine/<engine>` | YES |
| Row storage: SQL, tx, scanning (managed databases/roles/operations/TLS/HBA) | `internal/db/*_repo.go` | NO — shared |
| Node-level engine detection (`PostgresInstalled`, version, etc. on a Node) | `internal/db/node_repo.go`, `internal/models` | engine-specific node state, left as-is |
| Agent-side command *execution* (actually running `pg_ensure_database`) | `internal/agent` | YES (agent-side) |
| Wire API (proto) | `proto/`, `gen/` | mostly neutral |

Key rule: **a repository's job is identical regardless of engine** — open a tx,
INSERT a `managed_databases` row, queue an `agent_commands` row, commit. That is
why the repos are NOT inside `internal/engine/postgres`. Putting them there would
force a `mariadb/database_repo.go` doing identical SQL = the exact duplication we
removed.

There is also a hard constraint: `internal/db` imports `internal/engine` (for
`LogicalOpForAction`). If repos lived in `internal/engine/postgres` you would get
the cycle `engine/postgres -> db -> engine`, which does not compile. Repos sit
*below* the engine abstraction.

## 3. The `engine.Provider` interface

Defined in `internal/engine/engine.go`:

```go
type Provider interface {
    Engine() models.EngineType            // which engine this serves
    Modules() []Module                    // ordered UI capability set
    Supports(ModuleID) bool               // capability check
    Action(LogicalOp) (string, bool)      // logical op -> agent action string
    ValidateRoleName(string) error
    ValidateDatabaseName(string) error
    DefaultPort() int
}
```

A provider is **stateless** and safe for concurrent use.

### ModuleID — the UI capabilities

```go
ModuleOverview, ModuleConnection, ModuleDatabases, ModuleRoles,
ModuleNetwork, ModuleTLS, ModuleExtensions, ModuleSettings, ModuleDiagnostics
```

`Module{ID, Label}`. The ordered slice a provider returns from `Modules()` IS the
UI sidebar. Universal modules (overview/connection/settings/diagnostics) appear
for every engine. Feature modules (databases/roles/network/tls/extensions) appear
only if the provider advertises them. Example: PostgreSQL advertises
`ModuleExtensions`; a future MariaDB/MySQL provider simply omits it, so the
"Extensions" tab never renders for those clusters — with zero engine `if`s in the
UI.

### LogicalOp — engine-neutral operations

```go
OpEnsureDatabase, OpDropDatabase, OpGrantDatabasePrivileges,
OpEnsureRole, OpRotateRolePassword, OpDropRole,
OpApplyHBA, OpApplyTLS
```

A `LogicalOp` is what the control plane *means*. `Provider.Action(op)` translates
it to the concrete agent command string that engine understands, e.g. for
PostgreSQL `OpEnsureDatabase -> "pg_ensure_database"`. The repos and handlers
never hardcode `pg_*`; they receive the action from the provider on writes.

## 4. The registry + reverse lookup

Providers self-register via `init()` (the `database/sql` driver pattern):

```go
// internal/engine/postgres/postgres.go
func init() { engine.Register(Provider{}) }
```

For the registration to actually run, the package must be imported. We do a blank
import in `internal/server/server.go`:

```go
_ "github.com/zhinea/skylex/internal/engine/postgres" // register the provider
```

> GOTCHA: if `engine.For("postgresql")` returns "no provider registered", it
> means nobody imported the provider package. Add the blank import. This is the
> single most likely confusion for a future agent.

`Register` also builds a reverse map: every `(engine, action string)` ->
`LogicalOp`. So command **result** handlers route generically:

```go
if op, ok := engine.LogicalOpForAction(action); !ok || op != engine.OpApplyHBA {
    return false, nil // not ours
}
```

Resolution helpers:
- `engine.For(EngineType) (Provider, error)` — get provider for a cluster's engine.
- `engine.LogicalOpForAction(string) (LogicalOp, bool)` — reverse lookup.

Duplicate registration or duplicate action strings **panic at startup** on
purpose, so wiring mistakes surface immediately, not at runtime.

## 5. Request flow (write path)

Creating a managed database for cluster X:

1. Handler `CreateDatabase` resolves the provider:
   `provider, err := s.requireModule(ctx, clusterID, engine.ModuleDatabases)`.
   - `requireModule` loads the cluster, finds its provider via `engine.For`,
     and returns `FailedPrecondition` if the engine does not `Supports` that
     module. This is the API **boundary check** — unsupported features are
     rejected here, before business logic.
2. Handler validates the name with `provider.ValidateDatabaseName(name)`.
3. Handler resolves the action: `action, _ := provider.Action(engine.OpEnsureDatabase)`.
4. Handler calls the repo, passing the action string in the Tx input
   (e.g. `CreateDatabaseTxInput.EnsureAction`). The repo inserts the
   `managed_databases` row + queues an `agent_commands` row with that action.

## 6. Request flow (result path)

When the agent reports a command result, `agent_service.go` fans the result out
to the repos. Each repo decides "is this mine?" via the reverse lookup:

```go
op, ok := engine.LogicalOpForAction(action)
// route on op (OpEnsureDatabase / OpDropDatabase / ...) — never on "pg_*"
```

This is what makes adding an engine require **zero repo edits**: the repos speak
`LogicalOp`, not engine action strings.

## 7. Database schema (engine-neutral)

Migration `internal/db/migrations/{sqlite,postgres}/000020_engine_abstraction.sql`
renamed the entity tables to drop the `postgres_` prefix:

| Old | New |
|---|---|
| `postgres_roles` | `managed_roles` |
| `postgres_databases` | `managed_databases` |
| `postgres_operations` | `service_operations` |
| `postgres_tls_certificate_authorities` | `service_tls_authorities` |

It also **consolidated** the two near-identical per-node status tables
(`postgres_hba_apply_status`, `postgres_tls_apply_status`) into one:

```
node_feature_apply_status(
  cluster_id, node_id, feature,    -- feature IN ('hba','tls')
  command_id, status, error,
  detail,                          -- JSON: feature-specific fields
  applied_at, updated_at,
  PRIMARY KEY (cluster_id, node_id, feature)
)
```

TLS-specific fields (`requested_tls_mode`, `tls_active`) live inside `detail` JSON
(`tlsApplyDetail` struct in `tls_apply_repo.go`). HBA carries no extra detail so
its `detail` is `{}`.

Why no per-row engine column: engine is static per cluster and derivable via
`cluster_id -> clusters.engine`. Adding a column would denormalize without need.
The apply-status table holds **ephemeral operational state** re-derived on the
next Apply, which is why migration 020 drops/recreates it instead of migrating
old rows.

## 8. UI mechanism (Option B: modules embedded in cluster response)

`GetClusterResponse` carries `repeated EngineModule modules` (id + label). The
server fills it in `cluster_service.go` via `engineModulesProto(cluster.Engine)`,
which reads `provider.Modules()`. The UI (`ui/app/routes/clusters.$id.tsx`) builds
its sidebar from `clusterData.modules`, falling back to a built-in static list
only if `modules` is empty (older server / unknown engine).

Result: the menu is engine-driven with no extra network round trip and no
engine-specific branching in the UI.

## 9. Naming conventions (do not regress these)

- `internal/db` repos & types are engine-neutral: `ManagedDatabase`,
  `ManagedRole`, `ServiceOperation`, `NetworkAccessRepository`,
  `TLSApplyRepository`, `ServiceTLSCA`, `HBAApplyStatus`, `TLSApplyStatus`.
  Files: `managed_database_repo.go`, `managed_role_repo.go`,
  `service_operation_repo.go`, `network_access_repo.go`, `tls_apply_repo.go`,
  `service_tls_ca_repo.go`. Do NOT reintroduce `Postgres*` here — these serve
  every engine.
- Still `Postgres*` ON PURPOSE:
  - `skylexv1.PostgresRole` / `skylexv1.PostgresDatabase` — proto-generated
    types. Renaming them is a separate (cosmetic, large-churn) proto follow-up;
    do it deliberately, not incidentally.
  - `PostgresManagementService` (proto service name) — same; wire name kept
    stable to avoid churn across `gen/`, `connect.go`, and UI hooks.
  - `Node.PostgresInstalled / PostgresVersion / PostgresDataInitialized` — these
    are real engine-specific node-detection fields, not neutral resource storage.

## 10. How to add a new engine (e.g. MariaDB) — checklist

1. Create `internal/engine/mariadb/mariadb.go` implementing `engine.Provider`:
   - `Modules()` returns the MariaDB capability set (e.g. databases, roles,
     network, tls — but NOT extensions).
   - `Action()` maps logical ops to MariaDB agent action strings
     (e.g. `OpEnsureDatabase -> "mariadb_ensure_database"`).
   - `ValidateRoleName` / `ValidateDatabaseName` with MariaDB identifier rules.
   - `init() { engine.Register(Provider{}) }`.
2. Blank-import it where providers are registered (alongside the postgres import
   in `internal/server/server.go`).
3. Implement the agent-side execution for those action strings in
   `internal/agent` (the actual `mariadb_*` handlers).
4. That's it for the control plane: repos, migrations, the management service
   boundary checks, and the UI menu all work unchanged. The MariaDB cluster's
   managed databases/roles flow into the SAME `managed_databases` / `managed_roles`
   tables, gated by the SAME `requireModule` checks, surfaced by the SAME UI menu
   logic.

## 11. Common confusion points (read this twice)

- "Why are postgres repos in `db/` not `engine/postgres/`?" — They are
  engine-NEUTRAL storage shared by all engines, and moving them would create an
  import cycle (`engine/postgres -> db -> engine`). See §2.
- "`engine.For` says no provider registered." — Missing blank import. See §4.
- "Where do I add a UI tab for a new engine feature?" — Add a `ModuleID` and put
  it in the provider's `Modules()`. Do not hardcode it in the UI. See §3/§8.
- "Should I add an `engine` column to `managed_databases`?" — No. Derive from
  `cluster_id -> clusters.engine`. See §7.
