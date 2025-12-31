package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
)

// Agent manages the connection to Hub and local Sentinel instance.
type Agent struct {
	client   *Client
	sentinel *SentinelManager
	state    *StateManager

	// Configuration
	heartbeatInterval time.Duration

	// State
	running   bool
	runningMu sync.RWMutex

	// Channels
	stopCh chan struct{}
}

// Config holds configuration for creating a new Agent.
type Config struct {
	HubURL            string
	InstanceID        string
	InstanceName      string
	SentinelConfig    string
	StatePath         string // Path to agent state file
	HeartbeatInterval time.Duration
	AgentVersion      string
	SentinelVersion   string
	Labels            map[string]string
}

// New creates a new Agent instance.
func New(cfg Config) (*Agent, error) {
	sentinel := NewSentinelManager(cfg.SentinelConfig)

	// Initialize state manager
	statePath := cfg.StatePath
	if statePath == "" {
		statePath = "/var/lib/sentinel-agent/state.json"
	}
	stateManager := NewStateManager(statePath)

	// Load existing state
	state, err := stateManager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load agent state: %w", err)
	}

	// Determine instance ID: config > state > generate new
	instanceID := cfg.InstanceID
	if instanceID == "" && state.InstanceID != "" {
		instanceID = state.InstanceID
		log.Info().Str("instance_id", instanceID).Msg("Using instance ID from persisted state")
	}
	// If still empty, NewClient will generate one and we'll save it

	agent := &Agent{
		sentinel:          sentinel,
		state:             stateManager,
		heartbeatInterval: cfg.HeartbeatInterval,
		stopCh:            make(chan struct{}),
	}

	// Create client with agent as event handler
	client, err := NewClient(ClientConfig{
		HubURL:          cfg.HubURL,
		InstanceID:      instanceID,
		InstanceName:    cfg.InstanceName,
		AgentVersion:    cfg.AgentVersion,
		SentinelVersion: cfg.SentinelVersion,
		Labels:          cfg.Labels,
		Capabilities:    []string{"config-reload", "health-check"},
		EventHandler:    agent,
	})
	if err != nil {
		return nil, err
	}

	agent.client = client

	// Persist instance ID if it was generated
	if state.InstanceID == "" {
		if err := stateManager.SetInstanceID(client.InstanceID()); err != nil {
			log.Warn().Err(err).Msg("Failed to persist instance ID")
		}
	}

	// Restore config state from persisted state or read from disk
	if state.ConfigVersion != "" {
		client.UpdateConfigState(state.ConfigVersion, state.ConfigHash)
		log.Info().
			Str("version", state.ConfigVersion).
			Msg("Restored config state from persisted state")
	} else if content, err := sentinel.ReadCurrentConfig(); err == nil && content != "" {
		client.SetConfigFromContent("unknown", content)
	}

	return agent, nil
}

// Run starts the agent and blocks until Stop is called.
func (a *Agent) Run(ctx context.Context) error {
	a.runningMu.Lock()
	a.running = true
	a.runningMu.Unlock()

	defer func() {
		a.runningMu.Lock()
		a.running = false
		a.runningMu.Unlock()
	}()

	log.Info().Msg("Starting agent...")

	// Check for interrupted deployment from previous run
	if a.state != nil {
		if deploymentID := a.state.GetActiveDeployment(); deploymentID != "" {
			log.Warn().
				Str("deployment_id", deploymentID).
				Msg("Found interrupted deployment from previous run, will report failure after connecting")
		}
	}

	// Main loop with reconnection
	backoff := time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		default:
		}

		// Connect to Hub
		if err := a.client.Connect(ctx); err != nil {
			log.Error().Err(err).Dur("backoff", backoff).Msg("Failed to connect, retrying...")
			select {
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			case <-ctx.Done():
				return ctx.Err()
			case <-a.stopCh:
				return nil
			}
		}

		// Register with Hub
		if err := a.client.Register(ctx); err != nil {
			log.Error().Err(err).Dur("backoff", backoff).Msg("Failed to register, retrying...")
			a.client.Close()
			select {
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			case <-ctx.Done():
				return ctx.Err()
			case <-a.stopCh:
				return nil
			}
		}

		// Reset backoff on successful connection
		backoff = time.Second

		// Report any interrupted deployment from previous run
		a.reportInterruptedDeployment(ctx)

		// Run the main agent loop
		if err := a.runLoop(ctx); err != nil {
			log.Error().Err(err).Msg("Agent loop exited with error")
		}

		// Deregister on disconnect
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		a.client.Deregister(shutdownCtx, "disconnected")
		cancel()

		a.client.Close()

		// Check if we should stop
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		default:
			// Continue reconnection loop
			log.Info().Dur("backoff", backoff).Msg("Reconnecting...")
			select {
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
			case <-ctx.Done():
				return ctx.Err()
			case <-a.stopCh:
				return nil
			}
		}
	}
}

// runLoop runs the main agent loop (heartbeat + event subscription).
func (a *Agent) runLoop(ctx context.Context) error {
	// Create child context for this session
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start event subscription in background
	eventErrCh := make(chan error, 1)
	go func() {
		eventErrCh <- a.client.Subscribe(sessionCtx)
	}()

	// Heartbeat ticker
	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	// Send initial heartbeat
	a.sendHeartbeat(sessionCtx)

	for {
		select {
		case <-sessionCtx.Done():
			return sessionCtx.Err()

		case <-a.stopCh:
			cancel()
			return nil

		case err := <-eventErrCh:
			// Event subscription ended
			if err != nil {
				log.Error().Err(err).Msg("Event subscription failed")
			}
			return err

		case <-ticker.C:
			a.sendHeartbeat(sessionCtx)
		}
	}
}

// sendHeartbeat sends a heartbeat to the Hub.
func (a *Agent) sendHeartbeat(ctx context.Context) {
	// Determine health state
	state := pb.InstanceState_INSTANCE_STATE_HEALTHY
	message := "ok"

	if !a.sentinel.IsRunning() {
		state = pb.InstanceState_INSTANCE_STATE_UNHEALTHY
		message = "sentinel not running"
	}

	// TODO: Collect real metrics from Sentinel
	metrics := &pb.InstanceMetrics{
		RequestsTotal:  0,
		RequestsFailed: 0,
		LatencyP50Ms:   0,
		LatencyP99Ms:   0,
	}

	resp, err := a.client.Heartbeat(ctx, state, message, metrics)
	if err != nil {
		log.Error().Err(err).Msg("Heartbeat failed")
		return
	}

	// Process pending actions
	for _, action := range resp.Actions {
		a.processAction(ctx, action)
	}

	// Fetch config if update available
	if resp.ConfigUpdateAvailable {
		log.Info().Str("version", resp.LatestConfigVersion).Msg("Config update available")
		cfg, err := a.client.FetchConfig(ctx, resp.LatestConfigVersion)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch config")
		} else {
			if err := a.OnConfigUpdate(cfg.Version, cfg.Hash, cfg.Content); err != nil {
				log.Error().Err(err).Msg("Failed to apply config")
			}
		}
	}
}

// processAction processes a pending action from the Hub.
func (a *Agent) processAction(ctx context.Context, action *pb.PendingAction) {
	log.Debug().
		Str("action_id", action.ActionId).
		Str("type", action.Type.String()).
		Msg("Processing pending action")

	switch action.Type {
	case pb.ActionType_ACTION_TYPE_FETCH_CONFIG:
		version := action.Params["version"]
		cfg, err := a.client.FetchConfig(ctx, version)
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch config")
			return
		}
		if err := a.OnConfigUpdate(cfg.Version, cfg.Hash, cfg.Content); err != nil {
			log.Error().Err(err).Msg("Failed to apply config")
		}

	case pb.ActionType_ACTION_TYPE_DRAIN:
		log.Info().Msg("Drain requested")
		// TODO: Implement drain logic

	default:
		log.Warn().Str("type", action.Type.String()).Msg("Unknown action type")
	}
}

// Stop stops the agent gracefully.
func (a *Agent) Stop(ctx context.Context) error {
	log.Info().Msg("Stopping agent...")

	// Signal stop
	close(a.stopCh)

	// Deregister from Hub
	if a.client.IsConnected() {
		a.client.Deregister(ctx, "shutdown")
	}

	// Close connection
	return a.client.Close()
}

// OnConfigUpdate implements EventHandler.
func (a *Agent) OnConfigUpdate(version, hash, content string) error {
	log.Info().
		Str("version", version).
		Str("hash", hash[:16]+"...").
		Msg("Applying config update...")

	// Write config to disk
	if err := a.sentinel.WriteConfig(content); err != nil {
		return err
	}

	// Reload Sentinel
	if err := a.sentinel.Reload(); err != nil {
		log.Warn().Err(err).Msg("Failed to reload Sentinel, config saved but not applied")
		// Don't return error - config is saved, just not reloaded
	}

	// Update client state
	a.client.UpdateConfigState(version, hash)

	// Persist config state to disk
	if a.state != nil {
		if err := a.state.SetConfigState(version, hash, ""); err != nil {
			log.Warn().Err(err).Msg("Failed to persist config state")
		}
	}

	log.Info().Str("version", version).Msg("Config update applied successfully")
	return nil
}

// OnDeployment implements EventHandler.
func (a *Agent) OnDeployment(deploymentID, configID, configVersion string, isRollback bool) error {
	log.Info().
		Str("deployment_id", deploymentID).
		Str("config_id", configID).
		Str("config_version", configVersion).
		Bool("rollback", isRollback).
		Msg("Processing deployment...")

	// Track active deployment for crash recovery
	if a.state != nil {
		if err := a.state.SetActiveDeployment(deploymentID); err != nil {
			log.Warn().Err(err).Msg("Failed to persist active deployment")
		}
	}

	// Parse version number
	var versionNum int
	fmt.Sscanf(configVersion, "%d", &versionNum)

	// Fetch the config for this deployment using config_id
	cfg, err := a.client.FetchConfigVersion(context.Background(), configID, versionNum)
	if err != nil {
		a.clearActiveDeployment()
		return err
	}

	// Apply the config (this also persists config state)
	if err := a.OnConfigUpdate(fmt.Sprintf("%d", cfg.VersionNumber), cfg.Hash, cfg.Content); err != nil {
		a.clearActiveDeployment()
		// If this was a rollback and it failed, we're in trouble
		if isRollback {
			log.Error().Err(err).Msg("Rollback failed!")
		}
		return err
	}

	// Also persist the config ID for better tracking
	if a.state != nil {
		if err := a.state.SetConfigState(fmt.Sprintf("%d", cfg.VersionNumber), cfg.Hash, configID); err != nil {
			log.Warn().Err(err).Msg("Failed to persist config state with config ID")
		}
	}

	// Clear active deployment on success
	a.clearActiveDeployment()

	log.Info().
		Str("deployment_id", deploymentID).
		Str("config_version", configVersion).
		Msg("Deployment completed successfully")

	return nil
}

// OnDrain implements EventHandler.
func (a *Agent) OnDrain(timeoutSecs int, reason string) error {
	log.Info().
		Int("timeout_secs", timeoutSecs).
		Str("reason", reason).
		Msg("Drain requested")

	// TODO: Implement drain logic
	// This would typically:
	// 1. Stop accepting new connections
	// 2. Wait for existing connections to complete
	// 3. Signal ready for shutdown

	return nil
}

// IsRunning returns true if the agent is running.
func (a *Agent) IsRunning() bool {
	a.runningMu.RLock()
	defer a.runningMu.RUnlock()
	return a.running
}

// Client returns the Hub client.
func (a *Agent) Client() *Client {
	return a.client
}

// Sentinel returns the Sentinel manager.
func (a *Agent) Sentinel() *SentinelManager {
	return a.sentinel
}

// State returns the state manager.
func (a *Agent) State() *StateManager {
	return a.state
}

// reportInterruptedDeployment reports any deployment that was interrupted
// by a previous agent crash/restart.
func (a *Agent) reportInterruptedDeployment(ctx context.Context) {
	if a.state == nil {
		return
	}

	deploymentID := a.state.GetActiveDeployment()
	if deploymentID == "" {
		return
	}

	log.Warn().
		Str("deployment_id", deploymentID).
		Msg("Reporting interrupted deployment as failed")

	err := a.client.ReportDeploymentStatus(
		ctx,
		deploymentID,
		pb.DeploymentState_DEPLOYMENT_STATE_FAILED,
		"Deployment interrupted by agent restart",
		"agent_restart: deployment was in progress when agent restarted",
	)
	if err != nil {
		log.Error().Err(err).
			Str("deployment_id", deploymentID).
			Msg("Failed to report interrupted deployment")
		return
	}

	// Clear the active deployment after reporting
	if err := a.state.ClearActiveDeployment(); err != nil {
		log.Warn().Err(err).Msg("Failed to clear active deployment after reporting")
	}

	log.Info().
		Str("deployment_id", deploymentID).
		Msg("Successfully reported interrupted deployment as failed")
}

// clearActiveDeployment clears the active deployment from state.
func (a *Agent) clearActiveDeployment() {
	if a.state == nil {
		return
	}
	if err := a.state.ClearActiveDeployment(); err != nil {
		log.Warn().Err(err).Msg("Failed to clear active deployment")
	}
}
