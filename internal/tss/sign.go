package tss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// StartSigning starts a new signing operation
func (s *Service) StartSigning(ctx context.Context, req *SigningRequest) (*Operation, error) {
	// Load key data
	keyData, err := s.loadKeyData(ctx, req.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	// Create operation
	operationID := uuid.New().String()
	sessionID := uuid.New().String()

	// Create participant list
	participants, err := s.createParticipantList(req.Participants)
	if err != nil {
		return nil, fmt.Errorf("failed to create participant list: %w", err)
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
		return nil, fmt.Errorf("this node (%s) is not in the participant list", s.nodeID)
	}

	// Create TSS parameters - use the original threshold from keygen
	ctx2 := tss.NewPeerContext(participants)
	threshold := len(keyData.Ks) - 1 // Get original threshold from stored key data
	params := tss.NewParameters(tss.S256(), ctx2, ourPartyID, len(participants), threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *common.SignatureData, 1)

	// Hash the message to sign
	hash := sha256.Sum256(req.Message)

	// Create signing party
	party := signing.NewLocalParty(new(big.Int).SetBytes(hash[:]), params, *keyData, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a shorter timeout for signing operations (5 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	operation := &Operation{
		ID:           operationID,
		Type:         OperationSigning,
		SessionID:    sessionID,
		Participants: participants,
		Party:        party,
		OutCh:        outCh,
		EndCh:        make(chan interface{}, 1), // Generic channel for interface{}
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Request:      req, // Store the original request
		cancel:       cancel,
	}

	// Store operation
	s.mutex.Lock()
	s.operations[operationID] = operation
	s.mutex.Unlock()

	// Start operation in a goroutine
	go s.runSigningOperation(operationCtx, operation, endCh)

	// Broadcast signing operation sync message to other participants
	go s.broadcastSigningOperation(operationID, sessionID,
			threshold, len(participants), req.Participants, req.KeyID, req.Message)
	// Broadcast own mapping
	s.broadcastOwnMapping(context.Background(), sessionID)

	return operation, nil
}

func (s *Service) broadcastSigningOperation(operationID, sessionID string, threshold, parties int, participants []string, keyID string, message []byte) {
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

	// Load key data
	keyData, err := s.loadKeyData(ctx, syncData.KeyID)
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
	threshold := len(keyData.Ks) - 1 // Get original threshold from stored key data
	params := tss.NewParameters(tss.S256(), ctx2, ourPartyID, len(participants), threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *common.SignatureData, 1)

	// Hash the message to sign
	hash := sha256.Sum256(syncData.Message)

	// Create signing party
	party := signing.NewLocalParty(new(big.Int).SetBytes(hash[:]), params, *keyData, outCh, endCh)

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
			s.handleOperationFailure(ctx, operation, err)
		}
	}()

	// Handle outgoing messages
	go s.handleOutgoingMessages(ctx, operation)

	// Wait for completion or cancellation
	select {
	case result := <-endCh:
		// Save result
		if err := s.saveSigningResult(ctx, operation, result); err != nil {
			s.logger.Error("Failed to save signing result", zap.Error(err))
			s.handleOperationFailure(ctx, operation, err)
		} else {
			// Send to generic channel
			operation.EndCh <- result
			operation.Lock()
			operation.Status = StatusCompleted
			now := time.Now()
			operation.CompletedAt = &now
			operation.Unlock()
			
			// Move completed operation to persistent storage
			go func() {
				if err := s.moveCompletedOperationToStorage(ctx, operation.ID); err != nil {
					s.logger.Error("Failed to move completed signing operation to persistent storage",
						zap.Error(err),
						zap.String("operation_id", operation.ID))
				}
			}()
		}
	case <-ctx.Done():
		s.logger.Info("Signing operation cancelled", zap.String("operation_id", operation.ID))
	}
}

// saveSigningResult saves signing result
func (s *Service) saveSigningResult(_ context.Context, operation *Operation, result *common.SignatureData) error {
	// Create signing result
	signingResult := &SigningResult{
		Signature: hex.EncodeToString(result.Signature),
		R:         hex.EncodeToString(result.R),
		S:         hex.EncodeToString(result.S),
	}

	operation.Lock()
	operation.Result = signingResult
	operation.Unlock()

	s.logger.Info("Saved signing result")

	return nil
}