# Sentinel Hub Architecture

**Last Updated:** 2025-12-31

---

## Overview

Sentinel Hub is a fleet management control plane for Sentinel reverse proxies. It follows a hub-and-spoke architecture where a central Hub server manages multiple Sentinel instances through lightweight agents.

```
                                   ┌─────────────────────────────────┐
                                   │         Sentinel Hub            │
                                   │                                 │
    ┌──────────┐                   │  ┌─────────┐    ┌──────────┐   │
    │  Web UI  │◀──────REST/WS────▶│  │   API   │◀──▶│ Database │   │
    │ (React)  │                   │  │  Server │    │(SQLite/PG)│   │
    └──────────┘                   │  └────┬────┘    └──────────┘   │
                                   │       │                         │
    ┌──────────┐                   │  ┌────┴────┐                   │
    │   CLI    │◀──────REST───────▶│  │  gRPC   │                   │
    │ (hubctl) │                   │  │ Server  │                   │
    └──────────┘                   │  └────┬────┘                   │
                                   │       │                         │
                                   └───────┼─────────────────────────┘
                                           │
                    ┌──────────────────────┼──────────────────────┐
                    │                      │                      │
               ┌────┴────┐            ┌────┴────┐            ┌────┴────┐
               │  Agent  │            │  Agent  │            │  Agent  │
               └────┬────┘            └────┬────┘            └────┬────┘
                    │                      │                      │
               ┌────┴────┐            ┌────┴────┐            ┌────┴────┐
               │Sentinel │            │Sentinel │            │Sentinel │
               │  Proxy  │            │  Proxy  │            │  Proxy  │
               └─────────┘            └─────────┘            └─────────┘
```

---

## System Components

### 1. Hub API Server

The central server that exposes REST and gRPC APIs.

**Responsibilities:**
- Serve REST API for UI and external integrations
- Serve gRPC API for agent communication
- Manage configuration lifecycle (CRUD, versioning)
- Orchestrate deployments across fleet
- Handle authentication and authorization
- Maintain audit logs

**Technology:**
- Language: Go 1.22+
- HTTP Router: chi/v5
- gRPC: google.golang.org/grpc
- Database: sqlc for type-safe queries
- Auth: JWT tokens, bcrypt for passwords

**Key Packages:**
```
cmd/hub/              # Application entry point
├── main.go           # CLI setup, server initialization

internal/
├── api/              # REST API handlers
│   ├── router.go     # Route definitions
│   ├── middleware/   # Auth, logging, CORS
│   ├── handlers/     # Request handlers
│   └── response.go   # Response helpers
├── grpc/             # gRPC service implementations
│   ├── server.go     # gRPC server setup
│   └── agent/        # Agent service
├── domain/           # Business logic
│   ├── config/       # Configuration management
│   ├── instance/     # Instance management
│   ├── deployment/   # Deployment orchestration
│   └── user/         # User management
├── store/            # Data access layer
│   ├── db.go         # Database connection
│   ├── migrations/   # SQL migrations
│   └── queries/      # sqlc generated code
└── auth/             # Authentication/authorization
    ├── jwt.go        # Token handling
    └── rbac.go       # Permission checks
```

### 2. Web UI

Single-page application for fleet management.

**Responsibilities:**
- Provide intuitive interface for operators
- Real-time updates via WebSocket
- Configuration editing with validation
- Deployment monitoring
- Metrics visualization

**Technology:**
- Framework: React 18 with TypeScript
- Build: Vite
- State: TanStack Query + Zustand
- UI: Radix UI + Tailwind CSS
- Routing: React Router v6

**Key Structure:**
```
web/
├── src/
│   ├── api/          # API client functions
│   ├── components/   # Reusable UI components
│   │   ├── ui/       # Base components (Button, Input, etc.)
│   │   └── domain/   # Domain-specific components
│   ├── hooks/        # Custom React hooks
│   ├── pages/        # Page components
│   │   ├── instances/
│   │   ├── configs/
│   │   ├── deployments/
│   │   └── settings/
│   ├── stores/       # Zustand stores
│   ├── types/        # TypeScript types
│   └── utils/        # Utility functions
├── public/           # Static assets
└── index.html        # Entry HTML
```

### 3. Sentinel Agent

Lightweight daemon running alongside each Sentinel instance.

**Responsibilities:**
- Register instance with Hub
- Report health status and metrics
- Receive configuration updates
- Apply configuration to Sentinel (file write + SIGHUP)
- Report deployment status

**Technology:**
- Language: Go (shared codebase with Hub)
- Communication: gRPC with mTLS
- Config format: KDL file

**Key Structure:**
```
cmd/agent/
├── main.go           # Entry point

internal/agent/
├── client.go         # gRPC client to Hub
├── config.go         # Configuration management
├── health.go         # Health check logic
├── metrics.go        # Metrics collection
└── sentinel.go       # Sentinel process interaction
```

### 4. Data Store

Persistent storage for all Hub data.

**Supported Backends:**
- **SQLite:** Single-file database for simple deployments
- **PostgreSQL:** Production-grade for HA deployments

**Schema Design Principles:**
- Immutable audit logs (append-only)
- Soft deletes for configurations
- Version history preserved forever
- Efficient queries for common access patterns

---

## Data Flow

### Configuration Update Flow

```
1. User edits config in UI
           │
           ▼
2. UI sends PUT /api/v1/configs/:id
           │
           ▼
3. API validates KDL syntax
           │
           ▼
4. API creates new ConfigVersion
           │
           ▼
5. API returns success to UI
           │
           ▼
6. User initiates deployment
           │
           ▼
7. API creates Deployment record
           │
           ▼
8. Deployment orchestrator starts
           │
           ▼
9. For each target instance:
   a. Send config via gRPC
   b. Agent writes config file
   c. Agent sends SIGHUP to Sentinel
   d. Agent reports success/failure
           │
           ▼
10. Deployment status updated
           │
           ▼
11. UI shows completion
```

### Agent Heartbeat Flow

```
1. Agent starts, loads config
           │
           ▼
2. Agent calls Register RPC
           │
           ▼
3. Hub creates/updates Instance record
           │
           ▼
4. Hub returns instance ID + config hash
           │
           ▼
5. Agent compares config hash
           │
           ▼
6. If different, Agent calls GetConfig RPC
           │
           ▼
7. Agent applies new config
           │
           ▼
8. Every 30s: Agent calls Heartbeat RPC
           │
           ▼
9. Hub updates last_seen_at
           │
           ▼
10. If no heartbeat for 90s: Hub marks offline
```

---

## Security Architecture

### Authentication

```
┌─────────────────────────────────────────────────────────┐
│                    Authentication                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐   ┌──────────────┐   ┌─────────────┐ │
│  │ Local Auth   │   │    OIDC      │   │  API Keys   │ │
│  │              │   │              │   │             │ │
│  │ email/pass   │   │ OAuth 2.0   │   │ Bearer tok  │ │
│  │ bcrypt hash  │   │ ID tokens    │   │ scoped      │ │
│  └──────┬───────┘   └──────┬───────┘   └──────┬──────┘ │
│         │                  │                  │         │
│         └──────────────────┼──────────────────┘         │
│                            ▼                            │
│                    ┌───────────────┐                    │
│                    │   JWT Token   │                    │
│                    │               │                    │
│                    │ sub: user_id  │                    │
│                    │ role: admin   │                    │
│                    │ exp: 24h      │                    │
│                    └───────────────┘                    │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Agent Authentication (mTLS)

```
┌─────────────────────────────────────────────────────────┐
│                Agent Authentication                      │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─────────────┐              ┌─────────────┐           │
│  │    Hub      │◀────mTLS────▶│   Agent     │           │
│  │             │              │             │           │
│  │ Server Cert │              │ Client Cert │           │
│  │ + CA Cert   │              │ + CA Cert   │           │
│  └─────────────┘              └─────────────┘           │
│                                                          │
│  Certificate Chain:                                      │
│  - Hub CA issues agent certificates                      │
│  - Each agent has unique certificate                     │
│  - Certificate CN = instance ID                          │
│  - Certificates can be revoked via CRL                   │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Authorization (RBAC)

```
Role: Admin
├── instances:* (all operations)
├── configs:* (all operations)
├── deployments:* (all operations)
├── users:* (all operations)
└── audit:read

Role: Operator
├── instances:read
├── instances:update (labels only)
├── configs:* (all operations)
├── deployments:* (all operations)
└── audit:read

Role: Viewer
├── instances:read
├── configs:read
├── deployments:read
└── audit:read
```

---

## Deployment Architecture

### Single Node (Development/Small)

```
┌─────────────────────────────────────┐
│           Single Server             │
│                                     │
│  ┌─────────────────────────────┐   │
│  │        Hub Process          │   │
│  │                             │   │
│  │  ┌─────┐ ┌─────┐ ┌──────┐  │   │
│  │  │REST │ │gRPC │ │SQLite│  │   │
│  │  │:8080│ │:9090│ │ .db  │  │   │
│  │  └─────┘ └─────┘ └──────┘  │   │
│  │                             │   │
│  │  ┌─────────────────────┐   │   │
│  │  │   Embedded Web UI   │   │   │
│  │  └─────────────────────┘   │   │
│  └─────────────────────────────┘   │
│                                     │
└─────────────────────────────────────┘
```

### High Availability (Production)

```
                    ┌─────────────────┐
                    │  Load Balancer  │
                    │   (L7 / Ingress)│
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
        ┌─────┴─────┐  ┌─────┴─────┐  ┌─────┴─────┐
        │  Hub #1   │  │  Hub #2   │  │  Hub #3   │
        │ (stateless)│  │(stateless)│  │(stateless)│
        └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
              │              │              │
              └──────────────┼──────────────┘
                             │
                    ┌────────┴────────┐
                    │   PostgreSQL    │
                    │  (Primary +     │
                    │   Replica)      │
                    └─────────────────┘
```

### Kubernetes Deployment

```yaml
# Simplified view
Namespace: sentinel-hub
├── Deployment: hub-api (replicas: 3)
│   └── Container: hub
│       ├── Port 8080 (REST)
│       └── Port 9090 (gRPC)
├── Service: hub-api (ClusterIP)
├── Service: hub-grpc (ClusterIP)
├── Ingress: hub.example.com
├── Secret: hub-jwt-secret
├── Secret: hub-db-credentials
├── ConfigMap: hub-config
└── PersistentVolumeClaim: hub-data (if SQLite)
```

---

## Integration Points

### Sentinel Integration

The Hub manages Sentinel instances through agents. Key integration points:

1. **Configuration Format:** Hub stores and validates Sentinel's KDL configuration format.
2. **Reload Mechanism:** Agent sends SIGHUP to trigger Sentinel's hot reload.
3. **Health Checks:** Agent can query Sentinel's `/_/health` endpoint.
4. **Metrics:** Agent scrapes Sentinel's `/metrics` endpoint.

### External Integrations

```
┌──────────────────────────────────────────────────────────┐
│                   External Systems                        │
├──────────────────────────────────────────────────────────┤
│                                                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│  │ Prometheus  │  │   Grafana   │  │    Slack    │      │
│  │             │  │             │  │  (webhooks) │      │
│  │ scrape      │  │ dashboards  │  │  alerts     │      │
│  │ /metrics    │  │             │  │             │      │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘      │
│         │                │                │              │
│         └────────────────┼────────────────┘              │
│                          │                               │
│                    ┌─────┴─────┐                         │
│                    │    Hub    │                         │
│                    └───────────┘                         │
│                                                           │
│  Future:                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│  │    Git      │  │  Terraform  │  │    OIDC     │      │
│  │  (GitOps)   │  │  Provider   │  │  Provider   │      │
│  └─────────────┘  └─────────────┘  └─────────────┘      │
│                                                           │
└──────────────────────────────────────────────────────────┘
```

---

## Performance Considerations

### Scalability Targets

| Metric | Target |
|--------|--------|
| Managed instances | 1,000+ |
| Concurrent UI users | 100 |
| API requests/sec | 1,000 |
| Config versions stored | Unlimited |
| Deployment batch size | 100 instances |

### Optimization Strategies

1. **Database:**
   - Indexed queries for common access patterns
   - Connection pooling
   - Read replicas for scale (PostgreSQL)

2. **API:**
   - Pagination for all list endpoints
   - ETag caching for configs
   - Gzip compression

3. **Agent Communication:**
   - Long-lived gRPC connections
   - Delta config updates (future)
   - Batch metrics reporting

4. **UI:**
   - Lazy loading of routes
   - Virtualized lists for large datasets
   - Optimistic updates

---

## Error Handling

### Error Categories

| Category | HTTP Status | gRPC Code | Example |
|----------|-------------|-----------|---------|
| Validation | 400 | INVALID_ARGUMENT | Invalid KDL syntax |
| Authentication | 401 | UNAUTHENTICATED | Missing/invalid token |
| Authorization | 403 | PERMISSION_DENIED | Viewer creating config |
| Not Found | 404 | NOT_FOUND | Config doesn't exist |
| Conflict | 409 | ALREADY_EXISTS | Duplicate instance name |
| Internal | 500 | INTERNAL | Database error |

### Error Response Format

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid configuration syntax",
    "details": [
      {
        "field": "content",
        "message": "KDL parse error at line 15: unexpected token",
        "line": 15,
        "column": 8
      }
    ]
  }
}
```

---

## Observability

### Logging

- Structured JSON logging (zerolog)
- Correlation IDs for request tracing
- Log levels: debug, info, warn, error

### Metrics

Hub exposes Prometheus metrics at `/metrics`:

```
# API metrics
hub_http_requests_total{method, path, status}
hub_http_request_duration_seconds{method, path}

# gRPC metrics
hub_grpc_requests_total{method, status}
hub_grpc_request_duration_seconds{method}

# Business metrics
hub_instances_total{status}
hub_configs_total
hub_deployments_total{status}
hub_active_agents
```

### Health Checks

```
GET /health       # Liveness probe (always 200 if process is running)
GET /ready        # Readiness probe (200 if database is connected)
```

---

## Future Architecture Considerations

### Multi-Region Support

```
                    ┌───────────────────┐
                    │   Global Router   │
                    └─────────┬─────────┘
                              │
          ┌───────────────────┼───────────────────┐
          │                   │                   │
    ┌─────┴─────┐       ┌─────┴─────┐       ┌─────┴─────┐
    │ Hub US    │       │ Hub EU    │       │ Hub APAC  │
    │           │◀─────▶│           │◀─────▶│           │
    │ Region    │ sync  │ Region    │ sync  │ Region    │
    └───────────┘       └───────────┘       └───────────┘
```

### Event-Driven Architecture

For advanced use cases, Hub could emit events:

```
Events:
- instance.registered
- instance.offline
- config.created
- config.updated
- deployment.started
- deployment.completed
- deployment.failed
```

Consumers could be:
- Notification service
- Audit archival
- Analytics pipeline
- GitOps sync
