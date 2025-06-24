package tss

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/p2p"
)

// resharingOperationParams contains parameters for creating a resharing operation
type resharingOperationParams struct {
	OperationID     string
	SessionID       string
	KeyID           string
	NewThreshold    int
	OldParticipants []string
	NewParticipants []string
}

// StartResharing starts a new resharing operation
func (s *Service) StartResharing(
	ctx context.Context,
	operationID,
	keyID string,
	newThreshold int,
	oldParticipants,
	newParticipants []string,
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

	// Create the resharing operation using common logic
	operation, err := s.createResharingOperation(ctx, &resharingOperationParams{
		OperationID:     operationID,
		SessionID:       sessionID,
		KeyID:           keyID,
		NewThreshold:    newThreshold,
		OldParticipants: oldParticipants,
		NewParticipants: newParticipants,
	})
	if err != nil {
		return nil, err
	}

	// Broadcast resharing operation sync message to other participants
	go s.broadcastResharingOperation(operationID, sessionID, keyID, newThreshold, oldParticipants, newParticipants)

	s.logger.Info("Started resharing operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.String("key_id", keyID))

	return operation, nil
}

func (s *Service) broadcastResharingOperation(
	operationID, sessionID string,
	keyID string,
	newThreshold int,
	oldParticipants, newParticipants []string,
) {
	s.logger.Info("Broadcast resharing operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.String("key_id", keyID),
		zap.Int("new_threshold", newThreshold),
		zap.Int("old_parties", len(oldParticipants)),
		zap.Int("new_parties", len(newParticipants)),
	)

	syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load key metadata to get the old threshold
	keyData, err := s.LoadKeyMetadata(syncCtx, keyID)
	if err != nil {
		s.logger.Error("Failed to load key metadata for resharing sync",
			zap.Error(err),
			zap.String("key_id", keyID))
		return
	}

	syncData := &ResharingSyncData{
		OperationSyncData: OperationSyncData{
			OperationID:   operationID,
			OperationType: "resharing",
			SessionID:     sessionID,
			Threshold:     newThreshold,
			Parties:       len(newParticipants),
			Participants:  newParticipants,
		},
		OldThreshold:    keyData.Threshold,
		NewThreshold:    newThreshold,
		OldParticipants: oldParticipants,
		NewParticipants: newParticipants,
		KeyID:           keyID,
	}

	if err := s.broadcastOperationSync(syncCtx, syncData); err != nil {
		s.logger.Error("Failed to broadcast resharing operation sync",
			zap.Error(err),
			zap.String("operation_id", operationID))
	} else {
		s.logger.Info("Resharing operation sync broadcasted successfully",
			zap.String("operation_id", operationID))
	}
}

// createResharingOperation creates a resharing operation with common logic
func (s *Service) createResharingOperation(ctx context.Context, params *resharingOperationParams) (*Operation, error) {
	// Load key data
	_, localParty, err := s.loadKeyData(ctx, params.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	// Create participant lists
	oldParticipantList, err := s.createParticipantList(params.OldParticipants)
	if err != nil {
		return nil, fmt.Errorf("failed to create old participant list: %w", err)
	}

	newParticipantList, err := s.createParticipantList(params.NewParticipants)
	if err != nil {
		return nil, fmt.Errorf("failed to create new participant list: %w", err)
	}

	// Find our party ID in the participant lists
	var ourPartyID *tss.PartyID
	// Check if we're in the old participants (for existing shareholder)
	for _, p := range oldParticipantList {
		if p.Id == s.nodeID {
			ourPartyID = p
			break
		}
	}
	// If not found in old, check new participants (for new shareholder)
	if ourPartyID == nil {
		for _, p := range newParticipantList {
			if p.Id == s.nodeID {
				ourPartyID = p
				break
			}
		}
	}
	if ourPartyID == nil {
		return nil, fmt.Errorf("this node (%s) is not in either old or new participant lists", s.nodeID)
	}

	// Load key metadata to get the old threshold
	keyData, err := s.LoadKeyMetadata(context.Background(), params.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key metadata for resharing: %w", err)
	}

	// Create TSS parameters for resharing
	oldCtx := tss.NewPeerContext(oldParticipantList)
	newCtx := tss.NewPeerContext(newParticipantList)
	// Parameters based on test cases: (curve, oldCtx, newCtx, partyID, oldParticipants, oldThreshold, newParticipants, newThreshold)
	// The 5th parameter should be the count of OLD participants (not new)
	tssParams := tss.NewReSharingParameters(tss.S256(), oldCtx, newCtx, ourPartyID,
		len(oldParticipantList), keyData.Threshold, len(newParticipantList), params.NewThreshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create resharing party
	party := resharing.NewLocalParty(tssParams, *localParty, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a longer timeout for resharing operations (15 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	// Create request for storage
	req := &ResharingRequest{
		OperationID:     params.OperationID,
		KeyID:           params.KeyID,
		NewThreshold:    params.NewThreshold,
		NewParties:      len(params.NewParticipants),
		OldParticipants: params.OldParticipants,
		NewParticipants: params.NewParticipants,
	}

	operation := &Operation{
		ID:           params.OperationID,
		Type:         OperationResharing,
		SessionID:    params.SessionID,
		Participants: newParticipantList, // Use new participants for message handling
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
	s.operations[params.OperationID] = operation
	s.mutex.Unlock()

	// Start operation in a goroutine
	go s.runResharingOperation(operationCtx, operation, endCh)

	return operation, nil
}

// runResharingOperation runs a resharing operation
func (s *Service) runResharingOperation(ctx context.Context, operation *Operation, endCh <-chan *keygen.LocalPartySaveData) {
	// Update status
	operation.Lock()
	operation.Status = StatusInProgress
	operation.Unlock()

	// Start the party
	go func() {
		if err := operation.Party.Start(); err != nil {
			s.logger.Error("Resharing party failed", zap.Error(err))
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
			s.logger.Error("Failed to move resharing operation to persistent storage during cleanup",
				zap.Error(err),
				zap.String("operation_id", operation.ID))
		}
	}()

	select {
	case result := <-endCh:
		// Save result
		if err := s.saveResharingResult(ctx, operation, result); err != nil {
			s.logger.Error("Failed to save resharing result", zap.Error(err))
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
		s.logger.Info("Resharing operation canceled or timed out",
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

// saveResharingResult saves resharing result
func (s *Service) saveResharingResult(ctx context.Context, operation *Operation, result *keygen.LocalPartySaveData) error {
	// This would update the existing key with new shares
	// For simplicity, we'll just store as a new key
	return s.saveKeygenResult(ctx, operation, result)
}

// createSyncedResharingOperation creates a resharing operation from a sync message
func (s *Service) createSyncedResharingOperation(ctx context.Context, msg *p2p.Message) error {
	// Parse operation sync data from message data
	var syncData ResharingSyncData
	if err := json.Unmarshal(msg.Data, &syncData); err != nil {
		s.logger.Error("Failed to unmarshal resharing sync data", zap.Error(err))
		return fmt.Errorf("failed to unmarshal resharing sync data: %w", err)
	}

	s.logger.Info("Creating synced resharing operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID),
		zap.String("key_id", syncData.KeyID),
		zap.Int("old_threshold", syncData.OldThreshold),
		zap.Int("new_threshold", syncData.NewThreshold),
		zap.Int("old_parties", len(syncData.OldParticipants)),
		zap.Int("new_parties", len(syncData.NewParticipants)),
		zap.Strings("old_participants", syncData.OldParticipants),
		zap.Strings("new_participants", syncData.NewParticipants))

	// Create the resharing operation using common logic
	_, err := s.createResharingOperation(ctx, &resharingOperationParams{
		OperationID:     syncData.OperationID,
		SessionID:       syncData.SessionID,
		KeyID:           syncData.KeyID,
		NewThreshold:    syncData.NewThreshold,
		OldParticipants: syncData.OldParticipants,
		NewParticipants: syncData.NewParticipants,
	})
	if err != nil {
		return fmt.Errorf("failed to create synced resharing operation: %w", err)
	}

	s.logger.Info("Started synced resharing operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID))

	return nil
}
