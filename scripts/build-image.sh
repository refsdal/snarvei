#!/usr/bin/env bash
# Multi-arch image from the native artifacts. Usage:
#   bash scripts/build-image.sh                # snarvei:local, loaded for the host arch
#   PUSH=1 IMAGE=ghcr.io/refsdal/snarvei:dev bash scripts/build-image.sh
set -euo pipefail
cd "$(dirname "$0")/.."

bash scripts/build-artifacts.sh

IMAGE="${IMAGE:-snarvei:local}"
if [ "${PUSH:-0}" = "1" ]; then
  docker buildx build --platform linux/amd64,linux/arm64 -t "$IMAGE" --push .
else
  docker buildx build --load -t "$IMAGE" .
fi
