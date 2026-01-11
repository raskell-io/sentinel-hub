package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/raskell-io/sentinel-hub/internal/auth"
	"github.com/raskell-io/sentinel-hub/internal/config"
	"github.com/raskell-io/sentinel-hub/internal/store"
	pb "github.com/raskell-io/sentinel-hub/pkg/hubpb"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Server wraps the gRPC server and fleet service.
type Server struct {
	grpcServer   *grpc.Server
	fleetService *FleetService
	spiffeAuth   *auth.SPIFFEAuthenticator
	port         int
	tlsEnabled   bool
}

// ServerOption is a function that configures the server.
type ServerOption func(*serverConfig)

type serverConfig struct {
	tlsConfig   *config.TLSConfig
	spiffeAuth  *auth.SPIFFEAuthenticator
}

// WithTLS enables TLS with the given configuration.
func WithTLS(cfg *config.TLSConfig) ServerOption {
	return func(c *serverConfig) {
		c.tlsConfig = cfg
	}
}

// WithSPIFFE enables SPIFFE authentication.
func WithSPIFFE(authenticator *auth.SPIFFEAuthenticator) ServerOption {
	return func(c *serverConfig) {
		c.spiffeAuth = authenticator
	}
}

// NewServer creates a new gRPC server.
func NewServer(s *store.Store, port int, opts ...ServerOption) *Server {
	// Apply options
	cfg := &serverConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build server options
	var serverOpts []grpc.ServerOption

	// Add interceptors
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		loggingUnaryInterceptor,
		recoveryUnaryInterceptor,
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		loggingStreamInterceptor,
		recoveryStreamInterceptor,
	}

	// Add SPIFFE authentication interceptor if enabled
	if cfg.spiffeAuth != nil && cfg.spiffeAuth.IsEnabled() {
		unaryInterceptors = append(unaryInterceptors, spiffeUnaryInterceptor(cfg.spiffeAuth))
		streamInterceptors = append(streamInterceptors, spiffeStreamInterceptor(cfg.spiffeAuth))
	}

	serverOpts = append(serverOpts,
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// Configure TLS if enabled
	var tlsEnabled bool
	if cfg.tlsConfig != nil && cfg.tlsConfig.Enabled {
		tlsCfg, err := cfg.tlsConfig.LoadTLSConfig()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load TLS config for gRPC server")
		}
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		tlsEnabled = true
		log.Info().
			Str("cert_file", cfg.tlsConfig.CertFile).
			Bool("require_client_cert", cfg.tlsConfig.RequireClientCert).
			Msg("TLS enabled for gRPC server")
	}

	// Create gRPC server
	grpcServer := grpc.NewServer(serverOpts...)

	// Create fleet service
	fleetService := NewFleetService(s)

	// Register services
	pb.RegisterFleetServiceServer(grpcServer, fleetService)

	// Register health service
	healthServer := health.NewServer()
	healthServer.SetServingStatus("sentinel.hub.v1.FleetService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Enable reflection for debugging (consider disabling in production)
	reflection.Register(grpcServer)

	return &Server{
		grpcServer:   grpcServer,
		fleetService: fleetService,
		spiffeAuth:   cfg.spiffeAuth,
		port:         port,
		tlsEnabled:   tlsEnabled,
	}
}

// spiffeUnaryInterceptor creates a unary interceptor for SPIFFE authentication.
func spiffeUnaryInterceptor(spiffeAuth *auth.SPIFFEAuthenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip authentication for health checks
		if info.FullMethod == "/grpc.health.v1.Health/Check" ||
			info.FullMethod == "/grpc.health.v1.Health/Watch" {
			return handler(ctx, req)
		}

		// Authenticate
		identity, err := spiffeAuth.AuthenticateFromContext(ctx)
		if err != nil {
			log.Warn().
				Err(err).
				Str("method", info.FullMethod).
				Msg("SPIFFE authentication failed")
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Add identity to context
		ctx = auth.ContextWithSPIFFEIdentity(ctx, identity)

		log.Debug().
			Str("spiffe_id", identity.SPIFFEID).
			Str("method", info.FullMethod).
			Msg("SPIFFE authentication successful")

		return handler(ctx, req)
	}
}

// spiffeStreamInterceptor creates a stream interceptor for SPIFFE authentication.
func spiffeStreamInterceptor(spiffeAuth *auth.SPIFFEAuthenticator) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip authentication for health checks
		if info.FullMethod == "/grpc.health.v1.Health/Watch" {
			return handler(srv, ss)
		}

		// Authenticate
		identity, err := spiffeAuth.AuthenticateFromContext(ss.Context())
		if err != nil {
			log.Warn().
				Err(err).
				Str("method", info.FullMethod).
				Msg("SPIFFE authentication failed")
			return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		log.Debug().
			Str("spiffe_id", identity.SPIFFEID).
			Str("method", info.FullMethod).
			Msg("SPIFFE authentication successful for stream")

		// Wrap the stream with the authenticated context
		wrappedStream := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          auth.ContextWithSPIFFEIdentity(ss.Context(), identity),
		}

		return handler(srv, wrappedStream)
	}
}

// authenticatedServerStream wraps a ServerStream with an authenticated context.
type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
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
