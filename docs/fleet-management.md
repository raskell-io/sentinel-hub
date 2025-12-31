# Fleet Management Architecture

**Last Updated:** 2025-12-31

This document describes the architecture for managing a fleet of Sentinel proxy instances through Sentinel Hub.

---

## Overview

Sentinel Hub uses a **hybrid push/pull model** for configuration distribution:

- **Pull**: Agents poll Hub for configuration changes (resilient, stateless)
- **Push**: Hub notifies agents of pending changes via gRPC streaming (instant updates)
- **Hybrid**: Agents subscribe to event streams but fetch actual configs via pull

This approach combines the reliability of polling with the responsiveness of push notifications.

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        SENTINEL HUB                              │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │  REST API   │  │  gRPC API   │  │  Deployment Orchestrator │  │
│  │  (Web UI)   │  │  (Agents)   │  │                         │  │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘  │
│         │                │                      │                │
│         └────────────────┴──────────────────────┘                │
│                          │                                       │
│              ┌───────────┴───────────┐                          │
│              │    Fleet Manager      │                          │
│              │  - Instance registry  │                          │
│              │  - Health tracking    │                          │
│              │  - Config versioning  │                          │
│              └───────────┬───────────┘                          │
│                          │                                       │
│              ┌───────────┴───────────┐                          │
│              │      Data Store       │                          │
│              │  (SQLite/PostgreSQL)  │                          │
│              └───────────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
                           │
                           │ gRPC (mTLS)
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │   Agent     │ │   Agent     │ │   Agent     │
    │  ┌───────┐  │ │  ┌───────┐  │ │  ┌───────┐  │
    │  │Sentinel│  │ │  │Sentinel│  │ │  │Sentinel│  │
    │  └───────┘  │ │  └───────┘  │ │  └───────┘  │
    └─────────────┘ └─────────────┘ └─────────────┘
```

---

## Agent-Hub Protocol

### gRPC Service Definition

```protobuf
syntax = "proto3";

package sentinel.hub.v1;

service FleetService {
  // ============================================
  // Agent Lifecycle
  // ============================================

  // Register a new agent with the Hub
  // Called once when agent starts
  rpc Register(RegisterRequest) returns (RegisterResponse);

  // Periodic heartbeat to report health and status
  // Called every 30 seconds by default
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);

  // Gracefully deregister when agent shuts down
  rpc Deregister(DeregisterRequest) returns (DeregisterResponse);

  // ============================================
  // Configuration (Pull Model)
  // ============================================

  // Fetch current configuration for this instance
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);

  // Get a specific config version (for rollback)
  rpc GetConfigVersion(GetConfigVersionRequest) returns (GetConfigVersionResponse);

  // ============================================
  // Event Stream (Push Notifications)
  // ============================================

  // Subscribe to events (config updates, deployment requests)
  // Long-lived bidirectional stream
  rpc Subscribe(SubscribeRequest) returns (stream Event);

  // ============================================
  // Deployment Coordination
  // ============================================

  // Acknowledge receipt of deployment request
  rpc AckDeployment(AckDeploymentRequest) returns (AckDeploymentResponse);

  // Report deployment progress/completion
  rpc ReportDeploymentStatus(DeploymentStatusRequest) returns (DeploymentStatusResponse);
}

// ============================================
// Messages
// ============================================

message RegisterRequest {
  string instance_id = 1;        // Unique instance identifier
  string instance_name = 2;      // Human-readable name
  string hostname = 3;           // Machine hostname
  string agent_version = 4;      // Agent binary version
  string sentinel_version = 5;   // Sentinel proxy version
  map<string, string> labels = 6; // Custom labels for filtering
  repeated string capabilities = 7; // Supported features
}

message RegisterResponse {
  string token = 1;              // Session token for subsequent calls
  string config_version = 2;     // Current config version to apply
  string config_hash = 3;        // Hash for change detection
  int32 heartbeat_interval = 4;  // Recommended heartbeat interval (seconds)
}

message HeartbeatRequest {
  string instance_id = 1;
  string token = 2;
  InstanceStatus status = 3;
  string current_config_version = 4;
  string current_config_hash = 5;
  InstanceMetrics metrics = 6;
}

message HeartbeatResponse {
  bool config_update_available = 1; // Hint to fetch new config
  string latest_config_version = 2;
  repeated PendingAction actions = 3; // Actions agent should take
}

message InstanceStatus {
  State state = 1;
  string message = 2;           // Human-readable status message
  int64 uptime_seconds = 3;
  int32 active_connections = 4;

  enum State {
    UNKNOWN = 0;
    HEALTHY = 1;
    DEGRADED = 2;     // Running but with issues
    UNHEALTHY = 3;    // Failing health checks
  }
}

message InstanceMetrics {
  int64 requests_total = 1;
  int64 requests_failed = 2;
  double latency_p50_ms = 3;
  double latency_p99_ms = 4;
  int64 bytes_sent = 5;
  int64 bytes_received = 6;
}

message Event {
  string event_id = 1;
  EventType type = 2;
  int64 timestamp = 3;
  oneof payload {
    ConfigUpdateEvent config_update = 4;
    DeploymentEvent deployment = 5;
    DrainEvent drain = 6;
  }

  enum EventType {
    UNKNOWN = 0;
    CONFIG_UPDATE = 1;    // New config available
    DEPLOYMENT = 2;       // Deployment targeting this instance
    DRAIN = 3;            // Drain connections and prepare for shutdown
    PING = 4;             // Keep-alive ping
  }
}

message ConfigUpdateEvent {
  string config_version = 1;
  string config_hash = 2;
  string change_summary = 3;
}

message DeploymentEvent {
  string deployment_id = 1;
  string config_version = 2;
  DeploymentStrategy strategy = 3;
  int32 batch_position = 4;    // Position in rolling deployment
  int32 batch_total = 5;       // Total batches
  int64 deadline = 6;          // Unix timestamp deadline
}

enum DeploymentStrategy {
  ALL_AT_ONCE = 0;
  ROLLING = 1;
  CANARY = 2;
}

message PendingAction {
  ActionType type = 1;
  string action_id = 2;
  map<string, string> params = 3;

  enum ActionType {
    FETCH_CONFIG = 0;
    APPLY_CONFIG = 1;
    REPORT_STATUS = 2;
    DRAIN = 3;
  }
}

message GetConfigRequest {
  string instance_id = 1;
  string token = 2;
  string version = 3;  // Optional: specific version, empty for latest
}

message GetConfigResponse {
  string version = 1;
  string hash = 2;
  string content = 3;      // KDL configuration content
  int64 created_at = 4;
}

message DeploymentStatusRequest {
  string instance_id = 1;
  string token = 2;
  string deployment_id = 3;
  DeploymentState state = 4;
  string message = 5;

  enum DeploymentState {
    PENDING = 0;
    IN_PROGRESS = 1;
    VALIDATING = 2;
    COMPLETED = 3;
    FAILED = 4;
    ROLLED_BACK = 5;
  }
}

message DeploymentStatusResponse {
  bool acknowledged = 1;
  string next_action = 2;  // Optional instruction from hub
}
```

---

## Communication Flows

### 1. Agent Registration

```
Agent                              Hub
  │                                 │
  │──── Register ──────────────────>│
  │     (instance_id, metadata)     │
  │                                 │ Create/update instance record
  │                                 │ Generate session token
  │<─── RegisterResponse ───────────│
  │     (token, config_version)     │
  │                                 │
  │──── Subscribe(token) ──────────>│
  │                                 │ Add to subscriber list
  │<═══ Event stream (open) ═══════>│
  │                                 │
  │──── GetConfig ─────────────────>│
  │     (version from response)     │
  │                                 │
  │<─── GetConfigResponse ──────────│
  │     (KDL content)               │
  │                                 │
  │     Apply config to Sentinel    │
  │                                 │
  │──── Heartbeat ─────────────────>│
  │     (status: HEALTHY)           │
  │<─── HeartbeatResponse ──────────│
```

### 2. Configuration Deployment (Rolling)

```
Hub                               Agent 1              Agent 2              Agent 3
 │                                   │                    │                    │
 │   User initiates deployment       │                    │                    │
 │   Strategy: ROLLING               │                    │                    │
 │   Batch size: 1                   │                    │                    │
 │                                   │                    │                    │
 │ ── Batch 1 ──────────────────────────────────────────────────────────────── │
 │                                   │                    │                    │
 │─── Event(DEPLOYMENT) ───────────>│                    │                    │
 │    (batch_position: 1/3)          │                    │                    │
 │                                   │                    │                    │
 │<── AckDeployment(accepted) ───────│                    │                    │
 │                                   │                    │                    │
 │                                   │── GetConfig ──────>│ (to Hub)           │
 │                                   │<─ Config content ──│                    │
 │                                   │                    │                    │
 │                                   │   Write config     │                    │
 │                                   │   SIGHUP Sentinel  │                    │
 │                                   │   Health check     │                    │
 │                                   │                    │                    │
 │<── ReportStatus(COMPLETED) ───────│                    │                    │
 │                                   │                    │                    │
 │ ── Batch 2 ──────────────────────────────────────────────────────────────── │
 │                                   │                    │                    │
 │─── Event(DEPLOYMENT) ────────────────────────────────>│                    │
 │    (batch_position: 2/3)          │                    │                    │
 │                                   │                    │                    │
 │   ... (same flow) ...             │                    │                    │
 │                                   │                    │                    │
 │ ── Batch 3 ──────────────────────────────────────────────────────────────── │
 │                                   │                    │                    │
 │─── Event(DEPLOYMENT) ───────────────────────────────────────────────────>│
 │    (batch_position: 3/3)          │                    │                    │
 │                                   │                    │                    │
 │   ... (same flow) ...             │                    │                    │
 │                                   │                    │                    │
 │   Deployment COMPLETED            │                    │                    │
```

### 3. Automatic Rollback on Failure

```
Hub                               Agent 1              Agent 2
 │                                   │                    │
 │─── Event(DEPLOYMENT, v2) ────────>│                    │
 │                                   │                    │
 │<── AckDeployment(accepted) ────────│                    │
 │                                   │                    │
 │                                   │   Apply config v2  │
 │                                   │   Health check...  │
 │                                   │   FAILED!          │
 │                                   │                    │
 │<── ReportStatus(FAILED) ──────────│                    │
 │    (message: "health check       │                    │
 │     failed after 3 attempts")     │                    │
 │                                   │                    │
 │   Deployment FAILED               │                    │
 │   Trigger rollback                │                    │
 │                                   │                    │
 │─── Event(DEPLOYMENT, v1) ────────>│                    │
 │    (rollback: true)               │                    │
 │                                   │                    │
 │                                   │   Apply config v1  │
 │                                   │   Health check OK  │
 │                                   │                    │
 │<── ReportStatus(ROLLED_BACK) ─────│                    │
 │                                   │                    │
 │   Instance rolled back to v1      │                    │
 │   Deployment cancelled            │                    │
 │   No changes to Agent 2           │                    │
```

---

## Instance State Machine

```
                              ┌──────────────┐
                              │   UNKNOWN    │
                              │  (initial)   │
                              └──────┬───────┘
                                     │
                                     │ Register()
                                     ▼
                              ┌──────────────┐
              ┌──────────────▶│    ONLINE    │◀──────────────┐
              │               │   (healthy)  │               │
              │               └──────┬───────┘               │
              │                      │                       │
              │      ┌───────────────┼───────────────┐       │
              │      │               │               │       │
              │      ▼               ▼               ▼       │
              │ ┌─────────┐   ┌───────────┐   ┌──────────┐   │
              │ │DEPLOYING│   │  DEGRADED │   │ OFFLINE  │   │
              │ │         │   │           │   │          │   │
              │ └────┬────┘   └─────┬─────┘   └────┬─────┘   │
              │      │              │              │         │
              │      │              │              │         │
              │      └──────────────┴──────────────┘         │
              │                     │                        │
              │     Recovery /      │      Heartbeat         │
              │     Heartbeat       │      resumes           │
              └─────────────────────┴────────────────────────┘
                                    │
                                    │ Deregister() or
                                    │ prolonged offline
                                    ▼
                             ┌──────────────┐
                             │   DRAINING   │
                             │              │
                             └──────┬───────┘
                                    │
                                    │ Drain complete
                                    ▼
                             ┌──────────────┐
                             │   REMOVED    │
                             │  (terminal)  │
                             └──────────────┘
```

### State Definitions

| State | Description | Transitions |
|-------|-------------|-------------|
| **UNKNOWN** | Initial state before registration | → ONLINE (on Register) |
| **ONLINE** | Agent connected, Sentinel healthy | → DEPLOYING, DEGRADED, OFFLINE |
| **DEPLOYING** | Configuration deployment in progress | → ONLINE (success), DEGRADED (partial), OFFLINE (agent lost) |
| **DEGRADED** | Sentinel running but with issues | → ONLINE (recovery), OFFLINE (failure) |
| **OFFLINE** | No heartbeat received within timeout | → ONLINE (reconnect), DRAINING (manual) |
| **DRAINING** | Graceful shutdown in progress | → REMOVED |
| **REMOVED** | Instance deregistered | Terminal state |

### Timeout Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `heartbeat_interval` | 30s | How often agents send heartbeats |
| `offline_threshold` | 90s | Time without heartbeat before marking OFFLINE |
| `deployment_timeout` | 5m | Max time for a single instance deployment |
| `drain_timeout` | 30s | Time allowed for connection draining |

---

## Deployment Strategies

### All-at-Once

Deploy to all target instances simultaneously.

```
Use case: Non-critical environments, quick updates
Risk: High - all instances affected simultaneously
Rollback: Manual, affects all instances
```

### Rolling

Deploy to instances in sequential batches.

```
Use case: Production deployments with gradual rollout
Configuration:
  - batch_size: Number of instances per batch (default: 1)
  - batch_delay: Wait time between batches (default: 30s)
  - failure_threshold: Max failures before aborting (default: 1)
Risk: Medium - limited blast radius
Rollback: Automatic on failure, only affects deployed batches
```

### Canary

Deploy to a small subset, monitor, then proceed.

```
Use case: High-risk changes requiring validation
Configuration:
  - canary_instances: Number or percentage for initial deploy
  - canary_duration: How long to monitor before proceeding
  - success_criteria: Metrics thresholds for success
Risk: Low - minimal initial exposure
Rollback: Automatic if canary fails metrics checks
```

---

## Offline Behavior

When an agent loses connection to Hub:

### 1. Keep Running (Default)
- Continue serving traffic with current configuration
- Buffer metrics for later reporting
- Retry connection with exponential backoff
- Log connection failures

### 2. Configuration
```yaml
# Agent configuration
offline_behavior:
  mode: keep_running  # keep_running | shutdown | degraded
  max_offline_duration: 24h  # Optional: shutdown after this duration
  retry_backoff:
    initial: 1s
    max: 5m
    multiplier: 2
```

### 3. Reconnection
On reconnection:
1. Re-register with Hub
2. Compare config hashes
3. Fetch updated config if changed
4. Resume normal heartbeat/event flow

---

## Security Considerations

### Agent Authentication

Agents authenticate to Hub using **mTLS**:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Certificate Hierarchy                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│                    ┌──────────────┐                             │
│                    │   Hub CA     │                             │
│                    │  (root cert) │                             │
│                    └──────┬───────┘                             │
│                           │                                      │
│              ┌────────────┼────────────┐                        │
│              │            │            │                        │
│              ▼            ▼            ▼                        │
│       ┌──────────┐ ┌──────────┐ ┌──────────┐                   │
│       │ Hub Cert │ │Agent Cert│ │Agent Cert│                   │
│       │          │ │ (inst-1) │ │ (inst-2) │                   │
│       └──────────┘ └──────────┘ └──────────┘                   │
│                                                                  │
│  Certificate Fields:                                             │
│  - CN: instance-id (for agents)                                 │
│  - O: organization                                               │
│  - OU: sentinel-agent                                           │
│  - SAN: hostname, IP (for hub)                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Token-Based Alternative

For simpler setups without PKI:

```
1. Pre-shared registration token (env var or file)
2. Agent presents token during Register()
3. Hub validates and issues session JWT
4. JWT used for all subsequent calls
5. JWT refreshed via Heartbeat response
```

### Configuration Security

Sensitive values in configurations:

```kdl
// Option 1: Reference secrets by name (Hub stores encrypted)
upstream "api" {
  tls {
    cert secret="api-cert"
    key secret="api-key"
  }
}

// Option 2: External secret reference
upstream "api" {
  tls {
    cert env="API_CERT_PATH"
    key vault="secret/data/api/tls#key"
  }
}
```

---

## Configuration Validation

### Validation Levels

1. **Syntax Validation** (Hub, on save)
   - Valid KDL syntax
   - Parse without errors

2. **Schema Validation** (Hub, on save)
   - Required fields present
   - Valid field types
   - Valid enum values

3. **Semantic Validation** (Hub, pre-deployment)
   - Upstream addresses resolvable
   - Port numbers in valid range
   - No duplicate route paths
   - TLS cert/key pairs match

4. **Runtime Validation** (Agent, on apply)
   - Sentinel accepts configuration
   - Health check passes

---

## Monitoring and Alerting

### Key Metrics to Track

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `hub_agents_online` | Connected agents | < expected count |
| `hub_agents_offline` | Disconnected agents | > 0 |
| `hub_deployment_duration_seconds` | Time to complete deployment | > 10m |
| `hub_deployment_failures_total` | Failed deployments | > 0 |
| `hub_config_sync_lag_seconds` | Time since last successful sync | > 5m |

### Recommended Alerts

```yaml
# Prometheus alerting rules
groups:
  - name: sentinel-hub
    rules:
      - alert: AgentOffline
        expr: hub_agents_offline > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "{{ $value }} Sentinel agents offline"

      - alert: DeploymentFailed
        expr: increase(hub_deployment_failures_total[1h]) > 0
        labels:
          severity: critical
        annotations:
          summary: "Sentinel deployment failed"

      - alert: ConfigSyncLag
        expr: hub_config_sync_lag_seconds > 300
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Config sync lagging for {{ $labels.instance }}"
```

---

## Implementation Priorities

### Phase 1: Core Agent Protocol
1. Define proto files
2. Implement Register/Heartbeat in Hub
3. Implement agent client
4. Basic instance tracking

### Phase 2: Configuration Distribution
1. GetConfig implementation
2. Config versioning in database
3. Agent config application
4. Sentinel reload integration

### Phase 3: Event Streaming
1. Subscribe implementation
2. Event dispatch system
3. Connection management
4. Reconnection handling

### Phase 4: Deployment Orchestration
1. Deployment model
2. Rolling deployment logic
3. Status tracking
4. Automatic rollback

### Phase 5: Advanced Features
1. Canary deployments
2. Metrics collection
3. Alerting integration
4. Multi-region support
