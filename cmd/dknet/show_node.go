package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/dreamer-zq/DKNet/internal/config"
)

func runShowNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show-node",
		Short: "Show node information including Multiaddr",
		Long: `Display node information from generated configuration files.
This command reads the node's configuration and key files to show:
- Node ID and Moniker
- Peer ID and Multiaddr
- Network configuration
- Bootstrap peers

This is useful for deployment and sharing node information with other nodes.`,
		RunE: runShowNode,
	}

	cmd.Flags().StringP(flagNodeDir, "", "", "Node directory containing config.yaml and node_key")
	cmd.Flags().BoolP("json", "j", false, "Output in JSON format")
	cmd.Flags().BoolP("multiaddr-only", "m", false, "Only output the multiaddr")

	_ = cmd.MarkFlagRequired(flagNodeDir)

	return cmd
}

func runShowNode(cmd *cobra.Command, args []string) error {
	// Get flags
	nodeDir, err := cmd.Flags().GetString(flagNodeDir)
	if err != nil {
		return fmt.Errorf("failed to get node directory: %w", err)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	multiaddrOnly, _ := cmd.Flags().GetBool("multiaddr-only")

	// Load configuration
	cfg, err := loadNodeConfig(nodeDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Load peer ID from key file
	keyFile := filepath.Join(nodeDir, "node_key")
	peerID, err := loadPeerIDFromKeyFile(keyFile)
	if err != nil {
		return fmt.Errorf("failed to load peer ID: %w", err)
	}

	// Extract network info and build multiaddr
	var listenAddr string
	var port int
	var multiaddr string

	if len(cfg.P2P.ListenAddrs) > 0 {
		addr := cfg.P2P.ListenAddrs[0]
		parts := strings.Split(addr, "/")
		if len(parts) >= 5 {
			listenAddr = parts[2]
			if _, err := fmt.Sscanf(parts[4], "%d", &port); err != nil {
				// If parsing fails, use default port
				port = 4001
			}
		}
	}

	// Try to determine the correct multiaddr for display
	multiaddr = buildDisplayMultiaddr(cfg, listenAddr, port, peerID.String())

	// Handle multiaddr-only output
	if multiaddrOnly {
		fmt.Println(multiaddr)
		return nil
	}

	// Prepare node information
	nodeInfo := NodeDisplayInfo{
		PeerID:         peerID.String(),
		Moniker:        cfg.TSS.Moniker,
		Multiaddr:      multiaddr,
		ListenAddr:     listenAddr,
		Port:           port,
		BootstrapPeers: cfg.P2P.BootstrapPeers,
		HTTPPort:       cfg.Server.HTTP.Port,
		GRPCPort:       cfg.Server.GRPC.Port,
	}

	// Output in requested format
	if jsonOutput {
		return outputJSON(&nodeInfo)
	}
	return outputText(&nodeInfo)
}

// NodeDisplayInfo is the information displayed by the show-node command
type NodeDisplayInfo struct {
	PeerID         string   `json:"peer_id"`
	Moniker        string   `json:"moniker"`
	Multiaddr      string   `json:"multiaddr"`
	ListenAddr     string   `json:"listen_addr"`
	Port           int      `json:"port"`
	BootstrapPeers []string `json:"bootstrap_peers"`
	HTTPPort       int      `json:"http_port"`
	GRPCPort       int      `json:"grpc_port"`
}

// loadNodeConfig loads the node configuration from the node directory
func loadNodeConfig(nodeDir string) (*config.NodeConfig, error) {
	configFile := filepath.Join(nodeDir, "config.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	var cfg config.NodeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// loadPeerIDFromKeyFile loads the peer ID from the key file
func loadPeerIDFromKeyFile(keyFile string) (peer.ID, error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read key file %s: %w", keyFile, err)
	}

	privKey, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to get peer ID from private key: %w", err)
	}

	return peerID, nil
}

// buildDisplayMultiaddr builds the display multiaddr for the node
func buildDisplayMultiaddr(cfg *config.NodeConfig, listenAddr string, port int, peerID string) string {
	// If listen address is 0.0.0.0 (Docker mode), try to infer the correct IP
	if listenAddr == defaultBindIP {
		// Try to extract our IP from a pattern in bootstrap peers
		// Docker nodes typically have IPs like 172.20.0.2, 172.20.0.3, etc.
		if dockerIP := inferDockerIPFromPeerID(peerID); dockerIP != "" {
			return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", dockerIP, port, peerID)
		}

		// If we can't infer Docker IP, check if we have bootstrap peers to get network pattern
		if len(cfg.P2P.BootstrapPeers) > 0 {
			if dockerIP := inferDockerIPFromBootstrapPeers(cfg.P2P.BootstrapPeers, peerID); dockerIP != "" {
				return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", dockerIP, port, peerID)
			}
		}

		// Fallback: use Docker network base + default IP
		return fmt.Sprintf("/ip4/172.20.0.2/tcp/%d/p2p/%s", port, peerID)
	}

	// For non-Docker mode or specific IP, use as-is
	return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", listenAddr, port, peerID)
}

func inferDockerIPFromPeerID(peerID string) string {
	// Try to extract node number from peerID like "node1", "node2", etc.
	if strings.HasPrefix(peerID, "node") && len(peerID) > 4 {
		nodeNumStr := peerID[4:]
		var nodeNum int
		if n, err := fmt.Sscanf(nodeNumStr, "%d", &nodeNum); n == 1 && err == nil {
			return fmt.Sprintf("172.20.0.%d", nodeNum+1) // node1 -> 172.20.0.2, node2 -> 172.20.0.3, etc.
		}
	}
	return ""
}

func inferDockerIPFromBootstrapPeers(bootstrapPeers []string, peerID string) string {
	// Extract network pattern from bootstrap peers
	// If we see IPs like 172.20.0.3, 172.20.0.4, we can infer that node1 is 172.20.0.2
	nodeNum := 0
	if strings.HasPrefix(peerID, "node") && len(peerID) > 4 {
		if _, err := fmt.Sscanf(peerID[4:], "%d", &nodeNum); err != nil {
			// If parsing fails, return empty string
			return ""
		}
	}

	if nodeNum > 0 {
		// Look for the Docker network pattern in bootstrap peers
		for _, peer := range bootstrapPeers {
			if strings.Contains(peer, "/ip4/172.20.0.") {
				// This confirms Docker network, return our expected IP
				return fmt.Sprintf("172.20.0.%d", nodeNum+1)
			}
		}
	}

	return ""
}

func outputJSON(info *NodeDisplayInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputText(info *NodeDisplayInfo) error {
	fmt.Printf(`TSS Node Information
====================

Node Details:
  Peer ID:    %s
  Moniker:    %s
  Multiaddr:  %s

Network Configuration:
  Listen Address: %s:%d
  HTTP Port:      %d
  gRPC Port:      %d

Bootstrap Peers:
`, info.PeerID, info.Moniker, info.Multiaddr,
		info.ListenAddr, info.Port, info.HTTPPort, info.GRPCPort)

	if len(info.BootstrapPeers) == 0 {
		fmt.Println("  None configured")
	} else {
		for i, peer := range info.BootstrapPeers {
			fmt.Printf("  %d. %s\n", i+1, peer)
		}
	}

	fmt.Printf(`
Deployment Information:
======================

Share this multiaddr with other nodes:
  %s

Use this in other nodes' bootstrap_peers configuration:
  bootstrap_peers:
    - "%s"

Security Note:
- Never share the node_key file
- Only share the peer ID and multiaddr
`, info.Multiaddr, info.Multiaddr)

	return nil
}
