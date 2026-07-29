# Knsight

Knsight is a Go-based multi-agent hub with registry discovery, MCP-compatible
tools, configurable sub-agents, persistent sessions, a sandbox, and a Next.js
web interface.

This repository contains only provider-neutral framework code. Production
credentials, private endpoints, organization-specific prompts, and operational
skills must be supplied outside Git.

## Requirements

- Go 1.24
- Node.js 20 (for the web interface)
- An OpenAI-compatible model endpoint, or the included mock server

## Quick start

Start the local mock services:

```bash
go run ./cmd/registry --addr :8081
go run ./cmd/mock_llm --addr :8090 --reply "hello"
go run ./cmd/mock_mcp --addr :8091 --base-url http://localhost:8091
```

In another terminal:

```bash
cp .env.example .env
set -a
. ./.env
set +a
go run ./cmd/hub --config deploy/hub.prod.yaml
```

Then send a request:

```bash
curl -X POST http://localhost:8080/v1/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"Explain the current system status."}'
```

## Configuration

All secrets are injected through environment variables. See
[`.env.example`](.env.example) and
[`deploy/hub.prod.yaml`](deploy/hub.prod.yaml).

Browser variables use `NEXT_PUBLIC_*` and are visible to every user. Never put
tokens or credentials in them.

SQLite is the default store. To use Redis, set:

```bash
STORE_BACKEND=redis
REDIS_URL=redis://localhost:6379/0
```

Authentication is disabled in the example configuration. For HS256 inbound
JWT authentication, set `KNSIGHT_INBOUND_JWT_SECRET`. For generic identity-token
mode, set `KNSIGHT_IDENTITY_TOKEN_SECRET` and use the `X-Identity-Token`
header.

## Development

```bash
go test ./...
bash scripts/check-secrets.sh
```

Frontend development:

```bash
cd frontend
cp .env.example .env.local
npm install
npm run dev
```

## Docker

```bash
docker build -t knsight .
docker run --rm -p 8080:8080 --env-file .env knsight
```

## Adding private integrations safely

- Keep organization-specific prompts and skills in a private repository or
  inject them during deployment.
- Reference service URLs through environment variables.
- Store credentials in a secret manager.
- Use synthetic data in tests and screenshots.
- Run the secret checker before every push.

## Contributing and security

See [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`SECURITY.md`](SECURITY.md).

Before publishing, choose and add an OSI-approved license. No license has been
selected automatically because that is a project-owner legal decision.
