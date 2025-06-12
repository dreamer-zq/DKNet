package tss

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dreamer-zq/DKNet/internal/config"
	"go.uber.org/zap"
)

// ValidationService defines the interface for external validation service
type ValidationService interface {
	// ValidateSigningRequest validates a signing request before processing
	ValidateSigningRequest(ctx context.Context, req *ValidationRequest) (*ValidationResponse, error)
}

// ValidationRequest represents the request sent to validation service
type ValidationRequest struct {
	// Message to be signed (hex encoded)
	Message string `json:"message"`
	// Key ID being used for signing
	KeyID string `json:"key_id"`
	// List of participant node IDs
	Participants []string `json:"participants"`
	// Node ID making the request
	NodeID string `json:"node_id"`
	// Request timestamp
	Timestamp int64 `json:"timestamp"`
	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ValidationResponse represents the response from validation service
type ValidationResponse struct {
	// Whether the request is approved for signing
	Approved bool `json:"approved"`
	// Reason for approval/rejection
	Reason string `json:"reason,omitempty"`
	// Additional response metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// HTTPValidationService implements ValidationService using HTTP API calls
type HTTPValidationService struct {
	config *config.ValidationServiceConfig
	client *http.Client
	logger *zap.Logger
	nodeID string
}

// NewHTTPValidationService creates a new HTTP validation service client
func NewHTTPValidationService(cfg *config.ValidationServiceConfig, nodeID string, logger *zap.Logger) *HTTPValidationService {
	// Create HTTP client with timeout and TLS configuration
	client := &http.Client{
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
	}

	// Configure TLS if needed
	if cfg.InsecureSkipVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	}

	return &HTTPValidationService{
		config: cfg,
		client: client,
		logger: logger,
		nodeID: nodeID,
	}
}

// ValidateSigningRequest validates a signing request with external service
func (v *HTTPValidationService) ValidateSigningRequest(ctx context.Context, req *ValidationRequest) (*ValidationResponse, error) {
	// Set node ID and timestamp
	req.NodeID = v.nodeID
	req.Timestamp = time.Now().Unix()

	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal validation request: %w", err)
	}

	v.logger.Info("Sending validation request",
		zap.String("url", v.config.URL),
		zap.String("key_id", req.KeyID),
		zap.String("message_hex", req.Message),
		zap.Strings("participants", req.Participants))

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", v.config.URL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "DKNet-TSS-Node/1.0")

	// Add custom headers from configuration
	for key, value := range v.config.Headers {
		httpReq.Header.Set(key, value)
	}

	// Send request
	resp, err := v.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send validation request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			v.logger.Warn("Failed to close response body", zap.Error(closeErr))
		}
	}()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read validation response: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		v.logger.Error("Validation service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(respBody)))
		return nil, fmt.Errorf("validation service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var validationResp ValidationResponse
	if err := json.Unmarshal(respBody, &validationResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal validation response: %w", err)
	}

	v.logger.Info("Received validation response",
		zap.Bool("approved", validationResp.Approved),
		zap.String("reason", validationResp.Reason))

	return &validationResp, nil
}

// validateSigningRequest validates a signing request using external validation service
func (s *Service) validateSigningRequest(ctx context.Context, req *SigningRequest) error {
	if s.validationService == nil {
		s.logger.Debug("Validation service not configured, skipping validation")
		return nil
	}

	// Prepare validation request
	validationReq := &ValidationRequest{
		Message:      hex.EncodeToString(req.Message),
		KeyID:        req.KeyID,
		Participants: req.Participants,
		Metadata: map[string]interface{}{
			"message_length": len(req.Message),
		},
	}

	// Call validation service
	validationResp, err := s.validationService.ValidateSigningRequest(ctx, validationReq)
	if err != nil {
		s.logger.Error("Validation service call failed",
			zap.Error(err),
			zap.String("key_id", req.KeyID))
		return fmt.Errorf("validation service call failed: %w", err)
	}

	// Check if request is approved
	if !validationResp.Approved {
		s.logger.Warn("Signing request rejected by validation service",
			zap.String("key_id", req.KeyID),
			zap.String("reason", validationResp.Reason))
		return fmt.Errorf("signing request rejected by validation service: %s", validationResp.Reason)
	}

	s.logger.Info("Signing request approved by validation service",
		zap.String("key_id", req.KeyID),
		zap.String("reason", validationResp.Reason))

	return nil
}
