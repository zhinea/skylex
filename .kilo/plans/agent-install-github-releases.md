# Plan: Agent Install Script from GitHub Releases

## Goal
Change the Skylex UI’s agent-install one-liner so it always downloads the install script from the project’s GitHub releases, never from the server’s own `/install.sh` endpoint. This eliminates the need for users to configure a self-hosted HTTPS domain just to distribute the install script.

## Decision / assumptions
1. Release asset name for the script: `install-agent.sh` (matches the repo source file name).
2. Local `/install.sh` endpoint is removed entirely to avoid maintaining a redundant distribution channel.
3. The agent binary itself is already downloaded from GitHub releases by the script, so only the script URL and the release upload step need to change.
4. No new server config option is added; the release repository is a Go constant (`zhinea/skylex`).

## Why
- The current UI builds `http://<window.location.origin>/install.sh`, which is HTTP in local dev and unusable for agents outside the dev machine.
- The backend already exposes `GetAgentInstallCommandResponse.script_url` but leaves it empty.
- GitHub releases are HTTPS by default and versioned by tag, making the one-liner work anywhere.

## Backend changes

### 1. `internal/server/auth_service.go`
- Import `fmt` if not already imported.
- Add a constant:
  ```go
  const defaultReleaseRepo = "zhinea/skylex"
  ```
- In `GetAgentInstallCommand`, build the script URL from the embedded version string:
  ```go
  ver := versionString()
  scriptURL := fmt.Sprintf("https://github.com/%s/releases/download/v%s/install-agent.sh", defaultReleaseRepo, ver)
  ```
- Populate `ScriptUrl` in `GetAgentInstallCommandResponse` with `scriptURL`.
- Reuse the existing `CreateAgentToken` call (no extra query).

### 2. `internal/server/connect.go`
- Remove `"/install.sh"` from `unauthenticatedPaths`.
- Remove `mux.HandleFunc("/install.sh", s.serveAgentInstallScript)`.
- Keep `/version`.

### 3. `internal/server/assets.go`
- Remove the `//go:embed assets/install-agent.sh` line.
- Remove the `installAgentScript` variable and the `installScript()` helper.
- Keep `versionString()` because `/version` still uses it.

### 4. `internal/server/connect_test.go`
- Remove `TestServeAgentInstallScript` and `TestServeAgentInstallScriptRejectsPost`.
- Keep `TestServeVersion`.

### 5. `Makefile`
- Change the `assets` target to copy only `version.txt`:
  ```make
  assets:
  	@mkdir -p $(ASSETS_DIR)
  	@cp version.txt $(ASSETS_DIR)/version.txt
  ```

### 6. `internal/server/assets/install-agent.sh`
- Delete the committed copy; the canonical source remains `scripts/install-agent.sh`.

## Release workflow changes

### `.github/workflows/release.yml`
After building binaries, prepare a version-substituted install script and upload it as a release asset.

```yaml
      - name: Prepare install script
        run: |
          version="${GITHUB_REF_NAME}"
          sed "s/@@VERSION@@/${version#v}/g" scripts/install-agent.sh > /tmp/install-agent.sh

      - name: Create release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          version="${GITHUB_REF_NAME}"
          if gh release view "$version" >/dev/null 2>&1; then
            gh release upload "$version" \
              bin/skylex-agent-linux-amd64 \
              bin/skylex-agent-linux-arm64 \
              /tmp/install-agent.sh \
              --clobber
          else
            gh release create "$version" \
              bin/skylex-agent-linux-amd64 \
              bin/skylex-agent-linux-arm64 \
              /tmp/install-agent.sh \
              --generate-notes \
              --title "$version"
          fi
```

This ensures the downloaded `install-agent.sh` from GitHub has the correct version baked in and does not require users to pass `--version`.

## Frontend changes

### 1. `ui/app/hooks/useAgentInstall.ts`
- Update the response type to include `script_url`:
  ```ts
  export interface InstallCommandData {
    script_url: string;
    server_addr: string;
    token: string;
  }
  ```
- In `generate`, set `data` directly from the API response (`{ script_url, server_addr, token }`).
- Remove the `/version` fetch and the `version` state.
- Remove the exported `useScriptUrl` hook (no longer used).

### 2. `ui/app/components/InstallAgentModal.tsx`
- Remove the local `scriptUrl` state and the `useEffect` that sets it from `window.location.origin`.
- Use `data?.script_url` as the install script URL.
- Simplify `buildCommand` signature to `(scriptUrl, serverAddr, token, docker)` and drop the `--version` argument handling.
- Remove the red HTTPS warning block (GitHub releases are always HTTPS).
- Keep prerequisites line showing `data?.server_addr` because that is still the gRPC endpoint the agent connects to.

## Verification
- Run `make build` to confirm assets target works without `install-agent.sh`.
- Run `make test` — tests should pass after removing install-script tests.
- Run `cd ui && npm run typecheck` to confirm TypeScript compiles.
- After the next release, the UI command should look like:
  ```bash
  curl -fsSL "https://github.com/zhinea/skylex/releases/download/v0.1.0/install-agent.sh" | sudo bash -s -- --server "<advertise_addr>" --token "sklx_at_..."
  ```

## Security notes
- Script URL is HTTPS (GitHub) and static/public; no secrets are embedded.
- Registration token is still generated server-side, shown once, and passed as a CLI argument.
- No new unauthenticated endpoints are added; `/version` remains the only public non-RPC path.

## Performance notes
- No additional database queries are introduced; `GetAgentInstallCommand` reuses the existing `CreateAgentToken` call.
- Script URL is computed from in-memory constants/strings (O(1)).
