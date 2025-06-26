package tss

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/google/uuid"
	"go.uber.org/zap"

	dknetCommon "github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/p2p"
)

// signingOperationParams contains parameters for creating a signing operation
type signingOperationParams struct {
	OperationID  string
	SessionID    string
	Message      []byte
	KeyID        string
	Participants []string
}

// StartSigning starts a new signing operation
func (s *Service) StartSigning(
	ctx context.Context,
	operationID string,
	message []byte,
	keyID string,
	participants []string,
) (*Operation, error) {
	// Check for existing operation (idempotency)
	existingOp, err := s.checkIdempotency(ctx, operationID)
	if err != nil {
		return nil, err
	}

	if existingOp != nil {
		return existingOp, nil
	}

	// Create request for validation
	req := &SigningRequest{
		OperationID:  operationID,
		Message:      message,
		KeyID:        keyID,
		Participants: participants,
	}

	// Validate signing request with external validation service (if configured)
	if err = s.validateSigningRequest(ctx, req); err != nil {
		s.logger.Error("Signing request validation failed",
			zap.Error(err),
			zap.String("key_id", keyID))
		return nil, fmt.Errorf("signing request validation failed: %w", err)
	}

	// Generate or use provided operation ID
	operationID = s.generateOrUseOperationID(operationID)
	sessionID := uuid.New().String()

	// Create the signing operation using common logic
	operation, threshold, err := s.createSigningOperation(ctx, &signingOperationParams{
		OperationID:  operationID,
		SessionID:    sessionID,
		Message:      message,
		KeyID:        keyID,
		Participants: participants,
	})
	if err != nil {
		return nil, err
	}

	// Broadcast signing operation sync message to other participants
	dknetCommon.SafeGo(operation.EndCh, func() any {
		return s.broadcastSigningOperation(
			operationID, sessionID,
			threshold, len(operation.Participants),
			participants, keyID, message,
		)
	})

	return operation, nil
}

// createSigningOperation creates a signing operation with shared logic
func (s *Service) createSigningOperation(ctx context.Context, params *signingOperationParams) (*Operation, int, error) {
	// Load key data and metadata
	keyData, localParty, err := s.loadKeyData(ctx, params.KeyID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to load key data: %w", err)
	}

	// Create participant list
	participantList, err := s.createParticipantList(params.Participants)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create participant list: %w", err)
	}

	// Find our party ID in the participants list
	ourPartyIndex := slices.IndexFunc(participantList, func(p *tss.PartyID) bool {
		return p.Id == s.nodeID
	})
	if ourPartyIndex == -1 {
		return nil, 0, fmt.Errorf("this node (%s) is not in the participant list", s.nodeID)
	}

	ourPartyID := participantList[ourPartyIndex]
	// Create TSS parameters - use the original threshold from keygen
	ctx2 := tss.NewPeerContext(participantList)
	threshold := keyData.Threshold // Use the original threshold from stored metadata
	tssParams := tss.NewParameters(tss.S256(), ctx2, ourPartyID, len(participantList), threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *common.SignatureData, 1)

	// Hash the message to sign - use Ethereum-compatible hash for ecrecover verification
	hash := hashMessageForEthereum(params.Message)

	// Create signing party
	party := signing.NewLocalParty(new(big.Int).SetBytes(hash), tssParams, *localParty, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a shorter timeout for signing operations (5 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Create request for storage
	req := &SigningRequest{
		OperationID:  params.OperationID,
		Message:      params.Message,
		KeyID:        params.KeyID,
		Participants: params.Participants,
	}

	operation := &Operation{
		ID:           params.OperationID,
		Type:         OperationSigning,
		SessionID:    params.SessionID,
		Participants: participantList,
		Party:        party,
		OutCh:        outCh,
		EndCh:        dknetCommon.ConvertToAnyCh(endCh), // Generic channel for interface{}
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Request:      req, // Store the request for persistence
		cancel:       cancel,
	}

	// Store operation
	s.mutex.Lock()
	s.operations[params.OperationID] = operation
	s.mutex.Unlock()

	// Wait for operation completion or cancellation
	go s.watchOperation(operationCtx, operation)
	// Start operation in a goroutine
	go s.runOperation(operationCtx, operation)

	return operation, threshold, nil
}

func (s *Service) broadcastSigningOperation(
	operationID, sessionID string,
	threshold, parties int,
	participants []string,
	keyID string,
	message []byte,
) error {
	syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncData := &SigningSyncData{
		OperationSyncData: OperationSyncData{
			OperationID:   operationID,
			OperationType: "signing",
			SessionID:     sessionID,
			Threshold:     threshold,
			Parties:       parties,
			Participants:  participants,
		},
		KeyID:   keyID,
		Message: message,
	}

	if err := s.broadcastOperationSync(syncCtx, syncData); err != nil {
		s.logger.Error("Failed to broadcast signing operation sync",
			zap.Error(err),
			zap.String("operation_id", operationID))
		return err
	}
	s.logger.Info("Signing operation sync broadcasted successfully",
		zap.String("operation_id", operationID),
		zap.String("key_id", keyID))
	return nil
}

func (s *Service) createSyncedSigningOperation(ctx context.Context, msg *p2p.Message) error {
	// Parse operation sync data from message data
	var syncData SigningSyncData
	if err := json.Unmarshal(msg.Data, &syncData); err != nil {
		s.logger.Error("Failed to unmarshal signing sync data", zap.Error(err))
		return fmt.Errorf("failed to unmarshal signing sync data: %w", err)
	}

	s.logger.Info("Creating synced signing operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID),
		zap.String("from", msg.From),
		zap.String("key_id", syncData.KeyID),
		zap.Int("message_len", len(syncData.Message)),
		zap.Strings("participants", syncData.Participants))

	// Validate that we have the required signing-specific information
	if syncData.KeyID == "" {
		return fmt.Errorf("key_id is required for signing operation sync")
	}
	if len(syncData.Message) == 0 {
		return fmt.Errorf("message is required for signing operation sync")
	}

	// Create SigningRequest for validation
	signingReq := &SigningRequest{
		Message:      syncData.Message,
		KeyID:        syncData.KeyID,
		Participants: syncData.Participants,
	}

	// Validate signing request with external validation service (if configured)
	if err := s.validateSigningRequest(ctx, signingReq); err != nil {
		s.logger.Error("Synced signing request validation failed",
			zap.Error(err),
			zap.String("key_id", syncData.KeyID),
			zap.String("operation_id", syncData.OperationID))
		return fmt.Errorf("synced signing request validation failed: %w", err)
	}

	// Create the signing operation using common logic
	_, _, err := s.createSigningOperation(ctx, &signingOperationParams{
		OperationID:  syncData.OperationID,
		SessionID:    syncData.SessionID,
		Message:      syncData.Message,
		KeyID:        syncData.KeyID,
		Participants: syncData.Participants,
	})
	if err != nil {
		s.logger.Error("Failed to create synced signing operation", zap.Error(err))
		return fmt.Errorf("failed to create synced signing operation: %w", err)
	}

	s.logger.Info("Synced signing operation created successfully",
		zap.String("operation_id", syncData.OperationID),
		zap.String("key_id", syncData.KeyID))

	return nil
}

// saveSigningResult saves signing result with Ethereum-compatible format
func (s *Service) saveSigningResult(_ context.Context, operation *Operation, result *common.SignatureData) error {
	// Ensure R and S are exactly 32 bytes each
	rBytes := result.R
	sBytes := result.S

	// Pad with leading zeros if necessary
	if len(rBytes) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(rBytes):], rBytes)
		rBytes = padded
	}
	if len(sBytes) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(sBytes):], sBytes)
		sBytes = padded
	}

	// Truncate if longer than 32 bytes (should not happen, but safety check)
	if len(rBytes) > 32 {
		rBytes = rBytes[len(rBytes)-32:]
	}
	if len(sBytes) > 32 {
		sBytes = sBytes[len(sBytes)-32:]
	}

	// Calculate Ethereum-compatible v value from recovery ID
	// Ethereum format: v = recovery_id + 27
	v := 27
	if len(result.SignatureRecovery) > 0 {
		recoveryID := int(result.SignatureRecovery[0])
		// Ensure recovery ID is in valid range (0 or 1)
		if recoveryID >= 0 && recoveryID <= 1 {
			v = recoveryID + 27
		} else {
			s.logger.Warn("Invalid recovery ID, using default v=27",
				zap.Int("recovery_id", recoveryID))
		}
	}

	// Create 65-byte Ethereum signature: R(32) + S(32) + V(1)
	signature65 := make([]byte, 65)
	copy(signature65[0:32], rBytes)  // R component
	copy(signature65[32:64], sBytes) // S component
	signature65[64] = byte(v)        // V component

	// Create signing result with both individual components and complete signature
	signingResult := &SigningResult{
		Signature: "0x" + hex.EncodeToString(signature65), // 65-byte signature for contract verification
		R:         "0x" + hex.EncodeToString(rBytes),      // R component (32 bytes)
		S:         "0x" + hex.EncodeToString(sBytes),      // S component (32 bytes)
		V:         v,                                      // V value (recovery_id + 27)
	}

	operation.Lock()
	operation.Result = signingResult
	operation.Unlock()

	s.logger.Info("Saved signing result (Ethereum-compatible 65-byte format)",
		zap.String("signature_65_bytes", signingResult.Signature),
		zap.String("r", signingResult.R),
		zap.String("s", signingResult.S),
		zap.Int("v", signingResult.V),
		zap.Int("signature_length", len(signature65)))

	return nil
}
