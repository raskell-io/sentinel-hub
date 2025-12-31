package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Instance represents a Sentinel proxy instance in the fleet.
type Instance struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Hostname             string            `json:"hostname"`
	AgentVersion         string            `json:"agent_version"`
	SentinelVersion      string            `json:"sentinel_version"`
	Status               InstanceStatus    `json:"status"`
	LastSeenAt           *time.Time        `json:"last_seen_at,omitempty"`
	CurrentConfigID      *string           `json:"current_config_id,omitempty"`
	CurrentConfigVersion *int              `json:"current_config_version,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	Capabilities         []string          `json:"capabilities,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// InstanceStatus represents the state of an instance.
type InstanceStatus string

const (
	InstanceStatusUnknown   InstanceStatus = "unknown"
	InstanceStatusOnline    InstanceStatus = "online"
	InstanceStatusOffline   InstanceStatus = "offline"
	InstanceStatusDegraded  InstanceStatus = "degraded"
	InstanceStatusDeploying InstanceStatus = "deploying"
	InstanceStatusDraining  InstanceStatus = "draining"
)

// Config represents a Sentinel configuration.
type Config struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    *string    `json:"description,omitempty"`
	CurrentVersion int        `json:"current_version"`
	CreatedBy      *string    `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// ConfigVersion represents an immutable version of a configuration.
type ConfigVersion struct {
	ID            string    `json:"id"`
	ConfigID      string    `json:"config_id"`
	Version       int       `json:"version"`
	Content       string    `json:"content"`
	ContentHash   string    `json:"content_hash"`
	ChangeSummary *string   `json:"change_summary,omitempty"`
	CreatedBy     *string   `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Deployment represents a configuration deployment to instances.
type Deployment struct {
	ID              string             `json:"id"`
	ConfigID        string             `json:"config_id"`
	ConfigVersion   int                `json:"config_version"`
	TargetInstances []string           `json:"target_instances"`
	Strategy        DeploymentStrategy `json:"strategy"`
	BatchSize       int                `json:"batch_size"`
	Status          DeploymentStatus   `json:"status"`
	Progress        *DeploymentProgress `json:"progress,omitempty"`
	StartedAt       *time.Time         `json:"started_at,omitempty"`
	CompletedAt     *time.Time         `json:"completed_at,omitempty"`
	CreatedBy       *string            `json:"created_by,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
}

// DeploymentStrategy defines how a deployment is executed.
type DeploymentStrategy string

const (
	DeploymentStrategyAllAtOnce DeploymentStrategy = "all_at_once"
	DeploymentStrategyRolling   DeploymentStrategy = "rolling"
	DeploymentStrategyCanary    DeploymentStrategy = "canary"
)

// DeploymentStatus represents the state of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending    DeploymentStatus = "pending"
	DeploymentStatusInProgress DeploymentStatus = "in_progress"
	DeploymentStatusCompleted  DeploymentStatus = "completed"
	DeploymentStatusFailed     DeploymentStatus = "failed"
	DeploymentStatusCancelled  DeploymentStatus = "cancelled"
)

// DeploymentProgress tracks the progress of a deployment.
type DeploymentProgress struct {
	TotalInstances     int    `json:"total_instances"`
	CompletedInstances int    `json:"completed_instances"`
	FailedInstances    int    `json:"failed_instances"`
	CurrentBatch       int    `json:"current_batch"`
	TotalBatches       int    `json:"total_batches"`
	FailureReason      string `json:"failure_reason,omitempty"`
}

// DeploymentInstance tracks per-instance deployment status.
type DeploymentInstance struct {
	ID           string                   `json:"id"`
	DeploymentID string                   `json:"deployment_id"`
	InstanceID   string                   `json:"instance_id"`
	Status       DeploymentInstanceStatus `json:"status"`
	StartedAt    *time.Time               `json:"started_at,omitempty"`
	CompletedAt  *time.Time               `json:"completed_at,omitempty"`
	LastStatusAt *time.Time               `json:"last_status_at,omitempty"` // Lease renewal timestamp
	ErrorMessage *string                  `json:"error_message,omitempty"`
}

// DeploymentInstanceStatus represents per-instance deployment state.
type DeploymentInstanceStatus string

const (
	DeploymentInstanceStatusPending    DeploymentInstanceStatus = "pending"
	DeploymentInstanceStatusInProgress DeploymentInstanceStatus = "in_progress"
	DeploymentInstanceStatusCompleted  DeploymentInstanceStatus = "completed"
	DeploymentInstanceStatusFailed     DeploymentInstanceStatus = "failed"
	DeploymentInstanceStatusRolledBack DeploymentInstanceStatus = "rolled_back"
)

// User represents a Hub user.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	Role         UserRole   `json:"role"`
	PasswordHash *string    `json:"-"`
	OIDCSubject  *string    `json:"oidc_subject,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// UserRole defines user permission levels.
type UserRole string

const (
	UserRoleAdmin    UserRole = "admin"
	UserRoleOperator UserRole = "operator"
	UserRoleViewer   UserRole = "viewer"
)

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID           string          `json:"id"`
	Timestamp    time.Time       `json:"timestamp"`
	UserID       *string         `json:"user_id,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *string         `json:"resource_id,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
	IPAddress    *string         `json:"ip_address,omitempty"`
}

// AgentSession represents an active agent session.
type AgentSession struct {
	ID         string     `json:"id"`
	InstanceID string     `json:"instance_id"`
	TokenHash  string     `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// UserSession represents an active user session (for JWT refresh tokens).
type UserSession struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	RefreshTokenHash string     `json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	IPAddress        *string    `json:"ip_address,omitempty"`
	UserAgent        *string    `json:"user_agent,omitempty"`
}

// ListUsersOptions contains options for listing users.
type ListUsersOptions struct {
	Role   UserRole
	Limit  int
	Offset int
}

// ListAuditLogsOptions contains options for listing audit logs.
type ListAuditLogsOptions struct {
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	Since        *time.Time
	Until        *time.Time
	Limit        int
	Offset       int
}

// NullString converts a *string to sql.NullString for database operations.
func NullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// NullTime converts a *time.Time to sql.NullTime for database operations.
func NullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// NullInt converts a *int to sql.NullInt64 for database operations.
func NullInt(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}

// StringPtr converts a sql.NullString to *string.
func StringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// TimePtr converts a sql.NullTime to *time.Time.
func TimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}

// IntPtr converts a sql.NullInt64 to *int.
func IntPtr(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	i := int(ni.Int64)
	return &i
}
