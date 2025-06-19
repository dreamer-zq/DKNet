package api

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/dreamer-zq/DKNet/internal/tss"
	healthv1 "github.com/dreamer-zq/DKNet/proto/health/v1"
	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// startGRPCServer starts the gRPC server
func (s *Server) startGRPCServer() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.GRPC.Host, s.config.Server.GRPC.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Create gRPC server with authentication interceptors
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(GRPCAuthInterceptor(s.authenticator, s.logger)),
		grpc.StreamInterceptor(GRPCAuthStreamInterceptor(s.authenticator, s.logger)),
	}
	s.grpcServer = grpc.NewServer(opts...)

	// Register services
	s.setupGRPCServices()

	// Start server in a goroutine
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			s.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// stopGRPCServer stops the gRPC server
func (s *Server) stopGRPCServer() {
	if s.grpcServer == nil {
		return
	}

	s.grpcServer.GracefulStop()
}

// setupGRPCServices sets up gRPC services
func (s *Server) setupGRPCServices() {
	tssServer := &gRPCTSSServer{
		tssService: s.tssService,
		network:    s.network,
		logger:     s.logger,
	}

	healthServer := &gRPCHealthServer{
		logger: s.logger,
	}

	// Register services with the gRPC server
	tssv1.RegisterTSSServiceServer(s.grpcServer, tssServer)
	healthv1.RegisterHealthServiceServer(s.grpcServer, healthServer)

	s.logger.Info("gRPC services registered successfully")
}

// gRPCTSSServer implements the TSS gRPC service
type gRPCTSSServer struct {
	tssv1.UnimplementedTSSServiceServer
	tssService *tss.Service
	network    *p2p.Network
	logger     *zap.Logger
}

// gRPCHealthServer implements the Health gRPC service
type gRPCHealthServer struct {
	healthv1.UnimplementedHealthServiceServer
	logger *zap.Logger
}

// StartKeygen implements TSSService.StartKeygen
func (g *gRPCTSSServer) StartKeygen(ctx context.Context, req *tssv1.StartKeygenRequest) (*tssv1.StartKeygenResponse, error) {
	// Start keygen operation
	operation, err := g.tssService.StartKeygen(
		ctx,
		req.OperationId,
		int(req.Threshold),
		req.Participants,
	)
	if err != nil {
		g.logger.Error("Failed to start keygen", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to start keygen: %v", err)
	}

	// Convert to proto response
	return &tssv1.StartKeygenResponse{
		OperationId: operation.ID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}, nil
}

// StartSigning implements TSSService.StartSigning
func (g *gRPCTSSServer) StartSigning(ctx context.Context, req *tssv1.StartSigningRequest) (*tssv1.StartSigningResponse, error) {
	// Start signing operation
	operation, err := g.tssService.StartSigning(
		ctx,
		req.OperationId,
		req.Message,
		req.KeyId,
		req.Participants,
	)
	if err != nil {
		g.logger.Error("Failed to start signing", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to start signing: %v", err)
	}

	// Convert to proto response
	return &tssv1.StartSigningResponse{
		OperationId: operation.ID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}, nil
}

// StartResharing implements TSSService.StartResharing
func (g *gRPCTSSServer) StartResharing(ctx context.Context, req *tssv1.StartResharingRequest) (*tssv1.StartResharingResponse, error) {
	// Start resharing operation
	operation, err := g.tssService.StartResharing(
		ctx,
		req.OperationId,
		req.KeyId,
		int(req.NewThreshold),
		req.OldParticipants,
		req.NewParticipants,
	)
	if err != nil {
		g.logger.Error("Failed to start resharing", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to start resharing: %v", err)
	}

	// Convert to proto response
	return &tssv1.StartResharingResponse{
		OperationId: operation.ID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}, nil
}

// GetOperation implements TSSService.GetOperation
func (g *gRPCTSSServer) GetOperation(ctx context.Context, req *tssv1.GetOperationRequest) (*tssv1.GetOperationResponse, error) {
	// First try to get from active operations in memory
	operation, exists := g.tssService.GetOperation(req.OperationId)
	if exists {
		operation.RLock()
		defer operation.RUnlock()

		return buildOperationResponse(operation), nil
	}

	// If not found in memory, try persistent storage
	operationData, err := g.tssService.GetOperationData(ctx, req.OperationId)
	if err != nil {
		g.logger.Warn("Operation not found", zap.String("operation_id", req.OperationId), zap.Error(err))
		return nil, status.Errorf(codes.NotFound, "operation not found")
	}

	// Use reflection to access private fields since operationData is private
	// This is a temporary solution until we can make the fields public or create a proper interface
	return buildOperationResponseFromStorage(operationData), nil
}

// Check implements HealthService.Check
func (g *gRPCHealthServer) Check(ctx context.Context, req *healthv1.CheckRequest) (*healthv1.CheckResponse, error) {
	return &healthv1.CheckResponse{
		Status:    healthv1.HealthStatus_HEALTH_STATUS_SERVING,
		Timestamp: timestamppb.Now(),
		Details:   "DKNet is healthy",
		Metadata: map[string]string{
			"service": "dknet",
			"version": "1.0.0",
		},
	}, nil
}

// Watch implements HealthService.Watch
func (g *gRPCHealthServer) Watch(req *healthv1.WatchRequest, stream healthv1.HealthService_WatchServer) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			resp := &healthv1.WatchResponse{
				Status:    healthv1.HealthStatus_HEALTH_STATUS_SERVING,
				Timestamp: timestamppb.Now(),
				Details:   "DKNet is healthy",
				Metadata: map[string]string{
					"service": "dknet",
					"version": "1.0.0",
				},
			}

			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}
