package grpc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *store.Store {
	t.Helper()

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

func TestNewFleetService(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	if fs == nil {
		t.Fatal("NewFleetService returned nil")
	}
	if fs.store != s {
		t.Error("store not set correctly")
	}
	if fs.subscribers == nil {
		t.Error("subscribers map not initialized")
	}
	if fs.sessions == nil {
		t.Error("sessions map not initialized")
	}
	if fs.heartbeatInterval != 30*time.Second {
		t.Errorf("heartbeatInterval = %v, want %v", fs.heartbeatInterval, 30*time.Second)
	}
	if fs.sessionTTL != 24*time.Hour {
		t.Errorf("sessionTTL = %v, want %v", fs.sessionTTL, 24*time.Hour)
	}
}

func TestGenerateToken(t *testing.T) {
	token1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	if len(token1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length = %d, want 64", len(token1))
	}

	// Tokens should be unique
	token2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("tokens should be unique")
	}
}

func TestHashToken(t *testing.T) {
	token := "test-token"
	hash1 := hashToken(token)
	hash2 := hashToken(token)

	// Same input should produce same hash
	if hash1 != hash2 {
		t.Error("hashing same token should produce same hash")
	}

	// Different input should produce different hash
	hash3 := hashToken("different-token")
	if hash1 == hash3 {
		t.Error("different tokens should produce different hashes")
	}

	// Hash should be 64 chars (SHA256 = 32 bytes = 64 hex)
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func TestFleetService_ValidateToken(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	// Add a valid session
	token := "valid-token"
	instanceID := "inst-1"
	fs.sessions[hashToken(token)] = instanceID

	// Valid token
	result, err := fs.validateToken(token)
	if err != nil {
		t.Fatalf("validateToken failed: %v", err)
	}
	if result != instanceID {
		t.Errorf("instanceID = %q, want %q", result, instanceID)
	}

	// Invalid token
	_, err = fs.validateToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestFleetService_Register_NewInstance(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()
	req := &pb.RegisterRequest{
		InstanceId:      "inst-1",
		InstanceName:    "test-instance",
		Hostname:        "test.local",
		AgentVersion:    "1.0.0",
		SentinelVersion: "2.0.0",
		Labels:          map[string]string{"env": "test"},
		Capabilities:    []string{"config-reload"},
	}

	resp, err := fs.Register(ctx, req)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if resp.Token == "" {
		t.Error("Token should not be empty")
	}
	if resp.HeartbeatIntervalSeconds != 30 {
		t.Errorf("HeartbeatIntervalSeconds = %d, want 30", resp.HeartbeatIntervalSeconds)
	}

	// Verify instance was created
	inst, err := s.GetInstance(ctx, "inst-1")
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}
	if inst == nil {
		t.Fatal("instance not created")
	}
	if inst.Name != "test-instance" {
		t.Errorf("Name = %q, want %q", inst.Name, "test-instance")
	}
	if inst.Status != store.InstanceStatusOnline {
		t.Errorf("Status = %q, want %q", inst.Status, store.InstanceStatusOnline)
	}

	// Verify session was created
	_, err = fs.validateToken(resp.Token)
	if err != nil {
		t.Error("session was not created")
	}
}

func TestFleetService_Register_ExistingInstance(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Create existing instance
	existing := &store.Instance{
		ID:       "inst-1",
		Name:     "old-name",
		Hostname: "old.local",
		Status:   store.InstanceStatusOffline,
	}
	if err := s.CreateInstance(ctx, existing); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// Register with updated info
	req := &pb.RegisterRequest{
		InstanceId:   "inst-1",
		InstanceName: "new-name",
		Hostname:     "new.local",
		AgentVersion: "1.0.0",
	}

	resp, err := fs.Register(ctx, req)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if resp.Token == "" {
		t.Error("Token should not be empty")
	}

	// Verify instance was updated
	inst, err := s.GetInstance(ctx, "inst-1")
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}
	if inst.Hostname != "new.local" {
		t.Errorf("Hostname = %q, want %q", inst.Hostname, "new.local")
	}
	if inst.Status != store.InstanceStatusOnline {
		t.Errorf("Status = %q, want %q", inst.Status, store.InstanceStatusOnline)
	}
}

func TestFleetService_Register_ValidationErrors(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Missing instance_id
	_, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceName: "test",
	})
	if err == nil {
		t.Error("expected error for missing instance_id")
	}

	// Missing instance_name
	_, err = fs.Register(ctx, &pb.RegisterRequest{
		InstanceId: "inst-1",
	})
	if err == nil {
		t.Error("expected error for missing instance_name")
	}
}

func TestFleetService_Heartbeat(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Register first
	regResp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "inst-1",
		InstanceName: "test-instance",
		Hostname:     "test.local",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Send heartbeat
	req := &pb.HeartbeatRequest{
		InstanceId: "inst-1",
		Token:      regResp.Token,
		Status: &pb.InstanceStatus{
			State:   pb.InstanceState_INSTANCE_STATE_HEALTHY,
			Message: "ok",
		},
		CurrentConfigVersion: "",
		CurrentConfigHash:    "",
	}

	resp, err := fs.Heartbeat(ctx, req)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// No config assigned, so no update available
	if resp.ConfigUpdateAvailable {
		t.Error("ConfigUpdateAvailable should be false when no config assigned")
	}
}

func TestFleetService_Heartbeat_InvalidToken(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	req := &pb.HeartbeatRequest{
		InstanceId: "inst-1",
		Token:      "invalid-token",
		Status: &pb.InstanceStatus{
			State: pb.InstanceState_INSTANCE_STATE_HEALTHY,
		},
	}

	_, err := fs.Heartbeat(ctx, req)
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestFleetService_Heartbeat_TokenMismatch(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Register instance 1
	regResp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "inst-1",
		InstanceName: "test-1",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Try to use inst-1's token for inst-2
	req := &pb.HeartbeatRequest{
		InstanceId: "inst-2",
		Token:      regResp.Token,
		Status: &pb.InstanceStatus{
			State: pb.InstanceState_INSTANCE_STATE_HEALTHY,
		},
	}

	_, err = fs.Heartbeat(ctx, req)
	if err == nil {
		t.Error("expected error for token mismatch")
	}
}

func TestFleetService_Heartbeat_StatusMapping(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Register
	regResp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "inst-1",
		InstanceName: "test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	tests := []struct {
		protoState pb.InstanceState
		wantStatus store.InstanceStatus
	}{
		{pb.InstanceState_INSTANCE_STATE_HEALTHY, store.InstanceStatusOnline},
		{pb.InstanceState_INSTANCE_STATE_DEGRADED, store.InstanceStatusDegraded},
		{pb.InstanceState_INSTANCE_STATE_UNHEALTHY, store.InstanceStatusOffline},
	}

	for _, tt := range tests {
		t.Run(tt.protoState.String(), func(t *testing.T) {
			req := &pb.HeartbeatRequest{
				InstanceId: "inst-1",
				Token:      regResp.Token,
				Status: &pb.InstanceStatus{
					State: tt.protoState,
				},
			}

			_, err := fs.Heartbeat(ctx, req)
			if err != nil {
				t.Fatalf("Heartbeat failed: %v", err)
			}

			inst, _ := s.GetInstance(ctx, "inst-1")
			if inst.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", inst.Status, tt.wantStatus)
			}
		})
	}
}

func TestFleetService_Deregister(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Register first
	regResp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "inst-1",
		InstanceName: "test-instance",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Deregister
	deregResp, err := fs.Deregister(ctx, &pb.DeregisterRequest{
		InstanceId: "inst-1",
		Token:      regResp.Token,
		Reason:     "shutdown",
	})
	if err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}

	if !deregResp.Acknowledged {
		t.Error("Acknowledged should be true")
	}

	// Verify instance status is offline
	inst, _ := s.GetInstance(ctx, "inst-1")
	if inst.Status != store.InstanceStatusOffline {
		t.Errorf("Status = %q, want %q", inst.Status, store.InstanceStatusOffline)
	}

	// Token should be invalidated
	_, err = fs.validateToken(regResp.Token)
	if err == nil {
		t.Error("token should be invalidated after deregister")
	}
}

func TestFleetService_GetConfig(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Create config
	cfg := &store.Config{Name: "test-config"}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	ver := &store.ConfigVersion{
		ConfigID:    cfg.ID,
		Version:     1,
		Content:     "server {}",
		ContentHash: "abc123",
	}
	if err := s.CreateConfigVersion(ctx, ver); err != nil {
		t.Fatalf("CreateConfigVersion failed: %v", err)
	}

	// Create instance with config assigned
	inst := &store.Instance{
		ID:              "inst-1",
		Name:            "test",
		CurrentConfigID: &cfg.ID,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	// Add session token
	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	// Get config
	resp, err := fs.GetConfig(ctx, &pb.GetConfigRequest{
		InstanceId: "inst-1",
		Token:      token,
	})
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if resp.Version != "1" {
		t.Errorf("Version = %q, want %q", resp.Version, "1")
	}
	if resp.Content != "server {}" {
		t.Errorf("Content = %q, want %q", resp.Content, "server {}")
	}
	if resp.Hash != "abc123" {
		t.Errorf("Hash = %q, want %q", resp.Hash, "abc123")
	}
}

func TestFleetService_GetConfig_NoConfigAssigned(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Create instance without config
	inst := &store.Instance{
		ID:   "inst-1",
		Name: "test",
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	_, err := fs.GetConfig(ctx, &pb.GetConfigRequest{
		InstanceId: "inst-1",
		Token:      token,
	})
	if err == nil {
		t.Error("expected error when no config assigned")
	}
}

func TestFleetService_GetConfigVersion(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Create config with version
	cfg := &store.Config{Name: "test-config"}
	if err := s.CreateConfig(ctx, cfg); err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	ver := &store.ConfigVersion{
		ConfigID:    cfg.ID,
		Version:     1,
		Content:     "server {}",
		ContentHash: "abc123",
	}
	if err := s.CreateConfigVersion(ctx, ver); err != nil {
		t.Fatalf("CreateConfigVersion failed: %v", err)
	}

	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	resp, err := fs.GetConfigVersion(ctx, &pb.GetConfigVersionRequest{
		InstanceId:    "inst-1",
		Token:         token,
		ConfigId:      cfg.ID,
		VersionNumber: 1,
	})
	if err != nil {
		t.Fatalf("GetConfigVersion failed: %v", err)
	}

	if resp.ConfigId != cfg.ID {
		t.Errorf("ConfigId = %q, want %q", resp.ConfigId, cfg.ID)
	}
	if resp.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", resp.VersionNumber)
	}
	if resp.Content != "server {}" {
		t.Errorf("Content = %q, want %q", resp.Content, "server {}")
	}
}

func TestFleetService_GetConfigVersion_NotFound(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	_, err := fs.GetConfigVersion(ctx, &pb.GetConfigVersionRequest{
		InstanceId:    "inst-1",
		Token:         token,
		ConfigId:      "nonexistent",
		VersionNumber: 1,
	})
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestFleetService_AckDeployment(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	// Accepted
	resp, err := fs.AckDeployment(ctx, &pb.AckDeploymentRequest{
		InstanceId:   "inst-1",
		Token:        token,
		DeploymentId: "dep-1",
		Accepted:     true,
	})
	if err != nil {
		t.Fatalf("AckDeployment failed: %v", err)
	}

	if !resp.Acknowledged {
		t.Error("Acknowledged should be true")
	}
	if resp.Instruction != "proceed with deployment" {
		t.Errorf("Instruction = %q, want %q", resp.Instruction, "proceed with deployment")
	}

	// Rejected
	resp, err = fs.AckDeployment(ctx, &pb.AckDeploymentRequest{
		InstanceId:      "inst-1",
		Token:           token,
		DeploymentId:    "dep-2",
		Accepted:        false,
		RejectionReason: "busy",
	})
	if err != nil {
		t.Fatalf("AckDeployment failed: %v", err)
	}

	if resp.Instruction != "deployment rejected by agent" {
		t.Errorf("Instruction = %q, want %q", resp.Instruction, "deployment rejected by agent")
	}
}

func TestFleetService_ReportDeploymentStatus(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Create instance
	inst := &store.Instance{
		ID:   "inst-1",
		Name: "test",
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	// Track handler calls
	var handlerCalled bool
	var handlerInstanceID, handlerDeploymentID string
	var handlerState pb.DeploymentState

	fs.SetDeploymentStatusHandler(func(instanceID, deploymentID string, state pb.DeploymentState, message, errorDetails string) {
		handlerCalled = true
		handlerInstanceID = instanceID
		handlerDeploymentID = deploymentID
		handlerState = state
	})

	resp, err := fs.ReportDeploymentStatus(ctx, &pb.DeploymentStatusRequest{
		InstanceId:   "inst-1",
		Token:        token,
		DeploymentId: "dep-1",
		State:        pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
		Message:      "done",
	})
	if err != nil {
		t.Fatalf("ReportDeploymentStatus failed: %v", err)
	}

	if !resp.Acknowledged {
		t.Error("Acknowledged should be true")
	}

	// Verify handler was called
	if !handlerCalled {
		t.Error("handler was not called")
	}
	if handlerInstanceID != "inst-1" {
		t.Errorf("handler instanceID = %q, want %q", handlerInstanceID, "inst-1")
	}
	if handlerDeploymentID != "dep-1" {
		t.Errorf("handler deploymentID = %q, want %q", handlerDeploymentID, "dep-1")
	}
	if handlerState != pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
		t.Errorf("handler state = %v, want COMPLETED", handlerState)
	}

	// Verify instance status was updated
	inst, _ = s.GetInstance(ctx, "inst-1")
	if inst.Status != store.InstanceStatusOnline {
		t.Errorf("Status = %q, want %q", inst.Status, store.InstanceStatusOnline)
	}
}

func TestFleetService_ReportDeploymentStatus_StateMapping(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	inst := &store.Instance{
		ID:   "inst-1",
		Name: "test",
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}

	token := "test-token"
	fs.sessions[hashToken(token)] = "inst-1"

	tests := []struct {
		state      pb.DeploymentState
		wantStatus store.InstanceStatus
	}{
		{pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS, store.InstanceStatusDeploying},
		{pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED, store.InstanceStatusOnline},
		{pb.DeploymentState_DEPLOYMENT_STATE_FAILED, store.InstanceStatusDegraded},
		{pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK, store.InstanceStatusOnline},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			_, err := fs.ReportDeploymentStatus(ctx, &pb.DeploymentStatusRequest{
				InstanceId:   "inst-1",
				Token:        token,
				DeploymentId: "dep-1",
				State:        tt.state,
			})
			if err != nil {
				t.Fatalf("ReportDeploymentStatus failed: %v", err)
			}

			inst, _ := s.GetInstance(ctx, "inst-1")
			if inst.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", inst.Status, tt.wantStatus)
			}
		})
	}
}

func TestFleetService_GetSubscriberCount(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	if fs.GetSubscriberCount() != 0 {
		t.Errorf("initial count = %d, want 0", fs.GetSubscriberCount())
	}

	// Add subscribers
	fs.subscribers["inst-1"] = make(chan *pb.Event, 10)
	fs.subscribers["inst-2"] = make(chan *pb.Event, 10)

	if fs.GetSubscriberCount() != 2 {
		t.Errorf("count = %d, want 2", fs.GetSubscriberCount())
	}
}

func TestFleetService_IsInstanceSubscribed(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	if fs.IsInstanceSubscribed("inst-1") {
		t.Error("inst-1 should not be subscribed initially")
	}

	fs.subscribers["inst-1"] = make(chan *pb.Event, 10)

	if !fs.IsInstanceSubscribed("inst-1") {
		t.Error("inst-1 should be subscribed")
	}

	if fs.IsInstanceSubscribed("inst-2") {
		t.Error("inst-2 should not be subscribed")
	}
}

func TestFleetService_SendEventToInstance(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	// Not subscribed
	event := &pb.Event{EventId: "evt-1", Type: pb.EventType_EVENT_TYPE_PING}
	err := fs.SendEventToInstance("inst-1", event)
	if err == nil {
		t.Error("expected error for unsubscribed instance")
	}

	// Subscribe
	ch := make(chan *pb.Event, 10)
	fs.subscribers["inst-1"] = ch

	err = fs.SendEventToInstance("inst-1", event)
	if err != nil {
		t.Fatalf("SendEventToInstance failed: %v", err)
	}

	// Check event was sent
	select {
	case received := <-ch:
		if received.EventId != "evt-1" {
			t.Errorf("EventId = %q, want %q", received.EventId, "evt-1")
		}
	default:
		t.Error("event was not sent")
	}
}

func TestFleetService_SendEventToInstance_ChannelFull(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	// Create a full channel (capacity 1)
	ch := make(chan *pb.Event, 1)
	ch <- &pb.Event{EventId: "existing"}
	fs.subscribers["inst-1"] = ch

	event := &pb.Event{EventId: "new"}
	err := fs.SendEventToInstance("inst-1", event)
	if err == nil {
		t.Error("expected error for full channel")
	}
}

func TestFleetService_BroadcastEvent(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ch1 := make(chan *pb.Event, 10)
	ch2 := make(chan *pb.Event, 10)
	fs.subscribers["inst-1"] = ch1
	fs.subscribers["inst-2"] = ch2

	event := &pb.Event{EventId: "broadcast-1", Type: pb.EventType_EVENT_TYPE_PING}
	fs.BroadcastEvent(event)

	// Both should receive
	select {
	case received := <-ch1:
		if received.EventId != "broadcast-1" {
			t.Errorf("inst-1 EventId = %q, want %q", received.EventId, "broadcast-1")
		}
	default:
		t.Error("inst-1 did not receive event")
	}

	select {
	case received := <-ch2:
		if received.EventId != "broadcast-1" {
			t.Errorf("inst-2 EventId = %q, want %q", received.EventId, "broadcast-1")
		}
	default:
		t.Error("inst-2 did not receive event")
	}
}

func TestFleetService_NotifyConfigUpdate(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ch := make(chan *pb.Event, 10)
	fs.subscribers["inst-1"] = ch

	fs.NotifyConfigUpdate([]string{"inst-1"}, "1", "hash123", "updated routing")

	select {
	case received := <-ch:
		if received.Type != pb.EventType_EVENT_TYPE_CONFIG_UPDATE {
			t.Errorf("Type = %v, want CONFIG_UPDATE", received.Type)
		}
		update := received.GetConfigUpdate()
		if update == nil {
			t.Fatal("ConfigUpdate payload is nil")
		}
		if update.ConfigVersion != "1" {
			t.Errorf("ConfigVersion = %q, want %q", update.ConfigVersion, "1")
		}
		if update.ConfigHash != "hash123" {
			t.Errorf("ConfigHash = %q, want %q", update.ConfigHash, "hash123")
		}
		if update.ChangeSummary != "updated routing" {
			t.Errorf("ChangeSummary = %q, want %q", update.ChangeSummary, "updated routing")
		}
	default:
		t.Error("event was not sent")
	}
}

func TestFleetService_NotifyDeployment(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ch := make(chan *pb.Event, 10)
	fs.subscribers["inst-1"] = ch

	deadline := time.Now().Add(5 * time.Minute)
	err := fs.NotifyDeployment("inst-1", "dep-1", "cfg-1", "1", pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING, 1, 3, deadline, false)
	if err != nil {
		t.Fatalf("NotifyDeployment failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.Type != pb.EventType_EVENT_TYPE_DEPLOYMENT {
			t.Errorf("Type = %v, want DEPLOYMENT", received.Type)
		}
		dep := received.GetDeployment()
		if dep == nil {
			t.Fatal("Deployment payload is nil")
		}
		if dep.DeploymentId != "dep-1" {
			t.Errorf("DeploymentId = %q, want %q", dep.DeploymentId, "dep-1")
		}
		if dep.ConfigId != "cfg-1" {
			t.Errorf("ConfigId = %q, want %q", dep.ConfigId, "cfg-1")
		}
		if dep.ConfigVersion != "1" {
			t.Errorf("ConfigVersion = %q, want %q", dep.ConfigVersion, "1")
		}
		if dep.Strategy != pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING {
			t.Errorf("Strategy = %v, want ROLLING", dep.Strategy)
		}
		if dep.BatchPosition != 1 {
			t.Errorf("BatchPosition = %d, want 1", dep.BatchPosition)
		}
		if dep.BatchTotal != 3 {
			t.Errorf("BatchTotal = %d, want 3", dep.BatchTotal)
		}
		if dep.IsRollback {
			t.Error("IsRollback should be false")
		}
	default:
		t.Error("event was not sent")
	}
}

func TestFleetService_NotifyDeployment_NotSubscribed(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	err := fs.NotifyDeployment("inst-1", "dep-1", "cfg-1", "1", pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING, 1, 1, time.Now(), false)
	if err == nil {
		t.Error("expected error for unsubscribed instance")
	}
}

func TestFleetService_SetDeploymentStatusHandler(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	if fs.deploymentStatusHandler != nil {
		t.Error("handler should be nil initially")
	}

	handler := func(instanceID, deploymentID string, state pb.DeploymentState, message, errorDetails string) {
		// Handler implementation
	}

	fs.SetDeploymentStatusHandler(handler)

	if fs.deploymentStatusHandler == nil {
		t.Error("handler should be set")
	}
}

func TestFleetService_ConcurrentAccess(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	ctx := context.Background()

	// Register multiple instances concurrently
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			fs.Register(ctx, &pb.RegisterRequest{
				InstanceId:   "inst-" + string(rune('0'+n)),
				InstanceName: "test-" + string(rune('0'+n)),
			})
		}(i)
	}

	// Concurrent subscriber operations
	for i := 0; i < 10; i++ {
		go func(n int) {
			id := "sub-" + string(rune('0'+n))
			fs.subscribers[id] = make(chan *pb.Event, 10)
			_ = fs.IsInstanceSubscribed(id)
			_ = fs.GetSubscriberCount()
		}(i)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(done)
	}()

	<-done
}
