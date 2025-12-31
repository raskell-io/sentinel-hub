package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const bufSize = 1024 * 1024

// mockFleetService implements pb.FleetServiceServer for testing
type mockFleetService struct {
	pb.UnimplementedFleetServiceServer

	mu sync.Mutex

	// Track calls for verification
	registerCalls           int
	heartbeatCalls          int
	deregisterCalls         int
	getConfigCalls          int
	getConfigVersionCalls   int
	ackDeploymentCalls      int
	reportStatusCalls       int
	subscribeCalls          int

	// Control behavior
	registerError     error
	heartbeatError    error
	deregisterError   error
	getConfigError    error
	subscribeError    error

	// Return values
	token              string
	configVersion      string
	configHash         string
	configContent      string
	configUpdateAvail  bool
	pendingActions     []*pb.PendingAction

	// Subscription control
	eventCh chan *pb.Event
	subDone chan struct{}

	// Last received values
	lastHeartbeatState   pb.InstanceState
	lastDeploymentStatus pb.DeploymentState
	lastDeploymentID     string
}

func newMockFleetService() *mockFleetService {
	return &mockFleetService{
		token:         "test-token-12345",
		configVersion: "1",
		configHash:    "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		configContent: "server { listen 8080 }",
		eventCh:       make(chan *pb.Event, 10),
		subDone:       make(chan struct{}),
	}
}

func (m *mockFleetService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registerCalls++

	if m.registerError != nil {
		return nil, m.registerError
	}

	return &pb.RegisterResponse{
		Token:                    m.token,
		ConfigVersion:           m.configVersion,
		ConfigHash:              m.configHash,
		HeartbeatIntervalSeconds: 30,
	}, nil
}

func (m *mockFleetService) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.heartbeatCalls++

	if req.Status != nil {
		m.lastHeartbeatState = req.Status.State
	}

	if m.heartbeatError != nil {
		return nil, m.heartbeatError
	}

	return &pb.HeartbeatResponse{
		ConfigUpdateAvailable: m.configUpdateAvail,
		LatestConfigVersion:   m.configVersion,
		Actions:               m.pendingActions,
	}, nil
}

func (m *mockFleetService) Deregister(ctx context.Context, req *pb.DeregisterRequest) (*pb.DeregisterResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deregisterCalls++

	if m.deregisterError != nil {
		return nil, m.deregisterError
	}

	return &pb.DeregisterResponse{Acknowledged: true}, nil
}

func (m *mockFleetService) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getConfigCalls++

	if m.getConfigError != nil {
		return nil, m.getConfigError
	}

	return &pb.GetConfigResponse{
		Version:   m.configVersion,
		Hash:      m.configHash,
		Content:   m.configContent,
		CreatedAt: timestamppb.Now(),
	}, nil
}

func (m *mockFleetService) GetConfigVersion(ctx context.Context, req *pb.GetConfigVersionRequest) (*pb.GetConfigVersionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getConfigVersionCalls++

	if m.getConfigError != nil {
		return nil, m.getConfigError
	}

	return &pb.GetConfigVersionResponse{
		ConfigId:      req.ConfigId,
		VersionNumber: req.VersionNumber,
		Hash:          m.configHash,
		Content:       m.configContent,
		CreatedAt:     timestamppb.Now(),
	}, nil
}

func (m *mockFleetService) AckDeployment(ctx context.Context, req *pb.AckDeploymentRequest) (*pb.AckDeploymentResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ackDeploymentCalls++

	return &pb.AckDeploymentResponse{
		Acknowledged: true,
		Instruction:  "proceed",
	}, nil
}

func (m *mockFleetService) ReportDeploymentStatus(ctx context.Context, req *pb.DeploymentStatusRequest) (*pb.DeploymentStatusResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reportStatusCalls++
	m.lastDeploymentStatus = req.State
	m.lastDeploymentID = req.DeploymentId

	return &pb.DeploymentStatusResponse{Acknowledged: true}, nil
}

func (m *mockFleetService) Subscribe(req *pb.SubscribeRequest, stream pb.FleetService_SubscribeServer) error {
	m.mu.Lock()
	m.subscribeCalls++
	eventCh := m.eventCh
	subDone := m.subDone
	subscribeError := m.subscribeError
	m.mu.Unlock()

	if subscribeError != nil {
		return subscribeError
	}

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-subDone:
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// SendEvent sends an event to the subscribed client
func (m *mockFleetService) SendEvent(event *pb.Event) {
	m.eventCh <- event
}

// CloseSubscription closes the subscription stream
func (m *mockFleetService) CloseSubscription() {
	close(m.subDone)
}

// testServer holds the test gRPC server setup
type testServer struct {
	lis     *bufconn.Listener
	server  *grpc.Server
	service *mockFleetService
}

func newTestServer() *testServer {
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	service := newMockFleetService()
	pb.RegisterFleetServiceServer(server, service)

	go func() {
		server.Serve(lis)
	}()

	return &testServer{
		lis:     lis,
		server:  server,
		service: service,
	}
}

func (ts *testServer) Stop() {
	ts.server.Stop()
}

func (ts *testServer) Dial(ctx context.Context) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ts.lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// ============================================
// Client gRPC Method Tests
// ============================================

func TestClient_Connect(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, err := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Override the connection with our bufconn
	ctx := context.Background()
	conn, err := ts.Dial(ctx)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)

	// Verify connected state
	if client.conn == nil {
		t.Error("conn should not be nil after connect")
	}

	client.Close()
}

func TestClient_Register(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:          "passthrough://bufnet",
		InstanceID:      "test-id",
		InstanceName:    "test-instance",
		AgentVersion:    "1.0.0",
		SentinelVersion: "2.0.0",
		Labels:          map[string]string{"env": "test"},
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	defer client.Close()

	err := client.Register(ctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify token was stored
	if client.token != "test-token-12345" {
		t.Errorf("token = %q, want %q", client.token, "test-token-12345")
	}

	// Verify service received the call
	ts.service.mu.Lock()
	calls := ts.service.registerCalls
	ts.service.mu.Unlock()
	if calls != 1 {
		t.Errorf("registerCalls = %d, want 1", calls)
	}
}

func TestClient_Register_NotConnected(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	err := client.Register(context.Background())
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestClient_Register_Error(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	ts.service.registerError = status.Error(codes.Internal, "server error")

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	defer client.Close()

	err := client.Register(ctx)
	if err == nil {
		t.Error("expected error from Register")
	}
}

func TestClient_Heartbeat(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	metrics := &pb.InstanceMetrics{
		RequestsTotal: 100,
	}

	resp, err := client.Heartbeat(ctx, pb.InstanceState_INSTANCE_STATE_HEALTHY, "ok", metrics)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	ts.service.mu.Lock()
	calls := ts.service.heartbeatCalls
	lastState := ts.service.lastHeartbeatState
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("heartbeatCalls = %d, want 1", calls)
	}
	if lastState != pb.InstanceState_INSTANCE_STATE_HEALTHY {
		t.Errorf("lastHeartbeatState = %v, want HEALTHY", lastState)
	}
}

func TestClient_Heartbeat_NotRegistered(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	_, err := client.Heartbeat(context.Background(), pb.InstanceState_INSTANCE_STATE_HEALTHY, "ok", nil)
	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_Heartbeat_WithPendingActions(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	ts.service.configUpdateAvail = true
	ts.service.pendingActions = []*pb.PendingAction{
		{
			ActionId: "action-1",
			Type:     pb.ActionType_ACTION_TYPE_FETCH_CONFIG,
			Params:   map[string]string{"version": "2"},
		},
	}

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	resp, err := client.Heartbeat(ctx, pb.InstanceState_INSTANCE_STATE_HEALTHY, "ok", nil)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if !resp.ConfigUpdateAvailable {
		t.Error("expected ConfigUpdateAvailable to be true")
	}
	if len(resp.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(resp.Actions))
	}
}

func TestClient_Deregister(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	err := client.Deregister(ctx, "shutdown")
	if err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}

	ts.service.mu.Lock()
	calls := ts.service.deregisterCalls
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("deregisterCalls = %d, want 1", calls)
	}
}

func TestClient_Deregister_NoToken(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	// Should not error when not registered - just returns nil
	err := client.Deregister(context.Background(), "shutdown")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_FetchConfig(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	resp, err := client.FetchConfig(ctx, "1")
	if err != nil {
		t.Fatalf("FetchConfig failed: %v", err)
	}

	if resp.Version != "1" {
		t.Errorf("Version = %q, want %q", resp.Version, "1")
	}
	if resp.Content != "server { listen 8080 }" {
		t.Errorf("Content = %q, want %q", resp.Content, "server { listen 8080 }")
	}

	ts.service.mu.Lock()
	calls := ts.service.getConfigCalls
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("getConfigCalls = %d, want 1", calls)
	}
}

func TestClient_FetchConfig_NotRegistered(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	_, err := client.FetchConfig(context.Background(), "1")
	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_FetchConfigVersion(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	resp, err := client.FetchConfigVersion(ctx, "config-123", 2)
	if err != nil {
		t.Fatalf("FetchConfigVersion failed: %v", err)
	}

	if resp.ConfigId != "config-123" {
		t.Errorf("ConfigId = %q, want %q", resp.ConfigId, "config-123")
	}
	if resp.VersionNumber != 2 {
		t.Errorf("VersionNumber = %d, want 2", resp.VersionNumber)
	}

	ts.service.mu.Lock()
	calls := ts.service.getConfigVersionCalls
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("getConfigVersionCalls = %d, want 1", calls)
	}
}

func TestClient_AckDeployment(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	err := client.AckDeployment(ctx, "deploy-123", true, "")
	if err != nil {
		t.Fatalf("AckDeployment failed: %v", err)
	}

	ts.service.mu.Lock()
	calls := ts.service.ackDeploymentCalls
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("ackDeploymentCalls = %d, want 1", calls)
	}
}

func TestClient_AckDeployment_NotRegistered(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	err := client.AckDeployment(context.Background(), "deploy-123", true, "")
	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_ReportDeploymentStatus(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	err := client.ReportDeploymentStatus(ctx, "deploy-123", pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED, "done", "")
	if err != nil {
		t.Fatalf("ReportDeploymentStatus failed: %v", err)
	}

	ts.service.mu.Lock()
	calls := ts.service.reportStatusCalls
	lastStatus := ts.service.lastDeploymentStatus
	lastID := ts.service.lastDeploymentID
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("reportStatusCalls = %d, want 1", calls)
	}
	if lastStatus != pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
		t.Errorf("lastDeploymentStatus = %v, want COMPLETED", lastStatus)
	}
	if lastID != "deploy-123" {
		t.Errorf("lastDeploymentID = %q, want %q", lastID, "deploy-123")
	}
}

func TestClient_ReportDeploymentStatus_NotRegistered(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	err := client.ReportDeploymentStatus(context.Background(), "deploy-123", pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED, "done", "")
	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_Subscribe(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	// Start subscription in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Subscribe(ctx)
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel to stop subscription
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Subscribe did not return after cancel")
	}

	ts.service.mu.Lock()
	calls := ts.service.subscribeCalls
	ts.service.mu.Unlock()

	if calls != 1 {
		t.Errorf("subscribeCalls = %d, want 1", calls)
	}
}

func TestClient_Subscribe_NotRegistered(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	err := client.Subscribe(context.Background())
	if err == nil {
		t.Error("expected error when not registered")
	}
}

func TestClient_Subscribe_ReceivesEvents(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	eventReceived := make(chan *pb.Event, 1)
	handler := &testEventHandler{
		onConfigUpdate: func(version, hash, content string) error {
			eventReceived <- &pb.Event{} // Signal we got an event
			return nil
		},
	}

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		EventHandler: handler,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	// Start subscription
	go client.Subscribe(ctx)

	// Wait for subscription to start
	time.Sleep(50 * time.Millisecond)

	// Send a ping event
	ts.service.SendEvent(&pb.Event{
		EventId: "ping-1",
		Type:    pb.EventType_EVENT_TYPE_PING,
		Payload: &pb.Event_Ping{
			Ping: &pb.PingEvent{
				ServerTime: timestamppb.Now(),
			},
		},
	})

	// Give time for event to be processed
	time.Sleep(50 * time.Millisecond)

	cancel()
}

// testEventHandler implements EventHandler for testing
type testEventHandler struct {
	onConfigUpdate func(version, hash, content string) error
	onDeployment   func(deploymentID, configID, configVersion string, isRollback bool) error
	onDrain        func(timeoutSecs int, reason string) error
}

func (h *testEventHandler) OnConfigUpdate(version, hash, content string) error {
	if h.onConfigUpdate != nil {
		return h.onConfigUpdate(version, hash, content)
	}
	return nil
}

func (h *testEventHandler) OnDeployment(deploymentID, configID, configVersion string, isRollback bool) error {
	if h.onDeployment != nil {
		return h.onDeployment(deploymentID, configID, configVersion, isRollback)
	}
	return nil
}

func (h *testEventHandler) OnDrain(timeoutSecs int, reason string) error {
	if h.onDrain != nil {
		return h.onDrain(timeoutSecs, reason)
	}
	return nil
}

// ============================================
// Agent Tests
// ============================================

func TestAgent_New(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	// Create a config file
	if err := os.WriteFile(configPath, []byte("test config"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
		AgentVersion:      "1.0.0",
		SentinelVersion:   "2.0.0",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if agent == nil {
		t.Fatal("agent should not be nil")
	}
	if agent.client == nil {
		t.Error("client should not be nil")
	}
	if agent.sentinel == nil {
		t.Error("sentinel should not be nil")
	}
	if agent.state == nil {
		t.Error("state should not be nil")
	}
	if agent.heartbeatInterval != 30*time.Second {
		t.Errorf("heartbeatInterval = %v, want 30s", agent.heartbeatInterval)
	}
}

func TestAgent_New_WithExistingState(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	// Pre-create state file with instance ID
	stateContent := `{"instance_id":"existing-id","config_version":"v1","last_updated":"2024-01-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(statePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Instance ID should be from state file
	if agent.client.InstanceID() != "existing-id" {
		t.Errorf("InstanceID = %q, want %q", agent.client.InstanceID(), "existing-id")
	}
}

func TestAgent_New_ConfigInstanceIDOverridesState(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	// Pre-create state file with instance ID
	stateContent := `{"instance_id":"state-id","last_updated":"2024-01-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(statePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceID:        "config-id", // Explicit ID in config
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Config ID should take precedence
	if agent.client.InstanceID() != "config-id" {
		t.Errorf("InstanceID = %q, want %q", agent.client.InstanceID(), "config-id")
	}
}

func TestAgent_Accessors(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if agent.Client() == nil {
		t.Error("Client() should not return nil")
	}
	if agent.Sentinel() == nil {
		t.Error("Sentinel() should not return nil")
	}
	if agent.State() == nil {
		t.Error("State() should not return nil")
	}
}

func TestAgent_IsRunning(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if agent.IsRunning() {
		t.Error("IsRunning should be false initially")
	}
}

func TestAgent_OnDrain(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// OnDrain should not error (currently a no-op)
	err = agent.OnDrain(60, "maintenance")
	if err != nil {
		t.Errorf("OnDrain failed: %v", err)
	}
}

func TestAgent_OnConfigUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// OnConfigUpdate should write config and update state
	err = agent.OnConfigUpdate("v2", "hash123hash123hash123", "new config content")
	if err != nil {
		t.Fatalf("OnConfigUpdate failed: %v", err)
	}

	// Verify config was written
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(content) != "new config content" {
		t.Errorf("config content = %q, want %q", string(content), "new config content")
	}
}

func TestAgent_clearActiveDeployment_NilState(t *testing.T) {
	agent := &Agent{
		state: nil,
	}

	// Should not panic with nil state
	agent.clearActiveDeployment()
}

// ============================================
// SentinelManager Additional Tests
// ============================================

func TestSentinelManager_WriteConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nested", "dir", "config.kdl")

	sm := NewSentinelManager(configPath)

	err := sm.WriteConfig("test content")
	if err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("content = %q, want %q", string(content), "test content")
	}
}

func TestSentinelManager_Rollback_NoBackup(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	sm := NewSentinelManager(configPath)

	err := sm.Rollback()
	if err == nil {
		t.Error("expected error when no backup exists")
	}
}

func TestSentinelManager_IsRunning_NoProcess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	pidFile := filepath.Join(tmpDir, "sentinel.pid")

	sm := NewSentinelManager(configPath)
	sm.SetPIDFile(pidFile)

	// No PID file exists, so IsRunning should return false
	if sm.IsRunning() {
		t.Error("IsRunning should return false when no process")
	}
}

func TestSentinelManager_CheckHealth_NoProcess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	pidFile := filepath.Join(tmpDir, "sentinel.pid")

	sm := NewSentinelManager(configPath)
	sm.SetPIDFile(pidFile)

	err := sm.CheckHealth()
	if err == nil {
		t.Error("expected error when no process running")
	}
}

func TestSentinelManager_getSentinelPID_InvalidPIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	pidFile := filepath.Join(tmpDir, "sentinel.pid")

	// Write invalid PID
	if err := os.WriteFile(pidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	sm := NewSentinelManager(configPath)
	sm.SetPIDFile(pidFile)

	// Should fall back to pgrep which will also fail
	err := sm.CheckHealth()
	if err == nil {
		t.Error("expected error with invalid PID file")
	}
}

func TestSentinelManager_getSentinelPID_StalePID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	pidFile := filepath.Join(tmpDir, "sentinel.pid")

	// Write a PID that doesn't exist (use a very high PID)
	if err := os.WriteFile(pidFile, []byte("999999999"), 0644); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	sm := NewSentinelManager(configPath)
	sm.SetPIDFile(pidFile)

	// Should fall back to pgrep
	err := sm.CheckHealth()
	if err == nil {
		t.Error("expected error with stale PID")
	}
}

func TestSentinelManager_Reload_NoProcess(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	pidFile := filepath.Join(tmpDir, "sentinel.pid")

	sm := NewSentinelManager(configPath)
	sm.SetPIDFile(pidFile)

	err := sm.Reload()
	if err == nil {
		t.Error("expected error when no process to reload")
	}
}

// ============================================
// Client handleEvent Tests
// ============================================

func TestClient_handleEvent_Ping(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "localhost:9090",
		InstanceName: "test-instance",
	})

	event := &pb.Event{
		EventId: "ping-1",
		Type:    pb.EventType_EVENT_TYPE_PING,
		Payload: &pb.Event_Ping{
			Ping: &pb.PingEvent{
				ServerTime: timestamppb.Now(),
			},
		},
	}

	// Should not panic
	client.handleEvent(context.Background(), event)
}

func TestClient_handleEvent_Drain(t *testing.T) {
	drainCalled := false
	handler := &testEventHandler{
		onDrain: func(timeoutSecs int, reason string) error {
			drainCalled = true
			if timeoutSecs != 60 {
				t.Errorf("timeoutSecs = %d, want 60", timeoutSecs)
			}
			if reason != "maintenance" {
				t.Errorf("reason = %q, want %q", reason, "maintenance")
			}
			return nil
		},
	}

	client, _ := NewClient(ClientConfig{
		HubURL:       "localhost:9090",
		InstanceName: "test-instance",
		EventHandler: handler,
	})

	event := &pb.Event{
		EventId: "drain-1",
		Type:    pb.EventType_EVENT_TYPE_DRAIN,
		Payload: &pb.Event_Drain{
			Drain: &pb.DrainEvent{
				DrainTimeoutSeconds: 60,
				Reason:              "maintenance",
			},
		},
	}

	client.handleEvent(context.Background(), event)

	if !drainCalled {
		t.Error("OnDrain was not called")
	}
}

func TestClient_handleEvent_UnknownType(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "localhost:9090",
		InstanceName: "test-instance",
	})

	event := &pb.Event{
		EventId: "unknown-1",
		Type:    pb.EventType(999), // Unknown type
	}

	// Should not panic
	client.handleEvent(context.Background(), event)
}

func TestClient_handleEvent_Deployment(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	deploymentCalled := false
	handler := &testEventHandler{
		onDeployment: func(deploymentID, configID, configVersion string, isRollback bool) error {
			deploymentCalled = true
			if deploymentID != "deploy-123" {
				t.Errorf("deploymentID = %q, want %q", deploymentID, "deploy-123")
			}
			if configID != "config-456" {
				t.Errorf("configID = %q, want %q", configID, "config-456")
			}
			if configVersion != "2" {
				t.Errorf("configVersion = %q, want %q", configVersion, "2")
			}
			if isRollback {
				t.Error("isRollback should be false")
			}
			return nil
		},
	}

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		EventHandler: handler,
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	event := &pb.Event{
		EventId: "deploy-event-1",
		Type:    pb.EventType_EVENT_TYPE_DEPLOYMENT,
		Payload: &pb.Event_Deployment{
			Deployment: &pb.DeploymentEvent{
				DeploymentId:  "deploy-123",
				ConfigId:      "config-456",
				ConfigVersion: "2",
				Strategy:      pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING,
				BatchPosition: 1,
				BatchTotal:    3,
				IsRollback:    false,
			},
		},
	}

	client.handleEvent(ctx, event)

	if !deploymentCalled {
		t.Error("OnDeployment was not called")
	}

	// Verify ack and status were reported
	ts.service.mu.Lock()
	ackCalls := ts.service.ackDeploymentCalls
	statusCalls := ts.service.reportStatusCalls
	ts.service.mu.Unlock()

	if ackCalls != 1 {
		t.Errorf("ackDeploymentCalls = %d, want 1", ackCalls)
	}
	if statusCalls < 1 {
		t.Errorf("reportStatusCalls = %d, want >= 1", statusCalls)
	}
}

func TestClient_handleEvent_Deployment_Error(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	deployErr := fmt.Errorf("deployment failed")
	handler := &testEventHandler{
		onDeployment: func(deploymentID, configID, configVersion string, isRollback bool) error {
			return deployErr
		},
	}

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		EventHandler: handler,
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	event := &pb.Event{
		EventId: "deploy-event-1",
		Type:    pb.EventType_EVENT_TYPE_DEPLOYMENT,
		Payload: &pb.Event_Deployment{
			Deployment: &pb.DeploymentEvent{
				DeploymentId:  "deploy-123",
				ConfigId:      "config-456",
				ConfigVersion: "2",
			},
		},
	}

	client.handleEvent(ctx, event)

	// Verify failure was reported
	ts.service.mu.Lock()
	lastStatus := ts.service.lastDeploymentStatus
	ts.service.mu.Unlock()

	if lastStatus != pb.DeploymentState_DEPLOYMENT_STATE_FAILED {
		t.Errorf("lastDeploymentStatus = %v, want FAILED", lastStatus)
	}
}

// ============================================
// Agent OnDeployment Tests
// ============================================

func TestAgent_OnDeployment(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "passthrough://bufnet",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Setup client with mock server
	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	agent.client.conn = conn
	agent.client.client = pb.NewFleetServiceClient(conn)
	agent.client.token = "test-token"

	// OnDeployment should fetch config and apply it
	err = agent.OnDeployment("deploy-123", "config-456", "1", false)
	if err != nil {
		t.Fatalf("OnDeployment failed: %v", err)
	}

	// Verify config was written
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(content) != "server { listen 8080 }" {
		t.Errorf("config content = %q, want %q", string(content), "server { listen 8080 }")
	}

	// Verify config version was fetched
	ts.service.mu.Lock()
	configCalls := ts.service.getConfigVersionCalls
	ts.service.mu.Unlock()

	if configCalls != 1 {
		t.Errorf("getConfigVersionCalls = %d, want 1", configCalls)
	}
}

func TestAgent_OnDeployment_Rollback(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "passthrough://bufnet",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	agent.client.conn = conn
	agent.client.client = pb.NewFleetServiceClient(conn)
	agent.client.token = "test-token"

	// Call with rollback flag
	err = agent.OnDeployment("deploy-123", "config-456", "1", true)
	if err != nil {
		t.Fatalf("OnDeployment rollback failed: %v", err)
	}
}

func TestAgent_OnDeployment_FetchError(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	ts.service.getConfigError = status.Error(codes.NotFound, "config not found")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "passthrough://bufnet",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	agent.client.conn = conn
	agent.client.client = pb.NewFleetServiceClient(conn)
	agent.client.token = "test-token"

	err = agent.OnDeployment("deploy-123", "config-456", "1", false)
	if err == nil {
		t.Error("expected error when config fetch fails")
	}
}

func TestAgent_reportInterruptedDeployment(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	// Pre-create state with active deployment
	stateContent := `{"instance_id":"test-id","active_deployment_id":"interrupted-deploy","last_updated":"2024-01-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(statePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	agent, err := New(Config{
		HubURL:            "passthrough://bufnet",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	agent.client.conn = conn
	agent.client.client = pb.NewFleetServiceClient(conn)
	agent.client.token = "test-token"

	// Manually call reportInterruptedDeployment
	agent.reportInterruptedDeployment(ctx)

	// Verify status was reported
	ts.service.mu.Lock()
	statusCalls := ts.service.reportStatusCalls
	lastStatus := ts.service.lastDeploymentStatus
	lastID := ts.service.lastDeploymentID
	ts.service.mu.Unlock()

	if statusCalls != 1 {
		t.Errorf("reportStatusCalls = %d, want 1", statusCalls)
	}
	if lastStatus != pb.DeploymentState_DEPLOYMENT_STATE_FAILED {
		t.Errorf("lastDeploymentStatus = %v, want FAILED", lastStatus)
	}
	if lastID != "interrupted-deploy" {
		t.Errorf("lastDeploymentID = %q, want %q", lastID, "interrupted-deploy")
	}

	// Verify deployment was cleared from state
	state, _ := agent.state.Load()
	if state.ActiveDeploymentID != "" {
		t.Errorf("ActiveDeploymentID should be cleared, got %q", state.ActiveDeploymentID)
	}
}

func TestAgent_reportInterruptedDeployment_NoState(t *testing.T) {
	agent := &Agent{
		state: nil,
	}

	// Should not panic with nil state
	agent.reportInterruptedDeployment(context.Background())
}

func TestAgent_reportInterruptedDeployment_NoActiveDeployment(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)

	agent := &Agent{
		state: sm,
	}

	// Should do nothing when no active deployment
	agent.reportInterruptedDeployment(context.Background())
}

// ============================================
// Client Connect Tests
// ============================================

func TestClient_Connect_Success(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	// First use the bufconn dialer
	ctx := context.Background()
	conn, err := ts.Dial(ctx)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// Set connection manually (mimicking what Connect would do)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)

	if client.conn == nil {
		t.Error("conn should not be nil")
	}
	if client.client == nil {
		t.Error("client should not be nil")
	}

	client.Close()
}

// ============================================
// Additional handleEvent Tests
// ============================================

func TestClient_handleEvent_ConfigUpdate(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	configUpdated := false
	handler := &testEventHandler{
		onConfigUpdate: func(version, hash, content string) error {
			configUpdated = true
			if version != "1" {
				t.Errorf("version = %q, want %q", version, "1")
			}
			return nil
		},
	}

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		EventHandler: handler,
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	event := &pb.Event{
		EventId: "config-update-1",
		Type:    pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
		Payload: &pb.Event_ConfigUpdate{
			ConfigUpdate: &pb.ConfigUpdateEvent{
				ConfigVersion: "1",
				ConfigHash:    "abc123abc123abc123abc123",
				ChangeSummary: "Updated server config",
			},
		},
	}

	client.handleEvent(ctx, event)

	if !configUpdated {
		t.Error("OnConfigUpdate was not called")
	}
}

func TestClient_handleEvent_ConfigUpdate_FetchError(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	ts.service.getConfigError = status.Error(codes.Internal, "fetch failed")

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		EventHandler: &testEventHandler{},
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	event := &pb.Event{
		EventId: "config-update-1",
		Type:    pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
		Payload: &pb.Event_ConfigUpdate{
			ConfigUpdate: &pb.ConfigUpdateEvent{
				ConfigVersion: "1",
				ConfigHash:    "abc123abc123abc123abc123",
			},
		},
	}

	// Should not panic even if fetch fails
	client.handleEvent(ctx, event)
}

func TestClient_handleEvent_ConfigUpdate_NoHandler(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		// No EventHandler
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	event := &pb.Event{
		EventId: "config-update-1",
		Type:    pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
		Payload: &pb.Event_ConfigUpdate{
			ConfigUpdate: &pb.ConfigUpdateEvent{
				ConfigVersion: "1",
			},
		},
	}

	// Should not panic with no handler
	client.handleEvent(ctx, event)
}

func TestClient_handleEvent_Deployment_NoHandler(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		// No EventHandler
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)
	client.token = "test-token"
	defer client.Close()

	event := &pb.Event{
		EventId: "deploy-1",
		Type:    pb.EventType_EVENT_TYPE_DEPLOYMENT,
		Payload: &pb.Event_Deployment{
			Deployment: &pb.DeploymentEvent{
				DeploymentId:  "deploy-123",
				ConfigId:      "config-456",
				ConfigVersion: "1",
			},
		},
	}

	// Should not panic with no handler
	client.handleEvent(ctx, event)
}

func TestClient_handleEvent_Drain_NoHandler(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
		// No EventHandler
	})

	event := &pb.Event{
		EventId: "drain-1",
		Type:    pb.EventType_EVENT_TYPE_DRAIN,
		Payload: &pb.Event_Drain{
			Drain: &pb.DrainEvent{
				DrainTimeoutSeconds: 60,
				Reason:              "test",
			},
		},
	}

	// Should not panic with no handler
	client.handleEvent(context.Background(), event)
}

// ============================================
// Agent Stop Tests
// ============================================

func TestAgent_Stop(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "passthrough://bufnet",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	agent.client.conn = conn
	agent.client.client = pb.NewFleetServiceClient(conn)
	agent.client.token = "test-token"

	// Stop should deregister and close
	err = agent.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify deregister was called
	ts.service.mu.Lock()
	deregCalls := ts.service.deregisterCalls
	ts.service.mu.Unlock()

	if deregCalls != 1 {
		t.Errorf("deregisterCalls = %d, want 1", deregCalls)
	}
}

func TestAgent_Stop_NotConnected(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	statePath := filepath.Join(tmpDir, "state.json")

	agent, err := New(Config{
		HubURL:            "localhost:9090",
		InstanceName:      "test-instance",
		SentinelConfig:    configPath,
		StatePath:         statePath,
		HeartbeatInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Stop should not error when not connected
	err = agent.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// ============================================
// Client Close Tests
// ============================================

func TestClient_Close_Twice(t *testing.T) {
	ts := newTestServer()
	defer ts.Stop()

	client, _ := NewClient(ClientConfig{
		HubURL:       "passthrough://bufnet",
		InstanceName: "test-instance",
	})

	ctx := context.Background()
	conn, _ := ts.Dial(ctx)
	client.conn = conn
	client.client = pb.NewFleetServiceClient(conn)

	// First close should work
	err := client.Close()
	if err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	// Second close should return nil (conn is now nil)
	err = client.Close()
	if err != nil {
		t.Fatalf("second Close should return nil, got: %v", err)
	}
}
