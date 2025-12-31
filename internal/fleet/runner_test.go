package fleet

import (
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
)

func TestNewDeploymentRunner(t *testing.T) {
	dep := &store.Deployment{
		ID:              "dep-1",
		ConfigID:        "cfg-1",
		ConfigVersion:   1,
		TargetInstances: []string{"inst-1", "inst-2", "inst-3"},
		Strategy:        store.DeploymentStrategyRolling,
		BatchSize:       2,
	}

	ver := &store.ConfigVersion{
		ID:       "ver-1",
		ConfigID: "cfg-1",
		Version:  1,
		Content:  "test content",
	}

	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment:         dep,
		ConfigVersion:      ver,
		Store:              nil,
		FleetService:       nil,
		Timeout:            10 * time.Minute,
		HealthCheckRetries: 3,
		HealthCheckDelay:   5 * time.Second,
	})

	if runner == nil {
		t.Fatal("NewDeploymentRunner returned nil")
	}

	if runner.deployment != dep {
		t.Error("deployment not set correctly")
	}

	if runner.configVersion != ver {
		t.Error("configVersion not set correctly")
	}

	if runner.timeout != 10*time.Minute {
		t.Errorf("timeout = %v, want %v", runner.timeout, 10*time.Minute)
	}

	if runner.healthCheckRetries != 3 {
		t.Errorf("healthCheckRetries = %v, want 3", runner.healthCheckRetries)
	}

	if runner.batchDelay != 30*time.Second {
		t.Errorf("batchDelay = %v, want %v", runner.batchDelay, 30*time.Second)
	}

	// Check instance results initialized
	if len(runner.instanceResults) != 3 {
		t.Errorf("instanceResults count = %d, want 3", len(runner.instanceResults))
	}

	for _, id := range []string{"inst-1", "inst-2", "inst-3"} {
		result, ok := runner.instanceResults[id]
		if !ok {
			t.Errorf("instanceResults[%s] not initialized", id)
			continue
		}
		if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_PENDING {
			t.Errorf("instanceResults[%s].Status = %v, want PENDING", id, result.Status)
		}
	}
}

func TestDeploymentRunner_AllSucceeded(t *testing.T) {
	tests := []struct {
		name     string
		statuses map[string]pb.DeploymentState
		want     bool
	}{
		{
			name: "all completed",
			statuses: map[string]pb.DeploymentState{
				"inst-1": pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
				"inst-2": pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
			},
			want: true,
		},
		{
			name: "one failed",
			statuses: map[string]pb.DeploymentState{
				"inst-1": pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
				"inst-2": pb.DeploymentState_DEPLOYMENT_STATE_FAILED,
			},
			want: false,
		},
		{
			name: "one pending",
			statuses: map[string]pb.DeploymentState{
				"inst-1": pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
				"inst-2": pb.DeploymentState_DEPLOYMENT_STATE_PENDING,
			},
			want: false,
		},
		{
			name: "one in progress",
			statuses: map[string]pb.DeploymentState{
				"inst-1": pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
				"inst-2": pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS,
			},
			want: false,
		},
		{
			name: "one rolled back",
			statuses: map[string]pb.DeploymentState{
				"inst-1": pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
				"inst-2": pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK,
			},
			want: false,
		},
		{
			name:     "empty instances",
			statuses: map[string]pb.DeploymentState{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instances := make([]string, 0, len(tt.statuses))
			for id := range tt.statuses {
				instances = append(instances, id)
			}

			runner := NewDeploymentRunner(DeploymentRunnerConfig{
				Deployment: &store.Deployment{
					ID:              "dep-1",
					TargetInstances: instances,
				},
				ConfigVersion: &store.ConfigVersion{Version: 1},
			})

			// Set the statuses
			for id, status := range tt.statuses {
				runner.instanceResults[id].Status = status
			}

			got := runner.allSucceeded()
			if got != tt.want {
				t.Errorf("allSucceeded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeploymentRunner_CheckBatchHealth(t *testing.T) {
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1", "inst-2", "inst-3"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	// All pending initially
	if runner.checkBatchHealth([]string{"inst-1", "inst-2"}) {
		t.Error("checkBatchHealth should return false when instances are pending")
	}

	// Mark some as completed
	runner.instanceResults["inst-1"].Status = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
	runner.instanceResults["inst-2"].Status = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED

	if !runner.checkBatchHealth([]string{"inst-1", "inst-2"}) {
		t.Error("checkBatchHealth should return true when all batch instances are completed")
	}

	// inst-3 is still pending
	if runner.checkBatchHealth([]string{"inst-1", "inst-2", "inst-3"}) {
		t.Error("checkBatchHealth should return false when some instances are pending")
	}

	// Non-existent instance
	if runner.checkBatchHealth([]string{"inst-1", "nonexistent"}) {
		t.Error("checkBatchHealth should return false for non-existent instance")
	}
}

func TestDeploymentRunner_GetInstanceResults(t *testing.T) {
	now := time.Now().UTC()
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1", "inst-2"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	// Set some state
	runner.instanceResults["inst-1"].Status = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
	runner.instanceResults["inst-1"].StartedAt = &now
	runner.instanceResults["inst-1"].CompletedAt = &now

	runner.instanceResults["inst-2"].Status = pb.DeploymentState_DEPLOYMENT_STATE_FAILED
	runner.instanceResults["inst-2"].ErrorMessage = "connection refused"

	results := runner.GetInstanceResults()

	if len(results) != 2 {
		t.Fatalf("results count = %d, want 2", len(results))
	}

	// Check inst-1
	r1, ok := results["inst-1"]
	if !ok {
		t.Fatal("results[inst-1] not found")
	}
	if r1.InstanceID != "inst-1" {
		t.Errorf("r1.InstanceID = %q, want %q", r1.InstanceID, "inst-1")
	}
	if r1.Status != "DEPLOYMENT_STATE_COMPLETED" {
		t.Errorf("r1.Status = %q, want DEPLOYMENT_STATE_COMPLETED", r1.Status)
	}

	// Check inst-2
	r2, ok := results["inst-2"]
	if !ok {
		t.Fatal("results[inst-2] not found")
	}
	if r2.Status != "DEPLOYMENT_STATE_FAILED" {
		t.Errorf("r2.Status = %q, want DEPLOYMENT_STATE_FAILED", r2.Status)
	}
	if r2.ErrorMessage != "connection refused" {
		t.Errorf("r2.ErrorMessage = %q, want %q", r2.ErrorMessage, "connection refused")
	}
}

func TestDeploymentRunner_ReportInstanceStatus(t *testing.T) {
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1", "inst-2"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	// Report in progress
	runner.ReportInstanceStatus("inst-1", pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS, "starting", "")

	result := runner.instanceResults["inst-1"]
	if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS {
		t.Errorf("Status = %v, want IN_PROGRESS", result.Status)
	}
	if result.CompletedAt != nil {
		t.Error("CompletedAt should be nil for in-progress status")
	}

	// Report completed
	runner.ReportInstanceStatus("inst-1", pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED, "done", "")

	result = runner.instanceResults["inst-1"]
	if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
		t.Errorf("Status = %v, want COMPLETED", result.Status)
	}
	if result.CompletedAt == nil {
		t.Error("CompletedAt should be set for completed status")
	}

	// Report failed with error details
	runner.ReportInstanceStatus("inst-2", pb.DeploymentState_DEPLOYMENT_STATE_FAILED, "error", "config validation failed")

	result = runner.instanceResults["inst-2"]
	if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_FAILED {
		t.Errorf("Status = %v, want FAILED", result.Status)
	}
	if result.ErrorMessage != "config validation failed" {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, "config validation failed")
	}
	if result.CompletedAt == nil {
		t.Error("CompletedAt should be set for failed status")
	}

	// Report for non-existent instance (should not panic)
	runner.ReportInstanceStatus("nonexistent", pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED, "done", "")
}

func TestDeploymentRunner_SetInstanceError(t *testing.T) {
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	runner.setInstanceError("inst-1", "timeout waiting for response")

	result := runner.instanceResults["inst-1"]
	if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_FAILED {
		t.Errorf("Status = %v, want FAILED", result.Status)
	}
	if result.ErrorMessage != "timeout waiting for response" {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, "timeout waiting for response")
	}
	if result.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestDeploymentRunner_Cancel(t *testing.T) {
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	// Cancel should not block
	done := make(chan struct{})
	go func() {
		runner.Cancel()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Error("Cancel blocked")
	}

	// Context should be cancelled
	select {
	case <-runner.ctx.Done():
		// OK
	default:
		t.Error("Context not cancelled after Cancel()")
	}
}

func TestDeploymentRunner_ReportInstanceStatus_RolledBack(t *testing.T) {
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	runner.ReportInstanceStatus("inst-1", pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK, "rolled back", "")

	result := runner.instanceResults["inst-1"]
	if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK {
		t.Errorf("Status = %v, want ROLLED_BACK", result.Status)
	}
	if result.CompletedAt == nil {
		t.Error("CompletedAt should be set for rolled back status")
	}
}

func TestDeploymentRunner_ConcurrentAccess(t *testing.T) {
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment: &store.Deployment{
			ID:              "dep-1",
			TargetInstances: []string{"inst-1", "inst-2", "inst-3"},
		},
		ConfigVersion: &store.ConfigVersion{Version: 1},
	})

	// Test concurrent access to instance results
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			runner.ReportInstanceStatus("inst-1", pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS, "update", "")
		}
		close(done)
	}()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		_ = runner.GetInstanceResults()
		_ = runner.allSucceeded()
		_ = runner.checkBatchHealth([]string{"inst-1", "inst-2"})
	}

	<-done
}
