#!/usr/bin/env bash
# Builds everything the image COPYs, natively. Output:
#   dist/server/linux/amd64/snarvei
#   dist/server/linux/arm64/snarvei
# The SPA is embedded in both binaries; the overlay is restored on exit.
# Prerequisites: `mise install` and `bun install`.
set -euo pipefail
cd "$(dirname "$0")/.."

trap 'bash scripts/restore-embed-overlay.sh' EXIT
bash scripts/spa-embed-overlay.sh

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

echo "==> server binaries ($VERSION)"
rm -rf dist/server
for arch in amd64 arm64; do
  mkdir -p "dist/server/linux/$arch"
  (cd apps/server && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" \
    -o "../../dist/server/linux/$arch/snarvei" ./cmd/snarvei)
  echo "    dist/server/linux/$arch/snarvei"
done
ls -lh dist/server/linux/*/
