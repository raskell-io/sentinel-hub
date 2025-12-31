package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// ============================================
// SentinelManager Tests
// ============================================

func TestNewSentinelManager(t *testing.T) {
	sm := NewSentinelManager("/etc/sentinel/config.kdl")

	if sm == nil {
		t.Fatal("NewSentinelManager returned nil")
	}
	if sm.configPath != "/etc/sentinel/config.kdl" {
		t.Errorf("configPath = %q, want %q", sm.configPath, "/etc/sentinel/config.kdl")
	}
	if sm.pidFile != "/var/run/sentinel.pid" {
		t.Errorf("pidFile = %q, want %q", sm.pidFile, "/var/run/sentinel.pid")
	}
	if sm.backupDir != "/etc/sentinel" {
		t.Errorf("backupDir = %q, want %q", sm.backupDir, "/etc/sentinel")
	}
}

func TestSentinelManager_SetPIDFile(t *testing.T) {
	sm := NewSentinelManager("/etc/sentinel/config.kdl")
	sm.SetPIDFile("/custom/path/sentinel.pid")

	if sm.pidFile != "/custom/path/sentinel.pid" {
		t.Errorf("pidFile = %q, want %q", sm.pidFile, "/custom/path/sentinel.pid")
	}
}

func TestSentinelManager_SetBackupDir(t *testing.T) {
	sm := NewSentinelManager("/etc/sentinel/config.kdl")
	sm.SetBackupDir("/custom/backup")

	if sm.backupDir != "/custom/backup" {
		t.Errorf("backupDir = %q, want %q", sm.backupDir, "/custom/backup")
	}
}

func TestSentinelManager_GetConfigPath(t *testing.T) {
	sm := NewSentinelManager("/etc/sentinel/config.kdl")

	if sm.GetConfigPath() != "/etc/sentinel/config.kdl" {
		t.Errorf("GetConfigPath() = %q, want %q", sm.GetConfigPath(), "/etc/sentinel/config.kdl")
	}
}

func TestSentinelManager_GetCurrentConfig(t *testing.T) {
	sm := NewSentinelManager("/etc/sentinel/config.kdl")

	if sm.GetCurrentConfig() != "" {
		t.Errorf("GetCurrentConfig() = %q, want empty", sm.GetCurrentConfig())
	}

	sm.currentConfig = "test content"
	if sm.GetCurrentConfig() != "test content" {
		t.Errorf("GetCurrentConfig() = %q, want %q", sm.GetCurrentConfig(), "test content")
	}
}

func TestSentinelManager_ReadCurrentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	sm := NewSentinelManager(configPath)

	// Test reading non-existent file
	content, err := sm.ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCurrentConfig failed: %v", err)
	}
	if content != "" {
		t.Errorf("content = %q, want empty for non-existent file", content)
	}

	// Create a config file
	testContent := "server {\n  listen 8080\n}\n"
	if err := os.WriteFile(configPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test reading existing file
	content, err = sm.ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCurrentConfig failed: %v", err)
	}
	if content != testContent {
		t.Errorf("content = %q, want %q", content, testContent)
	}

	// Verify currentConfig is updated
	if sm.GetCurrentConfig() != testContent {
		t.Errorf("GetCurrentConfig() = %q, want %q", sm.GetCurrentConfig(), testContent)
	}
}

func TestSentinelManager_WriteConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sentinel", "config.kdl")

	sm := NewSentinelManager(configPath)
	sm.SetBackupDir(tmpDir)

	// Test writing to new file (creates directory)
	testContent := "server {\n  listen 8080\n}\n"
	err := sm.WriteConfig(testContent)
	if err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Verify file was written
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("written content = %q, want %q", string(content), testContent)
	}

	// Verify currentConfig is updated
	if sm.GetCurrentConfig() != testContent {
		t.Errorf("GetCurrentConfig() = %q, want %q", sm.GetCurrentConfig(), testContent)
	}

	// Test writing again (should create backup)
	newContent := "server {\n  listen 9090\n}\n"
	err = sm.WriteConfig(newContent)
	if err != nil {
		t.Fatalf("WriteConfig (second) failed: %v", err)
	}

	// Verify new content
	content, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	if string(content) != newContent {
		t.Errorf("updated content = %q, want %q", string(content), newContent)
	}

	// Verify backup was created
	backupPath := filepath.Join(tmpDir, "config.kdl.bak")
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}
	if string(backupContent) != testContent {
		t.Errorf("backup content = %q, want %q", string(backupContent), testContent)
	}
}

func TestSentinelManager_Rollback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")
	backupPath := filepath.Join(tmpDir, "config.kdl.bak")

	sm := NewSentinelManager(configPath)
	sm.SetBackupDir(tmpDir)

	// Test rollback without backup
	err := sm.Rollback()
	if err == nil {
		t.Error("Rollback should fail without backup")
	}

	// Create backup and current config
	backupContent := "original config\n"
	currentContent := "new config\n"
	if err := os.WriteFile(backupPath, []byte(backupContent), 0644); err != nil {
		t.Fatalf("failed to write backup: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(currentContent), 0644); err != nil {
		t.Fatalf("failed to write current: %v", err)
	}

	// Test successful rollback
	err = sm.Rollback()
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify config was restored
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(content) != backupContent {
		t.Errorf("restored content = %q, want %q", string(content), backupContent)
	}

	// Verify currentConfig is updated
	if sm.GetCurrentConfig() != backupContent {
		t.Errorf("GetCurrentConfig() = %q, want %q", sm.GetCurrentConfig(), backupContent)
	}
}

func TestSentinelManager_copyFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "src.txt")
	dstPath := filepath.Join(tmpDir, "dst.txt")

	sm := NewSentinelManager("/etc/sentinel/config.kdl")

	// Create source file
	content := "test file content"
	if err := os.WriteFile(srcPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// Test copy
	err := sm.copyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify destination
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(dstContent) != content {
		t.Errorf("copied content = %q, want %q", string(dstContent), content)
	}
}

func TestSentinelManager_copyFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSentinelManager("/etc/sentinel/config.kdl")

	err := sm.copyFile(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dst"))
	if err == nil {
		t.Error("copyFile should fail for non-existent source")
	}
}

// ============================================
// Client Tests
// ============================================

func TestNewClient(t *testing.T) {
	cfg := ClientConfig{
		HubURL:          "localhost:9090",
		InstanceID:      "inst-1",
		InstanceName:    "test-instance",
		AgentVersion:    "1.0.0",
		SentinelVersion: "2.0.0",
		Labels:          map[string]string{"env": "test"},
		Capabilities:    []string{"config-reload"},
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.hubURL != "localhost:9090" {
		t.Errorf("hubURL = %q, want %q", client.hubURL, "localhost:9090")
	}
	if client.instanceID != "inst-1" {
		t.Errorf("instanceID = %q, want %q", client.instanceID, "inst-1")
	}
	if client.instanceName != "test-instance" {
		t.Errorf("instanceName = %q, want %q", client.instanceName, "test-instance")
	}
	if client.agentVersion != "1.0.0" {
		t.Errorf("agentVersion = %q, want %q", client.agentVersion, "1.0.0")
	}
	if client.sentinelVersion != "2.0.0" {
		t.Errorf("sentinelVersion = %q, want %q", client.sentinelVersion, "2.0.0")
	}
	if client.labels["env"] != "test" {
		t.Errorf("labels[env] = %q, want %q", client.labels["env"], "test")
	}
	if len(client.capabilities) != 1 || client.capabilities[0] != "config-reload" {
		t.Errorf("capabilities = %v, want [config-reload]", client.capabilities)
	}
}

func TestNewClient_AutoGenerateID(t *testing.T) {
	cfg := ClientConfig{
		HubURL:       "localhost:9090",
		InstanceName: "test-instance",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client.instanceID == "" {
		t.Error("instanceID should be auto-generated")
	}
	// UUID format: 8-4-4-4-12
	if len(client.instanceID) != 36 {
		t.Errorf("instanceID length = %d, want 36 (UUID format)", len(client.instanceID))
	}
}

func TestClient_InstanceID(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:     "localhost:9090",
		InstanceID: "test-id",
	})

	if client.InstanceID() != "test-id" {
		t.Errorf("InstanceID() = %q, want %q", client.InstanceID(), "test-id")
	}
}

func TestClient_IsConnected(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL: "localhost:9090",
	})

	// Not connected initially
	if client.IsConnected() {
		t.Error("IsConnected() should be false initially")
	}

	// Simulate connected state
	client.connMu.Lock()
	client.token = "test-token"
	client.connMu.Unlock()

	// Still false because conn is nil
	if client.IsConnected() {
		t.Error("IsConnected() should be false without conn")
	}
}

func TestClient_UpdateConfigState(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL: "localhost:9090",
	})

	client.UpdateConfigState("v1.2.3", "abc123hash")

	client.configMu.RLock()
	version := client.currentConfigVersion
	hash := client.currentConfigHash
	client.configMu.RUnlock()

	if version != "v1.2.3" {
		t.Errorf("currentConfigVersion = %q, want %q", version, "v1.2.3")
	}
	if hash != "abc123hash" {
		t.Errorf("currentConfigHash = %q, want %q", hash, "abc123hash")
	}
}

func TestClient_SetConfigFromContent(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL: "localhost:9090",
	})

	content := "server { listen 8080 }"
	client.SetConfigFromContent("v1", content)

	client.configMu.RLock()
	version := client.currentConfigVersion
	hash := client.currentConfigHash
	client.configMu.RUnlock()

	if version != "v1" {
		t.Errorf("currentConfigVersion = %q, want %q", version, "v1")
	}
	// SHA256 hash should be 64 hex characters
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
	// Hash should be consistent
	client.SetConfigFromContent("v2", content)
	client.configMu.RLock()
	hash2 := client.currentConfigHash
	client.configMu.RUnlock()
	if hash != hash2 {
		t.Errorf("hash should be consistent for same content: %q != %q", hash, hash2)
	}
}

func TestClient_Close_Nil(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL: "localhost:9090",
	})

	// Close without connection should not error
	err := client.Close()
	if err != nil {
		t.Errorf("Close() on nil conn returned error: %v", err)
	}
}

// ============================================
// Agent Config Tests
// ============================================

func TestAgentConfig(t *testing.T) {
	cfg := Config{
		HubURL:            "localhost:9090",
		InstanceID:        "inst-1",
		InstanceName:      "test-agent",
		SentinelConfig:    "/etc/sentinel/config.kdl",
		HeartbeatInterval: 30,
		AgentVersion:      "1.0.0",
		SentinelVersion:   "2.0.0",
		Labels:            map[string]string{"env": "test", "region": "us-west"},
	}

	if cfg.HubURL != "localhost:9090" {
		t.Errorf("HubURL = %q, want %q", cfg.HubURL, "localhost:9090")
	}
	if cfg.InstanceID != "inst-1" {
		t.Errorf("InstanceID = %q, want %q", cfg.InstanceID, "inst-1")
	}
	if cfg.InstanceName != "test-agent" {
		t.Errorf("InstanceName = %q, want %q", cfg.InstanceName, "test-agent")
	}
	if cfg.SentinelConfig != "/etc/sentinel/config.kdl" {
		t.Errorf("SentinelConfig = %q, want %q", cfg.SentinelConfig, "/etc/sentinel/config.kdl")
	}
	if cfg.HeartbeatInterval != 30 {
		t.Errorf("HeartbeatInterval = %v, want 30", cfg.HeartbeatInterval)
	}
	if len(cfg.Labels) != 2 {
		t.Errorf("Labels count = %d, want 2", len(cfg.Labels))
	}
}

// ============================================
// MockEventHandler for testing
// ============================================

type mockEventHandler struct {
	configUpdates   []configUpdateCall
	deployments     []deploymentCall
	drains          []drainCall
	configUpdateErr error
	deploymentErr   error
	drainErr        error
}

type configUpdateCall struct {
	version, hash, content string
}

type deploymentCall struct {
	deploymentID, configID, configVersion string
	isRollback                            bool
}

type drainCall struct {
	timeoutSecs int
	reason      string
}

func (m *mockEventHandler) OnConfigUpdate(version, hash, content string) error {
	m.configUpdates = append(m.configUpdates, configUpdateCall{version, hash, content})
	return m.configUpdateErr
}

func (m *mockEventHandler) OnDeployment(deploymentID, configID, configVersion string, isRollback bool) error {
	m.deployments = append(m.deployments, deploymentCall{deploymentID, configID, configVersion, isRollback})
	return m.deploymentErr
}

func (m *mockEventHandler) OnDrain(timeoutSecs int, reason string) error {
	m.drains = append(m.drains, drainCall{timeoutSecs, reason})
	return m.drainErr
}

func TestMockEventHandler(t *testing.T) {
	handler := &mockEventHandler{}

	// Test OnConfigUpdate
	err := handler.OnConfigUpdate("v1", "hash123", "content")
	if err != nil {
		t.Errorf("OnConfigUpdate failed: %v", err)
	}
	if len(handler.configUpdates) != 1 {
		t.Fatalf("configUpdates count = %d, want 1", len(handler.configUpdates))
	}
	if handler.configUpdates[0].version != "v1" {
		t.Errorf("version = %q, want %q", handler.configUpdates[0].version, "v1")
	}

	// Test OnDeployment
	err = handler.OnDeployment("dep-1", "cfg-1", "1", false)
	if err != nil {
		t.Errorf("OnDeployment failed: %v", err)
	}
	if len(handler.deployments) != 1 {
		t.Fatalf("deployments count = %d, want 1", len(handler.deployments))
	}
	if handler.deployments[0].deploymentID != "dep-1" {
		t.Errorf("deploymentID = %q, want %q", handler.deployments[0].deploymentID, "dep-1")
	}

	// Test OnDrain
	err = handler.OnDrain(30, "maintenance")
	if err != nil {
		t.Errorf("OnDrain failed: %v", err)
	}
	if len(handler.drains) != 1 {
		t.Fatalf("drains count = %d, want 1", len(handler.drains))
	}
	if handler.drains[0].timeoutSecs != 30 {
		t.Errorf("timeoutSecs = %d, want 30", handler.drains[0].timeoutSecs)
	}
	if handler.drains[0].reason != "maintenance" {
		t.Errorf("reason = %q, want %q", handler.drains[0].reason, "maintenance")
	}
}

func TestClient_WithEventHandler(t *testing.T) {
	handler := &mockEventHandler{}

	cfg := ClientConfig{
		HubURL:       "localhost:9090",
		EventHandler: handler,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client.eventHandler == nil {
		t.Error("eventHandler should be set")
	}
}

// ============================================
// Concurrent Access Tests
// ============================================

func TestClient_ConcurrentConfigState(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL: "localhost:9090",
	})

	done := make(chan struct{})

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			client.UpdateConfigState("v1", "hash1")
		}
		close(done)
	}()

	// Concurrent reads via SetConfigFromContent
	for i := 0; i < 100; i++ {
		client.SetConfigFromContent("v2", "content")
	}

	<-done
}

func TestSentinelManager_ConcurrentConfigRead(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	// Create config file
	if err := os.WriteFile(configPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	sm := NewSentinelManager(configPath)

	done := make(chan struct{})

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			sm.ReadCurrentConfig()
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		sm.GetCurrentConfig()
	}

	<-done
}

// ============================================
// Reconnection Scenario Tests
// ============================================

func TestClient_StateAfterDisconnect(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:       "localhost:9090",
		InstanceID:   "inst-1",
		InstanceName: "test-instance",
	})

	// Simulate connected state with token and config
	client.connMu.Lock()
	client.token = "initial-token"
	client.connMu.Unlock()

	client.UpdateConfigState("v1", "hash123")

	// Verify connected state
	client.connMu.RLock()
	token := client.token
	client.connMu.RUnlock()
	if token != "initial-token" {
		t.Errorf("token = %q, want %q", token, "initial-token")
	}

	// Simulate disconnect by closing (clears connection but not token in this impl)
	client.Close()

	// Config state should be preserved across disconnect
	client.configMu.RLock()
	version := client.currentConfigVersion
	hash := client.currentConfigHash
	client.configMu.RUnlock()

	if version != "v1" {
		t.Errorf("config version after disconnect = %q, want %q", version, "v1")
	}
	if hash != "hash123" {
		t.Errorf("config hash after disconnect = %q, want %q", hash, "hash123")
	}
}

func TestClient_ConfigHashMismatchDetection(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:     "localhost:9090",
		InstanceID: "inst-1",
	})

	// Set initial config state (simulating what agent had before disconnect)
	client.SetConfigFromContent("v1", "old config content")

	client.configMu.RLock()
	oldHash := client.currentConfigHash
	client.configMu.RUnlock()

	// Simulate new config being available (different content = different hash)
	client.SetConfigFromContent("v2", "new config content")

	client.configMu.RLock()
	newHash := client.currentConfigHash
	client.configMu.RUnlock()

	// Hashes should be different for different content
	if oldHash == newHash {
		t.Error("hashes should differ for different config content")
	}

	// Verify hash is deterministic
	client.SetConfigFromContent("v3", "old config content")
	client.configMu.RLock()
	sameHash := client.currentConfigHash
	client.configMu.RUnlock()

	if sameHash != oldHash {
		t.Errorf("same content should produce same hash: %q != %q", sameHash, oldHash)
	}
}

func TestClient_ReconnectWithNewToken(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:     "localhost:9090",
		InstanceID: "inst-1",
	})

	// First connection
	client.connMu.Lock()
	client.token = "token-session-1"
	client.connMu.Unlock()

	// Simulate disconnect
	client.Close()

	// Simulate reconnect with new token (as would happen after Register)
	client.connMu.Lock()
	client.token = "token-session-2"
	client.connMu.Unlock()

	client.connMu.RLock()
	newToken := client.token
	client.connMu.RUnlock()

	if newToken != "token-session-2" {
		t.Errorf("token after reconnect = %q, want %q", newToken, "token-session-2")
	}
}

func TestEventHandler_ConfigUpdateOnReconnect(t *testing.T) {
	handler := &mockEventHandler{}

	// Simulate receiving config update after reconnection
	err := handler.OnConfigUpdate("v2", "newhash456", "updated config content")
	if err != nil {
		t.Fatalf("OnConfigUpdate failed: %v", err)
	}

	if len(handler.configUpdates) != 1 {
		t.Fatalf("expected 1 config update, got %d", len(handler.configUpdates))
	}

	update := handler.configUpdates[0]
	if update.version != "v2" {
		t.Errorf("version = %q, want %q", update.version, "v2")
	}
	if update.hash != "newhash456" {
		t.Errorf("hash = %q, want %q", update.hash, "newhash456")
	}
	if update.content != "updated config content" {
		t.Errorf("content = %q, want %q", update.content, "updated config content")
	}
}

func TestEventHandler_DeploymentOnReconnect(t *testing.T) {
	handler := &mockEventHandler{}

	// Simulate receiving deployment event after reconnection
	// This could happen if agent was offline during deployment start
	err := handler.OnDeployment("dep-123", "cfg-1", "3", false)
	if err != nil {
		t.Fatalf("OnDeployment failed: %v", err)
	}

	if len(handler.deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(handler.deployments))
	}

	dep := handler.deployments[0]
	if dep.deploymentID != "dep-123" {
		t.Errorf("deploymentID = %q, want %q", dep.deploymentID, "dep-123")
	}
	if dep.configID != "cfg-1" {
		t.Errorf("configID = %q, want %q", dep.configID, "cfg-1")
	}
	if dep.configVersion != "3" {
		t.Errorf("configVersion = %q, want %q", dep.configVersion, "3")
	}
}

func TestEventHandler_RollbackDeploymentOnReconnect(t *testing.T) {
	handler := &mockEventHandler{}

	// Simulate rollback deployment after reconnection
	err := handler.OnDeployment("dep-456", "cfg-1", "1", true)
	if err != nil {
		t.Fatalf("OnDeployment (rollback) failed: %v", err)
	}

	if len(handler.deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(handler.deployments))
	}

	dep := handler.deployments[0]
	if !dep.isRollback {
		t.Error("isRollback should be true for rollback deployment")
	}
}

func TestEventHandler_MultipleEventsAfterReconnect(t *testing.T) {
	handler := &mockEventHandler{}

	// Simulate burst of events after reconnection (catching up)
	handler.OnConfigUpdate("v1", "hash1", "content1")
	handler.OnConfigUpdate("v2", "hash2", "content2")
	handler.OnDeployment("dep-1", "cfg-1", "2", false)
	handler.OnDrain(60, "maintenance window")

	if len(handler.configUpdates) != 2 {
		t.Errorf("configUpdates = %d, want 2", len(handler.configUpdates))
	}
	if len(handler.deployments) != 1 {
		t.Errorf("deployments = %d, want 1", len(handler.deployments))
	}
	if len(handler.drains) != 1 {
		t.Errorf("drains = %d, want 1", len(handler.drains))
	}
}

func TestEventHandler_ErrorDuringReconnect(t *testing.T) {
	handler := &mockEventHandler{
		configUpdateErr: os.ErrPermission, // Simulate permission error writing config
	}

	err := handler.OnConfigUpdate("v1", "hash1", "content")
	if err != os.ErrPermission {
		t.Errorf("err = %v, want %v", err, os.ErrPermission)
	}

	// Event should still be recorded
	if len(handler.configUpdates) != 1 {
		t.Errorf("configUpdates = %d, want 1", len(handler.configUpdates))
	}
}

func TestSentinelManager_ConfigPersistenceAcrossReconnect(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	// First "session" - write config
	sm1 := NewSentinelManager(configPath)
	originalContent := "server { listen 8080 }"
	if err := sm1.WriteConfig(originalContent); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Simulate agent restart - new SentinelManager instance
	sm2 := NewSentinelManager(configPath)

	// Read config from disk (as agent would do on startup)
	content, err := sm2.ReadCurrentConfig()
	if err != nil {
		t.Fatalf("ReadCurrentConfig failed: %v", err)
	}

	if content != originalContent {
		t.Errorf("content = %q, want %q", content, originalContent)
	}
}

func TestSentinelManager_BackupAvailableAfterReconnect(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	sm := NewSentinelManager(configPath)
	sm.SetBackupDir(tmpDir)

	// Write initial config
	if err := sm.WriteConfig("version 1"); err != nil {
		t.Fatalf("WriteConfig v1 failed: %v", err)
	}

	// Write second config (creates backup of v1)
	if err := sm.WriteConfig("version 2"); err != nil {
		t.Fatalf("WriteConfig v2 failed: %v", err)
	}

	// Simulate agent restart
	sm2 := NewSentinelManager(configPath)
	sm2.SetBackupDir(tmpDir)

	// Current config should be v2
	content, _ := sm2.ReadCurrentConfig()
	if content != "version 2" {
		t.Errorf("current config = %q, want %q", content, "version 2")
	}

	// Rollback should restore v1
	if err := sm2.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	content, _ = sm2.ReadCurrentConfig()
	if content != "version 1" {
		t.Errorf("after rollback = %q, want %q", content, "version 1")
	}
}

func TestClient_ConfigStatePreservedDuringReconnectCycle(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:     "localhost:9090",
		InstanceID: "inst-1",
	})

	// Initial config state
	client.SetConfigFromContent("v1", "initial config")

	client.configMu.RLock()
	initialHash := client.currentConfigHash
	client.configMu.RUnlock()

	// Simulate multiple reconnect cycles
	for i := 0; i < 5; i++ {
		// Disconnect
		client.Close()

		// Reconnect (new token)
		client.connMu.Lock()
		client.token = "token-" + string(rune('A'+i))
		client.connMu.Unlock()
	}

	// Config state should be unchanged
	client.configMu.RLock()
	finalHash := client.currentConfigHash
	client.configMu.RUnlock()

	if finalHash != initialHash {
		t.Errorf("config hash changed during reconnect cycles: %q != %q", finalHash, initialHash)
	}
}

func TestClient_InstanceIDPreservedAcrossReconnects(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL:     "localhost:9090",
		InstanceID: "persistent-inst-id",
	})

	initialID := client.InstanceID()

	// Simulate reconnect cycles
	for i := 0; i < 3; i++ {
		client.Close()
		client.connMu.Lock()
		client.token = ""
		client.connMu.Unlock()
	}

	// Instance ID should never change
	if client.InstanceID() != initialID {
		t.Errorf("InstanceID changed: %q != %q", client.InstanceID(), initialID)
	}
	if client.InstanceID() != "persistent-inst-id" {
		t.Errorf("InstanceID = %q, want %q", client.InstanceID(), "persistent-inst-id")
	}
}

func TestClient_IsConnectedStateTransitions(t *testing.T) {
	client, _ := NewClient(ClientConfig{
		HubURL: "localhost:9090",
	})

	// Initially not connected
	if client.IsConnected() {
		t.Error("should not be connected initially")
	}

	// After setting token only (no conn) - still not connected
	client.connMu.Lock()
	client.token = "some-token"
	client.connMu.Unlock()

	if client.IsConnected() {
		t.Error("should not be connected with only token (no conn)")
	}

	// After disconnect (clear token)
	client.connMu.Lock()
	client.token = ""
	client.connMu.Unlock()

	if client.IsConnected() {
		t.Error("should not be connected after clearing token")
	}
}

func TestEventHandler_DeploymentErrorRecovery(t *testing.T) {
	handler := &mockEventHandler{
		deploymentErr: os.ErrNotExist, // Simulate config file not found
	}

	// First deployment fails
	err := handler.OnDeployment("dep-1", "cfg-1", "1", false)
	if err != os.ErrNotExist {
		t.Errorf("expected ErrNotExist, got %v", err)
	}

	// Clear error, retry succeeds
	handler.deploymentErr = nil
	err = handler.OnDeployment("dep-1", "cfg-1", "1", false)
	if err != nil {
		t.Errorf("retry should succeed, got %v", err)
	}

	// Both attempts recorded
	if len(handler.deployments) != 2 {
		t.Errorf("deployments = %d, want 2", len(handler.deployments))
	}
}

func TestSentinelManager_WriteConfigAtomicity(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.kdl")

	sm := NewSentinelManager(configPath)

	// Write config
	content := "atomic write test"
	if err := sm.WriteConfig(content); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Temp file should not exist after successful write
	tempPath := configPath + ".tmp"
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up after successful write")
	}

	// Main config should exist with correct content
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(data) != content {
		t.Errorf("config content = %q, want %q", string(data), content)
	}
}
