package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		hubURL         string
		instanceName   string
		sentinelConfig string
		heartbeatSecs  int
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(hubURL, instanceName, sentinelConfig, heartbeatSecs)
		},
	}

	cmd.Flags().StringVar(&hubURL, "hub-url", "localhost:9090", "Hub gRPC server URL")
	cmd.Flags().StringVar(&instanceName, "instance-name", "", "Instance name (defaults to hostname)")
	cmd.Flags().StringVar(&sentinelConfig, "sentinel-config", "/etc/sentinel/config.kdl", "Path to Sentinel config file")
	cmd.Flags().IntVar(&heartbeatSecs, "heartbeat-interval", 30, "Heartbeat interval in seconds")

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

func runAgent(hubURL, instanceName, sentinelConfig string, heartbeatSecs int) error {
	// Default instance name to hostname
	if instanceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}
		instanceName = hostname
	}

	log.Info().
		Str("hub_url", hubURL).
		Str("instance_name", instanceName).
		Str("sentinel_config", sentinelConfig).
		Int("heartbeat_interval", heartbeatSecs).
		Msg("Starting Sentinel Hub Agent")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Main loop
	ticker := time.NewTicker(time.Duration(heartbeatSecs) * time.Second)
	defer ticker.Stop()

	log.Info().Msg("Agent started, waiting for implementation...")

	for {
		select {
		case <-ticker.C:
			// TODO: Send heartbeat to Hub
			log.Debug().Msg("Heartbeat tick (not implemented)")
		case <-quit:
			log.Info().Msg("Shutting down agent...")
			return nil
		}
	}
}
