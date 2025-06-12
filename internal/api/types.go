package api

import (
	"github.com/dreamer-zq/DKNet/internal/tss"
	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// Config holds API server configuration
type Config struct {
	HTTP     HTTPConfig
	GRPC     GRPCConfig
	Security SecurityConfig
}

// HTTPConfig holds HTTP server configuration
type HTTPConfig struct {
	Host string
	Port int
}

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Host string
	Port int
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	TLSEnabled bool
	CertFile   string
	KeyFile    string
}

// Helper functions to convert between internal types and proto types
func convertOperationStatus(status tss.OperationStatus) tssv1.OperationStatus {
	switch status {
	case tss.StatusPending:
		return tssv1.OperationStatus_OPERATION_STATUS_PENDING
	case tss.StatusInProgress:
		return tssv1.OperationStatus_OPERATION_STATUS_IN_PROGRESS
	case tss.StatusCompleted:
		return tssv1.OperationStatus_OPERATION_STATUS_COMPLETED
	case tss.StatusFailed:
		return tssv1.OperationStatus_OPERATION_STATUS_FAILED
	case tss.StatusCancelled:
		return tssv1.OperationStatus_OPERATION_STATUS_CANCELLED
	default:
		return tssv1.OperationStatus_OPERATION_STATUS_UNSPECIFIED
	}
}

func convertOperationType(opType tss.OperationType) tssv1.OperationType {
	switch opType {
	case tss.OperationKeygen:
		return tssv1.OperationType_OPERATION_TYPE_KEYGEN
	case tss.OperationSigning:
		return tssv1.OperationType_OPERATION_TYPE_SIGNING
	case tss.OperationResharing:
		return tssv1.OperationType_OPERATION_TYPE_RESHARING
	default:
		return tssv1.OperationType_OPERATION_TYPE_UNSPECIFIED
	}
}
