# Phase 4: Security & Operations — Implementation Summary

**Date:** 2026-06-16  
**Commit:** `25c3bf0` — `feat(server): implement authentication, audit, and webhook features`  
**Files changed:** 16 files (+1430, -6)

---

## New Files (10)

### `internal/db/user_repo.go`
- **`UserRepository`** — `Create`, `GetByEmail`, `GetByID`, `List` (paginated), `Delete`
- **`APIKeyRepository`** — `Create`, `GetByKeyHash`, `ListByUserID`, `Delete`
- **`AgentTokenRepository`** — `Create`, `GetByTokenHash`, `List`, `Delete`
- Helper `NullTime()` for nullable `*time.Time` → `sql.NullTime` conversion

### `internal/db/audit_repo.go`
- **`AuditRepository`** — `Log` (inserts audit entry, returns auto-incremented ID), `List` (paginated with total count)
- Uses `sql.NullString` for nullable `user_id` column

### `internal/server/jwt.go`
- **`JWTManager`** — wraps `golang-jwt/jwt/v5` with HS256 signing
  - `GenerateAccessToken` — 30s JWT with `user_id`, `email`, `role` claims
  - `GenerateRefreshToken` — longer-lived JWT (no user claims, subject only)
  - `ValidateToken` — parses and validates access tokens, returns `JWTClaims`
  - `ValidateRefreshToken` — validates refresh token, returns subject (user ID)
- **`JWTClaims`** struct with `UserID`, `Email`, `Role` + standard registered claims

### `internal/server/auth_interceptor.go`
- **`AuthInterceptor`** — gRPC unary server interceptor for auth + RBAC
  - **Whitelist:** `Login`, `RefreshToken`, all `AgentService` methods bypass auth
  - **Bearer tokens:** validates JWT via `Authorization: Bearer <token>`
  - **API keys:** validates via `Authorization: ApiKey <key>` — SHA-256 hash lookup
  - **RBAC:** `viewer` role denied for write operations (all non-read methods)
  - Injects `user_id`, `user_role`, `user_email` into context
  - Exported helpers: `UserIDFromContext`, `UserRoleFromContext`, `UserEmailFromContext`

### `internal/server/auth_service.go`
- **`AuthService`** — implements `skylex.v1.AuthServiceServer` (8 RPC methods)
  - `Login` — email/password → JWT access + refresh tokens + user info
  - `RefreshToken` — refresh token → new access + refresh token pair
  - `ListUsers` — paginated user list (admin/operator)
  - `CreateUser` — creates user with bcrypt-hashed password
  - `DeleteUser` — removes user by ID
  - `CreateAPIKey` — generates 32-byte random key, stores SHA-256 hash, returns raw key (only time)
  - `ListAPIKeys` — lists keys for authenticated user
  - `DeleteAPIKey` — removes API key by ID
- Helper converters: `userToProto`, `apiKeyToProto` (model → protobuf)

### `internal/server/audit_interceptor.go`
- **`AuditInterceptor`** — gRPC unary interceptor that logs ALL state-changing RPCs
  - Maps method paths to `AuditAction` constants (create/delete cluster, backup, user, etc.)
  - Reads are NOT audited (only mutations)
  - Logs `user_id`, `action`, `resource` (method path), `detail`, `ip_address`
  - Audit failures log as errors but do NOT fail the RPC

### `internal/server/tls.go`
- **`LoadTLSCredentials`** — loads server TLS config from `TLSConfig`
  - TLS 1.3 minimum version
  - When `CAFile` is set → `RequireAndVerifyClientCert` (mTLS mode)
- **`LoadClientTLSCredentials`** — loads client TLS config for agent connections

### `internal/server/webhook.go`
- **`WebhookClient`** — sends JSON POST payloads to configured URLs
  - Events: `backup.failed`, `backup.completed`, `cluster.failover`, `node.online`, `node.offline`
  - 10s timeout per request (configurable)
  - Nil-safe (returns silently if no URLs configured)
  - Convenience methods: `NotifyBackupFailed`, `NotifyBackupCompleted`, `NotifyFailover`, `NotifyNodeOffline`, `NotifyNodeOnline`
- **`WebhookConfig`** struct with `URLs []string`, `Timeout time.Duration`

### `internal/server/metadata_backup.go`
- **`MetadataBackup`** — SQLite file-level backup/restore
  - `Backup` — copies active SQLite db file to `backups/metadata/skylex-metadata-<timestamp>.db`
  - `Restore` — replaces active SQLite file from backup (closes DB connection, copies file)
  - `ListBackups` — lists `.db` files in backup directory

### `README.md`
- Full project documentation: architecture, quick start, project layout, developer commands, configuration, tech stack, implementation phases

---

## Modified Files (6)

### `internal/server/server.go` (+29 lines)
- Added import: `"crypto/tls"`
- **New fields on `Server` struct:**
  - `authService *AuthService`
  - `jwtManager *JWTManager`
  - `authInterceptor *AuthInterceptor`
  - `auditInterceptor *AuditInterceptor`
  - `webhookClient *WebhookClient`
  - `metadataBackup *MetadataBackup`
  - `tlsConfig *tls.Config`
  - `auditRepo *db.AuditRepository`
- **New wiring in `Start()`:**
  - Creates `UserRepository`, `APIKeyRepository`, `AgentTokenRepository`, `AuditRepository`
  - Initializes `JWTManager` with config values
  - Creates `AuthInterceptor`, `AuditInterceptor`, `AuthService`, `WebhookClient`
  - Loads TLS credentials from config
  - Initializes `MetadataBackup` with `"backups/metadata"` directory

### `internal/server/grpc.go` (+28/-6 lines)
- Added import: `"google.golang.org/grpc/credentials"`
- **Interceptor chain** in `NewGRPCServer`:
  1. `grpcLoggingInterceptor` (always)
  2. `AuthInterceptor` (if configured)
  3. `AuditInterceptor` (if configured)
- **TLS:** when `srv.tlsConfig != nil`, wraps gRPC server with TLS credentials
- **AuthService registration:** `skylexv1.RegisterAuthServiceServer(grpcServer, srv.authService)`

### `internal/server/config.go` (+3 lines)
- Added `Webhook WebhookConfig` field to `Config` struct
- `setDefaults()` now calls `c.Webhook.setDefaults()`

### `config.example.yaml` (+6/-1 lines)
- Added `webhook` section:
  ```yaml
  webhook:
    urls: []
    timeout: 10s
  ```

### `go.mod` / `go.sum` (+3 lines)
- Added dependency: `github.com/golang-jwt/jwt/v5 v5.3.1`

---

## Security Architecture (Phase 4 gRPC flow)

```
gRPC Request
  │
  ▼
grpc.ChainUnaryInterceptor:
  ├─ 1. loggingInterceptor        — logs method + errors (always)
  ├─ 2. AuthInterceptor           — JWT/ApiKey validation + RBAC
  │     ├─ Unauthenticated:   Login, RefreshToken, AgentService.*
  │     ├─ Authenticated:      all other methods
  │     └─ Viewer read-only:  Get/List cluster/node/backup, ListUsers, ListAPIKeys
  └─ 3. AuditInterceptor          — logs state-changing calls to audit_logs
  │
  ▼
Service Handler (with user_id/role/email in context)
```

### Auth flow
```
Login(email, password)
  → bcrypt.VerifyPassword
  → JWTManager.GenerateAccessToken (HS256, 24h)
  → JWTManager.GenerateRefreshToken (HS256, 168h)
  → return {access_token, refresh_token, user}

ApiKey auth:
  Authorization: ApiKey <raw-key>
  → SHA-256(key) → lookup api_keys.key_hash
  → if valid + not expired → inject claims from user
```

### RBAC matrix
| Role     | Read (Get/List) | Write (Create/Update/Delete/Failover) | User mgmt | API Key mgmt |
|----------|:---:|:---:|:---:|:---:|
| admin    | ✓ | ✓ | ✓ | ✓ |
| operator | ✓ | ✓ | ✓ | ✓ |
| viewer   | ✓ | ✗ | ✗ | ✗ |

### TLS
- `tls.enabled: false` → plaintext gRPC
- `tls.enabled: true, ca_file: ""` → server TLS only
- `tls.enabled: true, ca_file: "<path>"` → mTLS (requires client certificate)

---

## Agent service methods (unauthenticated)

All `AgentService` methods bypass auth because agents authenticate via token during `RegisterAgent`:
- `RegisterAgent`
- `Heartbeat`
- `ReportStatus`
- `FetchCommand`
- `ReportCommandResult`

---

## Phase 4 Feature Checklist

| Feature | Status |
|---------|:------:|
| JWT-based authentication | ✅ |
| API key support | ✅ |
| RBAC (admin/operator/viewer) | ✅ |
| Auth service (Login, RefreshToken, user CRUD, API key CRUD) | ✅ |
| mTLS for gRPC server | ✅ |
| Encrypted secrets at rest (AES-256-GCM — existed from Phase 2) | ✅ |
| Audit logging interceptor | ✅ |
| Webhook alerting (backup, failover, node events) | ✅ |
| Control-plane metadata backup/restore | ✅ |