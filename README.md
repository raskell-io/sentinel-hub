# Sentinel Hub

Fleet management control plane for [Sentinel](https://github.com/raskell-io/sentinel) reverse proxies.

**Status:** Pre-alpha (under active development)

## Overview

Sentinel Hub provides centralized management for Sentinel proxy fleets:

- **Configuration Management** — Create, version, and deploy configurations
- **Fleet Visibility** — Monitor health and status across all instances
- **Safe Deployments** — Rolling updates with automatic rollback
- **Audit Logging** — Track all changes for compliance

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Web UI    │────▶│  Hub API    │────▶│  Database   │
│  (React)    │     │   (Go)      │     │ (SQLite/PG) │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                    ┌──────┴──────┐
                    │    gRPC     │
                    └──────┬──────┘
         ┌─────────────────┼─────────────────┐
         ▼                 ▼                 ▼
    ┌─────────┐       ┌─────────┐       ┌─────────┐
    │  Agent  │       │  Agent  │       │  Agent  │
    └────┬────┘       └────┬────┘       └────┬────┘
    ┌────┴────┐       ┌────┴────┐       ┌────┴────┐
    │Sentinel │       │Sentinel │       │Sentinel │
    └─────────┘       └─────────┘       └─────────┘
```

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 20+ (for web UI)
- SQLite or PostgreSQL

### Build

```bash
# Build all binaries
make build

# Or build individually
make build-hub
make build-agent
```

### Run

```bash
# Start the Hub server
./bin/hub serve --http-port 8080 --grpc-port 9090

# Start an agent (on each Sentinel host)
./bin/agent run --hub-url localhost:9090 --sentinel-config /etc/sentinel/config.kdl
```

### Development

```bash
# Start Hub in development mode
make dev

# Start web UI development server
make web-dev

# Run tests
make test

# Run linters
make lint
```

## Configuration

### Hub Server

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `HUB_PORT` | `8080` | HTTP server port |
| `HUB_GRPC_PORT` | `9090` | gRPC server port |
| `HUB_DATABASE_URL` | `sqlite://hub.db` | Database connection URL |
| `HUB_JWT_SECRET` | (required) | JWT signing secret |
| `HUB_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `HUB_LOG_FORMAT` | `console` | Log format (console, json) |

### Agent

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `AGENT_HUB_URL` | `localhost:9090` | Hub gRPC server URL |
| `AGENT_INSTANCE_NAME` | (hostname) | Instance identifier |
| `AGENT_SENTINEL_CONFIG` | `/etc/sentinel/config.kdl` | Sentinel config path |
| `AGENT_HEARTBEAT_INTERVAL` | `30` | Heartbeat interval (seconds) |

## API

### REST API

The Hub exposes a REST API for the web UI and external integrations:

```
GET    /api/v1/instances          # List instances
POST   /api/v1/instances          # Register instance
GET    /api/v1/instances/:id      # Get instance details

GET    /api/v1/configs            # List configurations
POST   /api/v1/configs            # Create configuration
PUT    /api/v1/configs/:id        # Update configuration
GET    /api/v1/configs/:id/versions  # List versions

POST   /api/v1/deployments        # Create deployment
GET    /api/v1/deployments/:id    # Get deployment status
```

### Health Endpoints

```
GET /health   # Liveness probe
GET /ready    # Readiness probe
```

## Documentation

- [Architecture](.claude/ARCHITECTURE.md) — System design and components
- [Roadmap](.claude/ROADMAP.md) — Development roadmap and milestones

## License

MIT License — see [LICENSE](LICENSE) for details.

## Related Projects

- [Sentinel](https://github.com/raskell-io/sentinel) — The reverse proxy this manages
- [Sentinel Bench](https://github.com/raskell-io/sentinel-bench) — Performance benchmarks
