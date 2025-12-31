package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	hubgrpc "github.com/raskell-io/sentinel-hub/internal/grpc"
	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
)

// Orchestrator manages deployment operations across the fleet.
type Orchestrator struct {
	store        *store.Store
	fleetService *hubgrpc.FleetService

	// Active deployments
	deployments   map[string]*DeploymentRunner
	deploymentsMu sync.RWMutex

	// Configuration
	defaultTimeout     time.Duration
	healthCheckRetries int
	healthCheckDelay   time.Duration

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOrchestrator creates a new deployment orchestrator.
func NewOrchestrator(s *store.Store, fs *hubgrpc.FleetService) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	return &Orchestrator{
		store:              s,
		fleetService:       fs,
		deployments:        make(map[string]*DeploymentRunner),
		defaultTimeout:     10 * time.Minute,
		healthCheckRetries: 3,
		healthCheckDelay:   5 * time.Second,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start starts the orchestrator background processes.
func (o *Orchestrator) Start() error {
	log.Info().Msg("Starting deployment orchestrator")

	// Recover any orphaned deployments from previous run
	if err := o.RecoverOrphanedDeployments(o.ctx); err != nil {
		log.Error().Err(err).Msg("Failed to recover orphaned deployments")
	}

	// Start cleanup routine
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.cleanupRoutine()
	}()

	return nil
}

// Stop gracefully stops the orchestrator.
func (o *Orchestrator) Stop() error {
	log.Info().Msg("Stopping deployment orchestrator")
	o.cancel()

	// Cancel all active deployments
	o.deploymentsMu.Lock()
	for _, runner := range o.deployments {
		runner.Cancel()
	}
	o.deploymentsMu.Unlock()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for orchestrator to stop")
	}
}

// CreateDeployment creates and starts a new deployment.
func (o *Orchestrator) CreateDeployment(ctx context.Context, req CreateDeploymentRequest) (*store.Deployment, error) {
	log.Info().
		Str("config_id", req.ConfigID).
		Int("config_version", req.ConfigVersion).
		Str("strategy", string(req.Strategy)).
		Int("target_count", len(req.TargetInstances)).
		Msg("Creating deployment")

	// Validate config exists
	cfg, err := o.store.GetConfig(ctx, req.ConfigID)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("config not found: %s", req.ConfigID)
	}

	// Use current version if not specified
	configVersion := req.ConfigVersion
	if configVersion == 0 {
		configVersion = cfg.CurrentVersion
	}

	// Validate config version exists
	ver, err := o.store.GetConfigVersion(ctx, req.ConfigID, configVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get config version: %w", err)
	}
	if ver == nil {
		return nil, fmt.Errorf("config version %d not found", configVersion)
	}

	// Resolve target instances
	targetIDs, err := o.resolveTargets(ctx, req.TargetInstances, req.TargetLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve targets: %w", err)
	}
	if len(targetIDs) == 0 {
		return nil, fmt.Errorf("no target instances found")
	}

	// Set defaults
	strategy := req.Strategy
	if strategy == "" {
		strategy = store.DeploymentStrategyRolling
	}
	batchSize := req.BatchSize
	if batchSize == 0 {
		batchSize = 1
	}

	// Create deployment record
	dep := &store.Deployment{
		ID:              uuid.New().String(),
		ConfigID:        req.ConfigID,
		ConfigVersion:   configVersion,
		TargetInstances: targetIDs,
		Strategy:        strategy,
		BatchSize:       batchSize,
		Status:          store.DeploymentStatusPending,
		CreatedBy:       req.CreatedBy,
		Progress: &store.DeploymentProgress{
			TotalInstances: len(targetIDs),
		},
	}

	if err := o.store.CreateDeployment(ctx, dep); err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Start deployment in background
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.runDeployment(dep.ID)
	}()

	log.Info().
		Str("deployment_id", dep.ID).
		Int("target_count", len(targetIDs)).
		Msg("Deployment created")

	return dep, nil
}

// CreateDeploymentRequest holds parameters for creating a deployment.
type CreateDeploymentRequest struct {
	ConfigID        string
	ConfigVersion   int
	TargetInstances []string            // Specific instance IDs
	TargetLabels    map[string]string   // Label selector (alternative to IDs)
	Strategy        store.DeploymentStrategy
	BatchSize       int
	CreatedBy       *string
}

// CancelDeployment cancels an in-progress deployment.
func (o *Orchestrator) CancelDeployment(ctx context.Context, deploymentID string) error {
	o.deploymentsMu.RLock()
	runner, exists := o.deployments[deploymentID]
	o.deploymentsMu.RUnlock()

	if exists {
		runner.Cancel()
	}

	// Update status in database
	dep, err := o.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}
	if dep == nil {
		return fmt.Errorf("deployment not found")
	}

	if dep.Status == store.DeploymentStatusPending || dep.Status == store.DeploymentStatusInProgress {
		now := time.Now().UTC()
		dep.Status = store.DeploymentStatusCancelled
		dep.CompletedAt = &now
		if err := o.store.UpdateDeployment(ctx, dep); err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
	}

	log.Info().Str("deployment_id", deploymentID).Msg("Deployment cancelled")
	return nil
}

// GetDeploymentStatus returns the current status of a deployment.
func (o *Orchestrator) GetDeploymentStatus(ctx context.Context, deploymentID string) (*DeploymentStatus, error) {
	dep, err := o.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found")
	}

	status := &DeploymentStatus{
		Deployment:      dep,
		InstanceResults: make(map[string]InstanceDeploymentResult),
	}

	// Get per-instance status from runner if active
	o.deploymentsMu.RLock()
	runner, exists := o.deployments[deploymentID]
	o.deploymentsMu.RUnlock()

	if exists {
		// Runner is active, get live status
		status.InstanceResults = runner.GetInstanceResults()
	} else {
		// Runner is gone, load from database
		instances, err := o.store.ListDeploymentInstances(ctx, deploymentID)
		if err != nil {
			log.Warn().Err(err).Str("deployment_id", deploymentID).Msg("Failed to load deployment instances from DB")
		} else {
			for _, di := range instances {
				status.InstanceResults[di.InstanceID] = InstanceDeploymentResult{
					InstanceID:   di.InstanceID,
					Status:       string(di.Status),
					StartedAt:    di.StartedAt,
					CompletedAt:  di.CompletedAt,
					ErrorMessage: stringValue(di.ErrorMessage),
				}
			}
		}
	}

	return status, nil
}

// stringValue safely dereferences a string pointer, returning empty string if nil.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// DeploymentStatus provides detailed status of a deployment.
type DeploymentStatus struct {
	Deployment      *store.Deployment
	InstanceResults map[string]InstanceDeploymentResult
}

// InstanceDeploymentResult tracks per-instance deployment status.
type InstanceDeploymentResult struct {
	InstanceID   string
	Status       string
	StartedAt    *time.Time
	CompletedAt  *time.Time
	ErrorMessage string
}

// resolveTargets resolves target instances from IDs or labels.
func (o *Orchestrator) resolveTargets(ctx context.Context, instanceIDs []string, labels map[string]string) ([]string, error) {
	if len(instanceIDs) > 0 {
		// Verify all instances exist
		for _, id := range instanceIDs {
			inst, err := o.store.GetInstance(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get instance %s: %w", id, err)
			}
			if inst == nil {
				return nil, fmt.Errorf("instance not found: %s", id)
			}
		}
		return instanceIDs, nil
	}

	// Resolve by labels
	instances, err := o.store.ListInstances(ctx, store.ListInstancesOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	var result []string
	for _, inst := range instances {
		if matchLabels(inst.Labels, labels) {
			result = append(result, inst.ID)
		}
	}

	return result, nil
}

// matchLabels checks if instance labels match the selector.
func matchLabels(instanceLabels, selector map[string]string) bool {
	if len(selector) == 0 {
		return true // Empty selector matches all
	}
	for k, v := range selector {
		if instanceLabels[k] != v {
			return false
		}
	}
	return true
}

// runDeployment executes a deployment.
func (o *Orchestrator) runDeployment(deploymentID string) {
	ctx := o.ctx

	dep, err := o.store.GetDeployment(ctx, deploymentID)
	if err != nil || dep == nil {
		log.Error().Err(err).Str("deployment_id", deploymentID).Msg("Failed to get deployment")
		return
	}

	// Get config content
	ver, err := o.store.GetConfigVersion(ctx, dep.ConfigID, dep.ConfigVersion)
	if err != nil || ver == nil {
		log.Error().Err(err).Str("deployment_id", deploymentID).Msg("Failed to get config version")
		o.failDeployment(ctx, dep, "Failed to get config version")
		return
	}

	// Create runner
	runner := NewDeploymentRunner(DeploymentRunnerConfig{
		Deployment:         dep,
		ConfigVersion:      ver,
		Store:              o.store,
		FleetService:       o.fleetService,
		Timeout:            o.defaultTimeout,
		HealthCheckRetries: o.healthCheckRetries,
		HealthCheckDelay:   o.healthCheckDelay,
	})

	// Register runner
	o.deploymentsMu.Lock()
	o.deployments[deploymentID] = runner
	o.deploymentsMu.Unlock()

	defer func() {
		o.deploymentsMu.Lock()
		delete(o.deployments, deploymentID)
		o.deploymentsMu.Unlock()
	}()

	// Run deployment
	if err := runner.Run(ctx); err != nil {
		log.Error().Err(err).Str("deployment_id", deploymentID).Msg("Deployment failed")
	}
}

// failDeployment marks a deployment as failed.
func (o *Orchestrator) failDeployment(ctx context.Context, dep *store.Deployment, reason string) {
	now := time.Now().UTC()
	dep.Status = store.DeploymentStatusFailed
	dep.CompletedAt = &now
	if err := o.store.UpdateDeployment(ctx, dep); err != nil {
		log.Error().Err(err).Str("deployment_id", dep.ID).Msg("Failed to update deployment status")
	}
}

// RecoverOrphanedDeployments marks any orphaned deployments as failed.
// This should be called on hub startup to handle deployments that were
// interrupted by a hub restart.
func (o *Orchestrator) RecoverOrphanedDeployments(ctx context.Context) error {
	now := time.Now().UTC()
	recoveredCount := 0

	// Find deployments stuck in pending or in_progress state
	for _, status := range []store.DeploymentStatus{
		store.DeploymentStatusPending,
		store.DeploymentStatusInProgress,
	} {
		deps, err := o.store.ListDeployments(ctx, store.ListDeploymentsOptions{
			Status: status,
		})
		if err != nil {
			return fmt.Errorf("failed to list %s deployments: %w", status, err)
		}

		for i := range deps {
			dep := deps[i]
			log.Warn().
				Str("deployment_id", dep.ID).
				Str("previous_status", string(dep.Status)).
				Msg("Recovering orphaned deployment")

			// Mark deployment as failed
			dep.Status = store.DeploymentStatusFailed
			dep.CompletedAt = &now
			if dep.Progress == nil {
				dep.Progress = &store.DeploymentProgress{}
			}
			dep.Progress.FailureReason = "hub_restart: deployment interrupted by hub restart"

			if err := o.store.UpdateDeployment(ctx, &dep); err != nil {
				log.Error().Err(err).
					Str("deployment_id", dep.ID).
					Msg("Failed to update orphaned deployment")
				continue
			}

			// Mark all pending/in-progress instances as failed
			instances, err := o.store.ListDeploymentInstances(ctx, dep.ID)
			if err != nil {
				log.Error().Err(err).
					Str("deployment_id", dep.ID).
					Msg("Failed to list deployment instances")
				continue
			}

			for _, di := range instances {
				if di.Status == store.DeploymentInstanceStatusPending ||
					di.Status == store.DeploymentInstanceStatusInProgress {
					di.Status = store.DeploymentInstanceStatusFailed
					di.CompletedAt = &now
					errMsg := "hub_restart: deployment interrupted by hub restart"
					di.ErrorMessage = &errMsg

					if err := o.store.UpdateDeploymentInstance(ctx, di); err != nil {
						log.Error().Err(err).
							Str("deployment_id", dep.ID).
							Str("instance_id", di.InstanceID).
							Msg("Failed to update deployment instance")
					}
				}
			}

			recoveredCount++
		}
	}

	if recoveredCount > 0 {
		log.Info().
			Int("count", recoveredCount).
			Msg("Recovered orphaned deployments")
	}

	return nil
}

// cleanupRoutine periodically cleans up old deployments.
func (o *Orchestrator) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			// Could clean up old deployment records here
			log.Debug().Msg("Deployment cleanup tick")
		}
	}
}

// ReportInstanceStatus handles status reports from agents.
func (o *Orchestrator) ReportInstanceStatus(instanceID, deploymentID string, state pb.DeploymentState, message, errorDetails string) {
	o.deploymentsMu.RLock()
	runner, exists := o.deployments[deploymentID]
	o.deploymentsMu.RUnlock()

	if !exists {
		log.Warn().
			Str("deployment_id", deploymentID).
			Str("instance_id", instanceID).
			Msg("Received status for unknown deployment")
		return
	}

	runner.ReportInstanceStatus(instanceID, state, message, errorDetails)
}
