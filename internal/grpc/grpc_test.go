package grpc

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// ============================================
// Server Tests
// ============================================

func TestNewServer(t *testing.T) {
	s := setupTestStore(t)
	server := NewServer(s, 9999)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.grpcServer == nil {
		t.Error("grpcServer should not be nil")
	}
	if server.fleetService == nil {
		t.Error("fleetService should not be nil")
	}
	if server.port != 9999 {
		t.Errorf("port = %d, want 9999", server.port)
	}
}

func TestServer_FleetService(t *testing.T) {
	s := setupTestStore(t)
	server := NewServer(s, 9999)

	fs := server.FleetService()
	if fs == nil {
		t.Error("FleetService() returned nil")
	}
	if fs != server.fleetService {
		t.Error("FleetService() should return the internal fleetService")
	}
}

func TestServer_StartAndStop(t *testing.T) {
	s := setupTestStore(t)

	// Find a free port
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	server := NewServer(s, port)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	server.Stop()

	// Check that Start returned (may have error after stop)
	select {
	case <-errCh:
		// Expected - server stopped
	case <-time.After(2 * time.Second):
		t.Error("server did not stop in time")
	}
}

// ============================================
// Interceptor Tests
// ============================================

func TestLoggingUnaryInterceptor_Success(t *testing.T) {
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := loggingUnaryInterceptor(context.Background(), "request", info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "response" {
		t.Errorf("resp = %v, want 'response'", resp)
	}
}

func TestLoggingUnaryInterceptor_Error(t *testing.T) {
	expectedErr := status.Error(codes.NotFound, "not found")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, expectedErr
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := loggingUnaryInterceptor(context.Background(), "request", info, handler)
	if err != expectedErr {
		t.Errorf("err = %v, want %v", err, expectedErr)
	}
}

func TestLoggingUnaryInterceptor_NonGRPCError(t *testing.T) {
	expectedErr := errors.New("plain error")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, expectedErr
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	_, err := loggingUnaryInterceptor(context.Background(), "request", info, handler)
	if err != expectedErr {
		t.Errorf("err = %v, want %v", err, expectedErr)
	}
}

// mockServerStream implements grpc.ServerStream for testing
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func TestLoggingStreamInterceptor_Success(t *testing.T) {
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod:     "/test.Service/Stream",
		IsClientStream: false,
		IsServerStream: true,
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := loggingStreamInterceptor(nil, stream, info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoggingStreamInterceptor_Error(t *testing.T) {
	expectedErr := status.Error(codes.Internal, "internal error")
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return expectedErr
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/Stream",
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := loggingStreamInterceptor(nil, stream, info, handler)
	if err != expectedErr {
		t.Errorf("err = %v, want %v", err, expectedErr)
	}
}

func TestLoggingStreamInterceptor_NonGRPCError(t *testing.T) {
	expectedErr := errors.New("plain error")
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return expectedErr
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/Stream",
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := loggingStreamInterceptor(nil, stream, info, handler)
	if err != expectedErr {
		t.Errorf("err = %v, want %v", err, expectedErr)
	}
}

func TestRecoveryUnaryInterceptor_Success(t *testing.T) {
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := recoveryUnaryInterceptor(context.Background(), "request", info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "response" {
		t.Errorf("resp = %v, want 'response'", resp)
	}
}

func TestRecoveryUnaryInterceptor_Panic(t *testing.T) {
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("test panic")
	}

	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}

	resp, err := recoveryUnaryInterceptor(context.Background(), "request", info, handler)
	if resp != nil {
		t.Errorf("resp should be nil after panic")
	}
	if err == nil {
		t.Fatal("expected error after panic")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", st.Code())
	}
}

func TestRecoveryStreamInterceptor_Success(t *testing.T) {
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/Stream",
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := recoveryStreamInterceptor(nil, stream, info, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecoveryStreamInterceptor_Panic(t *testing.T) {
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		panic("test panic")
	}

	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/Stream",
	}

	stream := &mockServerStream{ctx: context.Background()}
	err := recoveryStreamInterceptor(nil, stream, info, handler)
	if err == nil {
		t.Fatal("expected error after panic")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", st.Code())
	}
}

// ============================================
// Subscribe Tests
// ============================================

// mockSubscribeStream implements pb.FleetService_SubscribeServer for testing
type mockSubscribeStream struct {
	grpc.ServerStream
	ctx       context.Context
	cancel    context.CancelFunc
	events    []*pb.Event
	eventsMu  sync.Mutex
	sendErr   error
	sendCount int
}

func newMockSubscribeStream() *mockSubscribeStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockSubscribeStream{
		ctx:    ctx,
		cancel: cancel,
		events: make([]*pb.Event, 0),
	}
}

func (m *mockSubscribeStream) Context() context.Context {
	return m.ctx
}

func (m *mockSubscribeStream) Send(event *pb.Event) error {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}
	m.events = append(m.events, event)
	m.sendCount++
	return nil
}

func (m *mockSubscribeStream) Cancel() {
	m.cancel()
}

func (m *mockSubscribeStream) GetEvents() []*pb.Event {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	result := make([]*pb.Event, len(m.events))
	copy(result, m.events)
	return result
}

func TestFleetService_Subscribe_InvalidToken(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	stream := newMockSubscribeStream()
	req := &pb.SubscribeRequest{
		InstanceId: "test-instance",
		Token:      "invalid-token",
	}

	err := fs.Subscribe(req, stream)
	if err == nil {
		t.Error("expected error for invalid token")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("code = %v, want Unauthenticated", st.Code())
	}
}

func TestFleetService_Subscribe_TokenMismatch(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)
	ctx := context.Background()

	// Register an instance to get a valid token
	resp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "test-instance",
		InstanceName: "test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stream := newMockSubscribeStream()
	req := &pb.SubscribeRequest{
		InstanceId: "different-instance", // Different from registered
		Token:      resp.Token,
	}

	err = fs.Subscribe(req, stream)
	if err == nil {
		t.Error("expected error for token mismatch")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", st.Code())
	}
}

func TestFleetService_Subscribe_ReceivesEvents(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)
	ctx := context.Background()

	// Register an instance
	resp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "test-instance",
		InstanceName: "test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stream := newMockSubscribeStream()
	req := &pb.SubscribeRequest{
		InstanceId: "test-instance",
		Token:      resp.Token,
	}

	// Run Subscribe in goroutine
	subscribeDone := make(chan error, 1)
	go func() {
		subscribeDone <- fs.Subscribe(req, stream)
	}()

	// Wait for subscription to be established
	time.Sleep(50 * time.Millisecond)

	// Verify subscriber is registered
	if !fs.IsInstanceSubscribed("test-instance") {
		t.Error("instance should be subscribed")
	}

	// Send an event
	testEvent := &pb.Event{
		EventId: "test-event-1",
		Type:    pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
	}
	err = fs.SendEventToInstance("test-instance", testEvent)
	if err != nil {
		t.Fatalf("SendEventToInstance failed: %v", err)
	}

	// Wait for event to be processed
	time.Sleep(50 * time.Millisecond)

	// Cancel the stream
	stream.Cancel()

	// Wait for Subscribe to return
	select {
	case <-subscribeDone:
		// Expected
	case <-time.After(2 * time.Second):
		t.Error("Subscribe did not return after cancel")
	}

	// Verify event was received
	events := stream.GetEvents()
	if len(events) != 1 {
		t.Errorf("got %d events, want 1", len(events))
	}
}

func TestFleetService_Subscribe_ChannelClosed(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)
	ctx := context.Background()

	// Register an instance
	resp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "test-instance",
		InstanceName: "test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stream := newMockSubscribeStream()
	req := &pb.SubscribeRequest{
		InstanceId: "test-instance",
		Token:      resp.Token,
	}

	// Run Subscribe in goroutine
	subscribeDone := make(chan error, 1)
	go func() {
		subscribeDone <- fs.Subscribe(req, stream)
	}()

	// Wait for subscription to be established
	time.Sleep(50 * time.Millisecond)

	// Close the channel using RemoveSubscriber
	fs.RemoveSubscriber("test-instance")

	// Wait for Subscribe to return
	select {
	case err := <-subscribeDone:
		if err != nil {
			t.Errorf("Subscribe returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Subscribe did not return after channel closed")
	}
}

func TestFleetService_Subscribe_SendError(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)
	ctx := context.Background()

	// Register an instance
	resp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "test-instance",
		InstanceName: "test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stream := newMockSubscribeStream()
	stream.sendErr = errors.New("send failed")
	req := &pb.SubscribeRequest{
		InstanceId: "test-instance",
		Token:      resp.Token,
	}

	// Run Subscribe in goroutine
	subscribeDone := make(chan error, 1)
	go func() {
		subscribeDone <- fs.Subscribe(req, stream)
	}()

	// Wait for subscription to be established
	time.Sleep(50 * time.Millisecond)

	// Send an event - this should trigger the send error
	testEvent := &pb.Event{
		EventId: "test-event-1",
		Type:    pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
	}
	fs.SendEventToInstance("test-instance", testEvent)

	// Wait for Subscribe to return with error
	select {
	case err := <-subscribeDone:
		if err == nil {
			t.Error("expected error from Subscribe")
		}
	case <-time.After(2 * time.Second):
		t.Error("Subscribe did not return after send error")
	}
}

func TestFleetService_Subscribe_ReplacesExisting(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)
	ctx := context.Background()

	// Register an instance
	resp, err := fs.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "test-instance",
		InstanceName: "test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stream1 := newMockSubscribeStream()
	stream2 := newMockSubscribeStream()
	req := &pb.SubscribeRequest{
		InstanceId: "test-instance",
		Token:      resp.Token,
	}

	// Start first subscription
	sub1Done := make(chan error, 1)
	go func() {
		sub1Done <- fs.Subscribe(req, stream1)
	}()

	// Wait for first subscription
	time.Sleep(50 * time.Millisecond)

	// Start second subscription - should replace first
	sub2Done := make(chan error, 1)
	go func() {
		sub2Done <- fs.Subscribe(req, stream2)
	}()

	// Wait for second subscription
	time.Sleep(50 * time.Millisecond)

	// First subscription should have ended (channel closed)
	select {
	case err := <-sub1Done:
		if err != nil {
			t.Errorf("first Subscribe returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("first Subscribe did not return after replacement")
	}

	// Second subscription should still be active
	if !fs.IsInstanceSubscribed("test-instance") {
		t.Error("instance should still be subscribed")
	}

	// Cancel second subscription
	stream2.Cancel()
	<-sub2Done
}

// ============================================
// RemoveSubscriber Tests
// ============================================

func TestFleetService_RemoveSubscriber(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	// Add a subscriber manually
	ch := make(chan *pb.Event, 10)
	fs.SetSubscriber("test-instance", ch)

	if !fs.IsInstanceSubscribed("test-instance") {
		t.Error("instance should be subscribed")
	}

	// Remove subscriber
	fs.RemoveSubscriber("test-instance")

	if fs.IsInstanceSubscribed("test-instance") {
		t.Error("instance should not be subscribed after removal")
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed")
		}
	default:
		t.Error("channel should be closed and readable")
	}
}

func TestFleetService_RemoveSubscriber_NotExists(t *testing.T) {
	s := setupTestStore(t)
	fs := NewFleetService(s)

	// Should not panic when removing non-existent subscriber
	fs.RemoveSubscriber("non-existent")
}

// ============================================
// Integration Tests with bufconn
// ============================================

func setupBufconnServer(t *testing.T, s *store.Store) (*bufconn.Listener, *Server) {
	lis := bufconn.Listen(bufSize)

	server := NewServer(s, 0) // Port doesn't matter for bufconn

	go func() {
		if err := server.grpcServer.Serve(lis); err != nil {
			// Ignore errors after test cleanup
		}
	}()

	t.Cleanup(func() {
		server.Stop()
	})

	return lis, server
}

func dialBufconn(ctx context.Context, lis *bufconn.Listener) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

func TestIntegration_RegisterAndHeartbeat(t *testing.T) {
	s := setupTestStore(t)
	lis, _ := setupBufconnServer(t, s)

	ctx := context.Background()
	conn, err := dialBufconn(ctx, lis)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	client := pb.NewFleetServiceClient(conn)

	// Register
	regResp, err := client.Register(ctx, &pb.RegisterRequest{
		InstanceId:      "integration-test",
		InstanceName:    "Integration Test Instance",
		Hostname:        "localhost",
		AgentVersion:    "1.0.0",
		SentinelVersion: "2.0.0",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if regResp.Token == "" {
		t.Error("token should not be empty")
	}

	// Heartbeat
	hbResp, err := client.Heartbeat(ctx, &pb.HeartbeatRequest{
		InstanceId: "integration-test",
		Token:      regResp.Token,
		Status: &pb.InstanceStatus{
			State:   pb.InstanceState_INSTANCE_STATE_HEALTHY,
			Message: "ok",
		},
	})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if hbResp == nil {
		t.Error("heartbeat response should not be nil")
	}
}

func TestIntegration_Subscribe(t *testing.T) {
	s := setupTestStore(t)
	lis, server := setupBufconnServer(t, s)

	ctx := context.Background()
	conn, err := dialBufconn(ctx, lis)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	client := pb.NewFleetServiceClient(conn)

	// Register
	regResp, err := client.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "sub-test",
		InstanceName: "Subscribe Test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Subscribe with timeout context
	subCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	stream, err := client.Subscribe(subCtx, &pb.SubscribeRequest{
		InstanceId: "sub-test",
		Token:      regResp.Token,
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Wait for subscription to be established
	time.Sleep(50 * time.Millisecond)

	// Send event from server side
	testEvent := &pb.Event{
		EventId: "integration-event",
		Type:    pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
	}
	server.FleetService().SendEventToInstance("sub-test", testEvent)

	// Receive event
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	if event.EventId != "integration-event" {
		t.Errorf("EventId = %q, want 'integration-event'", event.EventId)
	}
}

func TestIntegration_FullFlow(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "hub-integration-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	defer os.Remove(tmpFile.Name() + "-shm")
	defer os.Remove(tmpFile.Name() + "-wal")

	s, err := store.New(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	// Create a config for testing
	ctx := context.Background()
	config := &store.Config{
		ID:             "test-config",
		Name:           "Test Config",
		CurrentVersion: 0,
	}
	if err := s.CreateConfig(ctx, config); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	configVersion := &store.ConfigVersion{
		ID:          "test-config-v1",
		ConfigID:    "test-config",
		Version:     1,
		Content:     "server { listen 8080 }",
		ContentHash: "abc123abc123abc123abc123abc123abc123abc123abc123abc123abc123abc1",
	}
	if err := s.CreateConfigVersion(ctx, configVersion); err != nil {
		t.Fatalf("failed to create config version: %v", err)
	}

	lis, _ := setupBufconnServer(t, s)

	conn, err := dialBufconn(ctx, lis)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	client := pb.NewFleetServiceClient(conn)

	// 1. Register
	regResp, err := client.Register(ctx, &pb.RegisterRequest{
		InstanceId:   "full-flow-test",
		InstanceName: "Full Flow Test",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 2. Assign config to instance
	instance, _ := s.GetInstance(ctx, "full-flow-test")
	instance.CurrentConfigID = &config.ID
	s.UpdateInstance(ctx, instance)

	// 3. GetConfig
	cfgResp, err := client.GetConfig(ctx, &pb.GetConfigRequest{
		InstanceId: "full-flow-test",
		Token:      regResp.Token,
	})
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if cfgResp.Content != "server { listen 8080 }" {
		t.Errorf("Content = %q, want 'server { listen 8080 }'", cfgResp.Content)
	}

	// 4. GetConfigVersion
	verResp, err := client.GetConfigVersion(ctx, &pb.GetConfigVersionRequest{
		InstanceId:    "full-flow-test",
		Token:         regResp.Token,
		ConfigId:      "test-config",
		VersionNumber: 1,
	})
	if err != nil {
		t.Fatalf("GetConfigVersion failed: %v", err)
	}

	if verResp.Content != "server { listen 8080 }" {
		t.Errorf("Content = %q, want 'server { listen 8080 }'", verResp.Content)
	}

	// 5. ReportDeploymentStatus
	_, err = client.ReportDeploymentStatus(ctx, &pb.DeploymentStatusRequest{
		InstanceId:   "full-flow-test",
		Token:        regResp.Token,
		DeploymentId: "deploy-1",
		State:        pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED,
		Message:      "done",
	})
	if err != nil {
		t.Fatalf("ReportDeploymentStatus failed: %v", err)
	}

	// 6. Deregister
	deregResp, err := client.Deregister(ctx, &pb.DeregisterRequest{
		InstanceId: "full-flow-test",
		Token:      regResp.Token,
		Reason:     "test complete",
	})
	if err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}

	if !deregResp.Acknowledged {
		t.Error("Deregister should be acknowledged")
	}
}
