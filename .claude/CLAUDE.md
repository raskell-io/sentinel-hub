# Sentinel Hub — Fleet Management Control Plane

**Tagline:** Centralized management for Sentinel proxy fleets. Configure, deploy, observe.

---

## 0) Purpose of this document
This file is a single source of truth for LLM agents and humans implementing a fleet management control plane for Sentinel reverse proxies. We are building a production-grade operations platform that enables teams to manage configurations, deployments, and observability across multiple Sentinel instances.

This document prioritizes: **operational simplicity**, **security**, **scalability**, and **developer experience**.

---

## 1) North Star
Build a control plane that makes operating Sentinel fleets boring (in a good way):

- **Centralized configuration management** with validation, versioning, and rollback.
- **Fleet-wide visibility** into health, metrics, and configuration drift.
- **Safe deployments** with staged rollouts, canary analysis, and automatic rollback.
- **Multi-tenant** support for teams managing different environments.
- **API-first** design enabling automation and GitOps workflows.

---

## 2) Principles (non-negotiables)

### 2.1 Operational Simplicity
- Single binary deployment for the control plane.
- Minimal external dependencies (embedded SQLite for small deployments, PostgreSQL for scale).
- Sensible defaults that work out of the box.
- Clear error messages and actionable guidance.

### 2.2 Security-First
- All API endpoints authenticated and authorized.
- Audit logging for all configuration changes.
- Secrets never stored in plaintext.
- TLS everywhere (control plane ↔ agents, UI ↔ API).
- RBAC with principle of least privilege.

### 2.3 Scalability
- Stateless API servers (horizontal scaling).
- Efficient agent polling or push-based updates.
- Support for 1000+ managed Sentinel instances.
- Lazy loading and pagination for large datasets.

### 2.4 Developer Experience
- Clean, responsive UI with keyboard shortcuts.
- Comprehensive REST API with OpenAPI spec.
- gRPC API for agent communication.
- CLI tool for automation and scripting.
- Terraform provider (future).

---

## 3) Target Outcomes

### 3.1 Must-have outcomes (v1.0)
- Configuration CRUD with validation against Sentinel's KDL schema.
- Configuration versioning with diff view and rollback.
- Fleet inventory with health status and metadata.
- Configuration push to Sentinel instances (via agent).
- Basic metrics dashboard (requests, errors, latency).
- User authentication (local + OIDC).
- Audit log for all changes.

### 3.2 Nice outcomes (v1.x)
- GitOps integration (sync from Git repository).
- Staged rollouts with automatic canary analysis.
- Alerting rules and notification integrations.
- Configuration templates and inheritance.
- Multi-cluster/multi-region support.
- Terraform provider.

### 3.3 Future outcomes (v2.x)
- AI-assisted configuration tuning.
- Automatic scaling recommendations.
- Cost analysis and optimization.
- Service mesh integration.

---

## 4) Non-goals (explicit)
- Replacing Kubernetes or other orchestrators for deployment.
- Implementing a full observability stack (use Prometheus/Grafana).
- Managing non-Sentinel infrastructure.
- Real-time log streaming (link to existing log aggregation).

---

## 5) High-level Architecture

### 5.1 Components

1) **Hub API Server (Go)**
   - REST API for UI and external integrations.
   - gRPC API for Sentinel agent communication.
   - Business logic for fleet management.
   - Handles authentication, authorization, audit logging.

2) **Hub Web UI (React/TypeScript)**
   - Single-page application.
   - Real-time updates via WebSocket.
   - Responsive design for desktop and tablet.

3) **Sentinel Agent (sidecar or standalone)**
   - Runs alongside each Sentinel instance.
   - Connects to Hub via gRPC.
   - Reports health, metrics, and current config.
   - Receives and applies configuration updates.
   - Triggers graceful reload on Sentinel.

4) **Data Store**
   - SQLite for single-node deployments.
   - PostgreSQL for production/HA deployments.
   - Stores: configs, versions, fleet state, users, audit logs.

### 5.2 Communication Flow

```
┌─────────────────────────────────────────────────────────────┐
│                        Hub Control Plane                      │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐      │
│  │   Web UI    │───▶│  REST API   │───▶│  Database   │      │
│  │  (React)    │    │    (Go)     │    │ (SQLite/PG) │      │
│  └─────────────┘    └──────┬──────┘    └─────────────┘      │
│                            │                                  │
│                     ┌──────┴──────┐                          │
│                     │  gRPC API   │                          │
│                     └──────┬──────┘                          │
└────────────────────────────┼────────────────────────────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
    ┌─────────┐         ┌─────────┐         ┌─────────┐
    │  Agent  │         │  Agent  │         │  Agent  │
    └────┬────┘         └────┬────┘         └────┬────┘
         │                   │                   │
    ┌────┴────┐         ┌────┴────┐         ┌────┴────┐
    │Sentinel │         │Sentinel │         │Sentinel │
    │Instance │         │Instance │         │Instance │
    └─────────┘         └─────────┘         └─────────┘
```

---

## 6) Tech Stack

### 6.1 Backend (Go)
- **Framework:** chi (lightweight HTTP router)
- **gRPC:** google.golang.org/grpc
- **Database:** sqlc for type-safe SQL, migrate for migrations
- **Auth:** JWT tokens, OIDC integration
- **Config:** Viper for configuration management
- **Logging:** zerolog (structured JSON logging)
- **Validation:** go-playground/validator

### 6.2 Frontend (Next.js/TypeScript)
- **Framework:** Next.js 15 with React 19 and TypeScript
- **State:** TanStack Query (server state), Zustand (client state)
- **UI Components:** shadcn/ui (Radix primitives + Tailwind CSS)
- **Routing:** Next.js App Router
- **Forms:** React Hook Form + Zod validation
- **Charts:** Recharts or Tremor

### 6.3 Agent
- **Language:** Go (shared code with Hub)
- **Communication:** gRPC with mTLS
- **Config reload:** SIGHUP to Sentinel process

---

## 7) API Design

### 7.1 REST API (for UI and external tools)

```
# Fleet Management
GET    /api/v1/instances              # List all Sentinel instances
GET    /api/v1/instances/:id          # Get instance details
POST   /api/v1/instances              # Register new instance
DELETE /api/v1/instances/:id          # Deregister instance

# Configuration Management
GET    /api/v1/configs                # List configurations
GET    /api/v1/configs/:id            # Get config with versions
POST   /api/v1/configs                # Create new config
PUT    /api/v1/configs/:id            # Update config (creates version)
DELETE /api/v1/configs/:id            # Delete config

GET    /api/v1/configs/:id/versions   # List config versions
GET    /api/v1/configs/:id/versions/:v # Get specific version
POST   /api/v1/configs/:id/rollback   # Rollback to version

# Deployments
POST   /api/v1/deployments            # Deploy config to instances
GET    /api/v1/deployments/:id        # Get deployment status
POST   /api/v1/deployments/:id/cancel # Cancel in-progress deployment

# Observability
GET    /api/v1/metrics/summary        # Fleet-wide metrics summary
GET    /api/v1/instances/:id/metrics  # Instance metrics

# Users & Auth
POST   /api/v1/auth/login             # Login
POST   /api/v1/auth/logout            # Logout
GET    /api/v1/users                  # List users (admin)
POST   /api/v1/users                  # Create user (admin)
```

### 7.2 gRPC API (for agents)

```protobuf
service HubAgent {
  // Agent registration and heartbeat
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);

  // Configuration sync
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
  rpc ReportConfigApplied(ConfigAppliedRequest) returns (ConfigAppliedResponse);

  // Metrics reporting
  rpc ReportMetrics(stream MetricsRequest) returns (MetricsResponse);
}
```

---

## 8) Data Model

### 8.1 Core Entities

```
Instance
├── id (UUID)
├── name (user-friendly name)
├── hostname
├── agent_version
├── sentinel_version
├── status (online/offline/unhealthy)
├── last_seen_at
├── current_config_id
├── current_config_version
├── labels (key-value for filtering)
├── created_at
└── updated_at

Config
├── id (UUID)
├── name (unique identifier)
├── description
├── content (KDL text)
├── content_hash (for change detection)
├── created_by
├── created_at
└── updated_at

ConfigVersion
├── id (UUID)
├── config_id (FK)
├── version (incrementing integer)
├── content (KDL text)
├── content_hash
├── change_summary
├── created_by
├── created_at

Deployment
├── id (UUID)
├── config_id
├── config_version
├── target_instances (array of IDs or label selector)
├── strategy (all-at-once/rolling/canary)
├── status (pending/in-progress/completed/failed/cancelled)
├── started_at
├── completed_at
├── created_by

User
├── id (UUID)
├── email
├── name
├── role (admin/operator/viewer)
├── password_hash (for local auth)
├── oidc_subject (for OIDC auth)
├── created_at
└── last_login_at

AuditLog
├── id (UUID)
├── timestamp
├── user_id
├── action (create/update/delete/deploy/rollback)
├── resource_type (config/instance/user)
├── resource_id
├── details (JSON)
├── ip_address
```

---

## 9) Security Model

### 9.1 Authentication
- **Local auth:** Email + password with bcrypt hashing.
- **OIDC:** Integration with identity providers (Okta, Auth0, Keycloak).
- **API keys:** For automation and CI/CD pipelines.
- **Agent auth:** mTLS with client certificates.

### 9.2 Authorization (RBAC)
- **Admin:** Full access to all resources.
- **Operator:** Manage configs and deployments, view instances.
- **Viewer:** Read-only access to all resources.
- Future: Fine-grained permissions per environment/label.

### 9.3 Audit Logging
- All write operations logged with user, timestamp, and details.
- Audit logs immutable (append-only).
- Retention policy configurable.

---

## 10) Configuration Validation

### 10.1 KDL Schema Validation
- Parse and validate KDL syntax.
- Validate against Sentinel's expected schema.
- Check for common misconfigurations.
- Provide helpful error messages with line numbers.

### 10.2 Semantic Validation
- Warn on unused upstreams.
- Warn on routes without rate limits.
- Check TLS certificate paths exist (if accessible).
- Validate upstream addresses are resolvable.

---

## 11) Roadmap

See `.claude/ROADMAP.md` for detailed roadmap.

---

## 12) Work Instructions for LLM Agents

When implementing anything, follow these rules:

1) **API-first:** Define OpenAPI spec before implementing endpoints.
2) **Type safety:** Use sqlc for database queries, TypeScript for frontend.
3) **Error handling:** Return structured errors with codes and messages.
4) **Logging:** Log all significant operations with correlation IDs.
5) **Testing:** Write tests for business logic and API endpoints.
6) **Security:** Validate all inputs, sanitize outputs, check permissions.
7) **Documentation:** Update API docs and README for new features.

### 12.1 Code Style

**Go:**
- Follow standard Go conventions (gofmt, golint).
- Use structured logging (zerolog).
- Handle errors explicitly, no panics in handlers.
- Use context for cancellation and timeouts.

**TypeScript/React:**
- Use functional components with hooks.
- Prefer composition over inheritance.
- Use TanStack Query for server state.
- Keep components small and focused.

### 12.2 Commit Messages
- Use conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`
- Reference issues when applicable.
- Keep subject line under 72 characters.

---

## 13) Definition of Done (per feature)
- Feature implemented and working.
- API endpoint documented in OpenAPI spec.
- Unit tests for business logic.
- Integration test for API endpoint.
- UI component with loading/error states.
- Audit logging for write operations.
- README updated if user-facing.

---

## 14) Getting Started

```bash
# Install tools (Go, Node.js)
mise install

# Setup development environment
mise run setup

# Run hub server
mise run dev

# Run web frontend (in another terminal)
mise run dev:web

# Run both together
mise run dev:all

# Run agent
mise run dev:agent
```

---

## 15) Environment Variables

```bash
# Hub Server
HUB_PORT=8080                    # HTTP port
HUB_GRPC_PORT=9090               # gRPC port for agents
HUB_DATABASE_URL=sqlite://hub.db # or postgres://...
HUB_JWT_SECRET=<random-secret>   # JWT signing key
HUB_OIDC_ISSUER=                 # OIDC provider URL (optional)
HUB_OIDC_CLIENT_ID=              # OIDC client ID (optional)
HUB_LOG_LEVEL=info               # debug, info, warn, error

# Agent
AGENT_HUB_URL=https://hub.example.com:9090
AGENT_INSTANCE_NAME=sentinel-prod-1
AGENT_SENTINEL_CONFIG=/etc/sentinel/config.kdl
AGENT_TLS_CERT=/etc/agent/cert.pem
AGENT_TLS_KEY=/etc/agent/key.pem
```
