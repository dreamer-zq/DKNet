package config

import (
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/viper"
)

// NodeConfig holds all configuration for the DKNet
type NodeConfig struct {
	Server   ServerConfig   `yaml:"server" mapstructure:"server"`
	P2P      P2PConfig      `yaml:"p2p" mapstructure:"p2p"`
	Storage  StorageConfig  `yaml:"storage" mapstructure:"storage"`
	TSS      TSSConfig      `yaml:"tss" mapstructure:"tss"`
	Security SecurityConfig `yaml:"security" mapstructure:"security"`
}

// ServerConfig holds HTTP and gRPC server configurations
type ServerConfig struct {
	HTTP HTTPConfig `yaml:"http" mapstructure:"http"`
	GRPC GRPCConfig `yaml:"grpc" mapstructure:"grpc"`
}

// HTTPConfig holds HTTP server configuration
type HTTPConfig struct {
	Port int    `yaml:"port" mapstructure:"port"`
	Host string `yaml:"host" mapstructure:"host"`
}

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Port int    `yaml:"port" mapstructure:"port"`
	Host string `yaml:"host" mapstructure:"host"`
}

// P2PConfig holds libp2p configuration
type P2PConfig struct {
	ListenAddrs    []string `yaml:"listen_addrs" mapstructure:"listen_addrs"`
	BootstrapPeers []string `yaml:"bootstrap_peers" mapstructure:"bootstrap_peers"`
	PrivateKeyFile string   `yaml:"private_key_file" mapstructure:"private_key_file"`
	MaxPeers       int      `yaml:"max_peers" mapstructure:"max_peers"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	Type    string            `yaml:"type" mapstructure:"type"` // "file", "leveldb"
	Path    string            `yaml:"path" mapstructure:"path"`
	Options map[string]string `yaml:"options" mapstructure:"options"`
}

// TSSConfig holds TSS protocol configuration
type TSSConfig struct {
	NodeID  string `yaml:"node_id" mapstructure:"node_id"`
	Moniker string `yaml:"moniker" mapstructure:"moniker"`
}

// NodeKeyInfo contains information about a node's P2P key
type NodeKeyInfo struct {
	NodeID     string
	PeerID     string
	KeyFile    string
	PrivateKey crypto.PrivKey
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	TLSEnabled bool   `yaml:"tls_enabled" mapstructure:"tls_enabled"`
	CertFile   string `yaml:"cert_file" mapstructure:"cert_file"`
	KeyFile    string `yaml:"key_file" mapstructure:"key_file"`
}

// Load loads configuration from file or environment variables
func Load(configFile string) (*NodeConfig, error) {
	v := viper.New()
	
	// Set default values
	setDefaults(v)
	
	// Read config file if provided
	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
	}
	
	// Read environment variables
	v.AutomaticEnv()
	
	// Try to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is okay, we'll use defaults and env vars
	}
	
	var config NodeConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	
	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.http.host", "0.0.0.0")
	v.SetDefault("server.http.port", 8080)
	v.SetDefault("server.grpc.host", "0.0.0.0")
	v.SetDefault("server.grpc.port", 9090)
	
	// P2P defaults
	v.SetDefault("p2p.listen_addrs", []string{"/ip4/0.0.0.0/tcp/4001"})
	v.SetDefault("p2p.bootstrap_peers", []string{})
	v.SetDefault("p2p.private_key_file", "./data/p2p_key")
	v.SetDefault("p2p.max_peers", 50)
	
	// Storage defaults
	v.SetDefault("storage.type", "leveldb")
	v.SetDefault("storage.path", "./data/storage")
	
	// TSS defaults
	hostname, _ := os.Hostname()
	v.SetDefault("tss.node_id", hostname)
	v.SetDefault("tss.moniker", hostname)
	
	// Security defaults
	v.SetDefault("security.tls_enabled", false)
	v.SetDefault("security.cert_file", "")
	v.SetDefault("security.key_file", "")
}

// validateConfig validates the configuration
func validateConfig(config *NodeConfig) error {
	if config.TSS.NodeID == "" {
		return fmt.Errorf("node_id cannot be empty")
	}
	
	if config.Storage.Type != "file" && config.Storage.Type != "leveldb" {
		return fmt.Errorf("unsupported storage type: %s", config.Storage.Type)
	}
	
	return nil
} 