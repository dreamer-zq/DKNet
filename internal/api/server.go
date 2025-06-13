package api

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/dreamer-zq/DKNet/internal/tss"
)

// Server provides HTTP and gRPC APIs for TSS operations
type Server struct {
	config         *Config
	tssService     *tss.Service
	network        *p2p.Network
	addressManager *p2p.AddressManager
	logger         *zap.Logger

	httpServer *http.Server
	grpcServer *grpc.Server
}

// NewServer creates a new API server
func NewServer(
	cfg *Config,
	tssService *tss.Service,
	network *p2p.Network,
	addressManager *p2p.AddressManager,
	logger *zap.Logger,
) (*Server, error) {
	return &Server{
		config:         cfg,
		tssService:     tssService,
		network:        network,
		addressManager: addressManager,
		logger:         logger,
	}, nil
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	// Start HTTP server
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start gRPC server
	if err := s.startGRPCServer(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	s.logger.Info("API servers started",
		zap.String("http_addr", fmt.Sprintf("%s:%d", s.config.HTTP.Host, s.config.HTTP.Port)),
		zap.String("grpc_addr", fmt.Sprintf("%s:%d", s.config.GRPC.Host, s.config.GRPC.Port)))

	return nil
}

// Stop stops the API server
func (s *Server) Stop() error {
	var errs []error

	// Stop HTTP server
	if err := s.stopHTTPServer(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop HTTP server: %w", err))
	}

	// Stop gRPC server
	s.stopGRPCServer()

	if len(errs) > 0 {
		return errs[0]
	}

	s.logger.Info("API servers stopped")
	return nil
}
