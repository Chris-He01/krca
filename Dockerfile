FROM golang:1.24-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/hub ./cmd/hub

FROM node:20-bookworm-slim AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install
COPY frontend ./
RUN npm run build

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl bash \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/hub /app/hub
COPY --from=frontend /src/out /app/frontend/out
COPY deploy/hub.prod.yaml /app/configs/hub.prod.yaml
COPY deploy/entrypoint.sh deploy/start.sh /app/
RUN chmod +x /app/entrypoint.sh /app/start.sh \
    && mkdir -p /app/data/skills /app/data/memory /app/logs /app/store /app/sandbox/workspace
EXPOSE 8080
CMD ["/app/entrypoint.sh"]
