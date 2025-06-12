package main

import "go.uber.org/zap"

var (
	cfgFile string
	logger  *zap.Logger

	// Common flags used by multiple commands
	outputDir  string
	dockerMode bool
)

// Common flag names as constants
const (
	flagOutput = "output"
	flagDocker = "docker"
)
