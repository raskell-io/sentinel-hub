package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateManager_LoadNoExistingState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state == nil {
		t.Fatal("Load() returned nil state")
	}
	if state.InstanceID != "" {
		t.Errorf("InstanceID = %q, want empty", state.InstanceID)
	}
	if state.ConfigVersion != "" {
		t.Errorf("ConfigVersion = %q, want empty", state.ConfigVersion)
	}
	if state.ActiveDeploymentID != "" {
		t.Errorf("ActiveDeploymentID = %q, want empty", state.ActiveDeploymentID)
	}
	if state.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want non-zero")
	}
}

func TestStateManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create and save state
	sm := NewStateManager(statePath)
	_, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := sm.SetInstanceID("test-instance-123"); err != nil {
		t.Fatalf("SetInstanceID() error = %v", err)
	}

	if err := sm.SetConfigState("v1.2.3", "abc123hash", "config-456"); err != nil {
		t.Fatalf("SetConfigState() error = %v", err)
	}

	if err := sm.SetActiveDeployment("deploy-789"); err != nil {
		t.Fatalf("SetActiveDeployment() error = %v", err)
	}

	// Load in a new state manager
	sm2 := NewStateManager(statePath)
	state, err := sm2.Load()
	if err != nil {
		t.Fatalf("Load() on new manager error = %v", err)
	}

	if state.InstanceID != "test-instance-123" {
		t.Errorf("InstanceID = %q, want %q", state.InstanceID, "test-instance-123")
	}
	if state.ConfigVersion != "v1.2.3" {
		t.Errorf("ConfigVersion = %q, want %q", state.ConfigVersion, "v1.2.3")
	}
	if state.ConfigHash != "abc123hash" {
		t.Errorf("ConfigHash = %q, want %q", state.ConfigHash, "abc123hash")
	}
	if state.ConfigID != "config-456" {
		t.Errorf("ConfigID = %q, want %q", state.ConfigID, "config-456")
	}
	if state.ActiveDeploymentID != "deploy-789" {
		t.Errorf("ActiveDeploymentID = %q, want %q", state.ActiveDeploymentID, "deploy-789")
	}
}

func TestStateManager_GetInstanceID(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	if _, err := sm.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Initially empty
	if got := sm.GetInstanceID(); got != "" {
		t.Errorf("GetInstanceID() = %q, want empty", got)
	}

	// After setting
	if err := sm.SetInstanceID("my-instance"); err != nil {
		t.Fatalf("SetInstanceID() error = %v", err)
	}
	if got := sm.GetInstanceID(); got != "my-instance" {
		t.Errorf("GetInstanceID() = %q, want %q", got, "my-instance")
	}
}

func TestStateManager_GetConfigState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	if _, err := sm.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Initially empty
	version, hash, configID := sm.GetConfigState()
	if version != "" || hash != "" || configID != "" {
		t.Errorf("GetConfigState() = (%q, %q, %q), want all empty", version, hash, configID)
	}

	// After setting
	if err := sm.SetConfigState("v2", "hashvalue", "cfg-1"); err != nil {
		t.Fatalf("SetConfigState() error = %v", err)
	}

	version, hash, configID = sm.GetConfigState()
	if version != "v2" {
		t.Errorf("version = %q, want %q", version, "v2")
	}
	if hash != "hashvalue" {
		t.Errorf("hash = %q, want %q", hash, "hashvalue")
	}
	if configID != "cfg-1" {
		t.Errorf("configID = %q, want %q", configID, "cfg-1")
	}
}

func TestStateManager_ActiveDeployment(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	if _, err := sm.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Initially empty
	if got := sm.GetActiveDeployment(); got != "" {
		t.Errorf("GetActiveDeployment() = %q, want empty", got)
	}

	// Set deployment
	if err := sm.SetActiveDeployment("deploy-123"); err != nil {
		t.Fatalf("SetActiveDeployment() error = %v", err)
	}
	if got := sm.GetActiveDeployment(); got != "deploy-123" {
		t.Errorf("GetActiveDeployment() = %q, want %q", got, "deploy-123")
	}

	// Clear deployment
	if err := sm.ClearActiveDeployment(); err != nil {
		t.Fatalf("ClearActiveDeployment() error = %v", err)
	}
	if got := sm.GetActiveDeployment(); got != "" {
		t.Errorf("GetActiveDeployment() = %q, want empty", got)
	}
}

func TestStateManager_CorruptedStateFile(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Write corrupted state
	if err := os.WriteFile(statePath, []byte("not valid json{{{"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sm := NewStateManager(statePath)
	state, err := sm.Load()

	// Should not error, should return fresh state
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if state == nil {
		t.Fatal("Load() returned nil state")
	}
	if state.InstanceID != "" {
		t.Errorf("InstanceID = %q, want empty", state.InstanceID)
	}

	// Corrupted file should be backed up
	backupPath := statePath + ".corrupted"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Expected backup file to exist")
	}
}

func TestStateManager_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	if _, err := sm.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := sm.SetInstanceID("atomic-test"); err != nil {
		t.Fatalf("SetInstanceID() error = %v", err)
	}

	// Verify no temp file left behind
	tempPath := statePath + ".tmp"
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("Expected temp file to not exist")
	}

	// Verify state file exists
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("Expected state file to exist")
	}
}

func TestStateManager_LastUpdated(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	initialUpdated := state.LastUpdated

	// Small delay
	time.Sleep(10 * time.Millisecond)

	if err := sm.SetInstanceID("update-test"); err != nil {
		t.Fatalf("SetInstanceID() error = %v", err)
	}

	// Reload and check
	sm2 := NewStateManager(statePath)
	state2, err := sm2.Load()
	if err != nil {
		t.Fatalf("Load() on new manager error = %v", err)
	}

	if !state2.LastUpdated.After(initialUpdated) {
		t.Error("Expected LastUpdated to be after initial value")
	}
}

func TestStateManager_StateCopy(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	sm := NewStateManager(statePath)
	if _, err := sm.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := sm.SetInstanceID("original"); err != nil {
		t.Fatalf("SetInstanceID() error = %v", err)
	}

	// Get copy and modify it
	stateCopy := sm.State()
	stateCopy.InstanceID = "modified"

	// Original should be unchanged
	if got := sm.GetInstanceID(); got != "original" {
		t.Errorf("GetInstanceID() = %q, want %q (copy modified original)", got, "original")
	}
}

func TestStateManager_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "nested", "dirs", "state.json")

	sm := NewStateManager(statePath)
	if _, err := sm.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := sm.SetInstanceID("nested-test"); err != nil {
		t.Fatalf("SetInstanceID() error = %v", err)
	}

	// Directory should have been created
	if _, err := os.Stat(filepath.Dir(statePath)); os.IsNotExist(err) {
		t.Error("Expected directory to be created")
	}
}

func TestStateManager_PersistenceAcrossRestarts(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Simulate first run
	{
		sm := NewStateManager(statePath)
		if _, err := sm.Load(); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if err := sm.SetInstanceID("persistent-instance"); err != nil {
			t.Fatalf("SetInstanceID() error = %v", err)
		}
		if err := sm.SetConfigState("v5", "hash5", "cfg-5"); err != nil {
			t.Fatalf("SetConfigState() error = %v", err)
		}
		if err := sm.SetActiveDeployment("deploy-in-progress"); err != nil {
			t.Fatalf("SetActiveDeployment() error = %v", err)
		}
	}

	// Simulate restart
	{
		sm := NewStateManager(statePath)
		state, err := sm.Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// All state should be restored
		if state.InstanceID != "persistent-instance" {
			t.Errorf("InstanceID = %q, want %q", state.InstanceID, "persistent-instance")
		}
		if state.ConfigVersion != "v5" {
			t.Errorf("ConfigVersion = %q, want %q", state.ConfigVersion, "v5")
		}
		if state.ActiveDeploymentID != "deploy-in-progress" {
			t.Errorf("ActiveDeploymentID = %q, want %q", state.ActiveDeploymentID, "deploy-in-progress")
		}
	}
}
