package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"
)

// keyseedCmd generates a new seed key for session encryption
func keyseedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keyseed",
		Short: "Generate a new seed key for session encryption",
		Long: `Generate a new 256-bit (32 bytes) seed key for session encryption.
This seed key should be shared among all participants and configured 
in their config.yaml files under security.session_encryption.seed_key.

Example:
  dknet keyseed
  
The generated key should be distributed securely to all participants 
through out-of-band communication (e.g., secure messaging, email, etc.).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeyseed()
		},
	}

	return cmd
}

func runKeyseed() error {
	// Generate 32 random bytes (256 bits) for AES-256
	seedKey := make([]byte, 32)
	if _, err := rand.Read(seedKey); err != nil {
		return fmt.Errorf("failed to generate random seed key: %w", err)
	}

	// Convert to hex string
	seedKeyHex := hex.EncodeToString(seedKey)

	fmt.Println("Generated Session Encryption Seed Key:")
	fmt.Println("======================================")
	fmt.Printf("%s\n", seedKeyHex)
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Println("--------------")
	fmt.Println("Add the following to your config.yaml:")
	fmt.Println()
	fmt.Println("security:")
	fmt.Println("  session_encryption:")
	fmt.Println("    enabled: true")
	fmt.Printf("    seed_key: \"%s\"\n", seedKeyHex)
	fmt.Println()
	fmt.Println("IMPORTANT:")
	fmt.Println("- This seed key must be shared with ALL participants")
	fmt.Println("- Each participant must configure the SAME seed key")
	fmt.Println("- Distribute this key through secure, out-of-band communication")
	fmt.Println("- Store this key securely and do not share it publicly")

	return nil
}
