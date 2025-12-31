package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *Store {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "store-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	s, err := New(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create test store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
		os.Remove(tmpFile.Name())
		os.Remove(tmpFile.Name() + "-shm")
		os.Remove(tmpFile.Name() + "-wal")
	})
	return s
}

// createTestConfigWithVersion creates a config and version for tests that require FK references.
func createTestConfigWithVersion(t *testing.T, s *Store, configID string, version int) {
	t.Helper()
	ctx := context.Background()

	cfg := &Config{
		ID:   configID,
		Name: configID + "-name",
	}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	cv := &ConfigVersion{
		ConfigID: configID,
		Version:  version,
		Content:  "test content",
	}
	if err := s.CreateConfigVersion(ctx, cv); err != nil {
		t.Fatalf("failed to create test config version: %v", err)
	}
}

// ============================================
// Store Tests
// ============================================

func TestNew(t *testing.T) {
	s := setupTestStore(t)

	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.db == nil {
		t.Error("db is nil")
	}
}

func TestNew_WithSqlitePrefix(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "store-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	s, err := New("sqlite://" + tmpFile.Name())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer s.Close()

	if s == nil {
		t.Fatal("New returned nil")
	}
}

func TestStore_Close(t *testing.T) {
	s := setupTestStore(t)
	err := s.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestStore_DB(t *testing.T) {
	s := setupTestStore(t)
	db := s.DB()
	if db == nil {
		t.Error("DB() returned nil")
	}
	if db != s.db {
		t.Error("DB() returned different instance")
	}
}

// ============================================
// Instance Tests
// ============================================

func TestStore_CreateInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{
		Name:            "test-instance",
		Hostname:        "test.local",
		AgentVersion:    "1.0.0",
		SentinelVersion: "2.0.0",
		Status:          InstanceStatusOnline,
		Labels:          map[string]string{"env": "test"},
		Capabilities:    []string{"config-reload"},
	}

	err := s.CreateInstance(ctx, inst)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	if inst.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if inst.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if inst.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestStore_CreateInstance_WithID(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{
		ID:     "custom-id",
		Name:   "test",
		Status: InstanceStatusOnline,
	}

	err := s.CreateInstance(ctx, inst)
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	if inst.ID != "custom-id" {
		t.Errorf("ID = %q, want %q", inst.ID, "custom-id")
	}
}

func TestStore_GetInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create config first due to foreign key constraint
	cfg := &Config{
		ID:   "cfg-1",
		Name: "test-config",
	}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	now := time.Now().UTC()
	configID := "cfg-1"
	configVersion := 1

	inst := &Instance{
		Name:                 "test-instance",
		Hostname:             "test.local",
		AgentVersion:         "1.0.0",
		SentinelVersion:      "2.0.0",
		Status:               InstanceStatusOnline,
		LastSeenAt:           &now,
		CurrentConfigID:      &configID,
		CurrentConfigVersion: &configVersion,
		Labels:               map[string]string{"env": "test"},
		Capabilities:         []string{"config-reload"},
	}

	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	retrieved, err := s.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetInstance returned nil")
	}
	if retrieved.ID != inst.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, inst.ID)
	}
	if retrieved.Name != "test-instance" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "test-instance")
	}
	if retrieved.Hostname != "test.local" {
		t.Errorf("Hostname = %q, want %q", retrieved.Hostname, "test.local")
	}
	if retrieved.Status != InstanceStatusOnline {
		t.Errorf("Status = %q, want %q", retrieved.Status, InstanceStatusOnline)
	}
	if retrieved.Labels["env"] != "test" {
		t.Errorf("Labels[env] = %q, want %q", retrieved.Labels["env"], "test")
	}
	if len(retrieved.Capabilities) != 1 || retrieved.Capabilities[0] != "config-reload" {
		t.Errorf("Capabilities = %v, want [config-reload]", retrieved.Capabilities)
	}
}

func TestStore_GetInstance_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst, err := s.GetInstance(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}
	if inst != nil {
		t.Error("expected nil for nonexistent instance")
	}
}

func TestStore_GetInstanceByName(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{
		Name:   "unique-name",
		Status: InstanceStatusOnline,
	}

	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	retrieved, err := s.GetInstanceByName(ctx, "unique-name")
	if err != nil {
		t.Fatalf("GetInstanceByName failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetInstanceByName returned nil")
	}
	if retrieved.ID != inst.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, inst.ID)
	}
}

func TestStore_GetInstanceByName_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst, err := s.GetInstanceByName(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetInstanceByName failed: %v", err)
	}
	if inst != nil {
		t.Error("expected nil for nonexistent instance")
	}
}

func TestStore_ListInstances(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create multiple instances
	for _, name := range []string{"alpha", "beta", "gamma"} {
		inst := &Instance{Name: name, Status: InstanceStatusOnline}
		if err := s.CreateInstance(ctx, inst); err != nil {
			t.Fatalf("CreateInstance failed: %v", err)
		}
	}

	instances, err := s.ListInstances(ctx, ListInstancesOptions{})
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}

	if len(instances) != 3 {
		t.Errorf("count = %d, want 3", len(instances))
	}

	// Should be sorted by name
	if instances[0].Name != "alpha" {
		t.Errorf("first instance = %q, want %q", instances[0].Name, "alpha")
	}
}

func TestStore_ListInstances_WithStatusFilter(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateInstance(ctx, &Instance{Name: "online-1", Status: InstanceStatusOnline})
	s.CreateInstance(ctx, &Instance{Name: "online-2", Status: InstanceStatusOnline})
	s.CreateInstance(ctx, &Instance{Name: "offline-1", Status: InstanceStatusOffline})

	instances, err := s.ListInstances(ctx, ListInstancesOptions{Status: InstanceStatusOnline})
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}

	if len(instances) != 2 {
		t.Errorf("count = %d, want 2", len(instances))
	}
}

func TestStore_ListInstances_WithPagination(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.CreateInstance(ctx, &Instance{Name: string(rune('a' + i)), Status: InstanceStatusOnline})
	}

	// Get first 2
	instances, err := s.ListInstances(ctx, ListInstancesOptions{Limit: 2})
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 2 {
		t.Errorf("count = %d, want 2", len(instances))
	}

	// Get next 2
	instances, err = s.ListInstances(ctx, ListInstancesOptions{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListInstances failed: %v", err)
	}
	if len(instances) != 2 {
		t.Errorf("count = %d, want 2", len(instances))
	}
}

func TestStore_UpdateInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{
		Name:   "test",
		Status: InstanceStatusOnline,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	originalUpdatedAt := inst.UpdatedAt

	// Update
	inst.Name = "updated-name"
	inst.Status = InstanceStatusOffline
	inst.Labels = map[string]string{"new": "label"}

	time.Sleep(10 * time.Millisecond) // Ensure time difference

	if err := s.UpdateInstance(ctx, inst); err != nil {
		t.Fatalf("UpdateInstance failed: %v", err)
	}

	if inst.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("UpdatedAt should be updated")
	}

	// Verify
	retrieved, _ := s.GetInstance(ctx, inst.ID)
	if retrieved.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "updated-name")
	}
	if retrieved.Status != InstanceStatusOffline {
		t.Errorf("Status = %q, want %q", retrieved.Status, InstanceStatusOffline)
	}
}

func TestStore_UpdateInstance_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{ID: "nonexistent", Name: "test"}
	err := s.UpdateInstance(ctx, inst)
	if err == nil {
		t.Error("expected error for nonexistent instance")
	}
}

func TestStore_DeleteInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{Name: "to-delete", Status: InstanceStatusOnline}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	err := s.DeleteInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("DeleteInstance failed: %v", err)
	}

	// Verify deleted
	retrieved, _ := s.GetInstance(ctx, inst.ID)
	if retrieved != nil {
		t.Error("instance should be deleted")
	}
}

func TestStore_DeleteInstance_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	err := s.DeleteInstance(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent instance")
	}
}

func TestStore_UpdateInstanceStatus(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	inst := &Instance{Name: "test", Status: InstanceStatusOnline}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	err := s.UpdateInstanceStatus(ctx, inst.ID, InstanceStatusOffline)
	if err != nil {
		t.Fatalf("UpdateInstanceStatus failed: %v", err)
	}

	retrieved, _ := s.GetInstance(ctx, inst.ID)
	if retrieved.Status != InstanceStatusOffline {
		t.Errorf("Status = %q, want %q", retrieved.Status, InstanceStatusOffline)
	}
	if retrieved.LastSeenAt == nil {
		t.Error("LastSeenAt should be set")
	}
}

// ============================================
// Config Tests
// ============================================

func TestStore_CreateConfig(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	description := "Test config"
	createdBy := "admin"

	cfg := &Config{
		Name:        "test-config",
		Description: &description,
		CreatedBy:   &createdBy,
	}

	err := s.CreateConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	if cfg.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if cfg.CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want 1", cfg.CurrentVersion)
	}
	if cfg.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestStore_GetConfig(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	description := "Test config"
	cfg := &Config{
		Name:        "test-config",
		Description: &description,
	}

	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	retrieved, err := s.GetConfig(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetConfig returned nil")
	}
	if retrieved.Name != "test-config" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "test-config")
	}
	if retrieved.Description == nil || *retrieved.Description != "Test config" {
		t.Errorf("Description = %v, want %q", retrieved.Description, "Test config")
	}
}

func TestStore_GetConfig_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg, err := s.GetConfig(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil for nonexistent config")
	}
}

func TestStore_ListConfigs(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"config-a", "config-b", "config-c"} {
		s.CreateConfig(ctx, &Config{Name: name})
	}

	configs, err := s.ListConfigs(ctx, ListConfigsOptions{})
	if err != nil {
		t.Fatalf("ListConfigs failed: %v", err)
	}

	if len(configs) != 3 {
		t.Errorf("count = %d, want 3", len(configs))
	}

	// Should be sorted by name
	if configs[0].Name != "config-a" {
		t.Errorf("first config = %q, want %q", configs[0].Name, "config-a")
	}
}

func TestStore_ListConfigs_WithPagination(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.CreateConfig(ctx, &Config{Name: string(rune('a' + i))})
	}

	configs, err := s.ListConfigs(ctx, ListConfigsOptions{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListConfigs failed: %v", err)
	}

	if len(configs) != 2 {
		t.Errorf("count = %d, want 2", len(configs))
	}
}

func TestStore_UpdateConfig(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "original"}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	cfg.Name = "updated"
	cfg.CurrentVersion = 2
	newDesc := "Updated description"
	cfg.Description = &newDesc

	if err := s.UpdateConfig(ctx, cfg); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	retrieved, _ := s.GetConfig(ctx, cfg.ID)
	if retrieved.Name != "updated" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "updated")
	}
	if retrieved.CurrentVersion != 2 {
		t.Errorf("CurrentVersion = %d, want 2", retrieved.CurrentVersion)
	}
}

func TestStore_UpdateConfig_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{ID: "nonexistent", Name: "test"}
	err := s.UpdateConfig(ctx, cfg)
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestStore_DeleteConfig(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "to-delete"}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	err := s.DeleteConfig(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("DeleteConfig failed: %v", err)
	}

	// Soft delete - should not be found via GetConfig
	retrieved, _ := s.GetConfig(ctx, cfg.ID)
	if retrieved != nil {
		t.Error("config should be soft-deleted")
	}
}

func TestStore_DeleteConfig_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	err := s.DeleteConfig(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestStore_DeleteConfig_AlreadyDeleted(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "to-delete"}
	s.CreateConfig(ctx, cfg)
	s.DeleteConfig(ctx, cfg.ID)

	// Try to delete again
	err := s.DeleteConfig(ctx, cfg.ID)
	if err == nil {
		t.Error("expected error for already deleted config")
	}
}

// ============================================
// ConfigVersion Tests
// ============================================

func TestStore_CreateConfigVersion(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	changeSummary := "Initial version"
	createdBy := "admin"

	ver := &ConfigVersion{
		ConfigID:      cfg.ID,
		Version:       1,
		Content:       "server {}",
		ContentHash:   "abc123",
		ChangeSummary: &changeSummary,
		CreatedBy:     &createdBy,
	}

	err := s.CreateConfigVersion(ctx, ver)
	if err != nil {
		t.Fatalf("CreateConfigVersion failed: %v", err)
	}

	if ver.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if ver.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestStore_GetConfigVersion(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	ver := &ConfigVersion{
		ConfigID:    cfg.ID,
		Version:     1,
		Content:     "server {}",
		ContentHash: "abc123",
	}
	s.CreateConfigVersion(ctx, ver)

	retrieved, err := s.GetConfigVersion(ctx, cfg.ID, 1)
	if err != nil {
		t.Fatalf("GetConfigVersion failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetConfigVersion returned nil")
	}
	if retrieved.Version != 1 {
		t.Errorf("Version = %d, want 1", retrieved.Version)
	}
	if retrieved.Content != "server {}" {
		t.Errorf("Content = %q, want %q", retrieved.Content, "server {}")
	}
}

func TestStore_GetConfigVersion_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	ver, err := s.GetConfigVersion(ctx, "nonexistent", 1)
	if err != nil {
		t.Fatalf("GetConfigVersion failed: %v", err)
	}
	if ver != nil {
		t.Error("expected nil for nonexistent version")
	}
}

func TestStore_GetLatestConfigVersion(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	// Create multiple versions
	for i := 1; i <= 3; i++ {
		s.CreateConfigVersion(ctx, &ConfigVersion{
			ConfigID:    cfg.ID,
			Version:     i,
			Content:     "content v" + string(rune('0'+i)),
			ContentHash: "hash" + string(rune('0'+i)),
		})
	}

	latest, err := s.GetLatestConfigVersion(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("GetLatestConfigVersion failed: %v", err)
	}

	if latest == nil {
		t.Fatal("GetLatestConfigVersion returned nil")
	}
	if latest.Version != 3 {
		t.Errorf("Version = %d, want 3", latest.Version)
	}
}

func TestStore_GetLatestConfigVersion_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	ver, err := s.GetLatestConfigVersion(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetLatestConfigVersion failed: %v", err)
	}
	if ver != nil {
		t.Error("expected nil for nonexistent config")
	}
}

func TestStore_ListConfigVersions(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg := &Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	for i := 1; i <= 3; i++ {
		s.CreateConfigVersion(ctx, &ConfigVersion{
			ConfigID:    cfg.ID,
			Version:     i,
			Content:     "content",
			ContentHash: "hash",
		})
	}

	versions, err := s.ListConfigVersions(ctx, cfg.ID)
	if err != nil {
		t.Fatalf("ListConfigVersions failed: %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("count = %d, want 3", len(versions))
	}

	// Should be sorted by version DESC
	if versions[0].Version != 3 {
		t.Errorf("first version = %d, want 3", versions[0].Version)
	}
}

// ============================================
// Deployment Tests
// ============================================

func TestStore_CreateDeployment(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	createdBy := "admin"
	dep := &Deployment{
		ConfigID:        "cfg-1",
		ConfigVersion:   1,
		TargetInstances: []string{"inst-1", "inst-2"},
		Strategy:        DeploymentStrategyRolling,
		BatchSize:       2,
		Status:          DeploymentStatusPending,
		CreatedBy:       &createdBy,
	}

	err := s.CreateDeployment(ctx, dep)
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	if dep.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if dep.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestStore_CreateDeployment_WithProgress(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	dep := &Deployment{
		ConfigID:        "cfg-1",
		ConfigVersion:   1,
		TargetInstances: []string{"inst-1"},
		Strategy:        DeploymentStrategyRolling,
		BatchSize:       1,
		Status:          DeploymentStatusInProgress,
		Progress: &DeploymentProgress{
			TotalInstances:     3,
			CompletedInstances: 1,
			FailedInstances:    0,
			CurrentBatch:       1,
			TotalBatches:       3,
		},
	}

	err := s.CreateDeployment(ctx, dep)
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	retrieved, _ := s.GetDeployment(ctx, dep.ID)
	if retrieved.Progress == nil {
		t.Fatal("Progress should be set")
	}
	if retrieved.Progress.TotalInstances != 3 {
		t.Errorf("TotalInstances = %d, want 3", retrieved.Progress.TotalInstances)
	}
}

func TestStore_GetDeployment(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	now := time.Now().UTC()
	dep := &Deployment{
		ConfigID:        "cfg-1",
		ConfigVersion:   1,
		TargetInstances: []string{"inst-1", "inst-2"},
		Strategy:        DeploymentStrategyAllAtOnce,
		BatchSize:       2,
		Status:          DeploymentStatusCompleted,
		StartedAt:       &now,
		CompletedAt:     &now,
	}
	s.CreateDeployment(ctx, dep)

	retrieved, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetDeployment returned nil")
	}
	if retrieved.ConfigID != "cfg-1" {
		t.Errorf("ConfigID = %q, want %q", retrieved.ConfigID, "cfg-1")
	}
	if len(retrieved.TargetInstances) != 2 {
		t.Errorf("TargetInstances count = %d, want 2", len(retrieved.TargetInstances))
	}
	if retrieved.Strategy != DeploymentStrategyAllAtOnce {
		t.Errorf("Strategy = %q, want %q", retrieved.Strategy, DeploymentStrategyAllAtOnce)
	}
	if retrieved.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestStore_GetDeployment_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	dep, err := s.GetDeployment(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if dep != nil {
		t.Error("expected nil for nonexistent deployment")
	}
}

func TestStore_ListDeployments(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	for i := 0; i < 3; i++ {
		s.CreateDeployment(ctx, &Deployment{
			ConfigID:        "cfg-1",
			ConfigVersion:   1,
			TargetInstances: []string{"inst-1"},
			Strategy:        DeploymentStrategyRolling,
			BatchSize:       1,
			Status:          DeploymentStatusPending,
		})
	}

	deployments, err := s.ListDeployments(ctx, ListDeploymentsOptions{})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}

	if len(deployments) != 3 {
		t.Errorf("count = %d, want 3", len(deployments))
	}
}

func TestStore_ListDeployments_WithStatusFilter(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	s.CreateDeployment(ctx, &Deployment{
		ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{"inst-1"},
		Strategy: DeploymentStrategyRolling, BatchSize: 1, Status: DeploymentStatusPending,
	})
	s.CreateDeployment(ctx, &Deployment{
		ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{"inst-1"},
		Strategy: DeploymentStrategyRolling, BatchSize: 1, Status: DeploymentStatusCompleted,
	})
	s.CreateDeployment(ctx, &Deployment{
		ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{"inst-1"},
		Strategy: DeploymentStrategyRolling, BatchSize: 1, Status: DeploymentStatusCompleted,
	})

	deployments, err := s.ListDeployments(ctx, ListDeploymentsOptions{Status: DeploymentStatusCompleted})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}

	if len(deployments) != 2 {
		t.Errorf("count = %d, want 2", len(deployments))
	}
}

func TestStore_ListDeployments_WithPagination(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	for i := 0; i < 5; i++ {
		s.CreateDeployment(ctx, &Deployment{
			ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{"inst-1"},
			Strategy: DeploymentStrategyRolling, BatchSize: 1, Status: DeploymentStatusPending,
		})
	}

	deployments, err := s.ListDeployments(ctx, ListDeploymentsOptions{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}

	if len(deployments) != 2 {
		t.Errorf("count = %d, want 2", len(deployments))
	}
}

func TestStore_UpdateDeployment(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	createTestConfigWithVersion(t, s, "cfg-1", 1)

	dep := &Deployment{
		ConfigID:        "cfg-1",
		ConfigVersion:   1,
		TargetInstances: []string{"inst-1"},
		Strategy:        DeploymentStrategyRolling,
		BatchSize:       1,
		Status:          DeploymentStatusPending,
	}
	s.CreateDeployment(ctx, dep)

	now := time.Now().UTC()
	dep.Status = DeploymentStatusCompleted
	dep.CompletedAt = &now
	dep.Progress = &DeploymentProgress{
		TotalInstances:     1,
		CompletedInstances: 1,
	}

	err := s.UpdateDeployment(ctx, dep)
	if err != nil {
		t.Fatalf("UpdateDeployment failed: %v", err)
	}

	retrieved, _ := s.GetDeployment(ctx, dep.ID)
	if retrieved.Status != DeploymentStatusCompleted {
		t.Errorf("Status = %q, want %q", retrieved.Status, DeploymentStatusCompleted)
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if retrieved.Progress == nil || retrieved.Progress.CompletedInstances != 1 {
		t.Error("Progress should be updated")
	}
}

func TestStore_UpdateDeployment_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	dep := &Deployment{ID: "nonexistent", Status: DeploymentStatusCompleted}
	err := s.UpdateDeployment(ctx, dep)
	if err == nil {
		t.Error("expected error for nonexistent deployment")
	}
}

// ============================================
// Helper Function Tests
// ============================================

func TestNullString(t *testing.T) {
	// nil input
	result := NullString(nil)
	if result.Valid {
		t.Error("NullString(nil) should not be valid")
	}

	// non-nil input
	s := "test"
	result = NullString(&s)
	if !result.Valid {
		t.Error("NullString(&s) should be valid")
	}
	if result.String != "test" {
		t.Errorf("String = %q, want %q", result.String, "test")
	}
}

func TestNullTime(t *testing.T) {
	// nil input
	result := NullTime(nil)
	if result.Valid {
		t.Error("NullTime(nil) should not be valid")
	}

	// non-nil input
	now := time.Now()
	result = NullTime(&now)
	if !result.Valid {
		t.Error("NullTime(&now) should be valid")
	}
	if !result.Time.Equal(now) {
		t.Error("Time should match")
	}
}

func TestNullInt(t *testing.T) {
	// nil input
	result := NullInt(nil)
	if result.Valid {
		t.Error("NullInt(nil) should not be valid")
	}

	// non-nil input
	i := 42
	result = NullInt(&i)
	if !result.Valid {
		t.Error("NullInt(&i) should be valid")
	}
	if result.Int64 != 42 {
		t.Errorf("Int64 = %d, want 42", result.Int64)
	}
}

func TestStringPtr(t *testing.T) {
	// invalid input
	result := StringPtr(sql.NullString{Valid: false})
	if result != nil {
		t.Error("StringPtr(invalid) should return nil")
	}

	// valid input
	result = StringPtr(sql.NullString{String: "test", Valid: true})
	if result == nil {
		t.Fatal("StringPtr(valid) should not return nil")
	}
	if *result != "test" {
		t.Errorf("result = %q, want %q", *result, "test")
	}
}

func TestTimePtr(t *testing.T) {
	// invalid input
	result := TimePtr(sql.NullTime{Valid: false})
	if result != nil {
		t.Error("TimePtr(invalid) should return nil")
	}

	// valid input
	now := time.Now()
	result = TimePtr(sql.NullTime{Time: now, Valid: true})
	if result == nil {
		t.Fatal("TimePtr(valid) should not return nil")
	}
	if !result.Equal(now) {
		t.Error("result should match input time")
	}
}

func TestIntPtr(t *testing.T) {
	// invalid input
	result := IntPtr(sql.NullInt64{Valid: false})
	if result != nil {
		t.Error("IntPtr(invalid) should return nil")
	}

	// valid input
	result = IntPtr(sql.NullInt64{Int64: 42, Valid: true})
	if result == nil {
		t.Fatal("IntPtr(valid) should not return nil")
	}
	if *result != 42 {
		t.Errorf("result = %d, want 42", *result)
	}
}

// ============================================
// Deployment Instance Tests
// ============================================

func TestStore_CreateDeploymentInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create required parent objects
	createTestConfigWithVersion(t, s, "cfg-1", 1)
	inst := &Instance{Name: "inst-1", Hostname: "host1", AgentVersion: "1.0", SentinelVersion: "1.0"}
	s.CreateInstance(ctx, inst)
	dep := &Deployment{ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{inst.ID}}
	s.CreateDeployment(ctx, dep)

	// Create deployment instance
	now := time.Now().UTC()
	di := &DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst.ID,
		Status:       DeploymentInstanceStatusPending,
		StartedAt:    &now,
	}

	err := s.CreateDeploymentInstance(ctx, di)
	if err != nil {
		t.Fatalf("CreateDeploymentInstance failed: %v", err)
	}

	if di.ID == "" {
		t.Error("ID should be generated")
	}
}

func TestStore_GetDeploymentInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create required parent objects
	createTestConfigWithVersion(t, s, "cfg-1", 1)
	inst := &Instance{Name: "inst-1", Hostname: "host1", AgentVersion: "1.0", SentinelVersion: "1.0"}
	s.CreateInstance(ctx, inst)
	dep := &Deployment{ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{inst.ID}}
	s.CreateDeployment(ctx, dep)

	// Create deployment instance
	now := time.Now().UTC()
	errorMsg := "test error"
	di := &DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst.ID,
		Status:       DeploymentInstanceStatusFailed,
		StartedAt:    &now,
		CompletedAt:  &now,
		ErrorMessage: &errorMsg,
	}
	s.CreateDeploymentInstance(ctx, di)

	// Retrieve it
	retrieved, err := s.GetDeploymentInstance(ctx, dep.ID, inst.ID)
	if err != nil {
		t.Fatalf("GetDeploymentInstance failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetDeploymentInstance returned nil")
	}

	if retrieved.Status != DeploymentInstanceStatusFailed {
		t.Errorf("Status = %q, want %q", retrieved.Status, DeploymentInstanceStatusFailed)
	}
	if retrieved.ErrorMessage == nil || *retrieved.ErrorMessage != "test error" {
		t.Error("ErrorMessage not preserved")
	}
}

func TestStore_GetDeploymentInstance_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	di, err := s.GetDeploymentInstance(ctx, "nonexistent", "nonexistent")
	if err != nil {
		t.Fatalf("GetDeploymentInstance failed: %v", err)
	}
	if di != nil {
		t.Error("expected nil for nonexistent deployment instance")
	}
}

func TestStore_ListDeploymentInstances(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create required parent objects
	createTestConfigWithVersion(t, s, "cfg-1", 1)
	instances := make([]*Instance, 3)
	for i := 0; i < 3; i++ {
		inst := &Instance{Name: "inst-" + string(rune('a'+i)), Hostname: "host", AgentVersion: "1.0", SentinelVersion: "1.0"}
		s.CreateInstance(ctx, inst)
		instances[i] = inst
	}

	instanceIDs := make([]string, 3)
	for i, inst := range instances {
		instanceIDs[i] = inst.ID
	}
	dep := &Deployment{ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: instanceIDs}
	s.CreateDeployment(ctx, dep)

	// Create deployment instances
	now := time.Now().UTC()
	for _, inst := range instances {
		di := &DeploymentInstance{
			DeploymentID: dep.ID,
			InstanceID:   inst.ID,
			Status:       DeploymentInstanceStatusCompleted,
			StartedAt:    &now,
			CompletedAt:  &now,
		}
		s.CreateDeploymentInstance(ctx, di)
	}

	// List them
	list, err := s.ListDeploymentInstances(ctx, dep.ID)
	if err != nil {
		t.Fatalf("ListDeploymentInstances failed: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("len(list) = %d, want 3", len(list))
	}
}

func TestStore_UpdateDeploymentInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create required parent objects
	createTestConfigWithVersion(t, s, "cfg-1", 1)
	inst := &Instance{Name: "inst-1", Hostname: "host1", AgentVersion: "1.0", SentinelVersion: "1.0"}
	s.CreateInstance(ctx, inst)
	dep := &Deployment{ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{inst.ID}}
	s.CreateDeployment(ctx, dep)

	// Create deployment instance
	now := time.Now().UTC()
	di := &DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst.ID,
		Status:       DeploymentInstanceStatusPending,
		StartedAt:    &now,
	}
	s.CreateDeploymentInstance(ctx, di)

	// Update it
	completedAt := time.Now().UTC()
	di.Status = DeploymentInstanceStatusCompleted
	di.CompletedAt = &completedAt

	err := s.UpdateDeploymentInstance(ctx, di)
	if err != nil {
		t.Fatalf("UpdateDeploymentInstance failed: %v", err)
	}

	// Verify update
	retrieved, _ := s.GetDeploymentInstance(ctx, dep.ID, inst.ID)
	if retrieved.Status != DeploymentInstanceStatusCompleted {
		t.Errorf("Status = %q, want %q", retrieved.Status, DeploymentInstanceStatusCompleted)
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestStore_UpdateDeploymentInstance_NotFound(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	di := &DeploymentInstance{
		DeploymentID: "nonexistent",
		InstanceID:   "nonexistent",
		Status:       DeploymentInstanceStatusCompleted,
	}

	err := s.UpdateDeploymentInstance(ctx, di)
	if err == nil {
		t.Error("expected error for nonexistent deployment instance")
	}
}

func TestStore_UpsertDeploymentInstance(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create required parent objects
	createTestConfigWithVersion(t, s, "cfg-1", 1)
	inst := &Instance{Name: "inst-1", Hostname: "host1", AgentVersion: "1.0", SentinelVersion: "1.0"}
	s.CreateInstance(ctx, inst)
	dep := &Deployment{ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{inst.ID}}
	s.CreateDeployment(ctx, dep)

	now := time.Now().UTC()

	// First upsert - should create
	di := &DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst.ID,
		Status:       DeploymentInstanceStatusPending,
		StartedAt:    &now,
	}
	err := s.UpsertDeploymentInstance(ctx, di)
	if err != nil {
		t.Fatalf("UpsertDeploymentInstance (create) failed: %v", err)
	}

	// Second upsert - should update
	di.Status = DeploymentInstanceStatusCompleted
	completedAt := time.Now().UTC()
	di.CompletedAt = &completedAt
	err = s.UpsertDeploymentInstance(ctx, di)
	if err != nil {
		t.Fatalf("UpsertDeploymentInstance (update) failed: %v", err)
	}

	// Verify
	retrieved, _ := s.GetDeploymentInstance(ctx, dep.ID, inst.ID)
	if retrieved.Status != DeploymentInstanceStatusCompleted {
		t.Errorf("Status = %q, want %q", retrieved.Status, DeploymentInstanceStatusCompleted)
	}
	// StartedAt should be preserved (not overwritten)
	if retrieved.StartedAt == nil {
		t.Error("StartedAt should be preserved")
	}
}

func TestStore_DeleteDeploymentInstances(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create required parent objects
	createTestConfigWithVersion(t, s, "cfg-1", 1)
	inst := &Instance{Name: "inst-1", Hostname: "host1", AgentVersion: "1.0", SentinelVersion: "1.0"}
	s.CreateInstance(ctx, inst)
	dep := &Deployment{ConfigID: "cfg-1", ConfigVersion: 1, TargetInstances: []string{inst.ID}}
	s.CreateDeployment(ctx, dep)

	// Create deployment instance
	di := &DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst.ID,
		Status:       DeploymentInstanceStatusCompleted,
	}
	s.CreateDeploymentInstance(ctx, di)

	// Delete all instances for deployment
	err := s.DeleteDeploymentInstances(ctx, dep.ID)
	if err != nil {
		t.Fatalf("DeleteDeploymentInstances failed: %v", err)
	}

	// Verify deleted
	list, _ := s.ListDeploymentInstances(ctx, dep.ID)
	if len(list) != 0 {
		t.Errorf("len(list) = %d, want 0", len(list))
	}
}
