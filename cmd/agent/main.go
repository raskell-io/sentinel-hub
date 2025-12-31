package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/raskell-io/sentinel-hub/internal/agent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Setup zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("AGENT_LOG_FORMAT") != "json" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Set log level
	level := os.Getenv("AGENT_LOG_LEVEL")
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	rootCmd := &cobra.Command{
		Use:     "agent",
		Short:   "Sentinel Hub Agent - Manages local Sentinel instance",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Failed to execute command")
	}
}

func runCmd() *cobra.Command {
	var (
		hubURL          string
		instanceID      string
		instanceName    string
		sentinelConfig  string
		sentinelVersion string
		heartbeatSecs   int
		labels          []string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(hubURL, instanceID, instanceName, sentinelConfig, sentinelVersion, heartbeatSecs, labels)
		},
	}

	cmd.Flags().StringVar(&hubURL, "hub-url", "localhost:9090", "Hub gRPC server URL")
	cmd.Flags().StringVar(&instanceID, "instance-id", "", "Instance ID (defaults to generated UUID)")
	cmd.Flags().StringVar(&instanceName, "instance-name", "", "Instance name (defaults to hostname)")
	cmd.Flags().StringVar(&sentinelConfig, "sentinel-config", "/etc/sentinel/config.kdl", "Path to Sentinel config file")
	cmd.Flags().StringVar(&sentinelVersion, "sentinel-version", "unknown", "Sentinel version")
	cmd.Flags().IntVar(&heartbeatSecs, "heartbeat-interval", 30, "Heartbeat interval in seconds")
	cmd.Flags().StringSliceVar(&labels, "label", nil, "Labels in key=value format (can be specified multiple times)")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Sentinel Hub Agent %s\n", version)
			fmt.Printf("  Commit: %s\n", commit)
			fmt.Printf("  Built:  %s\n", date)
		},
	}
}

func runAgent(hubURL, instanceID, instanceName, sentinelConfig, sentinelVersion string, heartbeatSecs int, labelArgs []string) error {
	// Default instance ID to UUID
	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	// Default instance name to hostname
	if instanceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}
		instanceName = hostname
	}

	// Parse labels
	labels := make(map[string]string)
	for _, l := range labelArgs {
		parts := splitLabel(l)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}

	log.Info().
		Str("hub_url", hubURL).
		Str("instance_id", instanceID).
		Str("instance_name", instanceName).
		Str("sentinel_config", sentinelConfig).
		Str("sentinel_version", sentinelVersion).
		Int("heartbeat_interval", heartbeatSecs).
		Interface("labels", labels).
		Msg("Starting Sentinel Hub Agent")

	// Create agent
	ag, err := agent.New(agent.Config{
		HubURL:            hubURL,
		InstanceID:        instanceID,
		InstanceName:      instanceName,
		SentinelConfig:    sentinelConfig,
		HeartbeatInterval: time.Duration(heartbeatSecs) * time.Second,
		AgentVersion:      version,
		SentinelVersion:   sentinelVersion,
		Labels:            labels,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Run agent in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- ag.Run(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Msg("Agent exited with error")
		}
	}

	// Stop agent
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := ag.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error during shutdown")
	}

	log.Info().Msg("Agent stopped")
	return nil
}

// splitLabel splits a label in "key=value" format.
func splitLabel(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
