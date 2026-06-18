#!/usr/bin/env bash
#
# Skylex agent installer
#
# Systemd install (default):
#   curl -fsSL https://skylex.example.com/install.sh | sudo bash -s -- \
#     --server skylex.example.com:9090 \
#     --token sklx_at_xxxxxxxxxxxxxxxx
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

SERVER_ADDR=""
TOKEN=""
HOSTNAME=""
PORT="5432"
DATA_DIR="/var/lib/postgresql/data"
VERSION="@@VERSION@@"
USER="skylex"
SKIP_USER=false
DOCKER=false
DOWNLOAD_URL=""

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
  --docker            run the agent in a Docker container instead of systemd
  --download-url URL  download the binary or image archive from a custom URL
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
    --docker) DOCKER=true; shift ;;
    --download-url) DOWNLOAD_URL="$2"; shift 2 ;;
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

log "installing skylex-agent ${VERSION} for ${OS}/${ARCH}"
log "reporting hostname: ${HOSTNAME}"

if $DOCKER; then
  if ! command -v docker >/dev/null 2>&1; then
    fail "docker is required for --docker mode"
  fi

  IMAGE="${REGISTRY_BASE}/skylex-agent:${VERSION}"
  if [[ -n "$DOWNLOAD_URL" ]]; then
    IMAGE="$DOWNLOAD_URL"
  fi

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
if [[ -n "$DOWNLOAD_URL" ]]; then
  log "downloading binary from ${DOWNLOAD_URL}"
  curl -fsSL -o "$BINARY" "$DOWNLOAD_URL"
else
  URL="${GITHUB_BASE}/v${VERSION}/skylex-agent-${OS}-${ARCH}"
  log "downloading binary from ${URL}"
  curl -fsSL -o "$BINARY" "$URL"
fi
chmod +x "$BINARY"

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
  systemctl enable --now skylex-agent
  log "started skylex-agent.service"
else
  log "systemd not available; service file written to /etc/systemd/system/skylex-agent.service"
fi

log ""
log "Installation complete."
log "Check status with: systemctl status skylex-agent"
log "The node should appear in the Skylex Nodes page within a few seconds."
