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
	"github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/config"
)

func main() {
	// Initialize with a basic logger first, will be reconfigured after loading config
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(err)
	}

	defer func() {
		_ = logger.Sync()
	}()

	rootCmd := &cobra.Command{
		Use:   "dknet",
		Short: "DKNet - Distributed Key Network TSS Server",
		Long: `DKNet is a distributed threshold signature scheme (TSS) server that enables
secure multi-party computation for cryptographic operations.

This server provides APIs for key generation, signing, and key management
using threshold cryptography to ensure no single point of failure.`,
		RunE: runServer,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the DKNet",
		Long:  "Start the DKNet with the specified configuration",
		RunE:  runServer,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(startCmd, runInitClusterCmd(), runInitNodeCmd(), runShowNodeCmd(), keyseedCmd(), generateTokenCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		// Don't call os.Exit directly here, let main return normally
		// The defer statement will execute properly this way
		return
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Reconfigure logger based on configuration
	configuredLogger, err := common.NewLogger(&cfg.Logging)
	if err != nil {
		logger.Warn("Failed to create configured logger, using default", zap.Error(err))
	} else {
		// Replace global logger
		logger = configuredLogger
		logger.Info("Logger reconfigured",
			zap.String("level", cfg.Logging.Level),
			zap.String("environment", cfg.Logging.Environment),
			zap.String("output", cfg.Logging.Output))
	}

	// Get encryption password from environment or interactive input
	fmt.Println("DKNet TSS Server - Secure Key Storage")
	fmt.Println("=====================================")
	fmt.Println("This server uses encrypted storage for TSS private keys.")

	// Try environment variable first
	password, err := common.ReadPassword()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Println("Using password from TSS_ENCRYPTION_PASSWORD environment variable.")

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
