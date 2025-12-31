package fleet

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	hubgrpc "github.com/raskell-io/sentinel-hub/internal/grpc"
	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
)

// ============================================
// Integration Test Helpers
// ============================================

// testEnv holds the test environment for integration tests.
type testEnv struct {
	store        *store.Store
	fleetService *hubgrpc.FleetService
	orchestrator *Orchestrator
	cleanup      func()
}

// setupIntegrationTest creates a full test environment.
func setupIntegrationTest(t *testing.T) *testEnv {
	t.Helper()

	// Create temp database
	tmpFile, err := os.CreateTemp("", "integration-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	s, err := store.New(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	// Create FleetService
	fs := hubgrpc.NewFleetService(s)

	// Create Orchestrator
	o := NewOrchestrator(s, fs)

	// Set up deployment status handler
	fs.SetDeploymentStatusHandler(func(instanceID, deploymentID string, state pb.DeploymentState, message, errorDetails string) {
		o.ReportInstanceStatus(instanceID, deploymentID, state, message, errorDetails)
	})

	env := &testEnv{
		store:        s,
		fleetService: fs,
		orchestrator: o,
		cleanup: func() {
			o.Stop()
			s.Close()
			os.Remove(tmpFile.Name())
			os.Remove(tmpFile.Name() + "-shm")
			os.Remove(tmpFile.Name() + "-wal")
		},
	}

	t.Cleanup(env.cleanup)
	return env
}

// NOTE: createTestConfig and createTestInstance are defined in orchestrator_test.go

// simulateAgentSubscription simulates an agent subscribing to the fleet service.
// It registers the instance and adds it to the subscribers map.
func simulateAgentSubscription(t *testing.T, fs *hubgrpc.FleetService, instanceID string) (string, func()) {
	return simulateAgentSubscriptionWithLabels(t, fs, instanceID, nil)
}

// simulateAgentSubscriptionWithLabels simulates agent subscription with custom labels.
func simulateAgentSubscriptionWithLabels(t *testing.T, fs *hubgrpc.FleetService, instanceID string, labels map[string]string) (string, func()) {
	t.Helper()

	ctx := context.Background()

	// Register the instance via the proper protobuf API
	resp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:      instanceID,
		InstanceName:    instanceID,
		Hostname:        instanceID + ".local",
		AgentVersion:    "1.0.0",
		SentinelVersion: "1.0.0",
		Labels:          labels,
	})
	if err != nil {
		t.Fatalf("failed to register instance: %v", err)
	}

	// Subscribe to events by adding to the subscribers map (direct access as in unit tests)
	eventCh := make(chan *pb.Event, 100)
	fs.SetSubscriber(instanceID, eventCh)

	cancel := func() {
		fs.RemoveSubscriber(instanceID)
	}

	return resp.Token, cancel
}

// simulateAgentDeploymentResponse simulates an agent responding to a deployment.
// It calls the deployment status handler that was set on the FleetService.
func simulateAgentDeploymentResponse(fs *hubgrpc.FleetService, token, instanceID, deploymentID string, success bool, delay time.Duration) {
	go func() {
		time.Sleep(delay)

		ctx := context.Background()
		var state pb.DeploymentState
		var message, errorDetails string

		if success {
			state = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
			message = "completed"
		} else {
			state = pb.DeploymentState_DEPLOYMENT_STATE_FAILED
			message = "failed"
			errorDetails = "simulated failure"
		}

		// Use the proper protobuf API to report deployment status
		fs.ReportDeploymentStatus(ctx, &pb.DeploymentStatusRequest{
			InstanceId:   instanceID,
			Token:        token,
			DeploymentId: deploymentID,
			State:        state,
			Message:      message,
			ErrorDetails: errorDetails,
		})
	}()
}

// ============================================
// Integration Tests
// ============================================

func TestIntegration_CreateDeployment_SingleInstance(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	// Create config
	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	// Create instance
	inst := createTestInstance(t, env.store, "inst-1", nil)

	// Simulate agent subscription
	token, cancel := simulateAgentSubscription(t, env.fleetService, inst.ID)
	defer cancel()

	// Simulate agent responding to deployment
	go func() {
		// Wait for deployment to start
		time.Sleep(100 * time.Millisecond)

		// Get active deployment ID
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) > 0 {
			simulateAgentDeploymentResponse(env.fleetService, token, inst.ID, deps[0].ID, true, 100*time.Millisecond)
		}
	}()

	// Create deployment
	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	if dep.ID == "" {
		t.Error("deployment ID should be set")
	}
	if dep.Status != store.DeploymentStatusPending {
		t.Errorf("initial status = %q, want %q", dep.Status, store.DeploymentStatusPending)
	}

	// Wait for deployment to complete
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if status.Deployment.Status == store.DeploymentStatusCompleted {
			return // Success
		}
		if status.Deployment.Status == store.DeploymentStatusFailed {
			t.Fatalf("deployment failed unexpectedly")
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Error("deployment did not complete in time")
}

func TestIntegration_CreateDeployment_MultipleInstances(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	// Create config
	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	// Create multiple instances
	instances := make([]*store.Instance, 3)
	tokens := make([]string, 3)
	for i := 0; i < 3; i++ {
		instances[i] = createTestInstance(t, env.store, "inst-"+string(rune('a'+i)), nil)
	}

	// Subscribe all instances
	cancels := make([]func(), 3)
	for i, inst := range instances {
		token, cancel := simulateAgentSubscription(t, env.fleetService, inst.ID)
		tokens[i] = token
		cancels[i] = cancel
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	// Simulate agent responses
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) > 0 {
			for i, inst := range instances {
				simulateAgentDeploymentResponse(env.fleetService, tokens[i], inst.ID, deps[0].ID, true, 50*time.Millisecond)
			}
		}
	}()

	// Create deployment
	instanceIDs := make([]string, len(instances))
	for i, inst := range instances {
		instanceIDs[i] = inst.ID
	}

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: instanceIDs,
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
		if status != nil && status.Deployment.Status == store.DeploymentStatusCompleted {
			// Note: InstanceResults are only available while runner is active.
			// Once deployment completes, runner is cleaned up and results are lost.
			// This is a design limitation - results aren't persisted to DB.
			return // Success - deployment completed
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Error("deployment did not complete in time")
}

func TestIntegration_CreateDeployment_RollingStrategy(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	// Create 4 instances for rolling deployment with batch size 2
	instances := make([]*store.Instance, 4)
	tokens := make([]string, 4)
	for i := 0; i < 4; i++ {
		instances[i] = createTestInstance(t, env.store, "inst-"+string(rune('a'+i)), nil)
		token, cancel := simulateAgentSubscription(t, env.fleetService, instances[i].ID)
		tokens[i] = token
		defer cancel()
	}

	// Simulate responses
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) > 0 {
			for i, inst := range instances {
				simulateAgentDeploymentResponse(env.fleetService, tokens[i], inst.ID, deps[0].ID, true, 50*time.Millisecond)
			}
		}
	}()

	instanceIDs := make([]string, len(instances))
	for i, inst := range instances {
		instanceIDs[i] = inst.ID
	}

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: instanceIDs,
		Strategy:        store.DeploymentStrategyRolling,
		BatchSize:       2,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	if dep.Strategy != store.DeploymentStrategyRolling {
		t.Errorf("strategy = %q, want %q", dep.Strategy, store.DeploymentStrategyRolling)
	}
	if dep.BatchSize != 2 {
		t.Errorf("batch_size = %d, want 2", dep.BatchSize)
	}

	// Wait for completion (rolling deployments take longer due to batch delays)
	// The runner has a 30s batch delay, but we'll just verify it starts correctly
	time.Sleep(200 * time.Millisecond)

	status, err := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	// Should be in progress or completed
	if status.Deployment.Status == store.DeploymentStatusPending {
		t.Error("deployment should have started")
	}
}

func TestIntegration_CreateDeployment_ByLabels(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	// Create instances with different labels
	prodInst1 := createTestInstance(t, env.store, "prod-1", map[string]string{"env": "prod"})
	prodInst2 := createTestInstance(t, env.store, "prod-2", map[string]string{"env": "prod"})
	devInst := createTestInstance(t, env.store, "dev-1", map[string]string{"env": "dev"})

	// Subscribe all - must pass labels to preserve them during registration
	token1, cancel1 := simulateAgentSubscriptionWithLabels(t, env.fleetService, prodInst1.ID, map[string]string{"env": "prod"})
	defer cancel1()
	token2, cancel2 := simulateAgentSubscriptionWithLabels(t, env.fleetService, prodInst2.ID, map[string]string{"env": "prod"})
	defer cancel2()
	_, cancel3 := simulateAgentSubscriptionWithLabels(t, env.fleetService, devInst.ID, map[string]string{"env": "dev"})
	defer cancel3()

	// Simulate responses for prod instances only
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) > 0 {
			simulateAgentDeploymentResponse(env.fleetService, token1, prodInst1.ID, deps[0].ID, true, 50*time.Millisecond)
			simulateAgentDeploymentResponse(env.fleetService, token2, prodInst2.ID, deps[0].ID, true, 50*time.Millisecond)
		}
	}()

	// Create deployment targeting prod instances by label
	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:     cfg.ID,
		TargetLabels: map[string]string{"env": "prod"},
		Strategy:     store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Should target only prod instances
	if len(dep.TargetInstances) != 2 {
		t.Errorf("target instances = %d, want 2", len(dep.TargetInstances))
	}

	// Verify dev instance is not included
	for _, id := range dep.TargetInstances {
		if id == devInst.ID {
			t.Error("dev instance should not be targeted")
		}
	}
}

func TestIntegration_CreateDeployment_InstanceFailure(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")
	inst := createTestInstance(t, env.store, "inst-1", nil)

	token, cancel := simulateAgentSubscription(t, env.fleetService, inst.ID)
	defer cancel()

	// Simulate agent failure
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) > 0 {
			simulateAgentDeploymentResponse(env.fleetService, token, inst.ID, deps[0].ID, false, 50*time.Millisecond)
		}
	}()

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Wait for failure
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
		if status != nil && status.Deployment.Status == store.DeploymentStatusFailed {
			return // Expected
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Error("deployment should have failed")
}

func TestIntegration_CancelDeployment(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")
	inst := createTestInstance(t, env.store, "inst-1", nil)

	_, cancel := simulateAgentSubscription(t, env.fleetService, inst.ID)
	defer cancel()

	// Don't simulate response - let it hang

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Wait a bit then cancel
	time.Sleep(100 * time.Millisecond)

	if err := env.orchestrator.CancelDeployment(ctx, dep.ID); err != nil {
		t.Fatalf("CancelDeployment failed: %v", err)
	}

	// Verify cancelled
	status, err := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	if status.Deployment.Status != store.DeploymentStatusCancelled {
		t.Errorf("status = %q, want %q", status.Deployment.Status, store.DeploymentStatusCancelled)
	}
}

func TestIntegration_CreateDeployment_NoInstances(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	// Try to create deployment with no instances
	_, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{},
	})

	if err == nil {
		t.Error("expected error for empty target instances")
	}
}

func TestIntegration_CreateDeployment_InvalidConfig(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	inst := createTestInstance(t, env.store, "inst-1", nil)

	_, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        "nonexistent-config",
		TargetInstances: []string{inst.ID},
	})

	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestIntegration_CreateDeployment_InvalidInstance(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	_, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{"nonexistent-instance"},
	})

	if err == nil {
		t.Error("expected error for invalid instance")
	}
}

func TestIntegration_CreateDeployment_InstanceNotConnected(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")
	inst := createTestInstance(t, env.store, "inst-1", nil)

	// Don't subscribe the instance - it should fail

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Wait for failure due to instance not connected
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
		if status != nil && status.Deployment.Status == store.DeploymentStatusFailed {
			return // Expected
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Error("deployment should have failed due to unconnected instance")
}

func TestIntegration_DeploymentProgress(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")

	// Create instances
	instances := make([]*store.Instance, 3)
	tokens := make([]string, 3)
	for i := 0; i < 3; i++ {
		instances[i] = createTestInstance(t, env.store, "inst-"+string(rune('a'+i)), nil)
		token, cancel := simulateAgentSubscription(t, env.fleetService, instances[i].ID)
		tokens[i] = token
		defer cancel()
	}

	// Track progress updates
	var progressMu sync.Mutex
	var progressSnapshots []store.DeploymentProgress

	// Simulate staggered responses
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) == 0 {
			return
		}
		depID := deps[0].ID

		for i, inst := range instances {
			time.Sleep(50 * time.Millisecond)
			simulateAgentDeploymentResponse(env.fleetService, tokens[i], inst.ID, depID, true, 0)

			// Capture progress
			time.Sleep(50 * time.Millisecond)
			status, _ := env.orchestrator.GetDeploymentStatus(ctx, depID)
			if status != nil && status.Deployment.Progress != nil {
				progressMu.Lock()
				progressSnapshots = append(progressSnapshots, *status.Deployment.Progress)
				progressMu.Unlock()
			}
			_ = i
		}
	}()

	instanceIDs := make([]string, len(instances))
	for i, inst := range instances {
		instanceIDs[i] = inst.ID
	}

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: instanceIDs,
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
		if status != nil && status.Deployment.Status == store.DeploymentStatusCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify we captured some progress
	progressMu.Lock()
	defer progressMu.Unlock()

	if len(progressSnapshots) == 0 {
		t.Log("Note: No progress snapshots captured (timing dependent)")
	}

	// Verify final deployment has progress
	status, _ := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
	if status != nil && status.Deployment.Progress != nil {
		if status.Deployment.Progress.TotalInstances != 3 {
			t.Errorf("TotalInstances = %d, want 3", status.Deployment.Progress.TotalInstances)
		}
	}
}

func TestIntegration_ConcurrentDeployments(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	// Create two configs
	cfg1, _ := createTestConfig(t, env.store, "config-1", "server { listen 8080 }")
	cfg2, _ := createTestConfig(t, env.store, "config-2", "server { listen 9090 }")

	// Create instances for each deployment
	inst1 := createTestInstance(t, env.store, "inst-1", nil)
	inst2 := createTestInstance(t, env.store, "inst-2", nil)

	token1, cancel1 := simulateAgentSubscription(t, env.fleetService, inst1.ID)
	defer cancel1()
	token2, cancel2 := simulateAgentSubscription(t, env.fleetService, inst2.ID)
	defer cancel2()

	// Map instance IDs to tokens for easy lookup
	tokenMap := map[string]string{
		inst1.ID: token1,
		inst2.ID: token2,
	}

	// Simulate responses for both
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		for _, dep := range deps {
			for _, instID := range dep.TargetInstances {
				if tok, ok := tokenMap[instID]; ok {
					simulateAgentDeploymentResponse(env.fleetService, tok, instID, dep.ID, true, 50*time.Millisecond)
				}
			}
		}
	}()

	// Create both deployments concurrently
	var wg sync.WaitGroup
	var dep1, dep2 *store.Deployment
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		dep1, err1 = env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
			ConfigID:        cfg1.ID,
			TargetInstances: []string{inst1.ID},
		})
	}()
	go func() {
		defer wg.Done()
		dep2, err2 = env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
			ConfigID:        cfg2.ID,
			TargetInstances: []string{inst2.ID},
		})
	}()
	wg.Wait()

	if err1 != nil {
		t.Errorf("deployment 1 failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("deployment 2 failed: %v", err2)
	}

	if dep1.ID == dep2.ID {
		t.Error("deployments should have different IDs")
	}

	// Wait for both to complete
	deadline := time.Now().Add(5 * time.Second)
	completed := 0
	for time.Now().Before(deadline) && completed < 2 {
		completed = 0
		for _, depID := range []string{dep1.ID, dep2.ID} {
			status, _ := env.orchestrator.GetDeploymentStatus(ctx, depID)
			if status != nil && status.Deployment.Status == store.DeploymentStatusCompleted {
				completed++
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	if completed != 2 {
		t.Errorf("completed deployments = %d, want 2", completed)
	}
}

func TestIntegration_GetDeploymentStatus_NotFound(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	_, err := env.orchestrator.GetDeploymentStatus(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent deployment")
	}
}

func TestIntegration_DeploymentUpdatesInstanceConfig(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	cfg, _ := createTestConfig(t, env.store, "test-config", "server { listen 8080 }")
	inst := createTestInstance(t, env.store, "inst-1", nil)

	token, cancel := simulateAgentSubscription(t, env.fleetService, inst.ID)
	defer cancel()

	// Simulate success
	go func() {
		time.Sleep(100 * time.Millisecond)
		deps, _ := env.store.ListDeployments(ctx, store.ListDeploymentsOptions{})
		if len(deps) > 0 {
			simulateAgentDeploymentResponse(env.fleetService, token, inst.ID, deps[0].ID, true, 50*time.Millisecond)
		}
	}()

	dep, err := env.orchestrator.CreateDeployment(ctx, CreateDeploymentRequest{
		ConfigID:        cfg.ID,
		TargetInstances: []string{inst.ID},
		Strategy:        store.DeploymentStrategyAllAtOnce,
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := env.orchestrator.GetDeploymentStatus(ctx, dep.ID)
		if status != nil && status.Deployment.Status == store.DeploymentStatusCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify instance was updated with config reference
	updatedInst, err := env.store.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}

	if updatedInst.CurrentConfigID == nil || *updatedInst.CurrentConfigID != cfg.ID {
		t.Errorf("CurrentConfigID = %v, want %q", updatedInst.CurrentConfigID, cfg.ID)
	}
	if updatedInst.CurrentConfigVersion == nil || *updatedInst.CurrentConfigVersion != 1 {
		t.Errorf("CurrentConfigVersion = %v, want 1", updatedInst.CurrentConfigVersion)
	}
}
