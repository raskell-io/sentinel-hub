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

func TestOrchestrator_RecoverOrphanedDeployments_InProgress(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a deployment that's stuck in "in_progress"
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	dep := &store.Deployment{
		ConfigID:        cfg.ID,
		ConfigVersion:   1,
		TargetInstances: []string{inst.ID},
		Status:          store.DeploymentStatusInProgress,
		Strategy:        store.DeploymentStrategyAllAtOnce,
	}
	if err := s.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Create a deployment instance that's in progress
	di := &store.DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst.ID,
		Status:       store.DeploymentInstanceStatusInProgress,
	}
	if err := s.CreateDeploymentInstance(ctx, di); err != nil {
		t.Fatalf("CreateDeploymentInstance failed: %v", err)
	}

	// Create orchestrator and run recovery
	o := NewOrchestrator(s, nil)
	if err := o.RecoverOrphanedDeployments(ctx); err != nil {
		t.Fatalf("RecoverOrphanedDeployments failed: %v", err)
	}

	// Verify deployment was marked as failed
	recovered, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if recovered.Status != store.DeploymentStatusFailed {
		t.Errorf("Status = %q, want %q", recovered.Status, store.DeploymentStatusFailed)
	}
	if recovered.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if recovered.Progress == nil || recovered.Progress.FailureReason == "" {
		t.Error("FailureReason should be set")
	}

	// Verify instance was marked as failed
	recoveredInst, err := s.GetDeploymentInstance(ctx, dep.ID, inst.ID)
	if err != nil {
		t.Fatalf("GetDeploymentInstance failed: %v", err)
	}
	if recoveredInst.Status != store.DeploymentInstanceStatusFailed {
		t.Errorf("Instance Status = %q, want %q", recoveredInst.Status, store.DeploymentInstanceStatusFailed)
	}
	if recoveredInst.ErrorMessage == nil || *recoveredInst.ErrorMessage == "" {
		t.Error("Instance ErrorMessage should be set")
	}
}

func TestOrchestrator_RecoverOrphanedDeployments_Pending(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a deployment that's stuck in "pending"
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)

	dep := &store.Deployment{
		ConfigID:        cfg.ID,
		ConfigVersion:   1,
		TargetInstances: []string{inst.ID},
		Status:          store.DeploymentStatusPending,
		Strategy:        store.DeploymentStrategyAllAtOnce,
	}
	if err := s.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Create orchestrator and run recovery
	o := NewOrchestrator(s, nil)
	if err := o.RecoverOrphanedDeployments(ctx); err != nil {
		t.Fatalf("RecoverOrphanedDeployments failed: %v", err)
	}

	// Verify deployment was marked as failed
	recovered, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if recovered.Status != store.DeploymentStatusFailed {
		t.Errorf("Status = %q, want %q", recovered.Status, store.DeploymentStatusFailed)
	}
}

func TestOrchestrator_RecoverOrphanedDeployments_CompletedNotAffected(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a deployment that's already completed
	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst := createTestInstance(t, s, "test-instance", nil)
	now := time.Now().UTC()

	dep := &store.Deployment{
		ConfigID:        cfg.ID,
		ConfigVersion:   1,
		TargetInstances: []string{inst.ID},
		Status:          store.DeploymentStatusCompleted,
		Strategy:        store.DeploymentStrategyAllAtOnce,
		CompletedAt:     &now,
	}
	if err := s.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Create orchestrator and run recovery
	o := NewOrchestrator(s, nil)
	if err := o.RecoverOrphanedDeployments(ctx); err != nil {
		t.Fatalf("RecoverOrphanedDeployments failed: %v", err)
	}

	// Verify deployment was NOT changed
	recovered, err := s.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if recovered.Status != store.DeploymentStatusCompleted {
		t.Errorf("Status = %q, want %q (should not be changed)", recovered.Status, store.DeploymentStatusCompleted)
	}
}

func TestOrchestrator_RecoverOrphanedDeployments_MultipleDeployments(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, s, "test-config", "content")

	// Create multiple orphaned deployments
	for i := 0; i < 3; i++ {
		inst := createTestInstance(t, s, "inst-"+string(rune('a'+i)), nil)
		status := store.DeploymentStatusInProgress
		if i == 0 {
			status = store.DeploymentStatusPending
		}
		dep := &store.Deployment{
			ConfigID:        cfg.ID,
			ConfigVersion:   1,
			TargetInstances: []string{inst.ID},
			Status:          status,
			Strategy:        store.DeploymentStrategyAllAtOnce,
		}
		if err := s.CreateDeployment(ctx, dep); err != nil {
			t.Fatalf("CreateDeployment failed: %v", err)
		}
	}

	// Create orchestrator and run recovery
	o := NewOrchestrator(s, nil)
	if err := o.RecoverOrphanedDeployments(ctx); err != nil {
		t.Fatalf("RecoverOrphanedDeployments failed: %v", err)
	}

	// Verify all deployments were marked as failed
	deps, err := s.ListDeployments(ctx, store.ListDeploymentsOptions{Status: store.DeploymentStatusFailed})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(deps) != 3 {
		t.Errorf("Failed deployment count = %d, want 3", len(deps))
	}
}

func TestOrchestrator_RecoverOrphanedDeployments_PreservesCompletedInstances(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, s, "test-config", "content")
	inst1 := createTestInstance(t, s, "inst-1", nil)
	inst2 := createTestInstance(t, s, "inst-2", nil)
	now := time.Now().UTC()

	// Create a deployment in progress
	dep := &store.Deployment{
		ConfigID:        cfg.ID,
		ConfigVersion:   1,
		TargetInstances: []string{inst1.ID, inst2.ID},
		Status:          store.DeploymentStatusInProgress,
		Strategy:        store.DeploymentStrategyRolling,
	}
	if err := s.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// First instance completed successfully
	di1 := &store.DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst1.ID,
		Status:       store.DeploymentInstanceStatusCompleted,
		CompletedAt:  &now,
	}
	s.CreateDeploymentInstance(ctx, di1)

	// Second instance was in progress when hub restarted
	di2 := &store.DeploymentInstance{
		DeploymentID: dep.ID,
		InstanceID:   inst2.ID,
		Status:       store.DeploymentInstanceStatusInProgress,
	}
	s.CreateDeploymentInstance(ctx, di2)

	// Run recovery
	o := NewOrchestrator(s, nil)
	if err := o.RecoverOrphanedDeployments(ctx); err != nil {
		t.Fatalf("RecoverOrphanedDeployments failed: %v", err)
	}

	// Verify first instance status was preserved
	recovered1, _ := s.GetDeploymentInstance(ctx, dep.ID, inst1.ID)
	if recovered1.Status != store.DeploymentInstanceStatusCompleted {
		t.Errorf("Instance 1 Status = %q, want %q (should be preserved)", recovered1.Status, store.DeploymentInstanceStatusCompleted)
	}

	// Verify second instance was marked as failed
	recovered2, _ := s.GetDeploymentInstance(ctx, dep.ID, inst2.ID)
	if recovered2.Status != store.DeploymentInstanceStatusFailed {
		t.Errorf("Instance 2 Status = %q, want %q", recovered2.Status, store.DeploymentInstanceStatusFailed)
	}
}
