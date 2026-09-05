# syntax=docker/dockerfile:1

# Snarvei: one image, one static Go binary, four modes selected by argv[1]
# (default migrate-then-serve, `server`, `migrate`, `healthcheck`). See
# apps/server/cmd/snarvei/main.go.
#
# NOTHING COMPILES IN HERE. Build natively first:
#   bash scripts/build-artifacts.sh   # -> dist/server/linux/{amd64,arm64}/snarvei
# This file only COPYs the binary matching TARGETPLATFORM, so a multi-arch
# buildx build is seconds of file copying with no QEMU.
#
# distroless "static" rather than scratch: same no-shell/no-libc surface, but
# with an up-to-date CA bundle, tzdata, /tmp and the nonroot user (uid 65532).
# Pinned by digest; Dependabot bumps it.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG TARGETPLATFORM
# scripts/build-artifacts.sh layout by default; GoReleaser passes BINARY_ROOT=.
ARG BINARY_ROOT=dist/server

# The fs storage driver's default mountpoint, pre-created OWNED BY nonroot so
# Docker copies that ownership onto a fresh named volume.
COPY --chown=nonroot:nonroot docker/data-skel/ /data/

COPY ${BINARY_ROOT}/${TARGETPLATFORM}/snarvei /app/snarvei

ENV PORT=3000
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/app/snarvei", "healthcheck"]

ENTRYPOINT ["/app/snarvei"]
