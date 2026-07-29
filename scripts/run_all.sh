#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

GO_BIN="$(command -v go || true)"
if [[ -z "${GO_BIN}" ]]; then
  GO_BIN="$HOME/.local/go/bin/go"
fi

RESERVED_PORTS=()

port_in_use() {
  local port
  port="$(echo "$1" | sed 's/^://')"
  if command -v ss >/dev/null 2>&1; then
    ss -lnt 2>/dev/null | awk '{print $4}' | grep -E "[.:]${port}\$" >/dev/null 2>&1
    return $?
  fi
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  if command -v netstat >/dev/null 2>&1; then
    netstat -lnt 2>/dev/null | awk '{print $4}' | grep -E "[.:]${port}\$" >/dev/null 2>&1
    return $?
  fi
  return 1
}

port_reserved() {
  local target="$1"
  for reserved in "${RESERVED_PORTS[@]-}"; do
    if [[ "${reserved}" == "${target}" ]]; then
      return 0
    fi
  done
  return 1
}

reserve_port() {
  RESERVED_PORTS+=("$1")
}

pick_free_port() {
  local base="$1"
  local out_var="$2"
  local port="$base"
  for _ in $(seq 1 50); do
    local addr=":${port}"
    if ! port_in_use "${addr}" && ! port_reserved "${addr}"; then
      reserve_port "${addr}"
      printf -v "${out_var}" "%s" "${addr}"
      return 0
    fi
    port=$((port + 1))
  done
  return 1
}

HUB_ADDR="${HUB_ADDR:-}"
LLM_ADDR="${LLM_ADDR:-}"
MCP_ADDR="${MCP_ADDR:-}"
EXT_LLM_ADDR="${EXT_LLM_ADDR:-}"

if [[ -n "${HUB_ADDR}" ]]; then reserve_port "${HUB_ADDR}"; fi
if [[ -n "${LLM_ADDR}" ]]; then reserve_port "${LLM_ADDR}"; fi
if [[ -n "${MCP_ADDR}" ]]; then reserve_port "${MCP_ADDR}"; fi
if [[ -n "${EXT_LLM_ADDR}" ]]; then reserve_port "${EXT_LLM_ADDR}"; fi

if [[ -z "${HUB_ADDR}" ]]; then pick_free_port 8080 HUB_ADDR; fi
if [[ -z "${LLM_ADDR}" ]]; then pick_free_port 8090 LLM_ADDR; fi
if [[ -z "${MCP_ADDR}" ]]; then pick_free_port 8091 MCP_ADDR; fi
if [[ -z "${EXT_LLM_ADDR}" ]]; then pick_free_port 8093 EXT_LLM_ADDR; fi

if [[ -z "${HUB_ADDR}" || -z "${LLM_ADDR}" || -z "${MCP_ADDR}" || -z "${EXT_LLM_ADDR}" ]]; then
  echo "failed to allocate free ports" >&2
  exit 1
fi

HUB_URL="http://localhost${HUB_ADDR}"
LLM_URL="http://localhost${LLM_ADDR}/v1"
MCP_BASE_URL="http://localhost${MCP_ADDR}"
MCP_SSE_URL="${MCP_BASE_URL}/sse"
EXT_LLM_URL="http://localhost${EXT_LLM_ADDR}/v1"

require_free_port() {
  local name="$1"
  local addr="$2"
  if port_in_use "$addr"; then
    echo "port already in use for ${name} (${addr}). Override with env: ${name}_ADDR=:<port>" >&2
    exit 1
  fi
}

wait_for() {
  local url="$1"
  local name="$2"
  local retries=30
  for _ in $(seq 1 "${retries}"); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  echo "timeout waiting for ${name} at ${url}" >&2
  exit 1
}

require_free_port HUB "${HUB_ADDR}"
require_free_port LLM "${LLM_ADDR}"
require_free_port MCP "${MCP_ADDR}"
require_free_port EXT_LLM "${EXT_LLM_ADDR}"

echo "using ports: HUB=${HUB_ADDR} LLM=${LLM_ADDR} MCP=${MCP_ADDR} EXT_LLM=${EXT_LLM_ADDR}"

cleanup() {
  for pid in "${HUB_PID:-}" "${EXT_AGENT_PID:-}" "${MCP_PID:-}" "${LLM_PID:-}"; do
    if [[ -n "${pid}" ]]; then
      kill "${pid}" >/dev/null 2>&1 || true
    fi
  done
}
trap cleanup EXIT

# Start mock LLM and MCP servers
"$GO_BIN" run ./cmd/mock_llm --addr "${LLM_ADDR}" --reply "xxx" &
LLM_PID=$!

"$GO_BIN" run ./cmd/mock_mcp --addr "${MCP_ADDR}" --base-url "${MCP_BASE_URL}" &
MCP_PID=$!

"$GO_BIN" run ./cmd/mock_llm --addr "${EXT_LLM_ADDR}" --reply "external-agent" &
EXT_AGENT_PID=$!

wait_for "http://localhost${LLM_ADDR}/healthz" "mock llm"
wait_for "${MCP_BASE_URL}/healthz" "mock mcp"

# Generate runtime config (YAML)
cat > "${ROOT_DIR}/configs/hub.runtime.yaml" <<EOF
listen_addr: "${HUB_ADDR}"
registry_url: ""

llm:
  base_url: "${LLM_URL}"
  model: mock
  api_key: mock

mcp:
  enabled: true
  sse_url: "${MCP_SSE_URL}"
  tool_names:
    - server_metrics

supervisor:
  name: InsightSupervisor
  description: "Top-level coordinator for RCA workflows."
  instruction: "Delegate diagnostics to InspectAgent and visualization to VisionAgent. Summarize findings."
  use_mcp: false

agents:
  - name: InspectAgent
    description: "MCP-enabled inspector for metrics and logs."
    instruction: "Use MCP tools to fetch metrics and return structured diagnostics."
    use_mcp: true

  - name: VisionAgent
    description: "Visualization agent that turns metrics into chart descriptions."
    instruction: "Summarize chart intent and key trends for RCA reports."
    use_mcp: false

external_agents: []

sandbox:
  enabled: true
  max_output_bytes: 100000
  command_timeout_seconds: 30
  restrict_to_workspace: false
  web_fetch_enabled: true

memory:
  enabled: false

skills:
  enabled: false
EOF

# Start hub (registry is embedded)
"$GO_BIN" run ./cmd/hub --config configs/hub.runtime.yaml &
HUB_PID=$!

wait_for "${HUB_URL}/healthz" "hub"

# Register external agent via embedded registry
curl -s -X POST "${HUB_URL}/v1/registry/register" \
  -H 'Content-Type: application/json' \
  -d '{"card":{"id":"external-analyst","name":"ExternalAnalyst","version":"0.1.0","description":"external openai agent","capabilities":["analysis"],"endpoint":"'"${EXT_LLM_URL}"'","protocol":"openai","model":"mock"},"ttl_seconds":30}' >/dev/null

# Test chat endpoint
echo ""
echo "=== Testing /v1/chat ==="
curl -s -X POST "${HUB_URL}/v1/chat" \
  -H 'Content-Type: application/json' \
  -d '{"message":"检查服务器内存指标并生成报告"}'

echo ""
echo "=== Testing /v1/registry/agents ==="
curl -s "${HUB_URL}/v1/registry/agents"

echo ""

if [[ "${KEEP_RUNNING:-false}" == "true" ]]; then
  wait
fi
