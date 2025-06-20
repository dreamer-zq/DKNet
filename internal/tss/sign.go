package tss

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/p2p"
)

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

	// Load key data and metadata
	keyData, localParty, err := s.loadKeyData(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	// Generate or use provided operation ID
	operationID = s.generateOrUseOperationID(operationID)
	sessionID := uuid.New().String()

	// Create participant list
	participantList, err := s.createParticipantList(participants)
	if err != nil {
		return nil, fmt.Errorf("failed to create participant list: %w", err)
	}

	// Find our party ID in the participants list
	var ourPartyID *tss.PartyID
	for _, p := range participantList {
		if p.Id == s.nodeID {
			ourPartyID = p
			break
		}
	}
	if ourPartyID == nil {
		return nil, fmt.Errorf("this node (%s) is not in the participant list", s.nodeID)
	}

	// Create TSS parameters - use the original threshold from keygen
	ctx2 := tss.NewPeerContext(participantList)
	threshold := keyData.Threshold // Use the original threshold from stored metadata
	params := tss.NewParameters(tss.S256(), ctx2, ourPartyID, len(participantList), threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *common.SignatureData, 1)

	// Hash the message to sign - use Ethereum-compatible hash for ecrecover verification
	hash := hashMessageForEthereum(message)

	// Create signing party
	party := signing.NewLocalParty(new(big.Int).SetBytes(hash), params, *localParty, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a shorter timeout for signing operations (5 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Update request with final operation ID
	req.OperationID = operationID

	operation := &Operation{
		ID:           operationID,
		Type:         OperationSigning,
		SessionID:    sessionID,
		Participants: participantList,
		Party:        party,
		OutCh:        outCh,
		EndCh:        make(chan interface{}, 1), // Generic channel for interface{}
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Request:      req, // Store the request for persistence
		cancel:       cancel,
	}

	// Store operation
	s.mutex.Lock()
	s.operations[operationID] = operation
	s.mutex.Unlock()

	// Start operation in a goroutine
	go s.runSigningOperation(operationCtx, operation, endCh)

	// Broadcast signing operation sync message to other participants
	go s.broadcastSigningOperation(
		operationID, sessionID,
		threshold, len(participantList), participants, keyID, message,
	)

	return operation, nil
}

func (s *Service) broadcastSigningOperation(
	operationID, sessionID string,
	threshold, parties int,
	participants []string,
	keyID string,
	message []byte,
) {
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
	} else {
		s.logger.Info("Signing operation sync broadcasted successfully",
			zap.String("operation_id", operationID),
			zap.String("key_id", keyID))
	}
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

	// Load key data and metadata
	keyData, localParty, err := s.loadKeyData(ctx, syncData.KeyID)
	if err != nil {
		return fmt.Errorf("failed to load key data for synced signing: %w", err)
	}
	// Create participant list
	participants, err := s.createParticipantList(syncData.Participants)
	if err != nil {
		return fmt.Errorf("failed to create participant list: %w", err)
	}

	// Find our party ID in the participants list
	var ourPartyID *tss.PartyID
	for _, p := range participants {
		if p.Id == s.nodeID {
			ourPartyID = p
			break
		}
	}
	if ourPartyID == nil {
		return fmt.Errorf("this node (%s) is not a participant in the signing operation", s.nodeID)
	}

	// Create TSS parameters - use the original threshold from keygen
	ctx2 := tss.NewPeerContext(participants)
	threshold := keyData.Threshold // Use the original threshold from stored metadata
	params := tss.NewParameters(tss.S256(), ctx2, ourPartyID, len(participants), threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *common.SignatureData, 1)

	// Hash the message to sign - use Ethereum-compatible hash for ecrecover verification
	hash := hashMessageForEthereum(syncData.Message)

	// Create signing party
	party := signing.NewLocalParty(new(big.Int).SetBytes(hash), params, *localParty, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	operationCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Reconstruct request from sync data
	request := &SigningRequest{
		Message:      syncData.Message,
		KeyID:        syncData.KeyID,
		Participants: syncData.Participants,
	}

	operation := &Operation{
		ID:           syncData.OperationID,
		Type:         OperationSigning,
		SessionID:    syncData.SessionID,
		Participants: participants,
		Party:        party,
		OutCh:        outCh,
		EndCh:        make(chan interface{}, 1), // Generic channel for interface{}
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Request:      request, // Store the reconstructed request
		cancel:       cancel,
	}

	// Store operation
	s.mutex.Lock()
	s.operations[operation.ID] = operation
	s.mutex.Unlock()

	// Start the signing operation in a goroutine
	go s.runSigningOperation(operationCtx, operation, endCh)

	s.logger.Info("Synced signing operation created successfully",
		zap.String("operation_id", syncData.OperationID),
		zap.String("key_id", syncData.KeyID))

	return nil
}

// runSigningOperation runs a signing operation
func (s *Service) runSigningOperation(ctx context.Context, operation *Operation, endCh <-chan *common.SignatureData) {
	// Update status
	operation.Lock()
	operation.Status = StatusInProgress
	operation.Unlock()

	// Start the party
	go func() {
		if err := operation.Party.Start(); err != nil {
			s.logger.Error("Signing party failed", zap.Error(err))
			// Set failure status
			operation.Lock()
			operation.Status = StatusFailed
			operation.Error = err
			now := time.Now()
			operation.CompletedAt = &now
			operation.Unlock()
		}
	}()

	// Handle outgoing messages
	go s.handleOutgoingMessages(ctx, operation)

	// Wait for completion or cancellation
	// Ensure operation is always cleaned up regardless of outcome
	defer func() {
		// Always move completed operation to persistent storage for cleanup
		if err := s.moveCompletedOperationToStorage(context.Background(), operation.ID); err != nil {
			s.logger.Error("Failed to move signing operation to persistent storage during cleanup",
				zap.Error(err),
				zap.String("operation_id", operation.ID))
		}
	}()

	select {
	case result := <-endCh:
		// Save result
		if err := s.saveSigningResult(ctx, operation, result); err != nil {
			s.logger.Error("Failed to save signing result", zap.Error(err))
			// Set failure status
			operation.Lock()
			operation.Status = StatusFailed
			operation.Error = err
			now := time.Now()
			operation.CompletedAt = &now
			operation.Unlock()
		} else {
			// Send to generic channel
			operation.EndCh <- result
			// Set success status
			operation.Lock()
			operation.Status = StatusCompleted
			now := time.Now()
			operation.CompletedAt = &now
			operation.Unlock()
		}
	case <-ctx.Done():
		s.logger.Info("Signing operation canceled or timed out",
			zap.String("operation_id", operation.ID),
			zap.Error(ctx.Err()))

		// Set canceled status
		operation.Lock()
		operation.Status = StatusCancelled
		now := time.Now()
		operation.CompletedAt = &now
		operation.Unlock()
	}
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
