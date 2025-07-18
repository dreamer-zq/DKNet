package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"

	"github.com/dreamer-zq/DKNet/templates"
)

const (
	defaultBindIP = "0.0.0.0"
)

// addCommonFlags adds flags that are common to multiple commands
func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringP(flagOutput, "o", "./", "Output directory for generated files")
	cmd.Flags().BoolP(flagDocker, "d", false, "Generate Docker-specific configurations")
}

// ensureNodeDirectory creates node directory if it doesn't exist
func ensureNodeDirectory(nodeDir string) error {
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}
	return nil
}

// generateAndSaveNodeKey generates P2P key pair and saves private key to file
func generateAndSaveNodeKey(nodeDir string) (crypto.PrivKey, peer.ID, error) {
	// Generate private key
	privKey, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 2048, rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Get peer ID
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get peer ID: %w", err)
	}

	// Marshal private key
	keyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Save private key
	keyFile := filepath.Join(nodeDir, "node_key")
	if err := os.WriteFile(keyFile, keyBytes, 0600); err != nil {
		return nil, "", fmt.Errorf("failed to save key file: %w", err)
	}

	return privKey, peerID, nil
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
		return defaultBindIP
	}
	return "127.0.0.1"
}

// DockerNodeConfig represents configuration for a Docker node
type DockerNodeConfig struct {
	Name         string
	ServiceName  string
	NodeDir      string
	HTTPPort     int
	GRPCPort     int
	P2PPort      int
	IP           string
	StartPeriod  int
	UseCustomIP  bool
	Dependencies []string
}

// DockerComposeConfig represents the full docker-compose configuration
type DockerComposeConfig struct {
	Nodes           []DockerNodeConfig
	UseCustomSubnet bool
	Subnet          string
}

// generateDockerCompose generates docker-compose.yaml file using template
func generateDockerCompose(outputDir string, nodes int) error {
	// Read template
	tmplContent, err := templates.GetTemplate("docker-compose.tmpl")
	if err != nil {
		return fmt.Errorf("failed to read docker-compose template: %w", err)
	}

	// Parse template
	tmpl, err := template.New("docker-compose").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse docker-compose template: %w", err)
	}

	// Generate node configurations
	var nodeConfigs []DockerNodeConfig
	for i := 1; i <= nodes; i++ {
		// Calculate dependencies (all previous nodes)
		var dependencies []string
		for j := 1; j < i; j++ {
			dependencies = append(dependencies, fmt.Sprintf("tss-node%d", j))
		}

		nodeConfig := DockerNodeConfig{
			Name:         fmt.Sprintf("Node %d", i),
			ServiceName:  fmt.Sprintf("tss-node%d", i),
			NodeDir:      fmt.Sprintf("node%d", i),
			HTTPPort:     18080 + i,
			GRPCPort:     19090 + i + 4,
			P2PPort:      14000 + i,
			IP:           fmt.Sprintf("172.20.0.%d", i+1),
			StartPeriod:  5 + (i-1)*5,
			UseCustomIP:  true, // Enable custom IP assignment
			Dependencies: dependencies,
		}
		nodeConfigs = append(nodeConfigs, nodeConfig)
	}

	// Create template data
	config := DockerComposeConfig{
		Nodes:           nodeConfigs,
		UseCustomSubnet: true,
		Subnet:          "172.20.0.0/16",
	}

	// Generate output file
	outputFile := filepath.Join(outputDir, "docker-compose.yaml")
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create docker-compose.yaml: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close file: %v\n", err)
		}
	}()

	// Execute template
	if err := tmpl.Execute(file, config); err != nil {
		return fmt.Errorf("failed to execute docker-compose template: %w", err)
	}

	return nil
}
