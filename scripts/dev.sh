#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE="docker compose \
  -f deploy/docker-compose/docker-compose.yaml \
  -f deploy/docker-compose/docker-compose.dev.yaml"

echo "[dev] starting dev infrastructure (etcd + minio)..."
# eval "$COMPOSE up -d etcd minio minio-init"
eval "$COMPOSE up -d etcd"

CLEANUP_DONE=0
cleanup() {
  if [ "$CLEANUP_DONE" -eq 0 ]; then
    CLEANUP_DONE=1
    echo "[dev] stopping dev infrastructure..."
    eval "$COMPOSE down" || true
  fi
}
trap cleanup EXIT INT TERM

# Wait until etcd is reachable from the host before starting the server.
echo "[dev] waiting for etcd to be healthy..."
for _ in $(seq 1 30); do
  if eval "$COMPOSE exec -T etcd etcdctl endpoint health" >/dev/null 2>&1; then
    echo "[dev] etcd healthy"
    break
  fi
  sleep 1
done

echo "[dev] starting skylex server..."
air
