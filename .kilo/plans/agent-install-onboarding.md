# Plan: Agent Install Onboarding from UI Nodes Page

## Goal
Add a self-service "Install Agent" / "Add Node" onboarding flow on the **Nodes** page so a new user can copy a single one-liner shell command, paste it on a target database server, and have the agent register itself automatically.

## Current State
- `skylex-agent` connects to `skylex-server:9090` and calls `RegisterAgent` with an agent token (`internal/agent/agent.go:RegisterAgent`).
- `AgentService.RegisterAgent` only generates an `agent_id`; it does **not** validate the token (`internal/server/agent_service.go:RegisterAgent`).
- `AgentToken` model and `agent_tokens` table exist, but there are no RPCs to create/list/delete them (`proto/skylex/v1/auth.proto`).
- There is no install script, no UI onboarding, and no agent-token management UI.
- The Nodes page (`ui/app/routes/nodes.tsx`) only lists already-registered nodes with an empty state saying "Deploy agents".

## Design Options

### Option A: Shared config token (fastest)
- Use the existing token from `SKYLEX_AGENT_TOKEN` / `config.agent_token` (default `dev-token`).
- Server embeds a static `install-agent.sh` and exposes it at `GET /install.sh`.
- UI shows the one-liner with the visible token.

Pros: trivial to implement, one paste works.  
Cons: token is exposed in UI, not revocable per-node, not production-safe.

### Option B: Generated agent tokens (recommended)
- Add `CreateAgentToken` / `ListAgentTokens` / `DeleteAgentToken` RPCs.
- UI lets an admin click "Add Node" → server generates a one-time token → UI displays it once inside the install command.
- `AgentService.RegisterAgent` validates the token against `agent_tokens`.
- Install script is still static/embedded, but the token is injected via the one-liner args.

Pros: revocable, auditable, production-appropriate.  
Cons: slightly more backend work.

### Option C: Pre-built agent bundle
- Server builds a tarball per platform containing binary + config + systemd unit.
- UI downloads the bundle.

Pros: air-gapped friendly.  
Cons: much more complexity; overkill for MVP.

**Selected**: Implement **Option B** (generated per-node tokens). No dev-token fallback; registration is rejected if the token is missing or revoked. For local development an admin must create a token through the UI first.

## Proposed User Flow

1. User navigates to **Nodes**.
2. If no nodes exist, show a prominent card: "No agents yet. Add your first database server."
3. User clicks **Add Node** / **Install Agent**.
4. Modal opens with:
   - Step 1: prerequisites (Linux, systemd, PostgreSQL 16, outbound gRPC to server:9090).
   - Step 2: copy the one-liner.
   - Step 3: wait for the node to appear; auto-refresh list.

### Example one-liner (systemd)
```bash
curl -fsSL https://skylex.example.com/install.sh | sudo bash -s -- \
  --server skylex.example.com:9090 \
  --token sklx_at_xxxxxxxxxxxxxxxx
```

### Example one-liner (Docker helper)
```bash
curl -fsSL https://skylex.example.com/install.sh | sudo bash -s -- \
  --server skylex.example.com:9090 \
  --token sklx_at_xxxxxxxxxxxxxxxx \
  --docker
```

## User Decisions

| Decision | Selected option |
| --- | --- |
| Token model | **Generated per-node token** — created by UI, shown once, stored hashed, revocable. |
| Binary source | **GitHub releases** — `scripts/install-agent.sh` downloads `skylex-agent` from release assets. |
| Target OS | **Linux + Docker helper** — default installs systemd service; `--docker` flag runs a container instead. |
| Server address | **Config `server.advertise_addr`** — the gRPC endpoint agents use is configured explicitly. |

## Backend Changes

### 1. Agent token management RPCs
Extend `AuthService` in `proto/skylex/v1/auth.proto`:
- `CreateAgentToken` → returns `AgentToken` + raw `token` (one-time display only).
- `ListAgentTokens` → list metadata (no raw token).
- `DeleteAgentToken` → revoke.
- `GetAgentInstallCommand` (or put on `NodeService`) → returns `{ scriptUrl, serverAddr, token }`.

Regenerate with `make proto`.

### 2. Agent token validation
Update `internal/server/agent_service.go:RegisterAgent`:
- Verify `req.AgentToken` matches a row in `agent_tokens`.
- Reject registration if invalid/revoked.
- Keep backward compatibility: if `agent_tokens` table is empty, accept the configured dev token.

### 3. Install script
Create `scripts/install-agent.sh` and maintain it in repo. Behavior:
- Parse flags: `--server`, `--token`, `--hostname`, `--port`, `--data-dir`, `--version`, `--user`, `--docker`, `--download-url`.
- Detect OS/arch (`linux`, `amd64`/`arm64`).
- `--docker` mode: pull/run `ghcr.io/zhinea/skylex-agent:${VERSION}` with env vars `SKYLEX_SERVER_ADDR`, `SKYLEX_AGENT_TOKEN`, mounted `/var/lib/postgresql/data`. Do not install binary or systemd.
- `--download-url` override: use custom base URL instead of GitHub releases.
- Default systemd mode:
  - Download from `https://github.com/zhinea/skylex/releases/download/v${VERSION}/skylex-agent-${OS}-${ARCH}`.
  - Place at `/usr/local/bin/skylex-agent` with `0755`.
  - Create `/etc/skylex/agent.yaml`.
  - Create `skylex` user and group (optional, skip on `--no-user`).
  - Write `/etc/systemd/system/skylex-agent.service`.
  - Run `systemctl daemon-reload && systemctl enable --now skylex-agent`.
- Print command summary, agent status, and next step (check Nodes page).

### 4. Serve the install script
In `internal/server/connect.go` add:
```go
mux.HandleFunc("/install.sh", s.serveAgentInstallScript)
```
The handler reads the embedded script, sets `Content-Type: text/x-shellscript`, and responds.

Embed script via `//go:embed scripts/install-agent.sh` in `internal/server/assets.go`.

### 5. Server address hint
Server needs to know the gRPC address agents should use. Prefer:
- Config value `server.advertise_addr` (new field, fallback to `server.listen_addr:grpc_port`).
- Or derive from request `Host` header for HTTP downloads.

## Frontend Changes

### 1. New API hooks
Create `ui/app/hooks/useAgentInstall.ts`:
```ts
useAgentInstallCommand() -> { scriptUrl, serverAddr, token, isLoading }
```

Create `ui/app/hooks/useAgentTokens.ts` if exposing token management UI.

### 2. Nodes page update (`ui/app/routes/nodes.tsx`)
- Add primary **Install Agent** / **Add Node** button.
- If list empty, replace empty paragraph with a friendly onboarding card containing the button.

### 3. Onboarding modal
Create `ui/app/components/InstallAgentModal.tsx`:
- Tabs/steps: Requirements → Copy command → Waiting for agent.
- Display one-liner in a read-only input with a **Copy** button.
- Show server address and a warning that the token should be kept secret.
- Poll every 5s for new nodes while modal is open.

### 4. API endpoint mapping
One-liner URL should point at the same host as the UI, via `window.location.origin`, falling back to `VITE_SKYLEX_SERVER_URL` if set. Example:
```ts
const scriptUrl = `${window.location.origin}/install.sh`;
```

### 5. Release pipeline prerequisite
Because the install script downloads from GitHub releases, the project must publish an asset named `skylex-agent-linux-amd64` (and `-arm64`) for each release. The minimum viable path:
- Add `.github/workflows/release.yml` (or document manual upload) that runs `make build-agent` for `linux/amd64` and `linux/arm64` and attaches them to the GitHub release.
- Use `go version` output or a `version.txt` file to keep script version in sync.

## Concerns & Tradeoffs

1. **Agent token exposure in UI**: mitigate by generating one-time tokens and allowing revocation. Do not log the token server-side.
2. **Architecture-specific binaries**: install script must handle `amd64` and `arm64`; if unsupported, print clear error.
3. **Air-gapped / custom builds**: install script must support overriding download URL (`--download-url`) for self-hosted binaries.
4. **HTTP vs HTTPS**: UI should warn strongly when not HTTPS.
5. **Root/sudo requirement**: install script must be run as root or with `sudo`; document this.

## Implementation Steps

1. Add `CreateAgentToken`, `ListAgentTokens`, `DeleteAgentToken`, `GetAgentInstallCommand` to proto.
2. Run `make proto`.
3. Implement agent token RPCs in `internal/server/auth_service.go`.
4. Add agent token validation in `internal/server/agent_service.go:RegisterAgent`.
5. Create `scripts/install-agent.sh` and embed it.
6. Add `/install.sh` HTTP handler in `internal/server/connect.go`.
7. Add `server.advertise_addr` config option in `internal/server/config.go`.
8. Add hooks `useAgentInstall.ts` and `useAgentTokens.ts`.
9. Build `InstallAgentModal.tsx`.
10. Update `ui/app/routes/nodes.tsx` with Add Node button and empty-state onboarding.
11. Add tests/lint and update `README.md` / screenshots section.

## Acceptance Criteria

- An admin can open the Nodes page, click **Add Node**, and see a copy-paste one-liner.
- Running the one-liner on a Linux server installs and starts the agent.
- The agent successfully registers and appears in the Nodes list within ~10 seconds.
- The raw token is only shown once in the UI; it cannot be retrieved again.
- Revoking a token prevents new agents from registering with it.
- The Docker helper variant works without installing systemd.

## Out of Scope (for this plan)

- macOS / launchd support.
- Air-gapped binary serving from the server itself.
- Automatic cluster assignment after registration.
- Auto-provisioning of PostgreSQL (init, basebackup) after the agent registers.
