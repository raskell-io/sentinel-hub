# Sentinel Hub Roadmap

**Last Updated:** 2025-12-31
**Current Version:** 0.0.0 (pre-alpha)
**Production Readiness:** 0%

---

## Executive Summary

Sentinel Hub is the fleet management control plane for Sentinel reverse proxies. It provides centralized configuration management, deployment orchestration, and observability for Sentinel instances across environments.

This roadmap outlines the path from initial scaffolding to production-ready fleet management platform.

---

## Phase 0 — Foundation (Current)

**Goal:** Project scaffolding and core infrastructure.

**Deliverables:**
- [x] Repository created
- [x] Project documentation (CLAUDE.md, ROADMAP.md, ARCHITECTURE.md)
- [ ] Go project structure with dependency management
- [ ] React/TypeScript project with build tooling
- [ ] Database schema and migrations
- [ ] Basic CI/CD pipeline (build, test, lint)
- [ ] Docker build for development

**Exit Criteria:**
- `make build` produces working binaries
- `make test` runs unit tests
- `make dev` starts development environment

---

## Phase 1 — Core API & Data Layer

**Goal:** Implement core data models and REST API.

**Deliverables:**
- [ ] Database layer with sqlc
  - [ ] Migrations infrastructure
  - [ ] Instance model and queries
  - [ ] Config model with versioning
  - [ ] User model
  - [ ] Audit log model
- [ ] REST API server
  - [ ] Chi router setup with middleware
  - [ ] Health check endpoints
  - [ ] OpenAPI spec generation
- [ ] Configuration management API
  - [ ] CRUD for configurations
  - [ ] Version history
  - [ ] Rollback endpoint
  - [ ] KDL validation (syntax only)
- [ ] Instance management API
  - [ ] Register/deregister instances
  - [ ] List with filtering
  - [ ] Instance details

**Exit Criteria:**
- API endpoints pass integration tests
- OpenAPI spec is complete and accurate
- Database migrations run cleanly

---

## Phase 2 — Authentication & Authorization

**Goal:** Secure the API with authentication and RBAC.

**Deliverables:**
- [ ] Local authentication
  - [ ] User registration (admin-only)
  - [ ] Login/logout with JWT
  - [ ] Password reset flow
- [ ] Session management
  - [ ] JWT token issuance
  - [ ] Token refresh
  - [ ] Token revocation
- [ ] Authorization middleware
  - [ ] RBAC (admin/operator/viewer)
  - [ ] Permission checks on endpoints
- [ ] Audit logging
  - [ ] Log all write operations
  - [ ] Include user, timestamp, details
  - [ ] Audit log API for viewing

**Exit Criteria:**
- All endpoints require authentication
- Role-based access control enforced
- Audit log captures all changes

---

## Phase 3 — Agent Communication

**Goal:** Enable Sentinel agents to communicate with Hub.

**Deliverables:**
- [ ] gRPC service definition
  - [ ] Agent registration
  - [ ] Heartbeat/health reporting
  - [ ] Configuration sync
  - [ ] Metrics reporting
- [ ] gRPC server implementation
  - [ ] TLS/mTLS support
  - [ ] Agent authentication
  - [ ] Connection management
- [ ] Agent implementation
  - [ ] gRPC client
  - [ ] Configuration file watcher
  - [ ] SIGHUP trigger for Sentinel
  - [ ] Health check reporting
- [ ] Instance status tracking
  - [ ] Online/offline detection
  - [ ] Config sync status
  - [ ] Last seen timestamp

**Exit Criteria:**
- Agent registers and maintains connection
- Config changes propagate to agents
- Instance status reflects reality

---

## Phase 4 — Web UI Foundation

**Goal:** Build the basic web interface.

**Deliverables:**
- [ ] React app scaffolding
  - [ ] Vite build setup
  - [ ] TypeScript configuration
  - [ ] Tailwind CSS setup
  - [ ] Component library (Radix UI)
- [ ] Authentication UI
  - [ ] Login page
  - [ ] Session management
  - [ ] Protected routes
- [ ] Layout and navigation
  - [ ] Sidebar navigation
  - [ ] Header with user menu
  - [ ] Responsive design
- [ ] Instance list view
  - [ ] Table with filtering/sorting
  - [ ] Status indicators
  - [ ] Search functionality
- [ ] Instance detail view
  - [ ] Metadata display
  - [ ] Current config
  - [ ] Health status

**Exit Criteria:**
- Users can log in and navigate
- Instance list shows real data
- UI is responsive and accessible

---

## Phase 5 — Configuration Management UI

**Goal:** Full configuration lifecycle in UI.

**Deliverables:**
- [ ] Configuration list view
  - [ ] Table with search/filter
  - [ ] Version count indicator
  - [ ] Last modified info
- [ ] Configuration editor
  - [ ] KDL syntax highlighting
  - [ ] Real-time validation
  - [ ] Error display with line numbers
  - [ ] Save with change summary
- [ ] Version history
  - [ ] List of versions
  - [ ] Diff view between versions
  - [ ] Rollback action
- [ ] Configuration assignment
  - [ ] Assign config to instances
  - [ ] Bulk assignment by label

**Exit Criteria:**
- Users can create/edit configs in UI
- Version history is visible and usable
- Rollback works correctly

---

## Phase 6 — Deployment Orchestration

**Goal:** Safe configuration deployments.

**Deliverables:**
- [ ] Deployment model and API
  - [ ] Create deployment
  - [ ] Track deployment status
  - [ ] Cancel deployment
- [ ] Deployment strategies
  - [ ] All-at-once
  - [ ] Rolling (percentage-based)
  - [ ] Canary (single instance first)
- [ ] Deployment execution
  - [ ] Push config to agents
  - [ ] Track apply status per instance
  - [ ] Automatic rollback on failure
- [ ] Deployment UI
  - [ ] Deploy wizard
  - [ ] Progress tracking
  - [ ] History view

**Exit Criteria:**
- Deployments execute reliably
- Rolling deployments respect batch size
- Failed deployments trigger rollback

---

## Phase 7 — Observability

**Goal:** Fleet-wide visibility and metrics.

**Deliverables:**
- [ ] Metrics collection
  - [ ] Agent reports metrics to Hub
  - [ ] Store time-series data (or forward to Prometheus)
  - [ ] Aggregate fleet-wide metrics
- [ ] Dashboard
  - [ ] Fleet overview (instances, status)
  - [ ] Request rate chart
  - [ ] Error rate chart
  - [ ] Latency percentiles
- [ ] Instance metrics view
  - [ ] Per-instance charts
  - [ ] Upstream health
  - [ ] Recent errors
- [ ] Alerting foundation
  - [ ] Alert rule configuration
  - [ ] Alert state tracking
  - [ ] Notification hooks (webhook)

**Exit Criteria:**
- Dashboard shows meaningful metrics
- Users can drill down to instance level
- Alerts fire on defined conditions

---

## Phase 8 — Production Hardening

**Goal:** Production-ready deployment.

**Deliverables:**
- [ ] High availability
  - [ ] Stateless API servers
  - [ ] PostgreSQL support
  - [ ] Connection pooling
- [ ] Performance optimization
  - [ ] Query optimization
  - [ ] Caching layer
  - [ ] Pagination everywhere
- [ ] Security hardening
  - [ ] Security headers
  - [ ] Rate limiting
  - [ ] Input sanitization audit
- [ ] Operational tooling
  - [ ] Helm chart
  - [ ] Docker Compose for dev
  - [ ] Backup/restore scripts
- [ ] Documentation
  - [ ] User guide
  - [ ] API reference
  - [ ] Deployment guide
  - [ ] Troubleshooting guide

**Exit Criteria:**
- Load test: 100 concurrent users
- No security vulnerabilities (OWASP scan)
- Documentation complete

---

## Future Phases (v1.x+)

### GitOps Integration
- Sync configurations from Git repository
- Automatic deployment on Git push
- Drift detection and reconciliation

### OIDC Authentication
- Integration with identity providers
- SSO support
- Group-based role mapping

### Advanced Deployment Strategies
- Blue/green deployments
- Automatic canary analysis
- Traffic shifting

### Multi-Cluster Support
- Cluster/environment abstraction
- Cross-cluster deployments
- Federated view

### CLI Tool
- `hubctl` command-line interface
- Scriptable operations
- CI/CD integration

### Terraform Provider
- Manage configs as code
- Import existing configurations
- State management

---

## Success Criteria

### For Internal Alpha (Phase 4)
- [ ] Core API functional
- [ ] Basic UI for config management
- [ ] Agent communication working
- [ ] Single-user deployment

### For Internal Beta (Phase 6)
- [ ] Multi-user with RBAC
- [ ] Deployment orchestration
- [ ] 10+ managed instances
- [ ] Basic documentation

### For Production (Phase 8)
- [ ] HA deployment tested
- [ ] Security audit passed
- [ ] Performance benchmarks met
- [ ] Complete documentation
- [ ] 100+ managed instances supported

---

## Technical Debt Register

| Item | Severity | Notes |
|------|----------|-------|
| (none yet) | - | Project just started |

---

## Contributing

When implementing roadmap items:

1. Create feature branch from `main`
2. Include tests for new functionality
3. Update API documentation
4. Add audit logging for write operations
5. Update this roadmap when complete

All changes must pass:
- `go test ./...` - Unit tests
- `go vet ./...` - Static analysis
- `npm run lint` - Frontend linting
- `npm run test` - Frontend tests
