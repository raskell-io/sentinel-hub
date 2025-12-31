package grpc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DeploymentStatusHandler is called when an agent reports deployment status.
type DeploymentStatusHandler func(instanceID, deploymentID string, state pb.DeploymentState, message, errorDetails string)

// FleetService implements the gRPC FleetService for agent communication.
type FleetService struct {
	pb.UnimplementedFleetServiceServer

	store *store.Store

	// Active subscriptions (instance_id -> channel)
	subscribers   map[string]chan *pb.Event
	subscribersMu sync.RWMutex

	// Session tokens (token_hash -> instance_id)
	sessions   map[string]string
	sessionsMu sync.RWMutex

	// Configuration
	heartbeatInterval time.Duration
	sessionTTL        time.Duration

	// External handler for deployment status reports
	deploymentStatusHandler DeploymentStatusHandler
	deploymentStatusMu      sync.RWMutex
}

// NewFleetService creates a new FleetService instance.
func NewFleetService(s *store.Store) *FleetService {
	return &FleetService{
		store:             s,
		subscribers:       make(map[string]chan *pb.Event),
		sessions:          make(map[string]string),
		heartbeatInterval: 30 * time.Second,
		sessionTTL:        24 * time.Hour,
	}
}

// generateToken creates a secure random token.
func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// hashToken creates a SHA256 hash of the token.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// validateToken checks if a token is valid and returns the instance ID.
func (s *FleetService) validateToken(token string) (string, error) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	hash := hashToken(token)
	instanceID, ok := s.sessions[hash]
	if !ok {
		return "", status.Error(codes.Unauthenticated, "invalid or expired token")
	}
	return instanceID, nil
}

// Register registers a new agent with the Hub.
func (s *FleetService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Info().
		Str("instance_id", req.InstanceId).
		Str("instance_name", req.InstanceName).
		Str("hostname", req.Hostname).
		Msg("Agent registration request")

	// Validate request
	if req.InstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "instance_id is required")
	}
	if req.InstanceName == "" {
		return nil, status.Error(codes.InvalidArgument, "instance_name is required")
	}

	// Check if instance exists
	existing, err := s.store.GetInstance(ctx, req.InstanceId)
	if err != nil {
		log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to check existing instance")
		return nil, status.Error(codes.Internal, "failed to check instance")
	}

	now := time.Now().UTC()
	if existing != nil {
		// Update existing instance
		existing.Hostname = req.Hostname
		existing.AgentVersion = req.AgentVersion
		existing.SentinelVersion = req.SentinelVersion
		existing.Status = store.InstanceStatusOnline
		existing.LastSeenAt = &now
		existing.Labels = req.Labels
		existing.Capabilities = req.Capabilities

		if err := s.store.UpdateInstance(ctx, existing); err != nil {
			log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to update instance")
			return nil, status.Error(codes.Internal, "failed to update instance")
		}
	} else {
		// Create new instance
		inst := &store.Instance{
			ID:              req.InstanceId,
			Name:            req.InstanceName,
			Hostname:        req.Hostname,
			AgentVersion:    req.AgentVersion,
			SentinelVersion: req.SentinelVersion,
			Status:          store.InstanceStatusOnline,
			LastSeenAt:      &now,
			Labels:          req.Labels,
			Capabilities:    req.Capabilities,
		}

		if err := s.store.CreateInstance(ctx, inst); err != nil {
			log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to create instance")
			return nil, status.Error(codes.Internal, "failed to create instance")
		}
	}

	// Generate session token
	token, err := generateToken()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate token")
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	// Store session
	s.sessionsMu.Lock()
	s.sessions[hashToken(token)] = req.InstanceId
	s.sessionsMu.Unlock()

	// Get latest config if any is assigned
	var configVersion, configHash string
	inst, _ := s.store.GetInstance(ctx, req.InstanceId)
	if inst != nil && inst.CurrentConfigID != nil {
		ver, err := s.store.GetLatestConfigVersion(ctx, *inst.CurrentConfigID)
		if err == nil && ver != nil {
			configVersion = fmt.Sprintf("%d", ver.Version)
			configHash = ver.ContentHash
		}
	}

	log.Info().
		Str("instance_id", req.InstanceId).
		Str("instance_name", req.InstanceName).
		Msg("Agent registered successfully")

	return &pb.RegisterResponse{
		Token:                    token,
		ConfigVersion:           configVersion,
		ConfigHash:              configHash,
		HeartbeatIntervalSeconds: int32(s.heartbeatInterval.Seconds()),
	}, nil
}

// Heartbeat processes a heartbeat from an agent.
func (s *FleetService) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	// Validate token
	instanceID, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	// Verify instance ID matches token
	if instanceID != req.InstanceId {
		return nil, status.Error(codes.PermissionDenied, "token does not match instance")
	}

	// Get instance
	inst, err := s.store.GetInstance(ctx, req.InstanceId)
	if err != nil {
		log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to get instance")
		return nil, status.Error(codes.Internal, "failed to get instance")
	}
	if inst == nil {
		return nil, status.Error(codes.NotFound, "instance not found")
	}

	// Update instance status
	now := time.Now().UTC()
	inst.LastSeenAt = &now

	// Map proto status to store status
	switch req.Status.State {
	case pb.InstanceState_INSTANCE_STATE_HEALTHY:
		inst.Status = store.InstanceStatusOnline
	case pb.InstanceState_INSTANCE_STATE_DEGRADED:
		inst.Status = store.InstanceStatusDegraded
	case pb.InstanceState_INSTANCE_STATE_UNHEALTHY:
		inst.Status = store.InstanceStatusOffline
	}

	if err := s.store.UpdateInstance(ctx, inst); err != nil {
		log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to update instance")
		return nil, status.Error(codes.Internal, "failed to update instance")
	}

	// Check if config update is available
	var configUpdateAvailable bool
	var latestConfigVersion string
	var actions []*pb.PendingAction

	if inst.CurrentConfigID != nil {
		latestVer, err := s.store.GetLatestConfigVersion(ctx, *inst.CurrentConfigID)
		if err == nil && latestVer != nil {
			latestConfigVersion = fmt.Sprintf("%d", latestVer.Version)

			// Compare with agent's current version
			if req.CurrentConfigVersion != latestConfigVersion ||
				req.CurrentConfigHash != latestVer.ContentHash {
				configUpdateAvailable = true
				actions = append(actions, &pb.PendingAction{
					Type:     pb.ActionType_ACTION_TYPE_FETCH_CONFIG,
					ActionId: uuid.New().String(),
					Params: map[string]string{
						"version": latestConfigVersion,
					},
				})
			}
		}
	}

	log.Debug().
		Str("instance_id", req.InstanceId).
		Bool("config_update", configUpdateAvailable).
		Msg("Heartbeat processed")

	return &pb.HeartbeatResponse{
		ConfigUpdateAvailable: configUpdateAvailable,
		LatestConfigVersion:   latestConfigVersion,
		Actions:               actions,
	}, nil
}

// Deregister handles agent deregistration.
func (s *FleetService) Deregister(ctx context.Context, req *pb.DeregisterRequest) (*pb.DeregisterResponse, error) {
	// Validate token
	instanceID, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	if instanceID != req.InstanceId {
		return nil, status.Error(codes.PermissionDenied, "token does not match instance")
	}

	log.Info().
		Str("instance_id", req.InstanceId).
		Str("reason", req.Reason).
		Msg("Agent deregistration request")

	// Update instance status to offline
	if err := s.store.UpdateInstanceStatus(ctx, req.InstanceId, store.InstanceStatusOffline); err != nil {
		log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to update instance status")
	}

	// Remove session
	s.sessionsMu.Lock()
	delete(s.sessions, hashToken(req.Token))
	s.sessionsMu.Unlock()

	// Remove subscriber if exists
	s.subscribersMu.Lock()
	if ch, ok := s.subscribers[req.InstanceId]; ok {
		close(ch)
		delete(s.subscribers, req.InstanceId)
	}
	s.subscribersMu.Unlock()

	return &pb.DeregisterResponse{Acknowledged: true}, nil
}

// GetConfig returns the configuration for an instance.
func (s *FleetService) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	// Validate token
	instanceID, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	if instanceID != req.InstanceId {
		return nil, status.Error(codes.PermissionDenied, "token does not match instance")
	}

	// Get instance to find assigned config
	inst, err := s.store.GetInstance(ctx, req.InstanceId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get instance")
	}
	if inst == nil {
		return nil, status.Error(codes.NotFound, "instance not found")
	}

	if inst.CurrentConfigID == nil {
		return nil, status.Error(codes.NotFound, "no configuration assigned to instance")
	}

	// Get config version
	var ver *store.ConfigVersion
	if req.Version != "" {
		// Specific version requested
		var versionNum int
		fmt.Sscanf(req.Version, "%d", &versionNum)
		ver, err = s.store.GetConfigVersion(ctx, *inst.CurrentConfigID, versionNum)
	} else {
		// Latest version
		ver, err = s.store.GetLatestConfigVersion(ctx, *inst.CurrentConfigID)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get configuration")
	}
	if ver == nil {
		return nil, status.Error(codes.NotFound, "configuration version not found")
	}

	log.Info().
		Str("instance_id", req.InstanceId).
		Str("config_id", *inst.CurrentConfigID).
		Int("version", ver.Version).
		Msg("Config fetched by agent")

	return &pb.GetConfigResponse{
		Version:   fmt.Sprintf("%d", ver.Version),
		Hash:      ver.ContentHash,
		Content:   ver.Content,
		CreatedAt: timestamppb.New(ver.CreatedAt),
	}, nil
}

// GetConfigVersion returns a specific configuration version.
func (s *FleetService) GetConfigVersion(ctx context.Context, req *pb.GetConfigVersionRequest) (*pb.GetConfigVersionResponse, error) {
	// Validate token
	_, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	ver, err := s.store.GetConfigVersion(ctx, req.ConfigId, int(req.VersionNumber))
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get configuration version")
	}
	if ver == nil {
		return nil, status.Error(codes.NotFound, "configuration version not found")
	}

	var changeSummary string
	if ver.ChangeSummary != nil {
		changeSummary = *ver.ChangeSummary
	}

	return &pb.GetConfigVersionResponse{
		ConfigId:      ver.ConfigID,
		VersionNumber: int32(ver.Version),
		Hash:          ver.ContentHash,
		Content:       ver.Content,
		ChangeSummary: changeSummary,
		CreatedAt:     timestamppb.New(ver.CreatedAt),
	}, nil
}

// Subscribe creates a server-streaming connection for push events.
func (s *FleetService) Subscribe(req *pb.SubscribeRequest, stream pb.FleetService_SubscribeServer) error {
	// Validate token
	instanceID, err := s.validateToken(req.Token)
	if err != nil {
		return err
	}

	if instanceID != req.InstanceId {
		return status.Error(codes.PermissionDenied, "token does not match instance")
	}

	log.Info().
		Str("instance_id", req.InstanceId).
		Msg("Agent subscribed to event stream")

	// Create event channel for this subscriber
	eventCh := make(chan *pb.Event, 100)

	// Register subscriber
	s.subscribersMu.Lock()
	// Close existing channel if any
	if oldCh, ok := s.subscribers[req.InstanceId]; ok {
		close(oldCh)
	}
	s.subscribers[req.InstanceId] = eventCh
	s.subscribersMu.Unlock()

	// Cleanup on exit
	defer func() {
		s.subscribersMu.Lock()
		if ch, ok := s.subscribers[req.InstanceId]; ok && ch == eventCh {
			delete(s.subscribers, req.InstanceId)
		}
		s.subscribersMu.Unlock()
		log.Info().Str("instance_id", req.InstanceId).Msg("Agent unsubscribed from event stream")
	}()

	// Send periodic pings to keep connection alive
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	log.Debug().Str("instance_id", req.InstanceId).Msg("Subscribe entering event loop")

	for {
		select {
		case <-stream.Context().Done():
			log.Debug().Str("instance_id", req.InstanceId).Err(stream.Context().Err()).Msg("Subscribe context done")
			return stream.Context().Err()

		case event, ok := <-eventCh:
			if !ok {
				// Channel closed, subscriber removed
				log.Debug().Str("instance_id", req.InstanceId).Msg("Subscribe channel closed")
				return nil
			}
			if err := stream.Send(event); err != nil {
				log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to send event")
				return err
			}

		case <-pingTicker.C:
			// Send ping event
			ping := &pb.Event{
				EventId:   uuid.New().String(),
				Type:      pb.EventType_EVENT_TYPE_PING,
				Timestamp: timestamppb.Now(),
				Payload: &pb.Event_Ping{
					Ping: &pb.PingEvent{
						ServerTime: timestamppb.Now(),
					},
				},
			}
			if err := stream.Send(ping); err != nil {
				log.Error().Err(err).Str("instance_id", req.InstanceId).Msg("Failed to send ping")
				return err
			}
		}
	}
}

// AckDeployment acknowledges receipt of a deployment request.
func (s *FleetService) AckDeployment(ctx context.Context, req *pb.AckDeploymentRequest) (*pb.AckDeploymentResponse, error) {
	// Validate token
	instanceID, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	if instanceID != req.InstanceId {
		return nil, status.Error(codes.PermissionDenied, "token does not match instance")
	}

	log.Info().
		Str("instance_id", req.InstanceId).
		Str("deployment_id", req.DeploymentId).
		Bool("accepted", req.Accepted).
		Msg("Deployment acknowledgment received")

	// TODO: Update deployment instance status in database

	var instruction string
	if req.Accepted {
		instruction = "proceed with deployment"
	} else {
		instruction = "deployment rejected by agent"
	}

	return &pb.AckDeploymentResponse{
		Acknowledged: true,
		Instruction:  instruction,
	}, nil
}

// ReportDeploymentStatus reports the status of a deployment on an instance.
func (s *FleetService) ReportDeploymentStatus(ctx context.Context, req *pb.DeploymentStatusRequest) (*pb.DeploymentStatusResponse, error) {
	// Validate token
	instanceID, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	if instanceID != req.InstanceId {
		return nil, status.Error(codes.PermissionDenied, "token does not match instance")
	}

	log.Info().
		Str("instance_id", req.InstanceId).
		Str("deployment_id", req.DeploymentId).
		Str("state", req.State.String()).
		Str("message", req.Message).
		Msg("Deployment status report received")

	// Update instance status based on deployment state
	switch req.State {
	case pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS:
		s.store.UpdateInstanceStatus(ctx, req.InstanceId, store.InstanceStatusDeploying)
	case pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED:
		s.store.UpdateInstanceStatus(ctx, req.InstanceId, store.InstanceStatusOnline)
	case pb.DeploymentState_DEPLOYMENT_STATE_FAILED:
		s.store.UpdateInstanceStatus(ctx, req.InstanceId, store.InstanceStatusDegraded)
	case pb.DeploymentState_DEPLOYMENT_STATE_ROLLED_BACK:
		s.store.UpdateInstanceStatus(ctx, req.InstanceId, store.InstanceStatusOnline)
	}

	// Notify orchestrator of status update
	s.deploymentStatusMu.RLock()
	handler := s.deploymentStatusHandler
	s.deploymentStatusMu.RUnlock()

	if handler != nil {
		handler(req.InstanceId, req.DeploymentId, req.State, req.Message, req.ErrorDetails)
	}

	return &pb.DeploymentStatusResponse{
		Acknowledged: true,
	}, nil
}

// BroadcastEvent sends an event to all subscribed agents.
func (s *FleetService) BroadcastEvent(event *pb.Event) {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()

	for instanceID, ch := range s.subscribers {
		select {
		case ch <- event:
			log.Debug().Str("instance_id", instanceID).Str("event_type", event.Type.String()).Msg("Event sent")
		default:
			log.Warn().Str("instance_id", instanceID).Msg("Event channel full, dropping event")
		}
	}
}

// SendEventToInstance sends an event to a specific instance.
func (s *FleetService) SendEventToInstance(instanceID string, event *pb.Event) error {
	s.subscribersMu.RLock()
	ch, ok := s.subscribers[instanceID]
	s.subscribersMu.RUnlock()

	if !ok {
		return fmt.Errorf("instance %s not subscribed", instanceID)
	}

	select {
	case ch <- event:
		log.Debug().Str("instance_id", instanceID).Str("event_type", event.Type.String()).Msg("Event sent")
		return nil
	default:
		return fmt.Errorf("event channel full for instance %s", instanceID)
	}
}

// NotifyConfigUpdate sends a config update event to specified instances.
func (s *FleetService) NotifyConfigUpdate(instanceIDs []string, configVersion, configHash, changeSummary string) {
	event := &pb.Event{
		EventId:   uuid.New().String(),
		Type:      pb.EventType_EVENT_TYPE_CONFIG_UPDATE,
		Timestamp: timestamppb.Now(),
		Payload: &pb.Event_ConfigUpdate{
			ConfigUpdate: &pb.ConfigUpdateEvent{
				ConfigVersion: configVersion,
				ConfigHash:    configHash,
				ChangeSummary: changeSummary,
			},
		},
	}

	for _, instanceID := range instanceIDs {
		if err := s.SendEventToInstance(instanceID, event); err != nil {
			log.Warn().Err(err).Str("instance_id", instanceID).Msg("Failed to notify config update")
		}
	}
}

// NotifyDeployment sends a deployment event to specified instances.
func (s *FleetService) NotifyDeployment(instanceID, deploymentID, configID, configVersion string, strategy pb.DeploymentStrategy, batchPos, batchTotal int, deadline time.Time, isRollback bool) error {
	event := &pb.Event{
		EventId:   uuid.New().String(),
		Type:      pb.EventType_EVENT_TYPE_DEPLOYMENT,
		Timestamp: timestamppb.Now(),
		Payload: &pb.Event_Deployment{
			Deployment: &pb.DeploymentEvent{
				DeploymentId:  deploymentID,
				ConfigId:      configID,
				ConfigVersion: configVersion,
				Strategy:      strategy,
				BatchPosition: int32(batchPos),
				BatchTotal:    int32(batchTotal),
				Deadline:      timestamppb.New(deadline),
				IsRollback:    isRollback,
			},
		},
	}

	return s.SendEventToInstance(instanceID, event)
}

// GetSubscriberCount returns the number of active subscribers.
func (s *FleetService) GetSubscriberCount() int {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()
	return len(s.subscribers)
}

// IsInstanceSubscribed checks if an instance is currently subscribed.
func (s *FleetService) IsInstanceSubscribed(instanceID string) bool {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()
	_, ok := s.subscribers[instanceID]
	return ok
}

// SetDeploymentStatusHandler sets the handler for deployment status reports.
func (s *FleetService) SetDeploymentStatusHandler(handler DeploymentStatusHandler) {
	s.deploymentStatusMu.Lock()
	defer s.deploymentStatusMu.Unlock()
	s.deploymentStatusHandler = handler
}
