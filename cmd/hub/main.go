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

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Check database connection
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Instance management
		r.Route("/instances", func(r chi.Router) {
			r.Get("/", listInstances)
			r.Post("/", createInstance)
			r.Get("/{id}", getInstance)
			r.Put("/{id}", updateInstance)
			r.Delete("/{id}", deleteInstance)
		})

		// Configuration management
		r.Route("/configs", func(r chi.Router) {
			r.Get("/", listConfigs)
			r.Post("/", createConfig)
			r.Get("/{id}", getConfig)
			r.Put("/{id}", updateConfig)
			r.Delete("/{id}", deleteConfig)
			r.Get("/{id}/versions", listConfigVersions)
			r.Post("/{id}/rollback", rollbackConfig)
		})

		// Deployments
		r.Route("/deployments", func(r chi.Router) {
			r.Get("/", listDeployments)
			r.Post("/", createDeployment)
			r.Get("/{id}", getDeployment)
			r.Post("/{id}/cancel", cancelDeployment)
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
		log.Info().Msg("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatal().Err(err).Msg("Could not gracefully shutdown server")
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

// Placeholder handlers - to be implemented
func listInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"instances":[]}`))
}

func createInstance(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func getInstance(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func updateInstance(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func deleteInstance(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func listConfigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"configs":[]}`))
}

func createConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func updateConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func deleteConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func listConfigVersions(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func rollbackConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func listDeployments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"deployments":[]}`))
}

func createDeployment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func getDeployment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}

func cancelDeployment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":"not implemented"}`))
}
