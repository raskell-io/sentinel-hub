package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/001_initial_schema.sql
var initialSchema string

//go:embed migrations/002_user_sessions.sql
var userSessionsSchema string

// Store provides database operations for the Hub.
type Store struct {
	db *sql.DB
}

// New creates a new Store instance and initializes the database.
func New(databaseURL string) (*Store, error) {
	// Parse database URL (sqlite://path or just path)
	dbPath := strings.TrimPrefix(databaseURL, "sqlite://")

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db}

	// Run migrations
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate runs database migrations.
func (s *Store) migrate() error {
	// Run all migrations in order
	migrations := []struct {
		name string
		sql  string
	}{
		{"001_initial_schema", initialSchema},
		{"002_user_sessions", userSessionsSchema},
	}

	for _, m := range migrations {
		_, err := s.db.Exec(m.sql)
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", m.name, err)
		}
	}

	return nil
}

// DB returns the underlying database connection for advanced queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// ============================================
// Instance Operations
// ============================================

// CreateInstance creates a new instance.
func (s *Store) CreateInstance(ctx context.Context, inst *Instance) error {
	if inst.ID == "" {
		inst.ID = uuid.New().String()
	}
	inst.CreatedAt = time.Now().UTC()
	inst.UpdatedAt = inst.CreatedAt

	labelsJSON, err := json.Marshal(inst.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	capsJSON, err := json.Marshal(inst.Capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO instances (
			id, name, hostname, agent_version, sentinel_version,
			status, last_seen_at, current_config_id, current_config_version,
			labels, capabilities, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		inst.ID, inst.Name, inst.Hostname, inst.AgentVersion, inst.SentinelVersion,
		inst.Status, NullTime(inst.LastSeenAt), NullString(inst.CurrentConfigID), NullInt(inst.CurrentConfigVersion),
		string(labelsJSON), string(capsJSON), inst.CreatedAt, inst.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert instance: %w", err)
	}

	return nil
}

// GetInstance retrieves an instance by ID.
func (s *Store) GetInstance(ctx context.Context, id string) (*Instance, error) {
	var inst Instance
	var labelsJSON, capsJSON string
	var lastSeenAt sql.NullTime
	var configID sql.NullString
	var configVersion sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, hostname, agent_version, sentinel_version,
			   status, last_seen_at, current_config_id, current_config_version,
			   labels, capabilities, created_at, updated_at
		FROM instances WHERE id = ?
	`, id).Scan(
		&inst.ID, &inst.Name, &inst.Hostname, &inst.AgentVersion, &inst.SentinelVersion,
		&inst.Status, &lastSeenAt, &configID, &configVersion,
		&labelsJSON, &capsJSON, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	inst.LastSeenAt = TimePtr(lastSeenAt)
	inst.CurrentConfigID = StringPtr(configID)
	inst.CurrentConfigVersion = IntPtr(configVersion)

	if labelsJSON != "" {
		if err := json.Unmarshal([]byte(labelsJSON), &inst.Labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}
	}
	if capsJSON != "" {
		if err := json.Unmarshal([]byte(capsJSON), &inst.Capabilities); err != nil {
			return nil, fmt.Errorf("failed to unmarshal capabilities: %w", err)
		}
	}

	return &inst, nil
}

// GetInstanceByName retrieves an instance by name.
func (s *Store) GetInstanceByName(ctx context.Context, name string) (*Instance, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM instances WHERE name = ?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance by name: %w", err)
	}
	return s.GetInstance(ctx, id)
}

// ListInstances retrieves all instances with optional filtering.
func (s *Store) ListInstances(ctx context.Context, opts ListInstancesOptions) ([]Instance, error) {
	query := `
		SELECT id, name, hostname, agent_version, sentinel_version,
			   status, last_seen_at, current_config_id, current_config_version,
			   labels, capabilities, created_at, updated_at
		FROM instances
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}

	query += " ORDER BY name ASC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer rows.Close()

	var instances []Instance
	for rows.Next() {
		var inst Instance
		var labelsJSON, capsJSON string
		var lastSeenAt sql.NullTime
		var configID sql.NullString
		var configVersion sql.NullInt64

		err := rows.Scan(
			&inst.ID, &inst.Name, &inst.Hostname, &inst.AgentVersion, &inst.SentinelVersion,
			&inst.Status, &lastSeenAt, &configID, &configVersion,
			&labelsJSON, &capsJSON, &inst.CreatedAt, &inst.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
		}

		inst.LastSeenAt = TimePtr(lastSeenAt)
		inst.CurrentConfigID = StringPtr(configID)
		inst.CurrentConfigVersion = IntPtr(configVersion)

		if labelsJSON != "" {
			json.Unmarshal([]byte(labelsJSON), &inst.Labels)
		}
		if capsJSON != "" {
			json.Unmarshal([]byte(capsJSON), &inst.Capabilities)
		}

		instances = append(instances, inst)
	}

	return instances, rows.Err()
}

// ListInstancesOptions provides filtering options for ListInstances.
type ListInstancesOptions struct {
	Status InstanceStatus
	Limit  int
	Offset int
}

// UpdateInstance updates an existing instance.
func (s *Store) UpdateInstance(ctx context.Context, inst *Instance) error {
	inst.UpdatedAt = time.Now().UTC()

	labelsJSON, err := json.Marshal(inst.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	capsJSON, err := json.Marshal(inst.Capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE instances SET
			name = ?, hostname = ?, agent_version = ?, sentinel_version = ?,
			status = ?, last_seen_at = ?, current_config_id = ?, current_config_version = ?,
			labels = ?, capabilities = ?, updated_at = ?
		WHERE id = ?
	`,
		inst.Name, inst.Hostname, inst.AgentVersion, inst.SentinelVersion,
		inst.Status, NullTime(inst.LastSeenAt), NullString(inst.CurrentConfigID), NullInt(inst.CurrentConfigVersion),
		string(labelsJSON), string(capsJSON), inst.UpdatedAt, inst.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update instance: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("instance not found")
	}

	return nil
}

// DeleteInstance deletes an instance by ID.
func (s *Store) DeleteInstance(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("instance not found")
	}

	return nil
}

// UpdateInstanceStatus updates just the status and last_seen_at fields.
func (s *Store) UpdateInstanceStatus(ctx context.Context, id string, status InstanceStatus) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE instances SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?
	`, status, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to update instance status: %w", err)
	}
	return nil
}

// ============================================
// Config Operations
// ============================================

// CreateConfig creates a new configuration.
func (s *Store) CreateConfig(ctx context.Context, cfg *Config) error {
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}
	cfg.CurrentVersion = 1
	cfg.CreatedAt = time.Now().UTC()
	cfg.UpdatedAt = cfg.CreatedAt

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO configs (id, name, description, current_version, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		cfg.ID, cfg.Name, NullString(cfg.Description), cfg.CurrentVersion,
		NullString(cfg.CreatedBy), cfg.CreatedAt, cfg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert config: %w", err)
	}

	return nil
}

// GetConfig retrieves a configuration by ID.
func (s *Store) GetConfig(ctx context.Context, id string) (*Config, error) {
	var cfg Config
	var description, createdBy sql.NullString
	var deletedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, current_version, created_by, created_at, updated_at, deleted_at
		FROM configs WHERE id = ? AND deleted_at IS NULL
	`, id).Scan(
		&cfg.ID, &cfg.Name, &description, &cfg.CurrentVersion,
		&createdBy, &cfg.CreatedAt, &cfg.UpdatedAt, &deletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	cfg.Description = StringPtr(description)
	cfg.CreatedBy = StringPtr(createdBy)
	cfg.DeletedAt = TimePtr(deletedAt)

	return &cfg, nil
}

// ListConfigs retrieves all configurations.
func (s *Store) ListConfigs(ctx context.Context, opts ListConfigsOptions) ([]Config, error) {
	query := `
		SELECT id, name, description, current_version, created_by, created_at, updated_at, deleted_at
		FROM configs
		WHERE deleted_at IS NULL
		ORDER BY name ASC
	`

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}
	defer rows.Close()

	var configs []Config
	for rows.Next() {
		var cfg Config
		var description, createdBy sql.NullString
		var deletedAt sql.NullTime

		err := rows.Scan(
			&cfg.ID, &cfg.Name, &description, &cfg.CurrentVersion,
			&createdBy, &cfg.CreatedAt, &cfg.UpdatedAt, &deletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan config: %w", err)
		}

		cfg.Description = StringPtr(description)
		cfg.CreatedBy = StringPtr(createdBy)
		cfg.DeletedAt = TimePtr(deletedAt)

		configs = append(configs, cfg)
	}

	return configs, rows.Err()
}

// ListConfigsOptions provides filtering options for ListConfigs.
type ListConfigsOptions struct {
	Limit  int
	Offset int
}

// UpdateConfig updates a configuration (creates a new version).
func (s *Store) UpdateConfig(ctx context.Context, cfg *Config) error {
	cfg.UpdatedAt = time.Now().UTC()

	result, err := s.db.ExecContext(ctx, `
		UPDATE configs SET
			name = ?, description = ?, current_version = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`,
		cfg.Name, NullString(cfg.Description), cfg.CurrentVersion, cfg.UpdatedAt, cfg.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("config not found")
	}

	return nil
}

// DeleteConfig soft-deletes a configuration.
func (s *Store) DeleteConfig(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE configs SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("config not found")
	}

	return nil
}

// ============================================
// Config Version Operations
// ============================================

// CreateConfigVersion creates a new configuration version.
func (s *Store) CreateConfigVersion(ctx context.Context, ver *ConfigVersion) error {
	if ver.ID == "" {
		ver.ID = uuid.New().String()
	}
	ver.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO config_versions (id, config_id, version, content, content_hash, change_summary, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		ver.ID, ver.ConfigID, ver.Version, ver.Content, ver.ContentHash,
		NullString(ver.ChangeSummary), NullString(ver.CreatedBy), ver.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert config version: %w", err)
	}

	return nil
}

// GetConfigVersion retrieves a specific configuration version.
func (s *Store) GetConfigVersion(ctx context.Context, configID string, version int) (*ConfigVersion, error) {
	var ver ConfigVersion
	var changeSummary, createdBy sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, config_id, version, content, content_hash, change_summary, created_by, created_at
		FROM config_versions WHERE config_id = ? AND version = ?
	`, configID, version).Scan(
		&ver.ID, &ver.ConfigID, &ver.Version, &ver.Content, &ver.ContentHash,
		&changeSummary, &createdBy, &ver.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get config version: %w", err)
	}

	ver.ChangeSummary = StringPtr(changeSummary)
	ver.CreatedBy = StringPtr(createdBy)

	return &ver, nil
}

// GetLatestConfigVersion retrieves the latest version of a configuration.
func (s *Store) GetLatestConfigVersion(ctx context.Context, configID string) (*ConfigVersion, error) {
	var ver ConfigVersion
	var changeSummary, createdBy sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, config_id, version, content, content_hash, change_summary, created_by, created_at
		FROM config_versions WHERE config_id = ? ORDER BY version DESC LIMIT 1
	`, configID).Scan(
		&ver.ID, &ver.ConfigID, &ver.Version, &ver.Content, &ver.ContentHash,
		&changeSummary, &createdBy, &ver.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest config version: %w", err)
	}

	ver.ChangeSummary = StringPtr(changeSummary)
	ver.CreatedBy = StringPtr(createdBy)

	return &ver, nil
}

// ListConfigVersions retrieves all versions of a configuration.
func (s *Store) ListConfigVersions(ctx context.Context, configID string) ([]ConfigVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, config_id, version, content, content_hash, change_summary, created_by, created_at
		FROM config_versions WHERE config_id = ? ORDER BY version DESC
	`, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to list config versions: %w", err)
	}
	defer rows.Close()

	var versions []ConfigVersion
	for rows.Next() {
		var ver ConfigVersion
		var changeSummary, createdBy sql.NullString

		err := rows.Scan(
			&ver.ID, &ver.ConfigID, &ver.Version, &ver.Content, &ver.ContentHash,
			&changeSummary, &createdBy, &ver.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan config version: %w", err)
		}

		ver.ChangeSummary = StringPtr(changeSummary)
		ver.CreatedBy = StringPtr(createdBy)

		versions = append(versions, ver)
	}

	return versions, rows.Err()
}

// ============================================
// Deployment Operations
// ============================================

// CreateDeployment creates a new deployment.
func (s *Store) CreateDeployment(ctx context.Context, dep *Deployment) error {
	if dep.ID == "" {
		dep.ID = uuid.New().String()
	}
	dep.CreatedAt = time.Now().UTC()

	targetsJSON, err := json.Marshal(dep.TargetInstances)
	if err != nil {
		return fmt.Errorf("failed to marshal target instances: %w", err)
	}

	var progressJSON []byte
	if dep.Progress != nil {
		progressJSON, err = json.Marshal(dep.Progress)
		if err != nil {
			return fmt.Errorf("failed to marshal progress: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO deployments (
			id, config_id, config_version, target_instances, strategy, batch_size,
			status, progress, started_at, completed_at, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		dep.ID, dep.ConfigID, dep.ConfigVersion, string(targetsJSON), dep.Strategy, dep.BatchSize,
		dep.Status, string(progressJSON), NullTime(dep.StartedAt), NullTime(dep.CompletedAt),
		NullString(dep.CreatedBy), dep.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert deployment: %w", err)
	}

	return nil
}

// GetDeployment retrieves a deployment by ID.
func (s *Store) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	var dep Deployment
	var targetsJSON, progressJSON string
	var startedAt, completedAt sql.NullTime
	var createdBy sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, config_id, config_version, target_instances, strategy, batch_size,
			   status, progress, started_at, completed_at, created_by, created_at
		FROM deployments WHERE id = ?
	`, id).Scan(
		&dep.ID, &dep.ConfigID, &dep.ConfigVersion, &targetsJSON, &dep.Strategy, &dep.BatchSize,
		&dep.Status, &progressJSON, &startedAt, &completedAt, &createdBy, &dep.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	if err := json.Unmarshal([]byte(targetsJSON), &dep.TargetInstances); err != nil {
		return nil, fmt.Errorf("failed to unmarshal target instances: %w", err)
	}

	if progressJSON != "" {
		var progress DeploymentProgress
		if err := json.Unmarshal([]byte(progressJSON), &progress); err != nil {
			return nil, fmt.Errorf("failed to unmarshal progress: %w", err)
		}
		dep.Progress = &progress
	}

	dep.StartedAt = TimePtr(startedAt)
	dep.CompletedAt = TimePtr(completedAt)
	dep.CreatedBy = StringPtr(createdBy)

	return &dep, nil
}

// ListDeployments retrieves all deployments.
func (s *Store) ListDeployments(ctx context.Context, opts ListDeploymentsOptions) ([]Deployment, error) {
	query := `
		SELECT id, config_id, config_version, target_instances, strategy, batch_size,
			   status, progress, started_at, completed_at, created_by, created_at
		FROM deployments
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var dep Deployment
		var targetsJSON, progressJSON string
		var startedAt, completedAt sql.NullTime
		var createdBy sql.NullString

		err := rows.Scan(
			&dep.ID, &dep.ConfigID, &dep.ConfigVersion, &targetsJSON, &dep.Strategy, &dep.BatchSize,
			&dep.Status, &progressJSON, &startedAt, &completedAt, &createdBy, &dep.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}

		json.Unmarshal([]byte(targetsJSON), &dep.TargetInstances)

		if progressJSON != "" {
			var progress DeploymentProgress
			json.Unmarshal([]byte(progressJSON), &progress)
			dep.Progress = &progress
		}

		dep.StartedAt = TimePtr(startedAt)
		dep.CompletedAt = TimePtr(completedAt)
		dep.CreatedBy = StringPtr(createdBy)

		deployments = append(deployments, dep)
	}

	return deployments, rows.Err()
}

// ListDeploymentsOptions provides filtering options for ListDeployments.
type ListDeploymentsOptions struct {
	Status DeploymentStatus
	Limit  int
	Offset int
}

// UpdateDeployment updates a deployment.
func (s *Store) UpdateDeployment(ctx context.Context, dep *Deployment) error {
	var progressJSON []byte
	var err error
	if dep.Progress != nil {
		progressJSON, err = json.Marshal(dep.Progress)
		if err != nil {
			return fmt.Errorf("failed to marshal progress: %w", err)
		}
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE deployments SET
			status = ?, progress = ?, started_at = ?, completed_at = ?
		WHERE id = ?
	`,
		dep.Status, string(progressJSON), NullTime(dep.StartedAt), NullTime(dep.CompletedAt), dep.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("deployment not found")
	}

	return nil
}

// ============================================
// Deployment Instance Operations
// ============================================

// CreateDeploymentInstance creates a new deployment instance record.
func (s *Store) CreateDeploymentInstance(ctx context.Context, di *DeploymentInstance) error {
	if di.ID == "" {
		di.ID = uuid.New().String()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_instances (id, deployment_id, instance_id, status, started_at, completed_at, last_status_at, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		di.ID, di.DeploymentID, di.InstanceID, di.Status,
		NullTime(di.StartedAt), NullTime(di.CompletedAt), NullTime(di.LastStatusAt), NullString(di.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("failed to create deployment instance: %w", err)
	}

	return nil
}

// GetDeploymentInstance retrieves a deployment instance by deployment and instance ID.
func (s *Store) GetDeploymentInstance(ctx context.Context, deploymentID, instanceID string) (*DeploymentInstance, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, deployment_id, instance_id, status, started_at, completed_at, last_status_at, error_message
		FROM deployment_instances
		WHERE deployment_id = ? AND instance_id = ?
	`, deploymentID, instanceID)

	var di DeploymentInstance
	var startedAt, completedAt, lastStatusAt sql.NullTime
	var errorMessage sql.NullString

	err := row.Scan(
		&di.ID, &di.DeploymentID, &di.InstanceID, &di.Status,
		&startedAt, &completedAt, &lastStatusAt, &errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment instance: %w", err)
	}

	di.StartedAt = TimePtr(startedAt)
	di.CompletedAt = TimePtr(completedAt)
	di.LastStatusAt = TimePtr(lastStatusAt)
	di.ErrorMessage = StringPtr(errorMessage)

	return &di, nil
}

// ListDeploymentInstances retrieves all instances for a deployment.
func (s *Store) ListDeploymentInstances(ctx context.Context, deploymentID string) ([]*DeploymentInstance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, deployment_id, instance_id, status, started_at, completed_at, last_status_at, error_message
		FROM deployment_instances
		WHERE deployment_id = ?
		ORDER BY started_at ASC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment instances: %w", err)
	}
	defer rows.Close()

	var instances []*DeploymentInstance
	for rows.Next() {
		var di DeploymentInstance
		var startedAt, completedAt, lastStatusAt sql.NullTime
		var errorMessage sql.NullString

		err := rows.Scan(
			&di.ID, &di.DeploymentID, &di.InstanceID, &di.Status,
			&startedAt, &completedAt, &lastStatusAt, &errorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment instance: %w", err)
		}

		di.StartedAt = TimePtr(startedAt)
		di.CompletedAt = TimePtr(completedAt)
		di.LastStatusAt = TimePtr(lastStatusAt)
		di.ErrorMessage = StringPtr(errorMessage)

		instances = append(instances, &di)
	}

	return instances, rows.Err()
}

// UpdateDeploymentInstance updates a deployment instance record.
func (s *Store) UpdateDeploymentInstance(ctx context.Context, di *DeploymentInstance) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE deployment_instances SET
			status = ?, started_at = ?, completed_at = ?, last_status_at = ?, error_message = ?
		WHERE deployment_id = ? AND instance_id = ?
	`,
		di.Status, NullTime(di.StartedAt), NullTime(di.CompletedAt), NullTime(di.LastStatusAt), NullString(di.ErrorMessage),
		di.DeploymentID, di.InstanceID,
	)
	if err != nil {
		return fmt.Errorf("failed to update deployment instance: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("deployment instance not found")
	}

	return nil
}

// UpsertDeploymentInstance creates or updates a deployment instance record.
func (s *Store) UpsertDeploymentInstance(ctx context.Context, di *DeploymentInstance) error {
	if di.ID == "" {
		di.ID = uuid.New().String()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_instances (id, deployment_id, instance_id, status, started_at, completed_at, last_status_at, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (deployment_id, instance_id) DO UPDATE SET
			status = excluded.status,
			started_at = COALESCE(deployment_instances.started_at, excluded.started_at),
			completed_at = excluded.completed_at,
			last_status_at = excluded.last_status_at,
			error_message = excluded.error_message
	`,
		di.ID, di.DeploymentID, di.InstanceID, di.Status,
		NullTime(di.StartedAt), NullTime(di.CompletedAt), NullTime(di.LastStatusAt), NullString(di.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert deployment instance: %w", err)
	}

	return nil
}

// DeleteDeploymentInstances deletes all instance records for a deployment.
func (s *Store) DeleteDeploymentInstances(ctx context.Context, deploymentID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM deployment_instances WHERE deployment_id = ?
	`, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to delete deployment instances: %w", err)
	}

	return nil
}

// ============================================
// User Operations
// ============================================

// CreateUser creates a new user.
func (s *Store) CreateUser(ctx context.Context, user *User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	user.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, name, role, password_hash, oidc_subject, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		user.ID, user.Email, user.Name, user.Role,
		NullString(user.PasswordHash), NullString(user.OIDCSubject),
		user.CreatedAt, NullTime(user.LastLoginAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("user with this email already exists")
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID.
func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var user User
	var passwordHash, oidcSubject sql.NullString
	var lastLoginAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, name, role, password_hash, oidc_subject, created_at, last_login_at
		FROM users WHERE id = ?
	`, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role,
		&passwordHash, &oidcSubject, &user.CreatedAt, &lastLoginAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	user.PasswordHash = StringPtr(passwordHash)
	user.OIDCSubject = StringPtr(oidcSubject)
	user.LastLoginAt = TimePtr(lastLoginAt)

	return &user, nil
}

// GetUserByEmail retrieves a user by email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	var passwordHash, oidcSubject sql.NullString
	var lastLoginAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, name, role, password_hash, oidc_subject, created_at, last_login_at
		FROM users WHERE email = ?
	`, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role,
		&passwordHash, &oidcSubject, &user.CreatedAt, &lastLoginAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	user.PasswordHash = StringPtr(passwordHash)
	user.OIDCSubject = StringPtr(oidcSubject)
	user.LastLoginAt = TimePtr(lastLoginAt)

	return &user, nil
}

// ListUsers retrieves all users with optional filtering.
func (s *Store) ListUsers(ctx context.Context, opts ListUsersOptions) ([]User, error) {
	query := `
		SELECT id, email, name, role, password_hash, oidc_subject, created_at, last_login_at
		FROM users
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.Role != "" {
		query += " AND role = ?"
		args = append(args, opts.Role)
	}

	query += " ORDER BY email ASC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var passwordHash, oidcSubject sql.NullString
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&user.ID, &user.Email, &user.Name, &user.Role,
			&passwordHash, &oidcSubject, &user.CreatedAt, &lastLoginAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		user.PasswordHash = StringPtr(passwordHash)
		user.OIDCSubject = StringPtr(oidcSubject)
		user.LastLoginAt = TimePtr(lastLoginAt)

		users = append(users, user)
	}

	return users, rows.Err()
}

// UpdateUser updates an existing user.
func (s *Store) UpdateUser(ctx context.Context, user *User) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET
			email = ?, name = ?, role = ?, password_hash = ?, oidc_subject = ?
		WHERE id = ?
	`,
		user.Email, user.Name, user.Role,
		NullString(user.PasswordHash), NullString(user.OIDCSubject),
		user.ID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("user with this email already exists")
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// DeleteUser deletes a user by ID.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateUserLastLogin updates the last_login_at timestamp for a user.
func (s *Store) UpdateUserLastLogin(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET last_login_at = ? WHERE id = ?
	`, now, id)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}
	return nil
}

// CountUsers returns the total number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return count, nil
}

// ============================================
// User Session Operations
// ============================================

// CreateUserSession creates a new user session.
func (s *Store) CreateUserSession(ctx context.Context, session *UserSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}
	session.CreatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, refresh_token_hash, created_at, expires_at, revoked_at, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		session.ID, session.UserID, session.RefreshTokenHash,
		session.CreatedAt, session.ExpiresAt, NullTime(session.RevokedAt),
		NullString(session.IPAddress), NullString(session.UserAgent),
	)
	if err != nil {
		return fmt.Errorf("failed to create user session: %w", err)
	}

	return nil
}

// GetUserSessionByTokenHash retrieves a session by refresh token hash.
func (s *Store) GetUserSessionByTokenHash(ctx context.Context, tokenHash string) (*UserSession, error) {
	var session UserSession
	var revokedAt sql.NullTime
	var ipAddress, userAgent sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, refresh_token_hash, created_at, expires_at, revoked_at, ip_address, user_agent
		FROM user_sessions WHERE refresh_token_hash = ?
	`, tokenHash).Scan(
		&session.ID, &session.UserID, &session.RefreshTokenHash,
		&session.CreatedAt, &session.ExpiresAt, &revokedAt,
		&ipAddress, &userAgent,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user session: %w", err)
	}

	session.RevokedAt = TimePtr(revokedAt)
	session.IPAddress = StringPtr(ipAddress)
	session.UserAgent = StringPtr(userAgent)

	return &session, nil
}

// RevokeUserSession marks a session as revoked.
func (s *Store) RevokeUserSession(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_sessions SET revoked_at = ? WHERE id = ?
	`, now, id)
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	return nil
}

// RevokeAllUserSessions revokes all sessions for a user.
func (s *Store) RevokeAllUserSessions(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL
	`, now, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke all sessions: %w", err)
	}
	return nil
}

// CleanupExpiredSessions deletes sessions that have expired.
func (s *Store) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM user_sessions WHERE expires_at < ?
	`, now)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// ============================================
// Audit Log Operations
// ============================================

// CreateAuditLog creates a new audit log entry.
func (s *Store) CreateAuditLog(ctx context.Context, log *AuditLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	log.Timestamp = time.Now().UTC()

	var detailsStr string
	if log.Details != nil {
		detailsStr = string(log.Details)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, timestamp, user_id, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		log.ID, log.Timestamp, NullString(log.UserID), log.Action,
		log.ResourceType, NullString(log.ResourceID), detailsStr, NullString(log.IPAddress),
	)
	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

// ListAuditLogs retrieves audit logs with optional filtering.
func (s *Store) ListAuditLogs(ctx context.Context, opts ListAuditLogsOptions) ([]AuditLog, error) {
	query := `
		SELECT id, timestamp, user_id, action, resource_type, resource_id, details, ip_address
		FROM audit_logs
		WHERE 1=1
	`
	args := []interface{}{}

	if opts.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, opts.UserID)
	}
	if opts.Action != "" {
		query += " AND action = ?"
		args = append(args, opts.Action)
	}
	if opts.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, opts.ResourceType)
	}
	if opts.ResourceID != "" {
		query += " AND resource_id = ?"
		args = append(args, opts.ResourceID)
	}
	if opts.Since != nil {
		query += " AND timestamp >= ?"
		args = append(args, *opts.Since)
	}
	if opts.Until != nil {
		query += " AND timestamp <= ?"
		args = append(args, *opts.Until)
	}

	query += " ORDER BY timestamp DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var userID, resourceID, ipAddress sql.NullString
		var details string

		err := rows.Scan(
			&log.ID, &log.Timestamp, &userID, &log.Action,
			&log.ResourceType, &resourceID, &details, &ipAddress,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		log.UserID = StringPtr(userID)
		log.ResourceID = StringPtr(resourceID)
		log.IPAddress = StringPtr(ipAddress)
		if details != "" {
			log.Details = json.RawMessage(details)
		}

		logs = append(logs, log)
	}

	return logs, rows.Err()
}
