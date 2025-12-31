package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"

	hubgrpc "github.com/raskell-io/sentinel-hub/internal/grpc"
	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
)

// DeploymentRunner executes a single deployment.
type DeploymentRunner struct {
	deployment    *store.Deployment
	configVersion *store.ConfigVersion
	store         *store.Store
	fleetService  *hubgrpc.FleetService

	// Configuration
	timeout            time.Duration
	instanceTimeout    time.Duration // Timeout for individual instance deployment
	leaseTimeout       time.Duration // How long before a lease is considered stale
	healthCheckRetries int
	healthCheckDelay   time.Duration
	batchDelay         time.Duration

	// State
	instanceResults   map[string]*instanceResult
	instanceResultsMu sync.RWMutex

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

type instanceResult struct {
	Status       pb.DeploymentState
	StartedAt    *time.Time
	CompletedAt  *time.Time
	LastStatusAt *time.Time // Last time agent reported status (lease)
	ErrorMessage string
}

// DeploymentRunnerConfig holds configuration for a deployment runner.
type DeploymentRunnerConfig struct {
	Deployment         *store.Deployment
	ConfigVersion      *store.ConfigVersion
	Store              *store.Store
	FleetService       *hubgrpc.FleetService
	Timeout            time.Duration
	InstanceTimeout    time.Duration // Timeout for individual instance deployment
	LeaseTimeout       time.Duration // How long before a lease is considered stale
	HealthCheckRetries int
	HealthCheckDelay   time.Duration
}

// NewDeploymentRunner creates a new deployment runner.
func NewDeploymentRunner(cfg DeploymentRunnerConfig) *DeploymentRunner {
	ctx, cancel := context.WithCancel(context.Background())

	// Set defaults
	instanceTimeout := cfg.InstanceTimeout
	if instanceTimeout == 0 {
		instanceTimeout = 5 * time.Minute
	}
	leaseTimeout := cfg.LeaseTimeout
	if leaseTimeout == 0 {
		leaseTimeout = 60 * time.Second // Agent must report every 60s or considered stale
	}

	runner := &DeploymentRunner{
		deployment:         cfg.Deployment,
		configVersion:      cfg.ConfigVersion,
		store:              cfg.Store,
		fleetService:       cfg.FleetService,
		timeout:            cfg.Timeout,
		instanceTimeout:    instanceTimeout,
		leaseTimeout:       leaseTimeout,
		healthCheckRetries: cfg.HealthCheckRetries,
		healthCheckDelay:   cfg.HealthCheckDelay,
		batchDelay:         30 * time.Second,
		instanceResults:    make(map[string]*instanceResult),
		ctx:                ctx,
		cancel:             cancel,
	}

	// Initialize instance results and persist to DB
	for _, instanceID := range cfg.Deployment.TargetInstances {
		runner.instanceResults[instanceID] = &instanceResult{
			Status: pb.DeploymentState_DEPLOYMENT_STATE_PENDING,
		}
		// Create initial DB record
		runner.persistInstanceStatus(context.Background(), instanceID)
	}

	return runner
}

// Run executes the deployment.
func (r *DeploymentRunner) Run(parentCtx context.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(parentCtx, r.timeout)
	defer cancel()

	// Merge with runner's cancel context
	go func() {
		select {
		case <-r.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	log.Info().
		Str("deployment_id", r.deployment.ID).
		Str("strategy", string(r.deployment.Strategy)).
		Int("batch_size", r.deployment.BatchSize).
		Int("target_count", len(r.deployment.TargetInstances)).
		Msg("Starting deployment")

	// Mark deployment as in progress
	if err := r.updateStatus(ctx, store.DeploymentStatusInProgress); err != nil {
		return err
	}

	var err error
	switch r.deployment.Strategy {
	case store.DeploymentStrategyAllAtOnce:
		err = r.runAllAtOnce(ctx)
	case store.DeploymentStrategyRolling:
		err = r.runRolling(ctx)
	case store.DeploymentStrategyCanary:
		err = r.runCanary(ctx)
	default:
		err = r.runRolling(ctx) // Default to rolling
	}

	if err != nil {
		log.Error().Err(err).Str("deployment_id", r.deployment.ID).Msg("Deployment failed")
		r.updateStatus(ctx, store.DeploymentStatusFailed)
		return err
	}

	// Check if all instances succeeded
	if r.allSucceeded() {
		log.Info().Str("deployment_id", r.deployment.ID).Msg("Deployment completed successfully")
		r.updateStatus(ctx, store.DeploymentStatusCompleted)
	} else {
		log.Warn().Str("deployment_id", r.deployment.ID).Msg("Deployment completed with failures")
		r.updateStatus(ctx, store.DeploymentStatusFailed)
		return fmt.Errorf("some instances failed")
	}

	return nil
}

// runAllAtOnce deploys to all instances simultaneously.
func (r *DeploymentRunner) runAllAtOnce(ctx context.Context) error {
	log.Info().
		Str("deployment_id", r.deployment.ID).
		Int("instance_count", len(r.deployment.TargetInstances)).
		Msg("Running all-at-once deployment")

	// Send deployment event to all instances
	var wg sync.WaitGroup
	errors := make(chan error, len(r.deployment.TargetInstances))

	for _, instanceID := range r.deployment.TargetInstances {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := r.deployToInstance(ctx, id, 1, 1); err != nil {
				errors <- fmt.Errorf("instance %s: %w", id, err)
			}
		}(instanceID)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d instances failed", len(errs))
	}

	return nil
}

// runRolling deploys to instances in batches.
func (r *DeploymentRunner) runRolling(ctx context.Context) error {
	instances := r.deployment.TargetInstances
	batchSize := r.deployment.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}

	totalBatches := (len(instances) + batchSize - 1) / batchSize

	log.Info().
		Str("deployment_id", r.deployment.ID).
		Int("batch_size", batchSize).
		Int("total_batches", totalBatches).
		Msg("Running rolling deployment")

	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := batchNum * batchSize
		end := start + batchSize
		if end > len(instances) {
			end = len(instances)
		}
		batch := instances[start:end]

		log.Info().
			Str("deployment_id", r.deployment.ID).
			Int("batch", batchNum+1).
			Int("total_batches", totalBatches).
			Int("batch_size", len(batch)).
			Msg("Deploying batch")

		// Deploy to batch
		if err := r.deployBatch(ctx, batch, batchNum+1, totalBatches); err != nil {
			// Rollback on failure
			log.Error().Err(err).
				Str("deployment_id", r.deployment.ID).
				Int("batch", batchNum+1).
				Msg("Batch failed, initiating rollback")

			r.rollbackDeployedInstances(ctx)
			return err
		}

		// Update progress
		r.updateProgress(ctx, batchNum+1, totalBatches)

		// Delay between batches (except for last batch)
		if batchNum < totalBatches-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.batchDelay):
			}
		}
	}

	return nil
}

// runCanary deploys to a small subset first, then proceeds if successful.
func (r *DeploymentRunner) runCanary(ctx context.Context) error {
	instances := r.deployment.TargetInstances
	if len(instances) < 2 {
		// Not enough instances for canary, fall back to all-at-once
		return r.runAllAtOnce(ctx)
	}

	// Canary size is 10% or at least 1
	canarySize := len(instances) / 10
	if canarySize < 1 {
		canarySize = 1
	}

	canaryInstances := instances[:canarySize]
	remainingInstances := instances[canarySize:]

	log.Info().
		Str("deployment_id", r.deployment.ID).
		Int("canary_size", canarySize).
		Int("remaining", len(remainingInstances)).
		Msg("Running canary deployment")

	// Deploy to canary instances
	log.Info().Str("deployment_id", r.deployment.ID).Msg("Deploying to canary instances")
	if err := r.deployBatch(ctx, canaryInstances, 1, 2); err != nil {
		log.Error().Err(err).Str("deployment_id", r.deployment.ID).Msg("Canary deployment failed")
		r.rollbackDeployedInstances(ctx)
		return fmt.Errorf("canary deployment failed: %w", err)
	}

	// Wait for canary validation period
	log.Info().Str("deployment_id", r.deployment.ID).Msg("Canary deployed, waiting for validation...")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(60 * time.Second): // Canary validation period
	}

	// Check canary health
	if !r.checkBatchHealth(canaryInstances) {
		log.Error().Str("deployment_id", r.deployment.ID).Msg("Canary health check failed")
		r.rollbackDeployedInstances(ctx)
		return fmt.Errorf("canary health check failed")
	}

	log.Info().Str("deployment_id", r.deployment.ID).Msg("Canary healthy, deploying to remaining instances")

	// Deploy to remaining instances (using rolling strategy)
	batchSize := r.deployment.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}

	totalBatches := (len(remainingInstances) + batchSize - 1) / batchSize

	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := batchNum * batchSize
		end := start + batchSize
		if end > len(remainingInstances) {
			end = len(remainingInstances)
		}
		batch := remainingInstances[start:end]

		if err := r.deployBatch(ctx, batch, batchNum+2, totalBatches+1); err != nil {
			r.rollbackDeployedInstances(ctx)
			return err
		}

		if batchNum < totalBatches-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.batchDelay):
			}
		}
	}

	return nil
}

// deployBatch deploys to a batch of instances.
func (r *DeploymentRunner) deployBatch(ctx context.Context, instanceIDs []string, batchNum, totalBatches int) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(instanceIDs))

	for _, instanceID := range instanceIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := r.deployToInstance(ctx, id, batchNum, totalBatches); err != nil {
				errors <- fmt.Errorf("instance %s: %w", id, err)
			}
		}(instanceID)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d instances in batch failed", len(errs))
	}

	return nil
}

// deployToInstance deploys to a single instance.
func (r *DeploymentRunner) deployToInstance(ctx context.Context, instanceID string, batchNum, totalBatches int) error {
	log.Debug().
		Str("deployment_id", r.deployment.ID).
		Str("instance_id", instanceID).
		Int("batch", batchNum).
		Msg("Deploying to instance")

	// Update instance result with initial lease
	now := time.Now().UTC()
	r.instanceResultsMu.Lock()
	r.instanceResults[instanceID] = &instanceResult{
		Status:       pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS,
		StartedAt:    &now,
		LastStatusAt: &now, // Initial lease
	}
	r.instanceResultsMu.Unlock()
	r.persistInstanceStatus(ctx, instanceID)

	// Check if instance is subscribed
	if !r.fleetService.IsInstanceSubscribed(instanceID) {
		r.setInstanceError(instanceID, "instance not connected")
		return fmt.Errorf("instance not connected")
	}

	// Send deployment event
	deadline := time.Now().Add(5 * time.Minute)
	strategy := pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ROLLING
	switch r.deployment.Strategy {
	case store.DeploymentStrategyAllAtOnce:
		strategy = pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ALL_AT_ONCE
	case store.DeploymentStrategyCanary:
		strategy = pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_CANARY
	}

	err := r.fleetService.NotifyDeployment(
		instanceID,
		r.deployment.ID,
		r.deployment.ConfigID,
		fmt.Sprintf("%d", r.configVersion.Version),
		strategy,
		batchNum,
		totalBatches,
		deadline,
		false, // not a rollback
	)
	if err != nil {
		r.setInstanceError(instanceID, err.Error())
		return err
	}

	// Wait for instance to report completion
	if err := r.waitForInstance(ctx, instanceID); err != nil {
		r.setInstanceError(instanceID, err.Error())
		return err
	}

	// Mark instance as completed
	completedAt := time.Now().UTC()
	r.instanceResultsMu.Lock()
	if result, ok := r.instanceResults[instanceID]; ok {
		result.Status = pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED
		result.CompletedAt = &completedAt
	}
	r.instanceResultsMu.Unlock()
	r.persistInstanceStatus(ctx, instanceID)

	// Update instance in database
	inst, err := r.store.GetInstance(ctx, instanceID)
	if err == nil && inst != nil {
		inst.CurrentConfigID = &r.deployment.ConfigID
		inst.CurrentConfigVersion = &r.deployment.ConfigVersion
		r.store.UpdateInstance(ctx, inst)
	}

	log.Debug().
		Str("deployment_id", r.deployment.ID).
		Str("instance_id", instanceID).
		Msg("Instance deployment completed")

	return nil
}

// waitForInstance waits for an instance to complete deployment.
// It checks both for timeout and lease expiry (agent not reporting status).
func (r *DeploymentRunner) waitForInstance(ctx context.Context, instanceID string) error {
	// Poll for instance status with timeout
	timeout := time.After(r.instanceTimeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for instance after %v", r.instanceTimeout)
		case <-ticker.C:
			r.instanceResultsMu.RLock()
			result, ok := r.instanceResults[instanceID]
			if !ok {
				r.instanceResultsMu.RUnlock()
				continue
			}

			status := result.Status
			lastStatusAt := result.LastStatusAt
			errorMessage := result.ErrorMessage
			r.instanceResultsMu.RUnlock()

			// Check for lease expiry (agent stopped reporting)
			if lastStatusAt != nil && status == pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS {
				if time.Since(*lastStatusAt) > r.leaseTimeout {
					return fmt.Errorf("lease expired: agent stopped reporting (last status %v ago)", time.Since(*lastStatusAt).Round(time.Second))
				}
			}

			switch status {
			case pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED:
				return nil
			case pb.DeploymentState_DEPLOYMENT_STATE_FAILED:
				return fmt.Errorf("deployment failed: %s", errorMessage)
			case pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK:
				return fmt.Errorf("instance rolled back")
			}
		}
	}
}

// setInstanceError marks an instance as failed.
func (r *DeploymentRunner) setInstanceError(instanceID, errorMsg string) {
	now := time.Now().UTC()
	r.instanceResultsMu.Lock()
	if result, ok := r.instanceResults[instanceID]; ok {
		result.Status = pb.DeploymentState_DEPLOYMENT_STATE_FAILED
		result.CompletedAt = &now
		result.ErrorMessage = errorMsg
	}
	r.instanceResultsMu.Unlock()
	r.persistInstanceStatus(context.Background(), instanceID)
}

// ReportInstanceStatus handles status reports from agents.
// Each status report acts as a lease renewal.
func (r *DeploymentRunner) ReportInstanceStatus(instanceID string, state pb.DeploymentState, message, errorDetails string) {
	now := time.Now().UTC()

	r.instanceResultsMu.Lock()
	result, ok := r.instanceResults[instanceID]
	if !ok {
		r.instanceResultsMu.Unlock()
		return
	}

	result.Status = state
	result.LastStatusAt = &now // Update lease
	if state == pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED ||
		state == pb.DeploymentState_DEPLOYMENT_STATE_FAILED ||
		state == pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK {
		result.CompletedAt = &now
	}
	if errorDetails != "" {
		result.ErrorMessage = errorDetails
	}
	r.instanceResultsMu.Unlock()

	// Persist to database
	r.persistInstanceStatus(context.Background(), instanceID)

	log.Debug().
		Str("deployment_id", r.deployment.ID).
		Str("instance_id", instanceID).
		Str("state", state.String()).
		Str("message", message).
		Msg("Instance status updated")
}

// GetInstanceResults returns the current instance results.
func (r *DeploymentRunner) GetInstanceResults() map[string]InstanceDeploymentResult {
	r.instanceResultsMu.RLock()
	defer r.instanceResultsMu.RUnlock()

	results := make(map[string]InstanceDeploymentResult)
	for id, result := range r.instanceResults {
		results[id] = InstanceDeploymentResult{
			InstanceID:   id,
			Status:       result.Status.String(),
			StartedAt:    result.StartedAt,
			CompletedAt:  result.CompletedAt,
			ErrorMessage: result.ErrorMessage,
		}
	}
	return results
}

// Cancel cancels the deployment.
func (r *DeploymentRunner) Cancel() {
	r.cancel()
}

// persistInstanceStatus saves the current instance status to the database.
func (r *DeploymentRunner) persistInstanceStatus(ctx context.Context, instanceID string) {
	// Skip if store is not available (e.g., in unit tests)
	if r.store == nil {
		return
	}

	r.instanceResultsMu.RLock()
	result, ok := r.instanceResults[instanceID]
	if !ok {
		r.instanceResultsMu.RUnlock()
		return
	}
	// Copy values while holding lock
	status := result.Status
	startedAt := result.StartedAt
	completedAt := result.CompletedAt
	lastStatusAt := result.LastStatusAt
	errorMsg := result.ErrorMessage
	r.instanceResultsMu.RUnlock()

	// Map protobuf state to store status
	var storeStatus store.DeploymentInstanceStatus
	switch status {
	case pb.DeploymentState_DEPLOYMENT_STATE_PENDING:
		storeStatus = store.DeploymentInstanceStatusPending
	case pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS:
		storeStatus = store.DeploymentInstanceStatusInProgress
	case pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED:
		storeStatus = store.DeploymentInstanceStatusCompleted
	case pb.DeploymentState_DEPLOYMENT_STATE_FAILED:
		storeStatus = store.DeploymentInstanceStatusFailed
	case pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK:
		storeStatus = store.DeploymentInstanceStatusRolledBack
	default:
		storeStatus = store.DeploymentInstanceStatusPending
	}

	di := &store.DeploymentInstance{
		DeploymentID: r.deployment.ID,
		InstanceID:   instanceID,
		Status:       storeStatus,
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
		LastStatusAt: lastStatusAt,
	}
	if errorMsg != "" {
		di.ErrorMessage = &errorMsg
	}

	if err := r.store.UpsertDeploymentInstance(ctx, di); err != nil {
		log.Warn().Err(err).
			Str("deployment_id", r.deployment.ID).
			Str("instance_id", instanceID).
			Msg("Failed to persist deployment instance status")
	}
}

// allSucceeded returns true if all instances completed successfully.
func (r *DeploymentRunner) allSucceeded() bool {
	r.instanceResultsMu.RLock()
	defer r.instanceResultsMu.RUnlock()

	for _, result := range r.instanceResults {
		if result.Status != pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
			return false
		}
	}
	return true
}

// checkBatchHealth checks if all instances in a batch are healthy.
func (r *DeploymentRunner) checkBatchHealth(instanceIDs []string) bool {
	r.instanceResultsMu.RLock()
	defer r.instanceResultsMu.RUnlock()

	for _, id := range instanceIDs {
		result, ok := r.instanceResults[id]
		if !ok || result.Status != pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
			return false
		}
	}
	return true
}

// rollbackDeployedInstances initiates rollback for successfully deployed instances.
func (r *DeploymentRunner) rollbackDeployedInstances(ctx context.Context) {
	log.Info().Str("deployment_id", r.deployment.ID).Msg("Initiating rollback")

	r.instanceResultsMu.RLock()
	var deployedInstances []string
	for id, result := range r.instanceResults {
		if result.Status == pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED {
			deployedInstances = append(deployedInstances, id)
		}
	}
	r.instanceResultsMu.RUnlock()

	if len(deployedInstances) == 0 {
		log.Info().Str("deployment_id", r.deployment.ID).Msg("No instances to rollback")
		return
	}

	// Get previous config version
	prevVersion := r.configVersion.Version - 1
	if prevVersion < 1 {
		log.Warn().Str("deployment_id", r.deployment.ID).Msg("No previous version to rollback to")
		return
	}

	prevConfig, err := r.store.GetConfigVersion(ctx, r.deployment.ConfigID, prevVersion)
	if err != nil || prevConfig == nil {
		log.Error().Err(err).Str("deployment_id", r.deployment.ID).Msg("Failed to get previous config version")
		return
	}

	// Send rollback events
	for _, instanceID := range deployedInstances {
		if !r.fleetService.IsInstanceSubscribed(instanceID) {
			continue
		}

		err := r.fleetService.NotifyDeployment(
			instanceID,
			r.deployment.ID+"-rollback",
			r.deployment.ConfigID,
			fmt.Sprintf("%d", prevVersion),
			pb.DeploymentStrategy_DEPLOYMENT_STRATEGY_ALL_AT_ONCE,
			1, 1,
			time.Now().Add(5*time.Minute),
			true, // is rollback
		)
		if err != nil {
			log.Error().Err(err).
				Str("instance_id", instanceID).
				Msg("Failed to send rollback event")
		}
	}

	log.Info().
		Str("deployment_id", r.deployment.ID).
		Int("instance_count", len(deployedInstances)).
		Int("rollback_version", prevVersion).
		Msg("Rollback initiated")
}

// updateStatus updates the deployment status in the database.
func (r *DeploymentRunner) updateStatus(ctx context.Context, status store.DeploymentStatus) error {
	r.deployment.Status = status
	now := time.Now().UTC()

	switch status {
	case store.DeploymentStatusInProgress:
		r.deployment.StartedAt = &now
	case store.DeploymentStatusCompleted, store.DeploymentStatusFailed, store.DeploymentStatusCancelled:
		r.deployment.CompletedAt = &now
	}

	return r.store.UpdateDeployment(ctx, r.deployment)
}

// updateProgress updates the deployment progress.
func (r *DeploymentRunner) updateProgress(ctx context.Context, currentBatch, totalBatches int) {
	r.instanceResultsMu.RLock()
	completed := 0
	failed := 0
	for _, result := range r.instanceResults {
		switch result.Status {
		case pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED:
			completed++
		case pb.DeploymentState_DEPLOYMENT_STATE_FAILED:
			failed++
		}
	}
	r.instanceResultsMu.RUnlock()

	r.deployment.Progress = &store.DeploymentProgress{
		TotalInstances:     len(r.deployment.TargetInstances),
		CompletedInstances: completed,
		FailedInstances:    failed,
		CurrentBatch:       currentBatch,
		TotalBatches:       totalBatches,
	}

	r.store.UpdateDeployment(ctx, r.deployment)
}
