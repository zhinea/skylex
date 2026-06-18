# Plan: Add CLI flags to `skylex-agent`

## Goal
Let users run the agent by passing options directly on the command line instead of exporting environment variables first:

```bash
./skylex-agent --server skylex.example.com:9090 --token sklx_at_...
./skylex-agent --server skylex.example.com:9090 --token-file /etc/skylex/token
```

Backwards compatibility is preserved: the existing environment variables and config file continue to work, and CLI flags take precedence.

## Decision / assumptions
1. Use the Go standard library `flag` package (already used in `cmd/bench/main.go`) — no new dependencies.
2. Flags take precedence over env vars, env vars take precedence over the config file, defaults are lowest.
3. Keep the existing `internal/agent.Config` struct; expose each option through CLI flags that write directly into the same struct.
4. Support both `--token` and `--token-file`. Passing both at the same time is an error; `--token-file` avoids exposing secrets in `ps`/shell history.
5. The agent config file path is configurable via `--config`/`-c` (default `/etc/skylex/agent.yaml`).
6. Only add flags for the settings that are commonly overridden at runtime; advanced settings still live in the config file.

## Why
- The current agent requires `export` before every run, which is awkward for local testing and one-off commands.
- A CLI interface is more idiomatic for Go binaries and matches the user request.
- Keeping config file + env + flags gives operators flexibility while avoiding breaking the Docker/systemd deployments.

## Backend changes

### 1. `cmd/agent/main.go`
Replace the manual `os.Getenv` calls with `flag` parsing and a clear precedence chain.

Imports to add:
```go
"flag"
"strings"
```

Flags to expose:
```go
var (
    configPath = flag.String("config", "/etc/skylex/agent.yaml", "path to agent config file")
    serverAddr = flag.String("server", "", "control plane gRPC address (host:port)")
    agentToken = flag.String("token", "", "agent registration token")
    tokenFile  = flag.String("token-file", "", "path to a file containing the agent registration token")
    hostname   = flag.String("hostname", "", "hostname reported to the control plane")
    port       = flag.Int("port", 0, "PostgreSQL port on this machine")
    dataDir    = flag.String("data-dir", "", "PostgreSQL data directory")
    logLevel   = flag.String("log-level", "", "log level: debug, info, warn, error")
    logFormat  = flag.String("log-format", "", "log format: json, text")
)
```

Precedence logic in `main()`:
```go
cfg := agent.DefaultConfig()

// 1. Load config file (optional).
_ = loadConfigFile(*configPath, &cfg) // already warns; do not fail for missing file.

// 2. Apply env vars (backwards compatibility).
cfg = applyEnv(cfg)

// 3. Apply CLI flags (highest precedence).
if *agentToken != "" && *tokenFile != "" {
    fmt.Fprintln(os.Stderr, "error: only one of --token or --token-file may be provided")
    os.Exit(1)
}
if *serverAddr != "" { cfg.ServerAddr = *serverAddr }
if *agentToken != "" { cfg.AgentToken = *agentToken }
if *tokenFile != "" {
    tok, err := readTokenFile(*tokenFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: read token file: %v\n", err)
        os.Exit(1)
    }
    cfg.AgentToken = tok
}
if *hostname != "" { cfg.Hostname = *hostname }
if *port != 0 { cfg.Port = *port }
if *dataDir != "" { cfg.PGDataDir = *dataDir }
if *logLevel != "" { cfg.LogLevel = *logLevel }
if *logFormat != "" { cfg.LogFormat = *logFormat }

if cfg.Hostname == "" {
    h, _ := os.Hostname()
    cfg.Hostname = h
}

// Optional: exit early if the token is still empty.
if cfg.AgentToken == "" {
    fmt.Fprintln(os.Stderr, "error: agent token is required; set --token, --token-file, SKYLEX_AGENT_TOKEN, or agent_token in the config file")
    os.Exit(1)
}
```

Helper functions (keep them in `cmd/agent/main.go`; do not create new packages):
```go
func applyEnv(cfg agent.Config) agent.Config {
    if v := os.Getenv("SKYLEX_AGENT_TOKEN"); v != "" { cfg.AgentToken = v }
    if v := os.Getenv("SKYLEX_SERVER_ADDR"); v != "" { cfg.ServerAddr = v }
    if v := os.Getenv("SKYLEX_HOSTNAME"); v != "" { cfg.Hostname = v }
    if v := os.Getenv("SKYLEX_PORT"); v != "" { /* parse int */ }
    if v := os.Getenv("SKYLEX_PG_DATA_DIR"); v != "" { cfg.PGDataDir = v }
    return cfg
}

func readTokenFile(path string) (string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(data)), nil
}
```

Use the existing `agent.New(cfg)` and `ag.Run(...)` calls unchanged.

### 2. `internal/agent/config.go`
No changes required. The existing `Config` struct and `DefaultConfig()` are reused.

### 3. `internal/agent/agent.go`
No changes required.

### 4. `scripts/install-agent.sh`
Since the installer already writes `/etc/skylex/agent.yaml` with `server_addr`, `agent_token`, `hostname`, `port`, and `pg_data_dir`, the systemd service can rely on the config file and does not need duplicated `Environment` lines.

Change the generated `[Service]` section from:
```systemd
Environment=SKYLEX_SERVER_ADDR=${SERVER_ADDR}
Environment=SKYLEX_AGENT_TOKEN=${TOKEN}
Environment=SKYLEX_HOSTNAME=${HOSTNAME}
Environment=SKYLEX_PORT=${PORT}
Environment=SKYLEX_PG_DATA_DIR=${DATA_DIR}
```
to simply:
```systemd
ExecStart=/usr/local/bin/skylex-agent
```

Leave the Docker path unchanged; Docker containers commonly pass settings via environment variables, and the agent will continue to load them as fallbacks.

### 5. `deploy/systemd/skylex-agent.service`
Clean up duplicated secrets. Remove `Environment=SKYLEX_AGENT_TOKEN=...` and keep `ExecStart=/usr/local/bin/skylex-agent`, relying on `/etc/skylex/agent.yaml` for configuration. Remove the malformed `%d/skylex-agent-token` token entry.

### 6. `AGENTS.md`
Update the agent run instructions:

```bash
# With flags
make run-agent ARGS='--server localhost:9090 --token dev-token'

# With a token file (recommended for production)
make run-agent ARGS='--server localhost:9090 --token-file /etc/skylex/token'
```

Also note that environment variables are still supported for backwards compatibility and for container deployments.

### 7. `deploy/docker-compose/docker-compose.yaml`
No change required. The existing `SKYLEX_SERVER_ADDR`/`SKYLEX_AGENT_TOKEN` environment variables continue to work.

## Security notes
- `--token TOKEN` leaks the secret into process listings (`ps aux`), shell history, and audit logs. Prefer `--token-file PATH` or the config file for production deployments.
- If both `--token` and `--token-file` are provided, the agent exits with an explicit error so a flag cannot accidentally override a more secure file source.
- The agent never logs the token value.
- File reading for `--token-file` uses normal `os.ReadFile`; in the future this could be replaced with a constant-time wipe if desired, but the current scope is minimal.

## Performance notes
- CLI parsing uses the standard library `flag` package: O(n) over the argument list and no additional allocations beyond a small string per flag.
- No new network calls or database queries are added.
- The config file is still read only once at startup.

## Verification
1. Run `make build-agent` and confirm no errors.
2. Run `./bin/skylex-agent --help` and verify the flags are listed.
3. Start with env vars and no flags (backwards compatibility):
   ```bash
   SKYLEX_SERVER_ADDR=localhost:9090 SKYLEX_AGENT_TOKEN=dev-token ./bin/skylex-agent
   ```
4. Start with flags overriding env:
   ```bash
   SKYLEX_AGENT_TOKEN=wrong ./bin/skylex-agent --server localhost:9090 --token correct-token
   ```
5. Start with a token file:
   ```bash
   echo -n 'dev-token' > /tmp/token
   ./bin/skylex-agent --server localhost:9090 --token-file /tmp/token
   ```
6. Verify that passing both `--token` and `--token-file` exits immediately with an error.
