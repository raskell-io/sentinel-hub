package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// AgentState represents the persisted state of an agent.
type AgentState struct {
	// Instance identity
	InstanceID string `json:"instance_id"`

	// Current config state
	ConfigVersion string `json:"config_version,omitempty"`
	ConfigHash    string `json:"config_hash,omitempty"`
	ConfigID      string `json:"config_id,omitempty"`

	// Active deployment (if any)
	ActiveDeploymentID string `json:"active_deployment_id,omitempty"`

	// Timestamps
	LastUpdated time.Time `json:"last_updated"`
	CreatedAt   time.Time `json:"created_at"`
}

// StateManager handles persistence of agent state to disk.
type StateManager struct {
	statePath string
	state     *AgentState
	mu        sync.RWMutex
}

// NewStateManager creates a new state manager.
func NewStateManager(statePath string) *StateManager {
	return &StateManager{
		statePath: statePath,
	}
}

// Load loads the state from disk, or creates a new state if none exists.
func (s *StateManager) Load() (*AgentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing state, create new
			s.state = &AgentState{
				CreatedAt:   time.Now().UTC(),
				LastUpdated: time.Now().UTC(),
			}
			log.Debug().Str("path", s.statePath).Msg("No existing state file, starting fresh")
			return s.state, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupted state file, backup and start fresh
		backupPath := s.statePath + ".corrupted"
		os.Rename(s.statePath, backupPath)
		log.Warn().Err(err).Str("backup", backupPath).Msg("Corrupted state file, backed up and starting fresh")
		s.state = &AgentState{
			CreatedAt:   time.Now().UTC(),
			LastUpdated: time.Now().UTC(),
		}
		return s.state, nil
	}

	s.state = &state
	log.Info().
		Str("instance_id", state.InstanceID).
		Str("config_version", state.ConfigVersion).
		Str("active_deployment", state.ActiveDeploymentID).
		Msg("Loaded agent state from disk")

	return s.state, nil
}

// Save persists the current state to disk.
func (s *StateManager) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil {
		return fmt.Errorf("no state to save")
	}

	s.state.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write atomically (temp file + rename)
	tempPath := s.statePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp state: %w", err)
	}

	if err := os.Rename(tempPath, s.statePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	log.Debug().Str("path", s.statePath).Msg("State saved to disk")
	return nil
}

// GetInstanceID returns the instance ID, generating one if not set.
func (s *StateManager) GetInstanceID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return ""
	}
	return s.state.InstanceID
}

// SetInstanceID sets the instance ID and saves.
func (s *StateManager) SetInstanceID(id string) error {
	s.mu.Lock()
	if s.state == nil {
		s.state = &AgentState{CreatedAt: time.Now().UTC()}
	}
	s.state.InstanceID = id
	s.mu.Unlock()

	return s.Save()
}

// GetConfigState returns the current config version and hash.
func (s *StateManager) GetConfigState() (version, hash, configID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return "", "", ""
	}
	return s.state.ConfigVersion, s.state.ConfigHash, s.state.ConfigID
}

// SetConfigState updates the config state and saves.
func (s *StateManager) SetConfigState(version, hash, configID string) error {
	s.mu.Lock()
	if s.state == nil {
		s.state = &AgentState{CreatedAt: time.Now().UTC()}
	}
	s.state.ConfigVersion = version
	s.state.ConfigHash = hash
	s.state.ConfigID = configID
	s.mu.Unlock()

	return s.Save()
}

// GetActiveDeployment returns the active deployment ID.
func (s *StateManager) GetActiveDeployment() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return ""
	}
	return s.state.ActiveDeploymentID
}

// SetActiveDeployment sets the active deployment ID and saves.
func (s *StateManager) SetActiveDeployment(deploymentID string) error {
	s.mu.Lock()
	if s.state == nil {
		s.state = &AgentState{CreatedAt: time.Now().UTC()}
	}
	s.state.ActiveDeploymentID = deploymentID
	s.mu.Unlock()

	return s.Save()
}

// ClearActiveDeployment clears the active deployment and saves.
func (s *StateManager) ClearActiveDeployment() error {
	return s.SetActiveDeployment("")
}

// State returns a copy of the current state.
func (s *StateManager) State() *AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return nil
	}

	// Return a copy
	stateCopy := *s.state
	return &stateCopy
}
