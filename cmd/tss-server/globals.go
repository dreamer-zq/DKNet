package main

import "go.uber.org/zap"

var (
	cfgFile string
	logger  *zap.Logger
	
	// Common flags used by multiple commands
	threshold   int
	outputDir   string
	dockerMode  bool
)

// Common flag names as constants
const (
	flagThreshold = "threshold"
	flagOutput    = "output"
	flagDocker    = "docker"
) 