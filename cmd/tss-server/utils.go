package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

// addCommonFlags adds flags that are common to multiple commands
func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().IntP(flagThreshold, "t", 2, "Threshold for TSS operations")
	cmd.Flags().StringP(flagOutput, "o", "./", "Output directory for generated files")
	cmd.Flags().BoolP(flagDocker, "d", false, "Generate Docker-specific configurations")
}

// parseCommonFlags parses common flags from the command
func parseCommonFlags(cmd *cobra.Command) error {
	var err error
	
	threshold, err = cmd.Flags().GetInt(flagThreshold)
	if err != nil {
		return fmt.Errorf("failed to parse threshold flag: %w", err)
	}
	
	outputDir, err = cmd.Flags().GetString(flagOutput)
	if err != nil {
		return fmt.Errorf("failed to parse output flag: %w", err)
	}
	
	dockerMode, err = cmd.Flags().GetBool(flagDocker)
	if err != nil {
		return fmt.Errorf("failed to parse docker flag: %w", err)
	}
	
	return nil
}

// validateThreshold validates threshold against total parties
func validateThreshold(threshold, parties int) error {
	if threshold >= parties {
		return fmt.Errorf("threshold (%d) must be less than number of parties (%d)", threshold, parties)
	}
	
	if threshold < 1 {
		return fmt.Errorf("threshold must be at least 1")
	}
	
	return nil
}

// ensureNodeDirectory creates node directory if it doesn't exist
func ensureNodeDirectory(nodeDir string) error {
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}
	return nil
}

// generateAndSaveNodeKey generates P2P key pair and saves private key to file
func generateAndSaveNodeKey(nodeDir, nodeID string) (crypto.PrivKey, peer.ID, error) {
	// Generate private key
	privKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate private key for %s: %w", nodeID, err)
	}

	// Get peer ID
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get peer ID for %s: %w", nodeID, err)
	}

	// Marshal private key
	keyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal private key for %s: %w", nodeID, err)
	}

	// Save private key
	keyFile := filepath.Join(nodeDir, "node_key")
	if err := os.WriteFile(keyFile, keyBytes, 0600); err != nil {
		return nil, "", fmt.Errorf("failed to save key file for %s: %w", nodeID, err)
	}

	return privKey, peerID, nil
}

// generateMultiaddr creates multiaddr string for a node
func generateMultiaddr(nodeIndex int, peerID string, dockerMode bool) string {
	if dockerMode {
		// Use Docker container names and internal network IPs
		ip := fmt.Sprintf("172.20.0.%d", nodeIndex+1) // Start from 172.20.0.2
		return fmt.Sprintf("/ip4/%s/tcp/4001/p2p/%s", ip, peerID)
	} else {
		// Use localhost with different ports
		port := 4000 + nodeIndex
		return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", port, peerID)
	}
}

// getNodeP2PPort returns the P2P port for a given node index
func getNodeP2PPort(nodeIndex int, dockerMode bool) int {
	if dockerMode {
		return 4001 // Docker mode uses same port with different IPs
	}
	return 4000 + nodeIndex // Local mode uses different ports
}

// getNodeListenAddr returns the listen address for a node
func getNodeListenAddr(dockerMode bool) string {
	if dockerMode {
		return "0.0.0.0"
	}
	return "127.0.0.1"
} 