#!/usr/bin/env sh
# Reproducible recording script for assets/demo/worldcheck-demo.gif.
#
# Builds the worldcheck binary, stages a clean demo directory (a copy of the
# bundled world, so the recording shows exactly what a fresh clone sees), and
# renders docs/demo.tape with vhs. Requires: go, docker (daemon running),
# vhs (https://github.com/charmbracelet/vhs).
set -eu
cd "$(dirname "$0")/.."

DEMO_HOME="${HOME}/worldcheck-demo"

mkdir -p "${DEMO_HOME}"
rm -rf "${DEMO_HOME}/worlds"
cp -R worlds "${DEMO_HOME}/worlds"
# The recording must start from a world that was never provisioned or
# checkpointed, or the demo would show a stale fingerprint and an old baseline.
rm -rf "${DEMO_HOME}/worlds/postgres-starter/manifest.json" "${DEMO_HOME}/worlds/postgres-starter/checkpoints"

go build -o "${DEMO_HOME}/bin/worldcheck" ./cmd/worldcheck

PATH="${DEMO_HOME}/bin:${PATH}" vhs docs/demo.tape
