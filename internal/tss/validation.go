package tss

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/plugin"
)

// validateSigningRequest validates a signing request using external validation service
func (s *Service) validateSigningRequest(ctx context.Context, req *SigningRequest) error {
	if s.validationService == nil {
		s.logger.Debug("Validation service not configured, skipping validation")
		return nil
	}

	// Prepare validation request
	validationReq := &plugin.ValidationRequest{
		Message:      req.Message,
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
