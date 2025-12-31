package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/raskell-io/sentinel-hub/internal/api"
	"github.com/raskell-io/sentinel-hub/internal/fleet"
	hubgrpc "github.com/raskell-io/sentinel-hub/internal/grpc"
	"github.com/raskell-io/sentinel-hub/internal/store"
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
	if os.Getenv("HUB_LOG_FORMAT") != "json" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	rootCmd := &cobra.Command{
		Use:     "hub",
		Short:   "Sentinel Hub - Fleet management control plane",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Failed to execute command")
	}
}

func serveCmd() *cobra.Command {
	var (
		httpPort int
		grpcPort int
		dbURL    string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Hub server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(httpPort, grpcPort, dbURL)
		},
	}

	cmd.Flags().IntVar(&httpPort, "http-port", 8080, "HTTP server port")
	cmd.Flags().IntVar(&grpcPort, "grpc-port", 9090, "gRPC server port")
	cmd.Flags().StringVar(&dbURL, "database-url", "sqlite://hub.db", "Database connection URL")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Sentinel Hub %s\n", version)
			fmt.Printf("  Commit: %s\n", commit)
			fmt.Printf("  Built:  %s\n", date)
		},
	}
}

func runServer(httpPort, grpcPort int, dbURL string) error {
	log.Info().
		Int("http_port", httpPort).
		Int("grpc_port", grpcPort).
		Str("database", dbURL).
		Msg("Starting Sentinel Hub")

	// Initialize database
	db, err := store.New(dbURL)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	log.Info().Msg("Database initialized successfully")

	// Create gRPC server
	grpcServer := hubgrpc.NewServer(db, grpcPort)

	// Create deployment orchestrator
	orchestrator := fleet.NewOrchestrator(db, grpcServer.FleetService())
	if err := orchestrator.Start(); err != nil {
		return fmt.Errorf("failed to start orchestrator: %w", err)
	}

	// Wire up status reporting from agents to orchestrator
	grpcServer.FleetService().SetDeploymentStatusHandler(orchestrator.ReportInstanceStatus)

	// Start gRPC server in background
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
		}
	}()

	// Create API handler
	handler := api.NewHandler(db, orchestrator)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS middleware for development
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	})

	// Health check endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check database connection
		if err := db.DB().Ping(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"not ready","error":"database unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	// Stats endpoint (shows connected agents)
	r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"connected_agents":%d}`, grpcServer.FleetService().GetSubscriberCount())
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Instance management
		r.Route("/instances", func(r chi.Router) {
			r.Get("/", handler.ListInstances)
			r.Post("/", handler.CreateInstance)
			r.Get("/{id}", handler.GetInstance)
			r.Put("/{id}", handler.UpdateInstance)
			r.Delete("/{id}", handler.DeleteInstance)
		})

		// Configuration management
		r.Route("/configs", func(r chi.Router) {
			r.Get("/", handler.ListConfigs)
			r.Post("/", handler.CreateConfig)
			r.Get("/{id}", handler.GetConfig)
			r.Put("/{id}", handler.UpdateConfig)
			r.Delete("/{id}", handler.DeleteConfig)
			r.Get("/{id}/versions", handler.ListConfigVersions)
			r.Post("/{id}/rollback", handler.RollbackConfig)
		})

		// Deployments
		r.Route("/deployments", func(r chi.Router) {
			r.Get("/", handler.ListDeployments)
			r.Post("/", handler.CreateDeployment)
			r.Get("/{id}", handler.GetDeployment)
			r.Post("/{id}/cancel", handler.CancelDeployment)
		})
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", httpPort),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info().Msg("Shutting down servers...")

		// Stop orchestrator first (cancels in-progress deployments)
		if err := orchestrator.Stop(); err != nil {
			log.Error().Err(err).Msg("Error stopping orchestrator")
		}

		// Stop gRPC server
		grpcServer.Stop()

		// Stop HTTP server
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatal().Err(err).Msg("Could not gracefully shutdown HTTP server")
		}
		close(done)
	}()

	log.Info().Msgf("HTTP server listening on :%d", httpPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	<-done
	log.Info().Msg("Server stopped")
	return nil
}
