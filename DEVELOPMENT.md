# Development

This repo is Go-first and uses `mise` for tool and task orchestration.

## Prerequisites
- `mise`
- Go (installed via `mise`)
- Docker (for container builds and integration tests)
- SSH client (for local E2E flow)

## Setup
```bash
mise install
```

## Build
```bash
mise exec -- go build ./cmd/viberun
mise exec -- go build ./cmd/viberun-server
```

## Test and vet
```bash
mise exec -- go test ./...
mise exec -- go vet ./...
```

## Container image
Preferred (if tasks exist):
```bash
mise run build:image
```

Fallback:
```bash
docker build -t viberun .
```

## Local E2E (localhost SSH)
```bash
bin/viberun-e2e-local
```

## Integration (host with Docker)
Preferred (if tasks exist):
```bash
mise run integration
```

Fallback:
```bash
bin/viberun-integration
```

## Notes
- Use `mise` for all tools/tasks when available.
- The E2E/integration scripts expect Docker on the host and may require SSH access.
