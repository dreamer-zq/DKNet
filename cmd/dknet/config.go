package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/dreamer-zq/DKNet/internal/config"
)

// generateAndSaveNodeConfig generates node configuration and saves it to file
func generateAndSaveNodeConfig(
	moniker string,
	bootstrapPeers []string,
	listenAddr string,
	p2pPort int,
	httpPort int,
	grpcPort int,
	configPath string,
	sessionSeedKey string,
	dockerMode bool,
) error {
	// Set the correct private key file path based on mode
	privateKeyFile := "./node_key"
	storagePath := "./data/storage"
	if dockerMode {
		privateKeyFile = "/app/node/node_key"
		storagePath = "/app/data/storage"
	}

	cfg := &config.NodeConfig{
		Server: config.ServerConfig{
			HTTP: config.HTTPConfig{
				Host: "0.0.0.0",
				Port: httpPort,
			},
			GRPC: config.GRPCConfig{
				Host: "0.0.0.0",
				Port: grpcPort,
			},
		},
		P2P: config.P2PConfig{
			ListenAddrs:    []string{fmt.Sprintf("/ip4/%s/tcp/%d", listenAddr, p2pPort)},
			BootstrapPeers: bootstrapPeers,
			PrivateKeyFile: privateKeyFile,
			MaxPeers:       50,
		},
		Storage: config.StorageConfig{
			Type:    "leveldb",
			Path:    storagePath,
			Options: make(map[string]string),
		},
		TSS: config.TSSConfig{
			Moniker: moniker,
			ValidationService: &config.ValidationServiceConfig{
				Enabled:            false,
				URL:                "",
				TimeoutSeconds:     30,
				Headers:            make(map[string]string),
				InsecureSkipVerify: false,
			},
		},
		Security: generateDefaultSecurityConfigWithSessionKey(sessionSeedKey),
		Logging: config.LoggingConfig{
			Level:       "debug",
			Environment: "dev",
			Output:      "stdout",
		},
	}

	// Save config to file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0o644)
}

// generateNodeInfo creates a node information file
func generateNodeInfo(
	nodeDir, peerID string,
	listenAddr string,
	p2pPort int,
	bootstrapPeers []string,
	dockerMode bool,
) error {
	infoFile := filepath.Join(nodeDir, "node-info.txt")

	content := fmt.Sprintf(`DKNet Node Information
====================

Node Details:
- Peer ID: %s

Network Configuration:
- Listen Address: %s:%d
- Multiaddr: /ip4/%s/tcp/%d/p2p/%s

Generated Files:
- Configuration: ./config.yaml
- Private Key: ./node_key

Bootstrap Peers:
`, peerID, listenAddr, p2pPort, listenAddr, p2pPort, peerID)

	if len(bootstrapPeers) == 0 {
		content += "- None specified (you need to add bootstrap peers to connect to the network)\n"
	} else {
		for i, peer := range bootstrapPeers {
			content += fmt.Sprintf("- Peer %d: %s\n", i+1, peer)
		}
	}

	content += fmt.Sprintf(`
Usage:
1. Copy this entire directory to your deployment location
2. Update bootstrap_peers in config.yaml if needed
3. Start the node:
   ./dknet start --config ./config.yaml

Share with other nodes:
- Your multiaddr: /ip4/%s/tcp/%d/p2p/%s
- Use this multiaddr in their bootstrap_peers configuration

Security Note:
- Keep node_key file secure and private
- Only share the peer ID and multiaddr, never the private key
`, listenAddr, p2pPort, peerID)

	return os.WriteFile(infoFile, []byte(content), 0o644)
}

// SecurityOptions holds security configuration options
type SecurityOptions struct {
	EnableAuth           bool
	JWTSecret            string
	JWTIssuer            string
	EnableTLS            bool
	CertFile             string
	KeyFile              string
	EnableAccessControl  bool
	AllowedPeers         []string
	EnableSessionEncrypt bool
	SessionSeedKey       string
}

// ValidationServiceOptions holds validation service configuration options
type ValidationServiceOptions struct {
	Enabled            bool
	URL                string
	TimeoutSeconds     int
	Headers            map[string]string
	InsecureSkipVerify bool
}

// generateDefaultSecurityConfigWithSessionKey creates a default security configuration with a session key
func generateDefaultSecurityConfigWithSessionKey(sessionSeedKey string) config.SecurityConfig {
	return config.SecurityConfig{
		TLSEnabled: false,
		CertFile:   "",
		KeyFile:    "",
		APIAuth: config.AuthConfig{
			Enabled:   false,
			JWTSecret: "",
			JWTIssuer: "",
		},
		AccessControl: config.AccessControlConfig{
			Enabled:      false,
			AllowedPeers: []string{},
		},
		SessionEncryption: config.SessionEncryptionConfig{
			Enabled: true,
			SeedKey: sessionSeedKey,
		},
	}
}
