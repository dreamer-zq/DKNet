package security

import (
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/config"
)

// AccessController defines the interface for access control
type AccessController interface {
	// IsAuthorized checks if a peerID is authorized to connect
	IsAuthorized(peerID string) bool

	// GetAuthorizedPeers returns all authorized peer IDs (for debugging)
	GetAuthorizedPeers() []string

	// IsEnabled returns whether access control is enabled
	IsEnabled() bool
}

// Controller implements access control functionality
type Controller struct {
	config       *config.AccessControlConfig
	allowedPeers map[string]bool
	logger       *zap.Logger
}

// NewController creates a new access controller
func NewController(cfg *config.AccessControlConfig, logger *zap.Logger) *Controller {
	controller := &Controller{
		config:       cfg,
		allowedPeers: make(map[string]bool),
		logger:       logger,
	}

	// Build fast lookup map
	for _, peerID := range cfg.AllowedPeers {
		controller.allowedPeers[peerID] = true
	}

	if cfg.Enabled {
		logger.Info("Access control enabled",
			zap.Int("allowed_peers_count", len(cfg.AllowedPeers)),
			zap.Strings("allowed_peers", cfg.AllowedPeers))
	} else {
		logger.Info("Access control disabled")
	}

	return controller
}

// IsAuthorized checks if a peerID is authorized to connect
func (c *Controller) IsAuthorized(peerID string) bool {
	if !c.config.Enabled {
		return true // Allow all connections when access control is disabled
	}
	return c.allowedPeers[peerID]
}

// GetAuthorizedPeers returns all authorized peer IDs
func (c *Controller) GetAuthorizedPeers() []string {
	return c.config.AllowedPeers
}

// IsEnabled returns whether access control is enabled
func (c *Controller) IsEnabled() bool {
	return c.config.Enabled
}
