package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Client manages the connection to the Hub and handles all gRPC communication.
type Client struct {
	// Configuration
	hubURL          string
	instanceID      string
	instanceName    string
	hostname        string
	agentVersion    string
	sentinelVersion string
	labels          map[string]string
	capabilities    []string

	// Connection state
	conn   *grpc.ClientConn
	client pb.FleetServiceClient
	token  string
	connMu sync.RWMutex

	// Current config state
	currentConfigVersion string
	currentConfigHash    string
	configMu             sync.RWMutex

	// Event handling
	eventHandler EventHandler

	// Reconnection settings
	reconnectBackoff    time.Duration
	maxReconnectBackoff time.Duration
}

// EventHandler is called when events are received from the Hub.
type EventHandler interface {
	OnConfigUpdate(version, hash, content string) error
	OnDeployment(deploymentID, configVersion string, isRollback bool) error
	OnDrain(timeoutSecs int, reason string) error
}

// ClientConfig holds configuration for creating a new Client.
type ClientConfig struct {
	HubURL          string
	InstanceID      string
	InstanceName    string
	AgentVersion    string
	SentinelVersion string
	Labels          map[string]string
	Capabilities    []string
	EventHandler    EventHandler
}

// NewClient creates a new Hub client.
func NewClient(cfg ClientConfig) (*Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	instanceID := cfg.InstanceID
	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	return &Client{
		hubURL:              cfg.HubURL,
		instanceID:          instanceID,
		instanceName:        cfg.InstanceName,
		hostname:            hostname,
		agentVersion:        cfg.AgentVersion,
		sentinelVersion:     cfg.SentinelVersion,
		labels:              cfg.Labels,
		capabilities:        cfg.Capabilities,
		eventHandler:        cfg.EventHandler,
		reconnectBackoff:    time.Second,
		maxReconnectBackoff: 5 * time.Minute,
	}, nil
}

// Connect establishes a connection to the Hub.
func (c *Client) Connect(ctx context.Context) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		c.conn.Close()
	}

	log.Info().Str("hub_url", c.hubURL).Msg("Connecting to Hub...")

	// TODO: Add TLS support
	conn, err := grpc.NewClient(
		c.hubURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.client = pb.NewFleetServiceClient(conn)

	log.Info().Str("hub_url", c.hubURL).Msg("Connected to Hub")
	return nil
}

// Close closes the connection to the Hub.
func (c *Client) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Register registers this agent with the Hub.
func (c *Client) Register(ctx context.Context) error {
	c.connMu.RLock()
	client := c.client
	c.connMu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	log.Info().
		Str("instance_id", c.instanceID).
		Str("instance_name", c.instanceName).
		Msg("Registering with Hub...")

	resp, err := client.Register(ctx, &pb.RegisterRequest{
		InstanceId:      c.instanceID,
		InstanceName:    c.instanceName,
		Hostname:        c.hostname,
		AgentVersion:    c.agentVersion,
		SentinelVersion: c.sentinelVersion,
		Labels:          c.labels,
		Capabilities:    c.capabilities,
	})
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	c.connMu.Lock()
	c.token = resp.Token
	c.connMu.Unlock()

	// Store initial config info
	if resp.ConfigVersion != "" {
		c.configMu.Lock()
		c.currentConfigVersion = resp.ConfigVersion
		c.currentConfigHash = resp.ConfigHash
		c.configMu.Unlock()
	}

	log.Info().
		Str("instance_id", c.instanceID).
		Int32("heartbeat_interval", resp.HeartbeatIntervalSeconds).
		Str("config_version", resp.ConfigVersion).
		Msg("Registered with Hub successfully")

	return nil
}

// Deregister gracefully deregisters from the Hub.
func (c *Client) Deregister(ctx context.Context, reason string) error {
	c.connMu.RLock()
	client := c.client
	token := c.token
	c.connMu.RUnlock()

	if client == nil || token == "" {
		return nil
	}

	log.Info().Str("reason", reason).Msg("Deregistering from Hub...")

	_, err := client.Deregister(ctx, &pb.DeregisterRequest{
		InstanceId: c.instanceID,
		Token:      token,
		Reason:     reason,
	})
	if err != nil {
		log.Warn().Err(err).Msg("Deregistration failed")
		return err
	}

	log.Info().Msg("Deregistered from Hub successfully")
	return nil
}

// Heartbeat sends a heartbeat to the Hub and returns any pending actions.
func (c *Client) Heartbeat(ctx context.Context, state pb.InstanceState, message string, metrics *pb.InstanceMetrics) (*pb.HeartbeatResponse, error) {
	c.connMu.RLock()
	client := c.client
	token := c.token
	c.connMu.RUnlock()

	if client == nil || token == "" {
		return nil, fmt.Errorf("not registered")
	}

	c.configMu.RLock()
	configVersion := c.currentConfigVersion
	configHash := c.currentConfigHash
	c.configMu.RUnlock()

	resp, err := client.Heartbeat(ctx, &pb.HeartbeatRequest{
		InstanceId:           c.instanceID,
		Token:                token,
		Status:               &pb.InstanceStatus{State: state, Message: message},
		CurrentConfigVersion: configVersion,
		CurrentConfigHash:    configHash,
		Metrics:              metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("heartbeat failed: %w", err)
	}

	log.Debug().
		Bool("config_update", resp.ConfigUpdateAvailable).
		Str("latest_version", resp.LatestConfigVersion).
		Int("pending_actions", len(resp.Actions)).
		Msg("Heartbeat sent")

	return resp, nil
}

// FetchConfig fetches the current configuration from the Hub.
func (c *Client) FetchConfig(ctx context.Context, version string) (*pb.GetConfigResponse, error) {
	c.connMu.RLock()
	client := c.client
	token := c.token
	c.connMu.RUnlock()

	if client == nil || token == "" {
		return nil, fmt.Errorf("not registered")
	}

	log.Info().Str("version", version).Msg("Fetching config from Hub...")

	resp, err := client.GetConfig(ctx, &pb.GetConfigRequest{
		InstanceId: c.instanceID,
		Token:      token,
		Version:    version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}

	log.Info().
		Str("version", resp.Version).
		Str("hash", resp.Hash[:16]+"...").
		Int("content_length", len(resp.Content)).
		Msg("Config fetched successfully")

	return resp, nil
}

// UpdateConfigState updates the local config state after applying a config.
func (c *Client) UpdateConfigState(version, hash string) {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.currentConfigVersion = version
	c.currentConfigHash = hash
}

// SetConfigFromContent calculates hash and updates state from content.
func (c *Client) SetConfigFromContent(version, content string) {
	hash := sha256.Sum256([]byte(content))
	c.UpdateConfigState(version, hex.EncodeToString(hash[:]))
}

// AckDeployment acknowledges a deployment request.
func (c *Client) AckDeployment(ctx context.Context, deploymentID string, accepted bool, reason string) error {
	c.connMu.RLock()
	client := c.client
	token := c.token
	c.connMu.RUnlock()

	if client == nil || token == "" {
		return fmt.Errorf("not registered")
	}

	_, err := client.AckDeployment(ctx, &pb.AckDeploymentRequest{
		InstanceId:      c.instanceID,
		Token:           token,
		DeploymentId:    deploymentID,
		Accepted:        accepted,
		RejectionReason: reason,
	})
	if err != nil {
		return fmt.Errorf("failed to ack deployment: %w", err)
	}

	log.Debug().
		Str("deployment_id", deploymentID).
		Bool("accepted", accepted).
		Msg("Deployment acknowledged")

	return nil
}

// ReportDeploymentStatus reports the status of a deployment.
func (c *Client) ReportDeploymentStatus(ctx context.Context, deploymentID string, state pb.DeploymentState, message, errorDetails string) error {
	c.connMu.RLock()
	client := c.client
	token := c.token
	c.connMu.RUnlock()

	if client == nil || token == "" {
		return fmt.Errorf("not registered")
	}

	_, err := client.ReportDeploymentStatus(ctx, &pb.DeploymentStatusRequest{
		InstanceId:   c.instanceID,
		Token:        token,
		DeploymentId: deploymentID,
		State:        state,
		Message:      message,
		ErrorDetails: errorDetails,
	})
	if err != nil {
		return fmt.Errorf("failed to report deployment status: %w", err)
	}

	log.Debug().
		Str("deployment_id", deploymentID).
		Str("state", state.String()).
		Msg("Deployment status reported")

	return nil
}

// Subscribe subscribes to events from the Hub.
// This is a blocking call that handles events until the context is cancelled.
func (c *Client) Subscribe(ctx context.Context) error {
	c.connMu.RLock()
	client := c.client
	token := c.token
	c.connMu.RUnlock()

	if client == nil || token == "" {
		return fmt.Errorf("not registered")
	}

	log.Info().Msg("Subscribing to Hub events...")

	stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{
		InstanceId: c.instanceID,
		Token:      token,
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Info().Msg("Subscribed to Hub events")

	for {
		event, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Info().Msg("Event stream closed by server")
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Check for specific gRPC errors
			if st, ok := status.FromError(err); ok {
				if st.Code() == codes.Canceled || st.Code() == codes.Unavailable {
					return err
				}
			}
			log.Error().Err(err).Msg("Error receiving event")
			return err
		}

		c.handleEvent(ctx, event)
	}
}

// handleEvent processes a single event from the Hub.
func (c *Client) handleEvent(ctx context.Context, event *pb.Event) {
	log.Debug().
		Str("event_id", event.EventId).
		Str("type", event.Type.String()).
		Msg("Received event")

	switch event.Type {
	case pb.EventType_EVENT_TYPE_PING:
		// Ping event - just log it
		log.Debug().Msg("Received ping from Hub")

	case pb.EventType_EVENT_TYPE_CONFIG_UPDATE:
		if update := event.GetConfigUpdate(); update != nil {
			log.Info().
				Str("version", update.ConfigVersion).
				Str("summary", update.ChangeSummary).
				Msg("Config update available")

			if c.eventHandler != nil {
				// Fetch the new config
				cfg, err := c.FetchConfig(ctx, update.ConfigVersion)
				if err != nil {
					log.Error().Err(err).Msg("Failed to fetch updated config")
					return
				}
				if err := c.eventHandler.OnConfigUpdate(cfg.Version, cfg.Hash, cfg.Content); err != nil {
					log.Error().Err(err).Msg("Failed to apply config update")
				} else {
					c.UpdateConfigState(cfg.Version, cfg.Hash)
				}
			}
		}

	case pb.EventType_EVENT_TYPE_DEPLOYMENT:
		if dep := event.GetDeployment(); dep != nil {
			log.Info().
				Str("deployment_id", dep.DeploymentId).
				Str("config_version", dep.ConfigVersion).
				Str("strategy", dep.Strategy.String()).
				Int32("batch", dep.BatchPosition).
				Int32("total", dep.BatchTotal).
				Bool("rollback", dep.IsRollback).
				Msg("Deployment event received")

			// Acknowledge the deployment
			if err := c.AckDeployment(ctx, dep.DeploymentId, true, ""); err != nil {
				log.Error().Err(err).Msg("Failed to ack deployment")
			}

			// Report in progress
			c.ReportDeploymentStatus(ctx, dep.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_IN_PROGRESS, "Starting deployment", "")

			if c.eventHandler != nil {
				if err := c.eventHandler.OnDeployment(dep.DeploymentId, dep.ConfigVersion, dep.IsRollback); err != nil {
					log.Error().Err(err).Msg("Deployment failed")
					c.ReportDeploymentStatus(ctx, dep.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_FAILED, "Deployment failed", err.Error())
				} else {
					c.ReportDeploymentStatus(ctx, dep.DeploymentId, pb.DeploymentState_DEPLOYMENT_STATE_COMPLETED, "Deployment completed", "")
				}
			}
		}

	case pb.EventType_EVENT_TYPE_DRAIN:
		if drain := event.GetDrain(); drain != nil {
			log.Info().
				Int32("timeout_secs", drain.DrainTimeoutSeconds).
				Str("reason", drain.Reason).
				Msg("Drain event received")

			if c.eventHandler != nil {
				c.eventHandler.OnDrain(int(drain.DrainTimeoutSeconds), drain.Reason)
			}
		}

	default:
		log.Warn().Str("type", event.Type.String()).Msg("Unknown event type")
	}
}

// InstanceID returns the instance ID.
func (c *Client) InstanceID() string {
	return c.instanceID
}

// IsConnected returns true if the client is connected and registered.
func (c *Client) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn != nil && c.token != ""
}
