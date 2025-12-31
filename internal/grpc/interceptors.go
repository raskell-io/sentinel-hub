package grpc

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// loggingUnaryInterceptor logs unary RPC calls.
func loggingUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start)
	code := codes.OK
	if err != nil {
		if st, ok := status.FromError(err); ok {
			code = st.Code()
		} else {
			code = codes.Unknown
		}
	}

	log.Debug().
		Str("method", info.FullMethod).
		Str("code", code.String()).
		Dur("duration", duration).
		Msg("gRPC unary call")

	return resp, err
}

// loggingStreamInterceptor logs streaming RPC calls.
func loggingStreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	err := handler(srv, ss)

	duration := time.Since(start)
	code := codes.OK
	if err != nil {
		if st, ok := status.FromError(err); ok {
			code = st.Code()
		} else {
			code = codes.Unknown
		}
	}

	log.Debug().
		Str("method", info.FullMethod).
		Str("code", code.String()).
		Dur("duration", duration).
		Bool("client_stream", info.IsClientStream).
		Bool("server_stream", info.IsServerStream).
		Msg("gRPC stream call")

	return err
}

// recoveryUnaryInterceptor recovers from panics in unary handlers.
func recoveryUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Str("method", info.FullMethod).
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("Recovered from panic in gRPC handler")
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(ctx, req)
}

// recoveryStreamInterceptor recovers from panics in stream handlers.
func recoveryStreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Str("method", info.FullMethod).
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("Recovered from panic in gRPC stream handler")
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()

	return handler(srv, ss)
}
