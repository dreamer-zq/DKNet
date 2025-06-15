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
) error {
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
			PrivateKeyFile: "./node_key",
			MaxPeers:       50,
		},
		Storage: config.StorageConfig{
			Type: "leveldb",
			Path: "./data/storage",
		},
		TSS: config.TSSConfig{
			Moniker: moniker,
		},
		Security: config.SecurityConfig{
			TLSEnabled: false,
		},
	}

	// Save config to file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
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

	return os.WriteFile(infoFile, []byte(content), 0644)
}

// generateSummary creates a cluster summary file
func generateSummary(outputDir string, nodeKeys map[string]config.NodeKeyInfo, multiaddrs []string, nodes int, dockerMode bool) error {
	summaryFile := filepath.Join(outputDir, "cluster-info.txt")

	content := fmt.Sprintf(`TSS Cluster Configuration Summary
=====================================

Cluster Parameters:
- Nodes: %d
- Mode: %s

Generated Files Structure:
Each node has its own directory with:
- %s/node1/config.yaml & node_key & node-info.txt
- %s/node2/config.yaml & node_key & node-info.txt
- %s/node3/config.yaml & node_key & node-info.txt

Node Information (Peer IDs):
`, nodes, map[bool]string{true: "Docker", false: "Local"}[dockerMode], outputDir, outputDir, outputDir)

	for i := 1; i <= nodes; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		nodeInfo := nodeKeys[nodeID]
		content += fmt.Sprintf("- node%d: %s\n", i, nodeInfo.PeerID)
	}

	content += "\nBootstrap Multiaddrs:\n"
	for i, addr := range multiaddrs {
		content += fmt.Sprintf("- node%d: %s\n", i+1, addr)
	}

	if dockerMode {
		content += fmt.Sprintf(`
Docker Usage:
1. Update your docker-compose.yml to mount each node directory:
   volumes:
     - ./%s/node1:/app/node:ro
   
2. Start the cluster:
   docker-compose up -d

3. Each container should reference ./node/config.yaml and ./node/node_key
`, outputDir)
	} else {
		content += fmt.Sprintf(`
Local Usage:
1. Start each node with its configuration:
   ./dknet start --config %s/node1/config.yaml
   ./dknet start --config %s/node2/config.yaml
   ./dknet start --config %s/node3/config.yaml

2. Each node runs on different ports as configured in their config.yaml
`, outputDir, outputDir, outputDir)
	}

	return os.WriteFile(summaryFile, []byte(content), 0644)
}
