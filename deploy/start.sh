#!/bin/sh
set -e

echo "=== Debug Info ==="
echo "PWD: $(pwd)"
echo "PORT: ${PORT:-<not set>}"
echo "KNSIGHT_SCENE: ${KNSIGHT_SCENE:-<not set>}"
echo "=================="

# Use PORT when provided by the runtime, otherwise listen on 8080.
export PORT="${PORT:-8080}"


# 确保运行时目录存在
mkdir -p /app/sandbox/workspace \
         /app/sandbox/sessions \
         /app/store \
         /app/logs \
         /app/data/memory \
         /app/data/skills

if [ -n "$KNSIGHT_SCENE" ]; then
    echo "Scene mode: $KNSIGHT_SCENE"
    mkdir -p "/app/data/memory/scene/$KNSIGHT_SCENE"
    mkdir -p "/app/sandbox/workspace/scene/$KNSIGHT_SCENE"
    SCENE_PROD_CONFIG="/app/configs/scene-${KNSIGHT_SCENE}.yaml"
    if [ -f "$SCENE_PROD_CONFIG" ] && [ -z "$HUB_CONFIG" ]; then
        export HUB_CONFIG="$SCENE_PROD_CONFIG"
        echo "  Using scene config: $SCENE_PROD_CONFIG"
    fi
fi

# 配置文件路径（使用运行时副本，避免修改原始挂载文件）
SRC_CONFIG="${HUB_CONFIG:-/app/configs/hub.prod.yaml}"
CONFIG="/app/configs/hub.runtime.yaml"

if [ ! -f "$SRC_CONFIG" ]; then
    echo "ERROR: config file not found: $SRC_CONFIG"
    exit 1
fi

# 生成运行时配置：替换端口 + 环境变量展开
cp "$SRC_CONFIG" "$CONFIG"
sed -i "s|listen_addr:.*|listen_addr: \":${PORT}\"|" "$CONFIG"

# If KNSIGHT_TRUSTED_HOSTS is provided (comma-separated), replace
# the trusted_hosts entry in the runtime config so operators can set hosts
# via env instead of editing YAML. Example: "a.com,b.com" -> ["a.com","b.com"]
if [ -n "$KNSIGHT_TRUSTED_HOSTS" ]; then
    OLDIFS=$IFS
    IFS=','
    set -- $KNSIGHT_TRUSTED_HOSTS
    IFS=$OLDIFS
    arr=""
    for h in "$@"; do
        # trim spaces
        h="$(echo "$h" | sed 's/^ *//;s/ *$//')"
        if [ -z "$h" ]; then
            continue
        fi
        if [ -n "$arr" ]; then
            arr="$arr, "
        fi
        arr="$arr\"$h\""
    done
    yaml_hosts="[$arr]"
    sed -i "s|trusted_hosts:.*|trusted_hosts: $yaml_hosts|" "$CONFIG"
fi

# Auth defaults: accept AccessProxy identity with cookie fallback unless overridden.
# Go's os.ExpandEnv does NOT support bash ${VAR:-default}, so we set defaults here.
#   KNSIGHT_AUTH_MODE=accessproxy → strict (token required, 401 on miss).
#   KNSIGHT_AUTH_MODE=auto        → AccessProxy first, then cookie fallback.
#   KNSIGHT_AUTH_MODE=cookie      → cookie only (legacy path).
export KNSIGHT_AUTH_MODE="${KNSIGHT_AUTH_MODE:-disabled}"
export KNSIGHT_AUTH_ENABLED="${KNSIGHT_AUTH_ENABLED:-false}"
export KNSIGHT_PUBLIC_HOST="${KNSIGHT_PUBLIC_HOST:-localhost}"
# Optional comma-separated list of trusted hosts for AccessProxy token host claim.
# Example: "agent.example.com,agent-staging.example.com"
export KNSIGHT_TRUSTED_HOSTS="${KNSIGHT_TRUSTED_HOSTS:-}"
export KNSIGHT_SERVICE_AUTH_HEADER="${KNSIGHT_SERVICE_AUTH_HEADER:-X-Knsight-Service-Token}"
export KNSIGHT_SERVICE_AUTH_USER="${KNSIGHT_SERVICE_AUTH_USER:-knsight-component}"
# Optional inbound inbound JWT authentication. The secret must be injected by the
# deployment platform; never commit it to source control.
export KNSIGHT_INBOUND_JWT_SECRET="${KNSIGHT_INBOUND_JWT_SECRET:-}"
export KNSIGHT_SSO_REQUIRED="${KNSIGHT_SSO_REQUIRED:-false}"
export KNSIGHT_SSO_URL="${KNSIGHT_SSO_URL:-}"

# Store / Redis / LLM defaults (Go's os.ExpandEnv does not support ${VAR:-default})
export STORE_BACKEND="${STORE_BACKEND:-sqlite}"
export REDIS_RESOURCE_NAME="${REDIS_RESOURCE_NAME:-redis://localhost:6379/0}"
export REDIS_PREFIX="${REDIS_PREFIX:-prod}"
export LLM_BASE_URL="${LLM_BASE_URL:-http://localhost:8090/v1}"
export LLM_MODEL="${LLM_MODEL:-mock}"

# Optional analytics gateway credentials.
export KNSIGHT_CK_TOKEN="${KNSIGHT_CK_TOKEN:-}"
export KNSIGHT_CK_USER="${KNSIGHT_CK_USER:-}"
export KNSIGHT_CK_PRINCIPAL="${KNSIGHT_CK_PRINCIPAL:-}"

echo ""
echo "Starting knsight-go Hub..."
echo "  Port:   $PORT"
echo "  Config: $CONFIG (from $SRC_CONFIG)"
echo "  Scene:  ${KNSIGHT_SCENE:-<default>}"
echo "  LLM:    $LLM_MODEL ($LLM_BASE_URL), max_tokens=${LLM_MAX_TOKENS:-<provider default>}"
echo "  Auth:   mode=$KNSIGHT_AUTH_MODE public_host=$KNSIGHT_PUBLIC_HOST"
echo "  Trusted Hosts: ${KNSIGHT_TRUSTED_HOSTS:-<from config>}"
echo "  Service Auth: ${KNSIGHT_SERVICE_AUTH_HEADER} user=${KNSIGHT_SERVICE_AUTH_USER} enabled=$([ -n "$KNSIGHT_SERVICE_AUTH_TOKEN" ] && echo true || echo false)"
echo "  inbound JWT: enabled=$([ -n "$KNSIGHT_INBOUND_JWT_SECRET" ] && echo true || echo false)"
echo "  SSO:    $KNSIGHT_SSO_REQUIRED ($KNSIGHT_SSO_URL)"
echo "  Host:   0.0.0.0"
echo ""

# Start the application.
exec /app/hub -config "$CONFIG"
