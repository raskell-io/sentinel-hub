-- Sentinel Hub Initial Schema
-- SQLite compatible

-- ============================================
-- Instances (Fleet Members)
-- ============================================
CREATE TABLE IF NOT EXISTS instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    hostname TEXT NOT NULL,
    agent_version TEXT NOT NULL,
    sentinel_version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'unknown',  -- unknown, online, offline, degraded, deploying, draining
    last_seen_at DATETIME,
    current_config_id TEXT,
    current_config_version INTEGER,
    labels TEXT,  -- JSON object
    capabilities TEXT,  -- JSON array
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (current_config_id) REFERENCES configs(id)
);

CREATE INDEX IF NOT EXISTS idx_instances_status ON instances(status);
CREATE INDEX IF NOT EXISTS idx_instances_name ON instances(name);
CREATE INDEX IF NOT EXISTS idx_instances_last_seen ON instances(last_seen_at);

-- ============================================
-- Configurations
-- ============================================
CREATE TABLE IF NOT EXISTS configs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    current_version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME  -- Soft delete
);

CREATE INDEX IF NOT EXISTS idx_configs_name ON configs(name);
CREATE INDEX IF NOT EXISTS idx_configs_deleted ON configs(deleted_at);

-- ============================================
-- Configuration Versions (Immutable History)
-- ============================================
CREATE TABLE IF NOT EXISTS config_versions (
    id TEXT PRIMARY KEY,
    config_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,  -- KDL configuration content
    content_hash TEXT NOT NULL,  -- SHA256 hash for change detection
    change_summary TEXT,
    created_by TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (config_id) REFERENCES configs(id),
    UNIQUE (config_id, version)
);

CREATE INDEX IF NOT EXISTS idx_config_versions_config ON config_versions(config_id);
CREATE INDEX IF NOT EXISTS idx_config_versions_hash ON config_versions(content_hash);

-- ============================================
-- Deployments
-- ============================================
CREATE TABLE IF NOT EXISTS deployments (
    id TEXT PRIMARY KEY,
    config_id TEXT NOT NULL,
    config_version INTEGER NOT NULL,
    target_instances TEXT NOT NULL,  -- JSON array of instance IDs or label selector
    strategy TEXT NOT NULL DEFAULT 'rolling',  -- all_at_once, rolling, canary
    batch_size INTEGER DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, in_progress, completed, failed, cancelled
    progress TEXT,  -- JSON object with deployment progress details
    started_at DATETIME,
    completed_at DATETIME,
    created_by TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (config_id) REFERENCES configs(id)
);

CREATE INDEX IF NOT EXISTS idx_deployments_config ON deployments(config_id);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_created ON deployments(created_at);

-- ============================================
-- Deployment Instance Status
-- ============================================
CREATE TABLE IF NOT EXISTS deployment_instances (
    id TEXT PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, in_progress, completed, failed, rolled_back
    started_at DATETIME,
    completed_at DATETIME,
    error_message TEXT,

    FOREIGN KEY (deployment_id) REFERENCES deployments(id),
    FOREIGN KEY (instance_id) REFERENCES instances(id),
    UNIQUE (deployment_id, instance_id)
);

CREATE INDEX IF NOT EXISTS idx_deployment_instances_deployment ON deployment_instances(deployment_id);
CREATE INDEX IF NOT EXISTS idx_deployment_instances_instance ON deployment_instances(instance_id);

-- ============================================
-- Users
-- ============================================
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',  -- admin, operator, viewer
    password_hash TEXT,  -- For local auth
    oidc_subject TEXT,  -- For OIDC auth
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_oidc ON users(oidc_subject);

-- ============================================
-- Audit Log (Append-only)
-- ============================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id TEXT,
    action TEXT NOT NULL,  -- create, update, delete, deploy, rollback, login, etc.
    resource_type TEXT NOT NULL,  -- instance, config, deployment, user
    resource_id TEXT,
    details TEXT,  -- JSON object with action details
    ip_address TEXT,

    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);

-- ============================================
-- Agent Sessions (for token management)
-- ============================================
CREATE TABLE IF NOT EXISTS agent_sessions (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,  -- SHA256 hash of token
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME,

    FOREIGN KEY (instance_id) REFERENCES instances(id)
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_instance ON agent_sessions(instance_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_token ON agent_sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_expires ON agent_sessions(expires_at);
