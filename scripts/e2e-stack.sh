#!/usr/bin/env bash
# Starts (or tears down) the stack the Playwright suite runs against: a
# throwaway Postgres and the REAL container image on port 3300.
#
#   bash scripts/e2e-stack.sh up      # builds snarvei:e2e if missing
#   bash scripts/e2e-stack.sh down
#
# E2E_REBUILD=1 rebuilds the artifacts and the image (reusing stale binaries
# ships an image without the change under test).
set -euo pipefail
cd "$(dirname "$0")/.."

NET=snarvei-e2e
PG=snarvei-e2e-pg
APP=snarvei-e2e-app
PORT="${E2E_PORT:-3300}"

down() {
  docker rm -f "$APP" "$PG" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  echo "e2e stack down"
}

case "${1:-up}" in
  down) down; exit 0 ;;
  up) ;;
  *) echo "usage: e2e-stack.sh [up|down]"; exit 2 ;;
esac

if [ -z "$(docker images -q snarvei:e2e)" ] || [ "${E2E_REBUILD:-0}" = "1" ]; then
  if [ "${E2E_REBUILD:-0}" = "1" ] || [ ! -e dist/server/linux/amd64/snarvei ]; then
    bash scripts/build-artifacts.sh
  fi
  docker build -t snarvei:e2e .
fi

down >/dev/null
docker network create "$NET" >/dev/null
docker run -d --name "$PG" --network "$NET" \
  -e POSTGRES_USER=snarvei -e POSTGRES_PASSWORD=snarvei -e POSTGRES_DB=snarvei \
  postgres:17-alpine >/dev/null

# TCP probe: initdb's first start accepts on the socket before TCP is up.
for i in $(seq 1 30); do
  docker exec "$PG" pg_isready -h 127.0.0.1 -U snarvei -d snarvei >/dev/null 2>&1 && break
  sleep 1
done

docker run -d --name "$APP" --network "$NET" -p "$PORT":3000 \
  -e DATABASE_URL=postgres://snarvei:snarvei@"$PG":5432/snarvei \
  -e APP_URL=http://127.0.0.1:"$PORT" \
  -e AUTH_SECRET=e2e-stack-secret-at-least-32-bytes-long \
  -e STORAGE_DRIVER=fs -e STORAGE_FS_PATH=/data \
  -e OPEN_SIGNUP=1 \
  snarvei:e2e >/dev/null

for i in $(seq 1 30); do
  curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "http://127.0.0.1:$PORT/readyz" | grep -q '"ok":true'
echo "e2e stack up on http://127.0.0.1:$PORT"
