package fleet

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *store.Store {
	t.Helper()

	// Create a temp file for the test database
	tmpFile, err := os.CreateTemp("", "hub-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	s, err := store.New(tmpFile.Name())
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

// createTestConfig creates a config with a version for testing.
func createTestConfig(t *testing.T, s *store.Store, name, content string) (*store.Config, *store.ConfigVersion) {
	t.Helper()
	ctx := context.Background()

	cfg := &store.Config{
		Name: name,
	}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	ver := &store.ConfigVersion{
		ConfigID:    cfg.ID,
		Version:     1,
		Content:     content,
		ContentHash: "abc123",
	}
	if err := s.CreateConfigVersion(ctx, ver); err != nil {
		t.Fatalf("failed to create config version: %v", err)
	}

	return cfg, ver
}

// createTestInstance creates an instance for testing.
func createTestInstance(t *testing.T, s *store.Store, name string, labels map[string]string) *store.Instance {
	t.Helper()
	ctx := context.Background()

	inst := &store.Instance{
		Name:     name,
		Hostname: name + ".local",
		Status:   store.InstanceStatusOnline,
		Labels:   labels,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}

	return inst
}

func TestMatchLabels(t *testing.T) {
	tests := []struct {
		name           string
		instanceLabels map[string]string
		selector       map[string]string
		want           bool
	}{
		{
			name:           "empty selector matches all",
			instanceLabels: map[string]string{"env": "prod", "region": "us-east"},
			selector:       map[string]string{},
			want:           true,
		},
		{
			name:           "nil selector matches all",
			instanceLabels: map[string]string{"env": "prod"},
			selector:       nil,
			want:           true,
		},
		{
			name:           "empty instance labels with empty selector",
			instanceLabels: map[string]string{},
			selector:       map[string]string{},
			want:           true,
		},
		{
			name:           "single label match",
			instanceLabels: map[string]string{"env": "prod", "region": "us-east"},
			selector:       map[string]string{"env": "prod"},
			want:           true,
		},
		{
			name:           "multiple label match",
			instanceLabels: map[string]string{"env": "prod", "region": "us-east"},
			selector:       map[string]string{"env": "prod", "region": "us-east"},
			want:           true,
		},
		{
			name:           "single label mismatch",
			instanceLabels: map[string]string{"env": "prod"},
			selector:       map[string]string{"env": "staging"},
			want:           false,
		},
		{
			name:           "missing label in instance",
			instanceLabels: map[string]string{"env": "prod"},
			selector:       map[string]string{"region": "us-east"},
			want:           false,
		},
		{
			name:           "partial match fails",
			instanceLabels: map[string]string{"env": "prod"},
			selector:       map[string]string{"env": "prod", "region": "us-east"},
			want:           false,
		},
		{
			name:           "empty instance labels with selector",
			instanceLabels: map[string]string{},
			selector:       map[string]string{"env": "prod"},
			want:           false,
		},
		{
			name:           "nil instance labels with selector",
			instanceLabels: nil,
			selector:       map[string]string{"env": "prod"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchLabels(tt.instanceLabels, tt.selector)
			if got != tt.want {
				t.Errorf("matchLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewOrchestrator(t *testing.T) {
	s := setupTestStore(t)

	o := NewOrchestrator(s, nil)

	if o == nil {
		t.Fatal("NewOrchestrator returned nil")
	}
	if o.store != s {
		t.Error("store not set correctly")
	}
	if o.deployments == nil {
		t.Error("deployments map not initialized")
	}
	if o.defaultTimeout != 10*time.Minute {
		t.Errorf("defaultTimeout = %v, want %v", o.defaultTimeout, 10*time.Minute)
	}
	if o.healthCheckRetries != 3 {
		t.Errorf("healthCheckRetries = %v, want 3", o.healthCheckRetries)
	}
	if o.healthCheckDelay != 5*time.Second {
		t.Errorf("healthCheckDelay = %v, want %v", o.healthCheckDelay, 5*time.Second)
	}
}

func TestOrchestrator_CreateDeployment_ConfigNotFound(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	_, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        "nonexistent-config-id",
		TargetInstances: []string{"inst-1"},
	})

	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
	if got := err.Error(); got != "config not found: nonexistent-config-id" {
		t.Errorf("error = %q, want config not found error", got)
	}
}

func TestOrchestrator_CreateDeployment_ConfigVersionNotFound(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")

	_, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		ConfigVersion:   999, // Non-existent version
		TargetInstances: []string{"inst-1"},
	})

	if err == nil {
		t.Fatal("expected error for nonexistent config version")
	}
	if got := err.Error(); got != "config version 999 not found" {
		t.Errorf("error = %q, want config version not found error", got)
	}
}

func TestOrchestrator_CreateDeployment_InstanceNotFound(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")

	_, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{"nonexistent-instance"},
	})

	if err == nil {
		t.Fatal("expected error for nonexistent instance")
	}
}

func TestOrchestrator_CreateDeployment_NoTargets(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")

	// No instances match the label selector
	_, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:     cfg.ID,
		TargetLabels: map[string]string{"env": "prod"},
	})

	if err == nil {
		t.Fatal("expected error for no matching targets")
	}
	if got := err.Error(); got != "no target instances found" {
		t.Errorf("error = %q, want 'no target instances found'", got)
	}
}

func TestOrchestrator_CreateDeployment_DefaultsApplied(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)
	t.Cleanup(func() { o.Stop() })

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	dep, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
		// No strategy or batch size specified
	})

	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Check defaults
	if dep.Strategy != store.DeploymentStrategyRolling {
		t.Errorf("Strategy = %q, want %q", dep.Strategy, store.DeploymentStrategyRolling)
	}
	if dep.BatchSize != 1 {
		t.Errorf("BatchSize = %d, want 1", dep.BatchSize)
	}
	if dep.ConfigVersion != 1 {
		t.Errorf("ConfigVersion = %d, want 1 (current version)", dep.ConfigVersion)
	}
	if dep.Status != store.DeploymentStatusPending {
		t.Errorf("Status = %q, want %q", dep.Status, store.DeploymentStatusPending)
	}
}

func TestOrchestrator_CreateDeployment_ExplicitValues(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)
	t.Cleanup(func() { o.Stop() })

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)
	createdBy := "test-user"

	dep, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		ConfigVersion:   1,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyAllAtOnce,
		BatchSize:       5,
		CreatedBy:       &createdBy,
	})

	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	if dep.Strategy != store.DeploymentStrategyAllAtOnce {
		t.Errorf("Strategy = %q, want %q", dep.Strategy, store.DeploymentStrategyAllAtOnce)
	}
	if dep.BatchSize != 5 {
		t.Errorf("BatchSize = %d, want 5", dep.BatchSize)
	}
	if dep.CreatedBy == nil || *dep.CreatedBy != createdBy {
		t.Errorf("CreatedBy = %v, want %q", dep.CreatedBy, createdBy)
	}
}

func TestOrchestrator_CreateDeployment_MultipleTargets(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)
	t.Cleanup(func() { o.Stop() })

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")

	// Create multiple instances
	inst1 := createTestInstance(t, s, "instance-1", map[string]string{"env": "prod"})
	inst2 := createTestInstance(t, s, "instance-2", map[string]string{"env": "prod"})
	createTestInstance(t, s, "instance-3", map[string]string{"env": "staging"})

	dep, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst1.ID, inst2.ID},
	})

	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	if len(dep.TargetInstances) != 2 {
		t.Errorf("TargetInstances count = %d, want 2", len(dep.TargetInstances))
	}
	if dep.Progress == nil {
		t.Error("Progress is nil")
	} else if dep.Progress.TotalInstances != 2 {
		t.Errorf("Progress.TotalInstances = %d, want 2", dep.Progress.TotalInstances)
	}
}

func TestOrchestrator_ResolveTargets_ByInstanceIDs(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	inst1 := createTestInstance(t, s, "instance-1", nil)
	inst2 := createTestInstance(t, s, "instance-2", nil)

	targets, err := o.resolveTargets(ctx, []string{inst1.ID, inst2.ID}, nil)
	if err != nil {
		t.Fatalf("resolveTargets failed: %v", err)
	}

	if len(targets) != 2 {
		t.Errorf("targets count = %d, want 2", len(targets))
	}
}

func TestOrchestrator_ResolveTargets_ByLabels(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	createTestInstance(t, s, "prod-1", map[string]string{"env": "prod", "region": "us-east"})
	createTestInstance(t, s, "prod-2", map[string]string{"env": "prod", "region": "us-west"})
	createTestInstance(t, s, "staging-1", map[string]string{"env": "staging"})

	// Select all prod instances
	targets, err := o.resolveTargets(ctx, nil, map[string]string{"env": "prod"})
	if err != nil {
		t.Fatalf("resolveTargets failed: %v", err)
	}

	if len(targets) != 2 {
		t.Errorf("targets count = %d, want 2", len(targets))
	}
}

func TestOrchestrator_ResolveTargets_ByMultipleLabels(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	createTestInstance(t, s, "prod-east", map[string]string{"env": "prod", "region": "us-east"})
	createTestInstance(t, s, "prod-west", map[string]string{"env": "prod", "region": "us-west"})
	createTestInstance(t, s, "staging", map[string]string{"env": "staging"})

	// Select only prod instances in us-east
	targets, err := o.resolveTargets(ctx, nil, map[string]string{"env": "prod", "region": "us-east"})
	if err != nil {
		t.Fatalf("resolveTargets failed: %v", err)
	}

	if len(targets) != 1 {
		t.Errorf("targets count = %d, want 1", len(targets))
	}
}

func TestOrchestrator_ResolveTargets_EmptySelector(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	createTestInstance(t, s, "instance-1", nil)
	createTestInstance(t, s, "instance-2", nil)
	createTestInstance(t, s, "instance-3", nil)

	// Empty selector should match all
	targets, err := o.resolveTargets(ctx, nil, map[string]string{})
	if err != nil {
		t.Fatalf("resolveTargets failed: %v", err)
	}

	if len(targets) != 3 {
		t.Errorf("targets count = %d, want 3", len(targets))
	}
}

func TestOrchestrator_ResolveTargets_InvalidInstanceID(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	createTestInstance(t, s, "instance-1", nil)

	_, err := o.resolveTargets(ctx, []string{"nonexistent-id"}, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent instance")
	}
}

func TestOrchestrator_GetDeploymentStatus(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)
	t.Cleanup(func() { o.Stop() })

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	dep, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Cancel immediately to stop background goroutine
	o.CancelDeployment(ctx, dep.ID)

	status, err := o.GetDeploymentStatus(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	if status.Deployment == nil {
		t.Error("status.Deployment is nil")
	}
	if status.Deployment.ID != dep.ID {
		t.Errorf("Deployment.ID = %q, want %q", status.Deployment.ID, dep.ID)
	}
	if status.InstanceResults == nil {
		t.Error("InstanceResults is nil")
	}
}

func TestOrchestrator_GetDeploymentStatus_NotFound(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	_, err := o.GetDeploymentStatus(ctx, "nonexistent-deployment")
	if err == nil {
		t.Fatal("expected error for nonexistent deployment")
	}
}

func TestOrchestrator_CancelDeployment(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)
	t.Cleanup(func() { o.Stop() })

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	dep, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Cancel immediately to prevent background goroutine issues
	err = o.CancelDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("CancelDeployment failed: %v", err)
	}

	// Verify status was updated
	updated, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if updated.Status != store.DeploymentStatusCancelled {
		t.Errorf("Status = %q, want %q", updated.Status, store.DeploymentStatusCancelled)
	}
	if updated.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestOrchestrator_CancelDeployment_NotFound(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	err := o.CancelDeployment(ctx, "nonexistent-deployment")
	if err == nil {
		t.Fatal("expected error for nonexistent deployment")
	}
}

func TestOrchestrator_CancelDeployment_AlreadyCompleted(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	// Create deployment directly in store (no background goroutine)
	now := time.Now().UTC()
	dep := &store.Deployment{
		ConfigID:        cfg.ID,
		ConfigVersion:   1,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyRolling,
		BatchSize:       1,
		Status:          store.DeploymentStatusCompleted,
		CompletedAt:     &now,
	}
	if err := s.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Try to cancel - should not change status since already completed
	err := o.CancelDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("CancelDeployment failed: %v", err)
	}

	// Verify status was NOT changed
	updated, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if updated.Status != store.DeploymentStatusCompleted {
		t.Errorf("Status = %q, want %q (should remain completed)", updated.Status, store.DeploymentStatusCompleted)
	}
}

func TestOrchestrator_StartStop(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)

	err := o.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = o.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestOrchestrator_CreateDeployment_StoredInDatabase(t *testing.T) {
	s := setupTestStore(t)
	o := NewOrchestrator(s, nil)
	t.Cleanup(func() { o.Stop() })

	ctx := context.Background()
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	dep, err := o.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Cancel immediately to stop background goroutine
	o.CancelDeployment(ctx, dep.ID)

	// Verify deployment was stored in database
	stored, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if stored == nil {
		t.Fatal("deployment not found in database")
	}
	if stored.ID != dep.ID {
		t.Errorf("stored ID = %q, want %q", stored.ID, dep.ID)
	}
	if stored.ConfigID != cfg.ID {
		t.Errorf("stored ConfigID = %q, want %q", stored.ConfigID, cfg.ID)
	}
	if len(stored.TargetInstances) != 1 {
		t.Errorf("stored TargetInstances count = %d, want 1", len(stored.TargetInstances))
	}
}
