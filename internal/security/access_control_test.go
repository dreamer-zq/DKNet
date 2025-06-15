package security

import (
	"testing"

	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/config"
)

func TestAccessController_IsAuthorized(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name         string
		config       *config.AccessControlConfig
		peerID       string
		expectedAuth bool
	}{
		{
			name: "access control disabled - should allow all",
			config: &config.AccessControlConfig{
				Enabled:      false,
				AllowedPeers: []string{"peer1", "peer2"},
			},
			peerID:       "unknown_peer",
			expectedAuth: true,
		},
		{
			name: "access control enabled - peer in whitelist",
			config: &config.AccessControlConfig{
				Enabled:      true,
				AllowedPeers: []string{"peer1", "peer2", "peer3"},
			},
			peerID:       "peer2",
			expectedAuth: true,
		},
		{
			name: "access control enabled - peer not in whitelist",
			config: &config.AccessControlConfig{
				Enabled:      true,
				AllowedPeers: []string{"peer1", "peer2", "peer3"},
			},
			peerID:       "unknown_peer",
			expectedAuth: false,
		},
		{
			name: "access control enabled - empty whitelist",
			config: &config.AccessControlConfig{
				Enabled:      true,
				AllowedPeers: []string{},
			},
			peerID:       "any_peer",
			expectedAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := NewController(tt.config, logger)

			result := controller.IsAuthorized(tt.peerID)

			if result != tt.expectedAuth {
				t.Errorf("IsAuthorized() = %v, expected %v", result, tt.expectedAuth)
			}
		})
	}
}

func TestAccessController_GetAuthorizedPeers(t *testing.T) {
	logger := zap.NewNop()

	allowedPeers := []string{"peer1", "peer2", "peer3"}
	cfg := &config.AccessControlConfig{
		Enabled:      true,
		AllowedPeers: allowedPeers,
	}

	controller := NewController(cfg, logger)
	result := controller.GetAuthorizedPeers()

	if len(result) != len(allowedPeers) {
		t.Errorf("GetAuthorizedPeers() returned %d peers, expected %d", len(result), len(allowedPeers))
	}

	// Check if all expected peers are present
	peerMap := make(map[string]bool)
	for _, peer := range result {
		peerMap[peer] = true
	}

	for _, expectedPeer := range allowedPeers {
		if !peerMap[expectedPeer] {
			t.Errorf("Expected peer %s not found in result", expectedPeer)
		}
	}
}

func TestAccessController_IsEnabled(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name     string
		enabled  bool
		expected bool
	}{
		{
			name:     "access control enabled",
			enabled:  true,
			expected: true,
		},
		{
			name:     "access control disabled",
			enabled:  false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.AccessControlConfig{
				Enabled: tt.enabled,
			}

			controller := NewController(cfg, logger)
			result := controller.IsEnabled()

			if result != tt.expected {
				t.Errorf("IsEnabled() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
