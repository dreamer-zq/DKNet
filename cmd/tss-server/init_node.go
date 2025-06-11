package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func runInitNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init-node",
		Short: "Generate configuration and P2P key for a single TSS node",
		Long: `Generate configuration and P2P key for a single TSS node.
This command creates:
- P2P private key for the node
- Configuration file for the node
- Node information file

This is suitable for production environments where each organization
generates their own node configuration independently.`,
		RunE: runInitNode,
	}
	addCommonFlags(cmd)
	// Add specific flags for init-node command
	cmd.Flags().StringP("node-id", "", "node1", "Node ID (e.g., node1, org-alpha, etc.)")
	cmd.Flags().StringP("moniker", "", "", "Human-readable node name (defaults to node-id)")
	cmd.Flags().StringSliceP("bootstrap-peers", "b", []string{}, "Bootstrap peer multiaddrs")
	cmd.Flags().IntP("http-port", "", 8080, "HTTP API port")
	cmd.Flags().IntP("grpc-port", "", 9090, "gRPC API port")
	cmd.Flags().IntP("p2p-port", "", 4001, "P2P listen port")
	cmd.Flags().StringP("listen-addr", "", "127.0.0.1", "P2P listen address")
	return cmd
}

func runInitNode(cmd *cobra.Command, args []string) error {
	// Parse common flags
	if err := parseCommonFlags(cmd); err != nil {
		return err
	}
	
	// Get node-specific flags
	nodeID, _ := cmd.Flags().GetString("node-id")
	moniker, _ := cmd.Flags().GetString("moniker")
	bootstrapPeers, _ := cmd.Flags().GetStringSlice("bootstrap-peers")
	httpPort, _ := cmd.Flags().GetInt("http-port")
	grpcPort, _ := cmd.Flags().GetInt("grpc-port")
	p2pPort, _ := cmd.Flags().GetInt("p2p-port")
	listenAddr, _ := cmd.Flags().GetString("listen-addr")

	if moniker == "" {
		moniker = nodeID
	}

	logger.Info("Initializing TSS node",
		zap.String("nodeID", nodeID),
		zap.String("moniker", moniker),
		zap.String("output", outputDir),
		zap.Bool("docker", dockerMode))

	// Create node directory
	nodeDir := filepath.Join(outputDir, nodeID)
	if err := ensureNodeDirectory(nodeDir); err != nil {
		return err
	}

	// Generate and save node key
	_, peerID, err := generateAndSaveNodeKey(nodeDir, nodeID)
	if err != nil {
		return err
	}

	logger.Info("Generated P2P key", zap.String("peerID", peerID.String()))

	// Generate and save configuration
	configFile := filepath.Join(nodeDir, "config.yaml")
	if err := generateAndSaveNodeConfig(nodeID, moniker, bootstrapPeers, 
		httpPort, grpcPort, p2pPort, listenAddr, configFile, dockerMode); err != nil {
		return fmt.Errorf("failed to save config file: %w", err)
	}

	logger.Info("Generated configuration", zap.String("file", configFile))

	// Generate node info
	if err := generateNodeInfo(nodeDir, nodeID, peerID.String(), 
		listenAddr, p2pPort, bootstrapPeers, dockerMode); err != nil {
		return fmt.Errorf("failed to generate node info: %w", err)
	}

	logger.Info("TSS node initialization completed successfully", 
		zap.String("nodeDir", nodeDir),
		zap.String("peerID", peerID.String()))
	return nil
} 