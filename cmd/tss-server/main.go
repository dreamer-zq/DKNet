package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/app"
	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/utils"
)

func main() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			// Ignore sync errors on stdout/stderr as they are common and harmless
			// Only log if it's not a sync error on stdout/stderr
			fmt.Fprintf(os.Stderr, "Warning: failed to sync logger: %v\n", syncErr)
		}
	}()

	rootCmd := &cobra.Command{
		Use:   "tss-server",
		Short: "TSS (Threshold Signature Scheme) Server",
		Long: `A server providing TSS services including keygen, signing, and resharing
with HTTP/gRPC APIs and libp2p communication between nodes.`,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the DKNet",
		Long:  "Start the DKNet with the specified configuration",
		RunE:  runServer,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(startCmd, runInitClusterCmd(), runInitNodeCmd(), runShowNodeCmd())

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Failed to execute command", zap.Error(err))
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Prompt for encryption password
	fmt.Println("DKNet TSS Server - Secure Key Storage")
	fmt.Println("=====================================")
	fmt.Println("This server uses encrypted storage for TSS private keys.")
	fmt.Println("Please enter a strong password to encrypt/decrypt stored keys.")
	fmt.Println()

	password, err := utils.ReadPasswordWithConfirmation()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	// Validate password strength (optional, but recommended)
	if err := utils.ValidatePassword(password); err != nil {
		logger.Warn("Password validation warning", zap.Error(err))
		fmt.Printf("Warning: %v\n", err)
		fmt.Println("Continue anyway? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return fmt.Errorf("operation cancelled")
		}
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize and start the application with encryption password
	application, err := app.New(cfg, logger, password)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}

	// Start the application
	if err := application.Start(ctx); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	// Wait for interrupt signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutdown signal received, stopping server...")

	// Graceful shutdown
	cancel()
	if err := application.Stop(); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
		return err
	}

	logger.Info("Server stopped gracefully")
	return nil
}
