package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/config"
)

func runInitClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init-cluster",
		Short: "Initialize TSS cluster configuration",
		Long: `Generate configuration files and P2P keys for a TSS cluster.
This command creates:
- P2P private keys for each node
- Configuration files for each node
- Bootstrap peer configurations

Note: This is primarily for testing environments. In production, 
each organization should generate their own keys independently.`,
		RunE: runInitCluster,
	}
	addCommonFlags(cmd)
	// Add specific flags for init-cluster command
	cmd.Flags().IntP("nodes", "n", 3, "Number of nodes in the cluster")
	cmd.Flags().StringP("network", "", "172.20.0.0/16", "Docker network subnet")
	return cmd
}

func runInitCluster(cmd *cobra.Command, args []string) error {
	// Parse common flags
	if err := parseCommonFlags(cmd); err != nil {
		return err
	}
	
	// Get cluster-specific flags
	nodes, _ := cmd.Flags().GetInt("nodes")

	logger.Info("Initializing TSS cluster",
		zap.Int("nodes", nodes),
		zap.Int("threshold", threshold),
		zap.String("output", outputDir),
		zap.Bool("docker", dockerMode))

	// Validate parameters
	if err := validateThreshold(threshold, nodes); err != nil {
		return err
	}

	// Create output directory
	if err := ensureNodeDirectory(outputDir); err != nil {
		return err
	}

	// Generate P2P keys and peer IDs for each node
	nodeKeys := make(map[string]config.NodeKeyInfo)
	for i := 1; i <= nodes; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		nodeDir := filepath.Join(outputDir, nodeID)
		
		// Create node directory
		if err := ensureNodeDirectory(nodeDir); err != nil {
			return err
		}
		
		// Generate and save node key
		privKey, peerID, err := generateAndSaveNodeKey(nodeDir, nodeID)
		if err != nil {
			return err
		}

		nodeKeys[nodeID] = config.NodeKeyInfo{
			NodeID:     nodeID,
			PeerID:     peerID.String(),
			KeyFile:    filepath.Join(nodeDir, "node_key"),
			PrivateKey: privKey,
		}

		logger.Info("Generated key for node", zap.String("node", nodeID), zap.String("peerID", peerID.String()))
	}

	// Generate bootstrap peers (all nodes except the current one)
	var allMultiaddrs []string
	for i := 1; i <= nodes; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		peerID := nodeKeys[nodeID].PeerID
		multiaddr := generateMultiaddr(i, peerID, dockerMode)
		allMultiaddrs = append(allMultiaddrs, multiaddr)
	}

	// Generate configuration files for each node
	for i := 1; i <= nodes; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		nodeDir := filepath.Join(outputDir, nodeID)

		// Create bootstrap peers (exclude current node)
		var bootstrapPeers []string
		for j, addr := range allMultiaddrs {
			if j+1 != i { // Exclude self
				bootstrapPeers = append(bootstrapPeers, addr)
			}
		}

		// Generate and save config file
		configFile := filepath.Join(nodeDir, "config.yaml")
		// In Docker mode, all nodes use the same internal ports (Docker handles external mapping)
		httpPort := 8080
		grpcPort := 9090
		if !dockerMode {
			// In local mode, use different ports for each node
			httpPort = 8080 + i - 1
			grpcPort = 9090 + i - 1
		}
		if err := generateAndSaveNodeConfig(nodeID, fmt.Sprintf("TSS Node %d", i), threshold, nodes, bootstrapPeers,
			httpPort, grpcPort, getNodeP2PPort(i, dockerMode), getNodeListenAddr(dockerMode), configFile, dockerMode); err != nil {
			return fmt.Errorf("failed to save config for %s: %w", nodeID, err)
		}

		logger.Info("Generated config for node", zap.String("node", nodeID), zap.String("file", configFile))
		
		// Generate node info file
		p2pPort := getNodeP2PPort(i, dockerMode)
		listenAddr := getNodeListenAddr(dockerMode)
		if err := generateNodeInfo(nodeDir, nodeID, nodeKeys[nodeID].PeerID, threshold, nodes,
			listenAddr, p2pPort, bootstrapPeers, dockerMode); err != nil {
			return fmt.Errorf("failed to generate node info for %s: %w", nodeID, err)
		}
	}

	// Generate summary
	if err := generateSummary(outputDir, nodeKeys, allMultiaddrs, nodes, threshold, dockerMode); err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	logger.Info("TSS cluster initialization completed successfully", zap.String("output", outputDir))
	return nil
} 