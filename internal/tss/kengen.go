package tss

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/tss"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"

	"github.com/dreamer-zq/DKNet/internal/p2p"
)

// StartKeygen starts a new keygen operation
func (s *Service) StartKeygen(
	ctx context.Context,
	operationID string,
	threshold int,
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

	// Create TSS parameters
	peerCtx := tss.NewPeerContext(participantList)
	params := tss.NewParameters(tss.S256(), peerCtx, ourPartyID, len(participants), threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create keygen party
	party := keygen.NewLocalParty(params, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a longer timeout for keygen operations (10 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Create request for storage
	req := &KeygenRequest{
		OperationID:  operationID,
		Threshold:    threshold,
		Participants: participants,
	}

	operation := &Operation{
		ID:           operationID,
		Type:         OperationKeygen,
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
	go s.runKeygenOperation(operationCtx, operation, endCh)

	// Broadcast keygen operation sync message to other participants
	go s.broadcastKeygenOperation(operationID, sessionID, threshold, participants)

	return operation, nil
}

func (s *Service) broadcastKeygenOperation(
	operationID, sessionID string,
	threshold int,
	participants []string,
) {
	s.logger.Info("Broadcast keygen operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.Int("threshold", threshold),
		zap.Int("parties", len(participants)),
	)

	syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	syncData := &KeygenSyncData{
		OperationSyncData: OperationSyncData{
			OperationID:   operationID,
			OperationType: "keygen",
			SessionID:     sessionID,
			Threshold:     threshold,
			Parties:       len(participants),
			Participants:  participants,
		},
	}

	if err := s.broadcastOperationSync(syncCtx, syncData); err != nil {
		s.logger.Error("Failed to broadcast keygen operation sync",
			zap.Error(err),
			zap.String("operation_id", operationID))
	} else {
		s.logger.Info("Keygen operation sync broadcasted successfully",
			zap.String("operation_id", operationID))
	}
}

// runKeygenOperation runs a keygen operation
func (s *Service) runKeygenOperation(ctx context.Context, operation *Operation, endCh <-chan *keygen.LocalPartySaveData) {
	s.logger.Info("Starting keygen operation goroutine", zap.String("operation_id", operation.ID))

	// Update status
	operation.Lock()
	operation.Status = StatusInProgress
	operation.Unlock()

	// Start the party
	go func() {
		s.logger.Info("Starting TSS party", zap.String("operation_id", operation.ID))
		if err := operation.Party.Start(); err != nil {
			s.logger.Error("Keygen party failed", zap.Error(err), zap.String("operation_id", operation.ID))
			// Set failure status
			operation.Lock()
			operation.Status = StatusFailed
			operation.Error = err
			now := time.Now()
			operation.CompletedAt = &now
			operation.Unlock()
			return
		}
		s.logger.Info("TSS party started successfully", zap.String("operation_id", operation.ID))
	}()

	// Handle outgoing messages
	go s.handleOutgoingMessages(ctx, operation)

	s.logger.Info("Waiting for keygen completion or cancellation", zap.String("operation_id", operation.ID))

	// Wait for completion or cancellation
	// Ensure operation is always cleaned up regardless of outcome
	defer func() {
		// Always move completed operation to persistent storage for cleanup
		if err := s.moveCompletedOperationToStorage(context.Background(), operation.ID); err != nil {
			s.logger.Error("Failed to move keygen operation to persistent storage during cleanup",
				zap.Error(err),
				zap.String("operation_id", operation.ID))
		}
	}()

	select {
	case result := <-endCh:
		s.logger.Info("Keygen completed successfully", zap.String("operation_id", operation.ID))
		// Save result
		if err := s.saveKeygenResult(ctx, operation, result); err != nil {
			s.logger.Error("Failed to save keygen result", zap.Error(err))
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
		s.logger.Info("Keygen operation canceled or timed out",
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

// saveKeygenResult saves keygen result with encryption
func (s *Service) saveKeygenResult(ctx context.Context, operation *Operation, result *keygen.LocalPartySaveData) error {
	// Generate public key bytes and Ethereum address in one go
	xBytes := result.ECDSAPub.X().Bytes()
	yBytes := result.ECDSAPub.Y().Bytes()
	xBytes = append(xBytes, yBytes...)
	pubKeyBytes := xBytes

	// Generate Ethereum address using Keccak-256
	hasher := sha3.NewLegacyKeccak256()
	_, err := hasher.Write(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to write public key bytes: %w", err)
	}
	hash := hasher.Sum(nil)
	keyID := "0x" + hex.EncodeToString(hash[12:]) // Take last 20 bytes for address

	// Prepare all data for storage and result
	publicKeyHex := hex.EncodeToString(pubKeyBytes)

	// Serialize key data (this contains the private key shares)
	keyDataBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal key data: %w", err)
	}

	// Encrypt the sensitive key data
	encryptedKeyData, err := s.encryption.Encrypt(keyDataBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt key data: %w", err)
	}

	// Get original threshold from operation request
	originalReq := operation.Request.(*KeygenRequest)

	// Store key data with encrypted KeyData field
	keyDataStruct := &keyData{
		Moniker:      s.moniker,
		KeyData:      encryptedKeyData,      // Store encrypted data
		Threshold:    originalReq.Threshold, // Store the original threshold from request
		Participants: originalReq.Participants,
	}

	keyDataStorageBytes, err := json.Marshal(keyDataStruct)
	if err != nil {
		return fmt.Errorf("failed to marshal key data struct: %w", err)
	}

	if err := s.storage.Save(ctx, keyID, keyDataStorageBytes); err != nil {
		return fmt.Errorf("failed to save key data: %w", err)
	}

	// Create and store result
	operation.Lock()
	operation.Result = &KeygenResult{
		PublicKey: publicKeyHex,
		KeyID:     keyID,
	}
	operation.Unlock()

	s.logger.Info("Saved encrypted keygen result",
		zap.String("key_id", keyID),
		zap.Int("encrypted_size", len(encryptedKeyData)),
		zap.Int("original_size", len(keyDataBytes)))

	return nil
}

// createSyncedKeygenOperation creates a keygen operation from a sync message
func (s *Service) createSyncedKeygenOperation(ctx context.Context, msg *p2p.Message) error {
	// Parse operation sync data from message data
	var syncData KeygenSyncData
	if err := json.Unmarshal(msg.Data, &syncData); err != nil {
		s.logger.Error("Failed to unmarshal keygen sync data", zap.Error(err))
		return fmt.Errorf("failed to unmarshal keygen sync data: %w", err)
	}

	s.logger.Info("Creating synced keygen operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID),
		zap.Int("threshold", syncData.Threshold),
		zap.Int("parties", syncData.Parties),
		zap.Strings("participants", syncData.Participants))

	// Create participant list
	participants, err := s.createParticipantList(syncData.Participants)
	if err != nil {
		s.logger.Error("Failed to create participant list for synced operation", zap.Error(err))
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
		s.logger.Error("This node not found in participant list for synced operation", zap.String("node_id", s.nodeID))
		return fmt.Errorf("this node (%s) is not in the participant list", s.nodeID)
	}

	s.logger.Info("Found our party ID for synced operation", zap.String("party_id", ourPartyID.Id))

	// Pre-compute parameters for faster keygen
	preParams, err := keygen.GeneratePreParams(1 * time.Minute)
	if err != nil {
		s.logger.Error("Failed to generate pre-params for synced operation", zap.Error(err))
		return fmt.Errorf("failed to generate pre-params: %w", err)
	}

	// Create TSS parameters
	peerCtx := tss.NewPeerContext(participants)
	params := tss.NewParameters(tss.S256(), peerCtx, ourPartyID, len(participants), syncData.Threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create keygen party
	party := keygen.NewLocalParty(params, outCh, endCh, *preParams)

	// Create operation context with cancellation
	operationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Reconstruct request from sync data
	request := &KeygenRequest{
		Threshold:    syncData.Threshold,
		Participants: syncData.Participants,
	}

	operation := &Operation{
		ID:           syncData.OperationID, // Use the operation ID from sync message
		Type:         OperationKeygen,
		SessionID:    syncData.SessionID, // Use the session ID from sync message
		Participants: participants,
		Party:        party,
		OutCh:        outCh,
		EndCh:        make(chan interface{}, 1),
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Request:      request, // Store the reconstructed request
		cancel:       cancel,
	}

	// Store operation
	s.mutex.Lock()
	s.operations[syncData.OperationID] = operation
	s.mutex.Unlock()

	s.logger.Info("Synced operation stored, starting keygen goroutine",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID))

	// Start operation in a goroutine
	go s.runKeygenOperation(operationCtx, operation, endCh)

	s.logger.Info("Started synced keygen operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID))

	return nil
}
