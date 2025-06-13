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
	addr := fmt.Sprintf("%s:%d", s.config.GRPC.Host, s.config.GRPC.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Create gRPC server
	s.grpcServer = grpc.NewServer()

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
	// Convert proto request to internal request
	tssReq := &tss.KeygenRequest{
		Threshold:    int(req.Threshold),
		Parties:      int(req.Parties),
		Participants: req.Participants,
	}

	// Start keygen operation
	operation, err := g.tssService.StartKeygen(ctx, tssReq)
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
	// Convert proto request to internal request
	tssReq := &tss.SigningRequest{
		Message:      req.Message,
		KeyID:        req.KeyId,
		Participants: req.Participants,
	}

	// Start signing operation
	operation, err := g.tssService.StartSigning(ctx, tssReq)
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
	// Convert proto request to internal request
	tssReq := &tss.ResharingRequest{
		KeyID:           req.KeyId,
		NewThreshold:    int(req.NewThreshold),
		NewParties:      int(req.NewParties),
		OldParticipants: req.OldParticipants,
		NewParticipants: req.NewParticipants,
	}

	// Start resharing operation
	operation, err := g.tssService.StartResharing(ctx, tssReq)
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

		response := &tssv1.GetOperationResponse{
			OperationId: operation.ID,
			Type:        convertOperationType(operation.Type),
			SessionId:   operation.SessionID,
			Status:      convertOperationStatus(operation.Status),
			CreatedAt:   timestamppb.New(operation.CreatedAt),
		}

		// Add participants
		for _, p := range operation.Participants {
			response.Participants = append(response.Participants, p.Id)
		}

		// Add completion time if available
		if operation.CompletedAt != nil {
			response.CompletedAt = timestamppb.New(*operation.CompletedAt)
		}

		// Add error if available
		if operation.Error != nil {
			errMsg := operation.Error.Error()
			response.Error = &errMsg
		}

		// Add result based on operation type
		if operation.Result != nil {
			switch operation.Type {
			case tss.OperationKeygen:
				if result, ok := operation.Result.(*tss.KeygenResult); ok {
					response.Result = &tssv1.GetOperationResponse_KeygenResult{
						KeygenResult: &tssv1.KeygenResult{
							PublicKey: result.PublicKey,
							KeyId:     result.KeyID,
						},
					}
				}
			case tss.OperationSigning:
				if result, ok := operation.Result.(*tss.SigningResult); ok {
					response.Result = &tssv1.GetOperationResponse_SigningResult{
						SigningResult: &tssv1.SigningResult{
							Signature: result.Signature,
							R:         result.R,
							S:         result.S,
						},
					}
				}
			case tss.OperationResharing:
				if result, ok := operation.Result.(*tss.KeygenResult); ok {
					response.Result = &tssv1.GetOperationResponse_ResharingResult{
						ResharingResult: &tssv1.KeygenResult{
							PublicKey: result.PublicKey,
							KeyId:     result.KeyID,
						},
					}
				}
			}
		}

		// Add original request
		if operation.Request != nil {
			switch req := operation.Request.(type) {
			case *tss.KeygenRequest:
				response.Request = &tssv1.GetOperationResponse_KeygenRequest{
					KeygenRequest: &tssv1.StartKeygenRequest{
						Threshold:    int32(req.Threshold),
						Parties:      int32(req.Parties),
						Participants: req.Participants,
					},
				}
			case *tss.SigningRequest:
				response.Request = &tssv1.GetOperationResponse_SigningRequest{
					SigningRequest: &tssv1.StartSigningRequest{
						Message:      req.Message,
						KeyId:        req.KeyID,
						Participants: req.Participants,
					},
				}
			case *tss.ResharingRequest:
				response.Request = &tssv1.GetOperationResponse_ResharingRequest{
					ResharingRequest: &tssv1.StartResharingRequest{
						KeyId:           req.KeyID,
						NewThreshold:    int32(req.NewThreshold),
						NewParties:      int32(req.NewParties),
						OldParticipants: req.OldParticipants,
						NewParticipants: req.NewParticipants,
					},
				}
			}
		}

		return response, nil
	}

	// If not found in memory, try persistent storage
	operationData, err := g.tssService.GetOperationData(ctx, req.OperationId)
	if err != nil {
		g.logger.Warn("Operation not found", zap.String("operation_id", req.OperationId), zap.Error(err))
		return nil, status.Errorf(codes.NotFound, "operation not found")
	}

	response := &tssv1.GetOperationResponse{
		OperationId:  operationData.ID,
		Type:         convertOperationType(operationData.Type),
		SessionId:    operationData.SessionID,
		Status:       convertOperationStatus(operationData.Status),
		Participants: operationData.Participants,
		CreatedAt:    timestamppb.New(operationData.CreatedAt),
	}

	// Add completion time if available
	if operationData.CompletedAt != nil {
		response.CompletedAt = timestamppb.New(*operationData.CompletedAt)
	}

	// Add error if available
	if operationData.Error != "" {
		response.Error = &operationData.Error
	}

	// Add result based on operation type if available
	if operationData.Result != nil {
		switch operationData.Type {
		case tss.OperationKeygen:
			if result, ok := operationData.Result.(*tss.KeygenResult); ok {
				response.Result = &tssv1.GetOperationResponse_KeygenResult{
					KeygenResult: &tssv1.KeygenResult{
						PublicKey: result.PublicKey,
						KeyId:     result.KeyID,
					},
				}
			}
		case tss.OperationSigning:
			if result, ok := operationData.Result.(*tss.SigningResult); ok {
				response.Result = &tssv1.GetOperationResponse_SigningResult{
					SigningResult: &tssv1.SigningResult{
						Signature: result.Signature,
						R:         result.R,
						S:         result.S,
					},
				}
			}
		case tss.OperationResharing:
			if result, ok := operationData.Result.(*tss.KeygenResult); ok {
				response.Result = &tssv1.GetOperationResponse_ResharingResult{
					ResharingResult: &tssv1.KeygenResult{
						PublicKey: result.PublicKey,
						KeyId:     result.KeyID,
					},
				}
			}
		}
	}

	// Add original request if available
	if operationData.Request != nil {
		switch req := operationData.Request.(type) {
		case *tss.KeygenRequest:
			response.Request = &tssv1.GetOperationResponse_KeygenRequest{
				KeygenRequest: &tssv1.StartKeygenRequest{
					Threshold:    int32(req.Threshold),
					Parties:      int32(req.Parties),
					Participants: req.Participants,
				},
			}
		case *tss.SigningRequest:
			response.Request = &tssv1.GetOperationResponse_SigningRequest{
				SigningRequest: &tssv1.StartSigningRequest{
					Message:      req.Message,
					KeyId:        req.KeyID,
					Participants: req.Participants,
				},
			}
		case *tss.ResharingRequest:
			response.Request = &tssv1.GetOperationResponse_ResharingRequest{
				ResharingRequest: &tssv1.StartResharingRequest{
					KeyId:           req.KeyID,
					NewThreshold:    int32(req.NewThreshold),
					NewParties:      int32(req.NewParties),
					OldParticipants: req.OldParticipants,
					NewParticipants: req.NewParticipants,
				},
			}
		}
	}

	return response, nil
}

// CancelOperation implements TSSService.CancelOperation
func (g *gRPCTSSServer) CancelOperation(ctx context.Context, req *tssv1.CancelOperationRequest) (*tssv1.CancelOperationResponse, error) {
	if err := g.tssService.CancelOperation(req.OperationId); err != nil {
		g.logger.Error("Failed to cancel operation", zap.String("operation_id", req.OperationId), zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to cancel operation: %v", err)
	}

	return &tssv1.CancelOperationResponse{
		Message: "operation canceled",
	}, nil
}

// ListOperations implements TSSService.ListOperations
func (g *gRPCTSSServer) ListOperations(ctx context.Context, req *tssv1.ListOperationsRequest) (*tssv1.ListOperationsResponse, error) {
	// For now, return a simplified response
	// In a complete implementation, you would need to add a GetAllOperations method to the TSS service
	// or iterate through persistent storage
	return &tssv1.ListOperationsResponse{
		Operations: []*tssv1.GetOperationResponse{},
		TotalCount: 0,
	}, nil
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

// GetNetworkAddresses implements TSSService.GetNetworkAddresses
func (g *gRPCTSSServer) GetNetworkAddresses(
	ctx context.Context,
	req *tssv1.GetNetworkAddressesRequest,
) (*tssv1.GetNetworkAddressesResponse, error) {
	mappings := g.network.GetAllNodeMappings()

	// Convert to proto NodeMapping array
	var result []*tssv1.NodeMapping
	for _, mapping := range mappings {
		result = append(result, &tssv1.NodeMapping{
			NodeId:    mapping.NodeID,
			PeerId:    mapping.PeerID,
			Moniker:   mapping.Moniker,
			Timestamp: timestamppb.New(mapping.Timestamp),
		})
	}

	return &tssv1.GetNetworkAddressesResponse{
		Mappings: result,
	}, nil
}
