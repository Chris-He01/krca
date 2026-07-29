#!/bin/bash
set -e

IMAGE_NAME="${IMAGE_NAME:-knsight-go}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
SCENE="${KNSIGHT_SCENE:-}"

# ---- Build Go binary ----
echo "Building Go binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -o bin/hub ./cmd/hub/

# ---- Build frontend ----
echo "Building frontend..."
cd frontend
npm install --frozen-lockfile || npm install
npx next build
cd ..

# Organization-specific skills are intentionally injected at deploy time.
echo "  Synced $(find data/skills -name 'SKILL.md' | wc -l) skills"

# ---- Build Docker image ----
if [ -n "$SCENE" ]; then
  IMAGE_TAG="${SCENE}-${IMAGE_TAG}"
  echo "Building scene image: ${IMAGE_NAME}:${IMAGE_TAG} (scene=${SCENE})"
else
  echo "Building default image: ${IMAGE_NAME}:${IMAGE_TAG}"
fi

docker build \
  -t "${IMAGE_NAME}:${IMAGE_TAG}" \
  -f Dockerfile \
  .

echo ""
echo "Done: ${IMAGE_NAME}:${IMAGE_TAG}"
echo "Image size: $(docker image inspect ${IMAGE_NAME}:${IMAGE_TAG} --format='{{.Size}}' | numfmt --to=iec 2>/dev/null || docker image inspect ${IMAGE_NAME}:${IMAGE_TAG} --format='{{.Size}}')"

if [ -n "$SCENE" ]; then
  echo ""
  echo "Run with scene mode:"
  echo "  docker run -e KNSIGHT_SCENE=${SCENE} -p 8083:8083 ${IMAGE_NAME}:${IMAGE_TAG}"
fi
