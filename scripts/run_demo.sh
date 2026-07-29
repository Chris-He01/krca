#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

GO_BIN="$(command -v go || true)"
if [[ -z "${GO_BIN}" ]]; then
  GO_BIN="$HOME/.local/go/bin/go"
fi

"$GO_BIN" run ./cmd/registry --addr :8081 &
REG_PID=$!

sleep 0.3

"$GO_BIN" run ./cmd/agent --config configs/agent_inspect.json &
INSPECT_PID=$!

"$GO_BIN" run ./cmd/agent --config configs/agent_vision.json &
VISION_PID=$!

sleep 0.5

"$GO_BIN" run ./cmd/hub --config configs/hub.json &
HUB_PID=$!

trap 'kill $HUB_PID $VISION_PID $INSPECT_PID $REG_PID' EXIT

wait
