package config

import (
	"fmt"
	"os"
	"path/filepath"

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

	// ConfigDir is the directory containing the config file (not saved to YAML)
	ConfigDir string `yaml:"-" mapstructure:"-"`
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
	Moniker string `yaml:"moniker" mapstructure:"moniker"`
	// Validation service configuration (optional)
	ValidationService *ValidationServiceConfig `yaml:"validation_service,omitempty" mapstructure:"validation_service"`
}

// ValidationServiceConfig holds validation service configuration
type ValidationServiceConfig struct {
	// Enable or disable validation service
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
	// Validation service HTTP endpoint URL
	URL string `yaml:"url" mapstructure:"url"`
	// Request timeout in seconds (default: 30)
	TimeoutSeconds int `yaml:"timeout_seconds" mapstructure:"timeout_seconds"`
	// HTTP headers to include in validation requests
	Headers map[string]string `yaml:"headers,omitempty" mapstructure:"headers"`
	// Skip TLS verification (for development only)
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`
}

// NodeKeyInfo contains information about a node's P2P key
type NodeKeyInfo struct {
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

	var configDir string
	// Read config file if provided
	if configFile != "" {
		v.SetConfigFile(configFile)
		// Extract directory from config file path
		configDir = filepath.Dir(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		// Default to current directory
		configDir = "."
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

	config := &NodeConfig{}
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Set the config directory
	config.ConfigDir = configDir

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
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
	v.SetDefault("tss.moniker", hostname)

	// Validation service defaults
	v.SetDefault("tss.validation_service.enabled", false)
	v.SetDefault("tss.validation_service.timeout_seconds", 30)
	v.SetDefault("tss.validation_service.insecure_skip_verify", false)

	// Security defaults
	v.SetDefault("security.tls_enabled", false)
	v.SetDefault("security.cert_file", "")
	v.SetDefault("security.key_file", "")
}

// validateConfig validates the configuration
func validateConfig(config *NodeConfig) error {
	if config.TSS.Moniker == "" {
		return fmt.Errorf("moniker cannot be empty")
	}

	if config.Storage.Type != "file" && config.Storage.Type != "leveldb" {
		return fmt.Errorf("unsupported storage type: %s", config.Storage.Type)
	}

	// Validate validation service configuration if enabled
	if config.TSS.ValidationService != nil && config.TSS.ValidationService.Enabled {
		if config.TSS.ValidationService.URL == "" {
			return fmt.Errorf("validation service URL cannot be empty when validation service is enabled")
		}
		if config.TSS.ValidationService.TimeoutSeconds <= 0 {
			return fmt.Errorf("validation service timeout must be positive")
		}
	}

	return nil
}
