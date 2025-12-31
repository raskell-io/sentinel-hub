package grpc

import (
	"fmt"
	"net"

	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Server wraps the gRPC server and fleet service.
type Server struct {
	grpcServer   *grpc.Server
	fleetService *FleetService
	port         int
}

// NewServer creates a new gRPC server.
func NewServer(s *store.Store, port int) *Server {
	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingUnaryInterceptor,
			recoveryUnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			loggingStreamInterceptor,
			recoveryStreamInterceptor,
		),
	)

	// Create fleet service
	fleetService := NewFleetService(s)

	// Register services
	pb.RegisterFleetServiceServer(grpcServer, fleetService)

	// Register health service
	healthServer := health.NewServer()
	healthServer.SetServingStatus("sentinel.hub.v1.FleetService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Enable reflection for debugging (disable in production)
	reflection.Register(grpcServer)

	return &Server{
		grpcServer:   grpcServer,
		fleetService: fleetService,
		port:         port,
	}
}

// Start starts the gRPC server.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	log.Info().Int("port", s.port).Msg("gRPC server listening")

	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	log.Info().Msg("Stopping gRPC server...")
	s.grpcServer.GracefulStop()
}

// FleetService returns the fleet service instance for external use.
func (s *Server) FleetService() *FleetService {
	return s.fleetService
}
