package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/api"
	"github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/dreamer-zq/DKNet/internal/security"
	"github.com/dreamer-zq/DKNet/internal/storage"
	"github.com/dreamer-zq/DKNet/internal/tss"
)

// App represents the main application
type App struct {
	config     *config.NodeConfig
	logger     *zap.Logger
	network    *p2p.Network
	tssService *tss.Service
	storage    storage.Storage
	api        *api.Server
}

// New creates a new application instance
func New(cfg *config.NodeConfig, logger *zap.Logger, password string) (*App, error) {
	// Initialize storage (always use plain storage, encryption is handled at TSS level)
	var store storage.Storage
	var err error

	switch cfg.Storage.Type {
	case "leveldb":
		store, err = storage.NewLevelDBStorage(cfg.Storage.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to create LevelDB storage: %w", err)
		}
		logger.Info("Initialized LevelDB storage")
	case "file":
		// TODO: Implement file storage
		return nil, fmt.Errorf("file storage not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}

	// Initialize access controller
	accessController := security.NewController(&cfg.Security.AccessControl, logger.Named("access_control"))

	// Create P2P network
	network, err := p2p.NewNetwork(&p2p.Config{
		ListenAddrs:    cfg.P2P.ListenAddrs,
		BootstrapPeers: cfg.P2P.BootstrapPeers,
		PrivateKeyFile: cfg.P2P.PrivateKeyFile,
		MaxPeers:       cfg.P2P.MaxPeers,
	}, accessController, logger.Named("p2p"))
	if err != nil {
		common.LogMsgDo("failed to create P2P network", func() error {
			return store.Close()
		})
		return nil, fmt.Errorf("failed to create P2P network: %w", err)
	}

	// Use peer ID as node ID for TSS service
	peerID := network.GetHostID().String()
	logger.Info("Using peer ID as TSS node ID",
		zap.String("peer_id", peerID),
		zap.String("moniker", cfg.TSS.Moniker))

	// Initialize TSS service with encryption
	tssService, err := tss.NewService(&tss.Config{
		PeerID:            peerID, // Use peer ID for TSS service
		Moniker:           cfg.TSS.Moniker,
		ValidationService: cfg.TSS.ValidationService,
	}, store, network, logger.Named("tss"), password)
	if err != nil {
		common.LogDo(func() error {
			return store.Close()
		})
		common.LogDo(func() error {
			return network.Stop()
		})
		return nil, fmt.Errorf("failed to create TSS service: %w", err)
	}
	logger.Info("Initialized TSS service with encrypted key storage")

	// Set TSS service as the message handler for P2P network
	network.SetMessageHandler(tssService)

	// Initialize API server
	apiServer, err := api.NewServer(&api.Config{
		HTTP: api.HTTPConfig{
			Host: cfg.Server.HTTP.Host,
			Port: cfg.Server.HTTP.Port,
		},
		GRPC: api.GRPCConfig{
			Host: cfg.Server.GRPC.Host,
			Port: cfg.Server.GRPC.Port,
		},
	}, tssService, network, logger.Named("api"))
	if err != nil {
		if closeErr := store.Close(); closeErr != nil {
			logger.Error("Failed to close storage during cleanup", zap.Error(closeErr))
		}
		if stopErr := network.Stop(); stopErr != nil {
			logger.Error("Failed to stop network during cleanup", zap.Error(stopErr))
		}
		return nil, fmt.Errorf("failed to create API server: %w", err)
	}

	return &App{
		config:     cfg,
		logger:     logger,
		storage:    store,
		network:    network,
		tssService: tssService,
		api:        apiServer,
	}, nil
}

// Start starts the application
func (a *App) Start(ctx context.Context) error {
	a.logger.Info("Starting DKNet application")

	// Start P2P network
	if err := a.network.Start(ctx, a.config.P2P.BootstrapPeers); err != nil {
		return fmt.Errorf("failed to start P2P network: %w", err)
	}

	// Start API server
	if err := a.api.Start(ctx); err != nil {
		common.LogMsgDo("failed to start API server", func() error {
			return a.network.Stop()
		})
		return fmt.Errorf("failed to start API server: %w", err)
	}

	a.logger.Info("DKNet application started successfully")
	return nil
}

// Stop stops the application
func (a *App) Stop() error {
	a.logger.Info("Stopping DKNet application")

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
		return errs[0]
	}

	a.logger.Info("DKNet application stopped successfully")
	return nil
}
