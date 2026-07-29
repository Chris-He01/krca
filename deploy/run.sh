#!/bin/bash
set -e

IMAGE_NAME="${IMAGE_NAME:-knsight-go}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
CONTAINER_NAME="${CONTAINER_NAME:-knsight-hub}"
PORT="${PORT:-8083}"

CONFIG_PATH="${CONFIG_PATH:-}"

echo "Starting ${CONTAINER_NAME} on port ${PORT} ..."

DOCKER_ARGS=(
  --name "${CONTAINER_NAME}"
  --rm
  -p "${PORT}:${PORT}"
  -e "AUTO_PORT0=${PORT}"
  -e "LLM_BASE_URL=${LLM_BASE_URL:-}"
  -e "LLM_MODEL=${LLM_MODEL:-}"
  -e "LLM_MAX_TOKENS=${LLM_MAX_TOKENS:-}"
  -e "QWEN_A30_BASE_URL=${QWEN_A30_BASE_URL:-}"
  -e "QWEN_A10_BASE_URL=${QWEN_A10_BASE_URL:-}"
  -e "QWEN_MAX_TOKENS=${QWEN_MAX_TOKENS:-}"
  # Persist data across restarts
  -v "${CONTAINER_NAME}-data:/app/data"
  -v "${CONTAINER_NAME}-store:/app/store"
  -v "${CONTAINER_NAME}-skills:/app/skills"
  -v "${CONTAINER_NAME}-logs:/app/logs"
  -v "${CONTAINER_NAME}-sandbox:/app/sandbox"
)

# Mount custom config if provided
if [ -n "$CONFIG_PATH" ]; then
  DOCKER_ARGS+=(-v "$(realpath "$CONFIG_PATH"):/app/configs/hub.prod.yaml:ro")
fi

docker run -d "${DOCKER_ARGS[@]}" "${IMAGE_NAME}:${IMAGE_TAG}"

echo "Container ${CONTAINER_NAME} started."
echo "  URL: http://localhost:${PORT}"
echo "  Logs: docker logs -f ${CONTAINER_NAME}"
