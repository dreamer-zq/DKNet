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
func (s *Service) StartKeygen(ctx context.Context, req *KeygenRequest) (*Operation, error) {
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

	// Create TSS parameters
	peerCtx := tss.NewPeerContext(participants)
	params := tss.NewParameters(tss.S256(), peerCtx, ourPartyID, req.Parties, req.Threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create keygen party
	party := keygen.NewLocalParty(params, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a longer timeout for keygen operations (10 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	operation := &Operation{
		ID:           operationID,
		Type:         OperationKeygen,
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
	go s.runKeygenOperation(operationCtx, operation, endCh)

	// Broadcast keygen operation sync message to other participants
	go s.broadcastKeygenOperation(operationID, sessionID, req.Threshold, req.Parties, req.Participants)

	// Broadcast own mapping
	s.broadcastOwnMapping(context.Background(), sessionID)

	return operation, nil
}

func (s *Service) broadcastKeygenOperation(
	operationID, sessionID string,
	threshold, parties int,
	participants []string,
) {
	s.logger.Info("Broadcast keygen operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.Int("threshold", threshold),
		zap.Int("parties", parties),
	)

	syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	syncData := &KeygenSyncData{
		OperationSyncData: OperationSyncData{
			OperationID:   operationID,
			OperationType: "keygen",
			SessionID:     sessionID,
			Threshold:     threshold,
			Parties:       parties,
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
			s.handleOperationFailure(ctx, operation, err)
			return
		}
		s.logger.Info("TSS party started successfully", zap.String("operation_id", operation.ID))
	}()

	// Handle outgoing messages
	go s.handleOutgoingMessages(ctx, operation)

	s.logger.Info("Waiting for keygen completion or cancellation", zap.String("operation_id", operation.ID))

	// Wait for completion or cancellation
	select {
	case result := <-endCh:
		s.logger.Info("Keygen completed successfully", zap.String("operation_id", operation.ID))
		// Save result
		if err := s.saveKeygenResult(ctx, operation, result); err != nil {
			s.logger.Error("Failed to save keygen result", zap.Error(err))
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
					s.logger.Error("Failed to move completed keygen operation to persistent storage",
						zap.Error(err),
						zap.String("operation_id", operation.ID))
				}
			}()
		}
	case <-ctx.Done():
		s.logger.Info("Keygen operation cancelled",
			zap.String("operation_id", operation.ID),
			zap.Error(ctx.Err()))
	}
}

// saveKeygenResult saves keygen result
func (s *Service) saveKeygenResult(ctx context.Context, operation *Operation, result *keygen.LocalPartySaveData) error {
	// Generate public key bytes and Ethereum address in one go
	xBytes := result.ECDSAPub.X().Bytes()
	yBytes := result.ECDSAPub.Y().Bytes()
	pubKeyBytes := append(xBytes, yBytes...)

	// Generate Ethereum address using Keccak-256
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(pubKeyBytes)
	hash := hasher.Sum(nil)
	keyID := "0x" + hex.EncodeToString(hash[12:]) // Take last 20 bytes for address

	// Prepare all data for storage and result
	publicKeyHex := hex.EncodeToString(pubKeyBytes)

	// Serialize key data
	keyDataBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal key data: %w", err)
	}

	// Get original threshold from operation request
	originalReq := operation.Request.(*KeygenRequest)
	
	// Store key data
	keyDataStruct := &KeyData{
		NodeID:    s.nodeID,
		Moniker:   s.moniker,
		KeyData:   keyDataBytes,
		Threshold: originalReq.Threshold, // Store the original threshold from request
		Parties:   len(result.Ks),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
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

	s.logger.Info("Saved keygen result", zap.String("key_id", keyID))

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
	ctx2 := tss.NewPeerContext(participants)
	params := tss.NewParameters(tss.S256(), ctx2, ourPartyID, len(participants), syncData.Threshold)

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
		Parties:      syncData.Parties,
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

	// Broadcast own mapping to help other nodes know our P2P peer ID
	s.broadcastOwnMapping(ctx, syncData.SessionID)

	return nil
}
