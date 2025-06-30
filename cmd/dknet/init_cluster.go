package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
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
	return cmd
}

func runInitCluster(cmd *cobra.Command, args []string) error {
	nodes, _ := cmd.Flags().GetInt("nodes")
	clusterOutputDir, _ := cmd.Flags().GetString("output")
	generateDocker, _ := cmd.Flags().GetBool("docker")

	// Default output directory
	if clusterOutputDir == "" {
		if generateDocker {
			clusterOutputDir = "deployments/docker-cluster"
		} else {
			clusterOutputDir = "tss-cluster"
		}
	}

	fmt.Printf("Initializing TSS cluster with %d nodes...\n", nodes)
	// Create output directory
	if err := os.MkdirAll(clusterOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Step 1: Generate keys for all nodes and collect peer info
	type NodeInfo struct {
		Index     int
		NodeDir   string
		PeerID    string
		Multiaddr string
	}

	var nodeInfos []NodeInfo
	for i := 1; i <= nodes; i++ {
		nodeDir := filepath.Join(clusterOutputDir, fmt.Sprintf("node%d", i))
		if createErr := os.MkdirAll(nodeDir, 0755); createErr != nil {
			return fmt.Errorf("failed to create node directory: %w", createErr)
		}

		// Generate node key
		nodeID := fmt.Sprintf("node%d", i)
		_, peerID, keyErr := generateAndSaveNodeKey(nodeDir, nodeID)
		if keyErr != nil {
			return fmt.Errorf("failed to generate node key for %s: %w", nodeID, keyErr)
		}

		// Generate multiaddr for this node
		listenAddr := getNodeListenAddr(generateDocker)
		p2pPort := getNodeP2PPort(i, generateDocker)
		var multiaddr string
		if generateDocker {
			// For Docker mode, use container hostnames
			multiaddr = fmt.Sprintf("/ip4/172.20.0.%d/tcp/4001/p2p/%s", i+1, peerID.String())
		} else {
			// For local mode, use localhost with different ports
			multiaddr = fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", listenAddr, p2pPort, peerID.String())
		}

		nodeInfos = append(nodeInfos, NodeInfo{
			Index:     i,
			NodeDir:   nodeDir,
			PeerID:    peerID.String(),
			Multiaddr: multiaddr,
		})

		fmt.Printf("Generated keys for node%d (peer: %s)\n", i, peerID.String())
	}

	// Step 2: Generate configuration files with bootstrap peers
	for _, nodeInfo := range nodeInfos {
		// Create bootstrap peers list (all other nodes)
		var bootstrapPeers []string
		for _, otherNode := range nodeInfos {
			if otherNode.Index != nodeInfo.Index {
				bootstrapPeers = append(bootstrapPeers, otherNode.Multiaddr)
			}
		}

		// Generate config file
		configFile := filepath.Join(nodeInfo.NodeDir, "config.yaml")

		// Set ports based on mode
		httpPort := 8080
		grpcPort := 9090
		if !generateDocker {
			// In local mode, use different ports for each node
			httpPort = 8080 + nodeInfo.Index
			grpcPort = 9090 + nodeInfo.Index + 4 // Offset to avoid conflicts
		}

		nodeName := fmt.Sprintf("TSS Node %d", nodeInfo.Index)
		listenAddr := getNodeListenAddr(generateDocker)
		p2pPort := getNodeP2PPort(nodeInfo.Index, generateDocker)

		configErr := generateAndSaveNodeConfig(nodeName, bootstrapPeers, listenAddr, p2pPort,
			httpPort, grpcPort, configFile, generateDocker)
		if configErr != nil {
			return fmt.Errorf("failed to generate config for node %d: %w", nodeInfo.Index, configErr)
		}

		// Generate node info file
		if err := generateNodeInfo(nodeInfo.NodeDir, nodeInfo.PeerID, listenAddr, p2pPort, bootstrapPeers, generateDocker); err != nil {
			return fmt.Errorf("failed to generate node info for node %d: %w", nodeInfo.Index, err)
		}

		fmt.Printf("Generated configuration for node%d (%d bootstrap peers)\n", nodeInfo.Index, len(bootstrapPeers))
	}

	// Generate Docker Compose configuration if requested
	if generateDocker {
		fmt.Println("Generating Docker Compose configuration...")

		if dockerErr := generateDockerCompose(clusterOutputDir, nodes); dockerErr != nil {
			return fmt.Errorf("failed to generate docker-compose.yaml: %w", dockerErr)
		}
		fmt.Println("Generated docker-compose.yaml")
	}

	fmt.Printf("✅ Cluster initialization completed!\n")
	if generateDocker {
		fmt.Printf("📁 Docker configuration saved to: %s\n", clusterOutputDir)
		fmt.Println("")
		fmt.Println("🐳 Before starting the cluster, build the Docker image:")
		fmt.Println("   docker build -t dknet:latest .")
		fmt.Println("")
		fmt.Println("🚀 To start the cluster:")
		fmt.Printf("   cd %s\n", clusterOutputDir)
		fmt.Println("   export TSS_ENCRYPTION_PASSWORD=\"YourSecurePassword123!\"")
		fmt.Println("   docker-compose up -d")
		fmt.Println("")
		fmt.Println("📊 To check status:")
		fmt.Println("   docker-compose ps")
		fmt.Println("   docker-compose logs -f")
	}

	return nil
}
