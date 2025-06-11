package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/api"
	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/dreamer-zq/DKNet/internal/storage"
	"github.com/dreamer-zq/DKNet/internal/tss"
)

// App represents the main application
type App struct {
	config  *config.NodeConfig
	logger  *zap.Logger
	storage storage.Storage
	network *p2p.Network
	tss     *tss.Service
	api     *api.Server
}

// New creates a new application instance
func New(cfg *config.NodeConfig, logger *zap.Logger) (*App, error) {
	// Initialize storage
	var store storage.Storage
	var err error
	
	switch cfg.Storage.Type {
	case "leveldb":
		store, err = storage.NewLevelDBStorage(cfg.Storage.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to create LevelDB storage: %w", err)
		}
	case "file":
		// TODO: Implement file storage
		return nil, fmt.Errorf("file storage not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
	
	// Initialize P2P network
	p2pConfig := &p2p.Config{
		ListenAddrs:    cfg.P2P.ListenAddrs,
		BootstrapPeers: cfg.P2P.BootstrapPeers,
		PrivateKeyFile: cfg.P2P.PrivateKeyFile,
		MaxPeers:       cfg.P2P.MaxPeers,
	}
	
	network, err := p2p.NewNetwork(p2pConfig, logger.Named("p2p"))
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create P2P network: %w", err)
	}
	
	// Initialize TSS service
	tssConfig := &tss.Config{
		NodeID:    cfg.TSS.NodeID,
		Moniker:   cfg.TSS.Moniker,
		Threshold: cfg.TSS.Threshold,
		Parties:   cfg.TSS.Parties,
	}
	
	tssService, err := tss.NewService(tssConfig, store, network, logger.Named("tss"))
	if err != nil {
		store.Close()
		network.Stop()
		return nil, fmt.Errorf("failed to create TSS service: %w", err)
	}
	
	// Set TSS service as the message handler for P2P network
	network.SetMessageHandler(tssService)
	
	// Initialize API server
	apiConfig := &api.Config{
		HTTP: api.HTTPConfig{
			Host: cfg.Server.HTTP.Host,
			Port: cfg.Server.HTTP.Port,
		},
		GRPC: api.GRPCConfig{
			Host: cfg.Server.GRPC.Host,
			Port: cfg.Server.GRPC.Port,
		},
		Security: api.SecurityConfig{
			TLSEnabled: cfg.Security.TLSEnabled,
			CertFile:   cfg.Security.CertFile,
			KeyFile:    cfg.Security.KeyFile,
		},
	}
	
	apiServer, err := api.NewServer(apiConfig, tssService, logger.Named("api"))
	if err != nil {
		store.Close()
		network.Stop()
		return nil, fmt.Errorf("failed to create API server: %w", err)
	}
	
	return &App{
		config:  cfg,
		logger:  logger,
		storage: store,
		network: network,
		tss:     tssService,
		api:     apiServer,
	}, nil
}

// Start starts the application
func (a *App) Start(ctx context.Context) error {
	a.logger.Info("Starting DKNet...")
	
	// Start P2P network
	if err := a.network.Start(ctx, a.config.P2P.BootstrapPeers); err != nil {
		return fmt.Errorf("failed to start P2P network: %w", err)
	}
	
	// Start API server
	if err := a.api.Start(ctx); err != nil {
		a.network.Stop()
		return fmt.Errorf("failed to start API server: %w", err)
	}
	
	a.logger.Info("DKNet started successfully",
		zap.String("node_id", a.config.TSS.NodeID),
		zap.String("p2p_peer_id", a.network.GetPeerID().String()),
		zap.Int("http_port", a.config.Server.HTTP.Port),
		zap.Int("grpc_port", a.config.Server.GRPC.Port))
	
	return nil
}

// Stop stops the application
func (a *App) Stop() error {
	a.logger.Info("Stopping DKNet...")
	
	var errs []error
	
	// Stop API server
	if err := a.api.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop API server: %w", err))
	}
	
	// Stop P2P network
	if err := a.network.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop P2P network: %w", err))
	}
	
	// Close storage
	if err := a.storage.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close storage: %w", err))
	}
	
	if len(errs) > 0 {
		a.logger.Error("Errors during shutdown", zap.Any("errors", errs))
		return errs[0] // Return the first error
	}
	
	a.logger.Info("DKNet stopped successfully")
	return nil
} 