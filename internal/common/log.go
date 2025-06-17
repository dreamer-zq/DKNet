package common

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/dreamer-zq/DKNet/internal/config"
)

const (
	// EnvDev is the development environment
	EnvDev = "dev"
	// EnvPro is the production environment
	EnvPro = "pro"

	// LevelDebug is the debug level
	LevelDebug = "debug"
	// LevelInfo is the info level
	LevelInfo = "info"
	// LevelWarn is the warn level
	LevelWarn = "warn"
	// LevelError is the error level
	LevelError = "error"
)

// LogDo logs the error if it occurs
func LogDo(fn func() error) {
	if err := fn(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error in Do: %v\n", err)
	}
}

// LogMsgDo logs the error if it occurs
func LogMsgDo(msg string, fn func() error) {
	if err := fn(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error in %s: %v\n", msg, err)
	}
}

// NewLogger creates a new zap logger based on configuration
func NewLogger(cfg *config.LoggingConfig) (*zap.Logger, error) {
	// Determine log level
	var level zapcore.Level
	switch cfg.Level {
	case LevelDebug:
		level = zapcore.DebugLevel
	case LevelInfo:
		level = zapcore.InfoLevel
	case LevelWarn:
		level = zapcore.WarnLevel
	case LevelError:
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Determine encoder based on environment
	var encoder zapcore.Encoder
	switch cfg.Environment {
	case EnvPro:
		encoder = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	case EnvDev:
		encoderConfig := zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		// Default to development format
		encoderConfig := zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Determine output destination
	var writeSyncer zapcore.WriteSyncer
	switch cfg.Output {
	case "stdout":
		writeSyncer = zapcore.AddSync(os.Stdout)
	default:
		// Assume it's a file path
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", cfg.Output, err)
		}
		writeSyncer = zapcore.AddSync(file)
	}

	// Create core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// Create logger with caller information for debug level
	var options []zap.Option
	if level == zapcore.DebugLevel {
		options = append(options, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	}

	return zap.New(core, options...), nil
}
