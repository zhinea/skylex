#!/usr/bin/env bash
#
# Skylex agent installer
#
# Systemd install (default):
#   curl -fsSL https://skylex.example.com/install.sh | sudo bash -s -- \
#     --server skylex.example.com:9090 \
#     --token sklx_at_xxxxxxxxxxxxxxxx
#
# Systemd install with Docker Engine pre-installed (recommended for Docker clusters):
#   curl -fsSL https://skylex.example.com/install.sh | sudo bash -s -- \
#     --server skylex.example.com:9090 \
#     --token sklx_at_xxxxxxxxxxxxxxxx \
#     --with-docker-engine
#
# Docker helper:
#   curl -fsSL https://skylex.example.com/install.sh | sudo bash -s -- \
#     --server skylex.example.com:9090 \
#     --token sklx_at_xxxxxxxxxxxxxxxx \
#     --docker
#
set -euo pipefail

REPO="zhinea/skylex"
GITHUB_BASE="https://github.com/${REPO}/releases/download"
REGISTRY_BASE="ghcr.io/${REPO}"
AGENT_BINARY_URL="@@AGENT_BINARY_URL@@"

SERVER_ADDR=""
TOKEN=""
HOSTNAME=""
PORT="5432"
DATA_DIR="/var/lib/postgresql/data"
VERSION="@@VERSION@@"
USER="skylex"
SKIP_USER=false
DOCKER=false
WITH_DOCKER_ENGINE=false
DEACTIVATION_MARKER=".skylex-agent-deactivated"

log() { echo "[skylex] $*"; }
fail() { echo "[skylex] error: $*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: install-agent.sh [OPTIONS]

Required:
  --server ADDR       gRPC address of the Skylex control plane (host:port)
  --token TOKEN       agent registration token generated from the UI

Optional:
  --hostname NAME     hostname reported to the control plane (default: current hostname)
  --port PORT         PostgreSQL port on this machine (default: 5432)
  --data-dir PATH     PostgreSQL data directory (default: /var/lib/postgresql/data)
  --version VERSION   release version to install (default: @@VERSION@@)
  --user USER         system user to create for the agent (default: skylex)
  --no-user           do not create a dedicated system user
  --with-docker-engine install Docker Engine and add the agent user to the docker group
  --docker            run the agent in a Docker container instead of systemd
  -h, --help          show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server) SERVER_ADDR="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --hostname) HOSTNAME="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --user) USER="$2"; shift 2 ;;
    --no-user) SKIP_USER=true; shift ;;
    --with-docker-engine) WITH_DOCKER_ENGINE=true; shift ;;
    --docker) DOCKER=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1"; ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  fail "this installer must be run as root or with sudo"
fi

if [[ -z "$SERVER_ADDR" ]]; then fail "--server is required"; fi
if [[ -z "$TOKEN" ]]; then fail "--token is required"; fi

if [[ "$HOSTNAME" == "" ]]; then
  HOSTNAME="$(hostname -s 2>/dev/null || hostname)"
fi

if [[ "$VERSION" == @@*@@ ]]; then
  fail "version is not set; pass --VERSION or use the install script served by skylex-server"
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "unsupported architecture: $ARCH_RAW" ;;
esac
case "$OS" in
  linux) ;;
  *) fail "unsupported operating system: $OS" ;;
esac

ensure_docker_engine() {
  if command -v docker >/dev/null 2>&1; then
    log "docker engine is already installed"
  elif $WITH_DOCKER_ENGINE; then
    log "docker engine not found; installing using the system package manager"
    if command -v apt-get >/dev/null 2>&1; then
      apt-get update
      apt-get install -y --no-install-recommends docker.io
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y docker || dnf install -y moby-engine
    elif command -v apk >/dev/null 2>&1; then
      apk add --no-cache docker
    elif command -v zypper >/dev/null 2>&1; then
      zypper --non-interactive install docker
    else
      fail "no supported package manager found for installing Docker Engine (supported: apt-get, dnf, apk, zypper)"
    fi
  else
    return
  fi

  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl start docker >/dev/null 2>&1 || true
  fi

  if $SKIP_USER; then
    return
  fi

  if ! id -u "$USER" >/dev/null 2>&1; then
    return
  fi

  if getent group docker >/dev/null 2>&1; then
    if ! id -nG "$USER" | grep -qw docker; then
      usermod -aG docker "$USER"
      log "added user ${USER} to the docker group"
    fi
  fi
}

install_native_sudoers() {
  if $SKIP_USER; then
    return
  fi

  if ! id -u "$USER" >/dev/null 2>&1; then
    return
  fi

  if ! command -v sudo >/dev/null 2>&1; then
    log "sudo not found; native cluster installs require running skylex-agent as root or installing sudo"
    return
  fi

  local sudoers_dir="/etc/sudoers.d"
  local sudoers_file="${sudoers_dir}/skylex-agent"
  local user_uid
  local user_gid
  user_uid="$(id -u "$USER")"
  user_gid="$(id -g "$USER")"
  mkdir -p "$sudoers_dir"
  cat > "$sudoers_file" <<EOF
${USER} ALL=(root) NOPASSWD: /usr/bin/apt-get update, /usr/bin/apt-get install -y --no-install-recommends postgresql-* postgresql-client-*, /usr/bin/apt-get purge -y postgresql-* postgresql-client-*, /usr/bin/dnf install -y postgresql* postgresql*-server, /usr/bin/dnf remove -y postgresql* postgresql*-server, /sbin/apk add --no-cache postgresql* postgresql*-client, /sbin/apk del postgresql* postgresql*-client, /usr/bin/zypper --non-interactive install postgresql* postgresql*-server, /usr/bin/zypper --non-interactive remove postgresql* postgresql*-server, /bin/systemctl stop postgresql, /bin/systemctl stop postgresql@*-main, /usr/bin/systemctl stop postgresql, /usr/bin/systemctl stop postgresql@*-main, /bin/install -d -o ${user_uid} -g ${user_gid} -m 0700 ${DATA_DIR}, /usr/bin/install -d -o ${user_uid} -g ${user_gid} -m 0700 ${DATA_DIR}, /bin/rm -rf -- ${DATA_DIR}, /usr/bin/rm -rf -- ${DATA_DIR}
EOF
  chmod 0440 "$sudoers_file"
  log "installed sudoers policy for native PostgreSQL provisioning"
}

clear_deactivation_markers() {
  local removed=false
  local marker
  local markers=(
    "/etc/skylex/${DEACTIVATION_MARKER}"
    "/var/lib/skylex/${DEACTIVATION_MARKER}"
    "${DATA_DIR}/${DEACTIVATION_MARKER}"
    "/tmp/skylex-agent-deactivated"
  )

  for marker in "${markers[@]}"; do
    if [[ -e "$marker" || -L "$marker" ]]; then
      rm -f -- "$marker"
      removed=true
    fi
  done

  if $removed; then
    log "cleared previous agent deactivation marker"
  fi
}

stop_existing_agent_service() {
  if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files skylex-agent.service >/dev/null 2>&1; then
    systemctl stop skylex-agent >/dev/null 2>&1 || true
  fi
}

log "installing skylex-agent ${VERSION} for ${OS}/${ARCH}"
log "reporting hostname: ${HOSTNAME}"
clear_deactivation_markers

if $DOCKER; then
  # The agent itself will run inside a container, but the host still needs a
  # working Docker Engine. Optionally install it and add the calling user to
  # the docker group first.
  ensure_docker_engine
  if ! command -v docker >/dev/null 2>&1; then
    fail "docker is required for --docker mode"
  fi

  IMAGE="${REGISTRY_BASE}/skylex-agent:${VERSION}"

  docker rm -f skylex-agent >/dev/null 2>&1 || true
  docker run -d \
    --name skylex-agent \
    --network host \
    --restart unless-stopped \
    -e SKYLEX_SERVER_ADDR="$SERVER_ADDR" \
    -e SKYLEX_AGENT_TOKEN="$TOKEN" \
    -e SKYLEX_HOSTNAME="$HOSTNAME" \
    -e SKYLEX_PORT="$PORT" \
    -e SKYLEX_PG_DATA_DIR="$DATA_DIR" \
    -v "$DATA_DIR:$DATA_DIR" \
    "$IMAGE"

  log "agent container started: skylex-agent"
  log ""
  log "Check status with: docker logs -f skylex-agent"
  log "The node should appear in the Skylex Nodes page within a few seconds."
  exit 0
fi

if ! command -v curl >/dev/null 2>&1; then
  fail "curl is required"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

BINARY="${TMP_DIR}/skylex-agent"
if [[ "$AGENT_BINARY_URL" != @@*@@ && -n "$AGENT_BINARY_URL" ]]; then
  log "downloading binary from ${AGENT_BINARY_URL}"
  curl -fsSL -o "$BINARY" "$AGENT_BINARY_URL"
else
  URL="${GITHUB_BASE}/v${VERSION}/skylex-agent-${OS}-${ARCH}"
  log "downloading binary from ${URL}"
  curl -fsSL -o "$BINARY" "$URL"
fi
chmod +x "$BINARY"

stop_existing_agent_service
cp "$BINARY" /usr/local/bin/skylex-agent
chmod 0755 /usr/local/bin/skylex-agent
log "installed binary to /usr/local/bin/skylex-agent"

mkdir -p "$DATA_DIR" /etc/skylex
cat > /etc/skylex/agent.yaml <<EOF
server_addr: "${SERVER_ADDR}"
agent_token: "${TOKEN}"
hostname: "${HOSTNAME}"
port: ${PORT}
pg_data_dir: "${DATA_DIR}"
EOF
chmod 0600 /etc/skylex/agent.yaml
log "wrote /etc/skylex/agent.yaml"

if ! $SKIP_USER; then
  if ! id -u "$USER" >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/skylex --shell /sbin/nologin "$USER" || \
      useradd --system --home-dir /var/lib/skylex --shell /usr/sbin/nologin "$USER"
    log "created system user: ${USER}"
  fi
  chown -R "${USER}:${USER}" /etc/skylex
fi

# Install Docker Engine and/or add the agent user to the docker group so that
# Docker-based clusters can be provisioned without manual host setup.
ensure_docker_engine
install_native_sudoers

cat > /etc/systemd/system/skylex-agent.service <<EOF
[Unit]
Description=Skylex Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/skylex-agent
Restart=always
RestartSec=5
User=${USER}
Group=${USER}

[Install]
WantedBy=multi-user.target
EOF

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  systemctl enable skylex-agent
  systemctl restart skylex-agent
  log "started skylex-agent.service"
else
  log "systemd not available; service file written to /etc/systemd/system/skylex-agent.service"
fi

log ""
log "Installation complete."
log "Check status with: systemctl status skylex-agent"
log "The node should appear in the Skylex Nodes page within a few seconds."
