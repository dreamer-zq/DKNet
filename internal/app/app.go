package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/api"
	"github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/p2p"
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
	store, err := storage.NewLevelDBStorage(cfg.Storage.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create LevelDB storage: %w", err)
	}

	// Create P2P network
	network, err := p2p.NewNetwork(&p2p.Config{
		ListenAddrs:    cfg.P2P.ListenAddrs,
		BootstrapPeers: cfg.P2P.BootstrapPeers,
		PrivateKeyFile: cfg.P2P.PrivateKeyFile,
		AccessControl:  &cfg.Security.AccessControl,
	}, logger.Named("p2p"))
	if err != nil {
		common.LogMsgDo("failed to create P2P network", func() error {
			return store.Close()
		})
		return nil, fmt.Errorf("failed to create P2P network: %w", err)
	}

	// Use peer ID as node ID for TSS service
	peerID := network.GetHostID()
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
	apiServer, err := api.NewServer(cfg, tssService, network, logger.Named("api"))
	if err != nil {
		common.LogMsgDo("failed to close storage", func() error {
			return store.Close()
		})
		common.LogMsgDo("failed to stop network", func() error {
			return network.Stop()
		})
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
	if err := a.network.Start(ctx); err != nil {
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
