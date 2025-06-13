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

	cmd.Flags().StringP("node-dir", "d", "./", "Node directory containing config.yaml and node_key")
	cmd.Flags().StringP("config", "c", "", "Path to config.yaml file (overrides node-dir)")
	cmd.Flags().StringP("key", "k", "", "Path to node_key file (overrides node-dir)")
	cmd.Flags().BoolP("json", "j", false, "Output in JSON format")
	cmd.Flags().BoolP("multiaddr-only", "m", false, "Only output the multiaddr")

	return cmd
}

func runShowNode(cmd *cobra.Command, args []string) error {
	// Get flags
	nodeDir, _ := cmd.Flags().GetString("node-dir")
	configFile, _ := cmd.Flags().GetString("config")
	keyFile, _ := cmd.Flags().GetString("key")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	multiaddrOnly, _ := cmd.Flags().GetBool("multiaddr-only")

	// Determine file paths
	if configFile == "" {
		configFile = filepath.Join(nodeDir, "config.yaml")
	}
	if keyFile == "" {
		keyFile = filepath.Join(nodeDir, "node_key")
	}

	// Load configuration
	cfg, err := loadNodeConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Load peer ID from key file
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
		NodeID:         cfg.TSS.NodeID,
		Moniker:        cfg.TSS.Moniker,
		PeerID:         peerID.String(),
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
	} else {
		return outputText(&nodeInfo)
	}
}

type NodeDisplayInfo struct {
	NodeID         string   `json:"node_id"`
	Moniker        string   `json:"moniker"`
	PeerID         string   `json:"peer_id"`
	Multiaddr      string   `json:"multiaddr"`
	ListenAddr     string   `json:"listen_addr"`
	Port           int      `json:"port"`
	BootstrapPeers []string `json:"bootstrap_peers"`
	HTTPPort       int      `json:"http_port"`
	GRPCPort       int      `json:"grpc_port"`
}

func loadNodeConfig(configFile string) (*config.NodeConfig, error) {
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

func buildDisplayMultiaddr(cfg *config.NodeConfig, listenAddr string, port int, peerID string) string {
	// If listen address is 0.0.0.0 (Docker mode), try to infer the correct IP
	if listenAddr == defaultBindIP {
		// Look for Docker network IP in bootstrap peers
		nodeID := cfg.TSS.NodeID

		// Try to extract our IP from a pattern in bootstrap peers
		// Docker nodes typically have IPs like 172.20.0.2, 172.20.0.3, etc.
		if dockerIP := inferDockerIPFromNodeID(nodeID); dockerIP != "" {
			return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", dockerIP, port, peerID)
		}

		// If we can't infer Docker IP, check if we have bootstrap peers to get network pattern
		if len(cfg.P2P.BootstrapPeers) > 0 {
			if dockerIP := inferDockerIPFromBootstrapPeers(cfg.P2P.BootstrapPeers, nodeID); dockerIP != "" {
				return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", dockerIP, port, peerID)
			}
		}

		// Fallback: use Docker network base + node number guess
		return fmt.Sprintf("/ip4/172.20.0.2/tcp/%d/p2p/%s", port, peerID)
	}

	// For non-Docker mode or specific IP, use as-is
	return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", listenAddr, port, peerID)
}

func inferDockerIPFromNodeID(nodeID string) string {
	// Try to extract node number from nodeID like "node1", "node2", etc.
	if strings.HasPrefix(nodeID, "node") && len(nodeID) > 4 {
		nodeNumStr := nodeID[4:]
		var nodeNum int
		if n, err := fmt.Sscanf(nodeNumStr, "%d", &nodeNum); n == 1 && err == nil {
			return fmt.Sprintf("172.20.0.%d", nodeNum+1) // node1 -> 172.20.0.2, node2 -> 172.20.0.3, etc.
		}
	}
	return ""
}

func inferDockerIPFromBootstrapPeers(bootstrapPeers []string, nodeID string) string {
	// Extract network pattern from bootstrap peers
	// If we see IPs like 172.20.0.3, 172.20.0.4, we can infer that node1 is 172.20.0.2
	nodeNum := 0
	if strings.HasPrefix(nodeID, "node") && len(nodeID) > 4 {
		if _, err := fmt.Sscanf(nodeID[4:], "%d", &nodeNum); err != nil {
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
  Node ID:    %s
  Moniker:    %s
  Peer ID:    %s
  Multiaddr:  %s

Network Configuration:
  Listen Address: %s:%d
  HTTP Port:      %d
  gRPC Port:      %d

Bootstrap Peers:
`, info.NodeID, info.Moniker, info.PeerID, info.Multiaddr,
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
