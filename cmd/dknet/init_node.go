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
		Short: "Initialize a single node",
		Long:  "Initialize a single node with configuration and key files.",
		RunE:  runInitNode,
	}

	// Add specific flags for init-node command
	cmd.Flags().StringP("moniker", "", "", "Human-readable node name (defaults to hostname)")
	cmd.Flags().StringP("output-dir", "o", "./", "Output directory for node files")
	cmd.Flags().StringSliceP("bootstrap-peers", "b", []string{}, "Bootstrap peer addresses")
	cmd.Flags().IntP("http-port", "", 8080, "HTTP API port")
	cmd.Flags().IntP("grpc-port", "", 9090, "gRPC API port")
	cmd.Flags().IntP("p2p-port", "", 4001, "P2P listen port")
	cmd.Flags().StringP("listen-addr", "", "0.0.0.0", "Listen address")

	return cmd
}

func runInitNode(cmd *cobra.Command, args []string) error {
	// Parse common flags
	if err := parseCommonFlags(cmd); err != nil {
		return err
	}

	// Get node-specific flags
	moniker, _ := cmd.Flags().GetString("moniker")
	bootstrapPeers, _ := cmd.Flags().GetStringSlice("bootstrap-peers")
	httpPort, _ := cmd.Flags().GetInt("http-port")
	grpcPort, _ := cmd.Flags().GetInt("grpc-port")
	p2pPort, _ := cmd.Flags().GetInt("p2p-port")
	listenAddr, _ := cmd.Flags().GetString("listen-addr")

	if moniker == "" {
		moniker = "node1"
	}

	logger.Info("Initializing TSS node",
		zap.String("moniker", moniker),
		zap.String("output", outputDir),
		zap.Bool("docker", dockerMode))

	// Create node directory
	nodeDir := filepath.Join(outputDir, "node1")
	if dirErr := ensureNodeDirectory(nodeDir); dirErr != nil {
		return dirErr
	}

	// Generate and save node key
	_, peerID, err := generateAndSaveNodeKey(nodeDir, "node1")
	if err != nil {
		return err
	}

	logger.Info("Generated P2P key", zap.String("peerID", peerID.String()))

	// Generate and save configuration
	configFile := filepath.Join(nodeDir, "config.yaml")
	if err := generateAndSaveNodeConfig(moniker, bootstrapPeers,
		listenAddr, p2pPort, httpPort, grpcPort, configFile, dockerMode); err != nil {
		return fmt.Errorf("failed to save config file: %w", err)
	}

	logger.Info("Generated configuration", zap.String("file", configFile))

	// Generate node info
	if err := generateNodeInfo(nodeDir, peerID.String(),
		listenAddr, p2pPort, bootstrapPeers, dockerMode); err != nil {
		return fmt.Errorf("failed to generate node info: %w", err)
	}

	logger.Info("TSS node initialization completed successfully",
		zap.String("nodeDir", nodeDir),
		zap.String("peerID", peerID.String()))
	return nil
}
