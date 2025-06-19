package tss

import (
	"context"
	"fmt"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

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

	// Load key data
	_, localParty, err := s.loadKeyData(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	// Generate or use provided operation ID
	operationID = s.generateOrUseOperationID(operationID)
	sessionID := uuid.New().String()

	// Create participant lists
	oldParticipantList, err := s.createParticipantList(oldParticipants)
	if err != nil {
		return nil, fmt.Errorf("failed to create old participant list: %w", err)
	}

	newParticipantList, err := s.createParticipantList(newParticipants)
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

	// Create TSS parameters for resharing
	oldCtx := tss.NewPeerContext(oldParticipantList)
	newCtx := tss.NewPeerContext(newParticipantList)
	params := tss.NewReSharingParameters(tss.S256(), oldCtx, newCtx, ourPartyID,
		len(oldParticipantList), newThreshold, len(newParticipantList), newThreshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create resharing party
	party := resharing.NewLocalParty(params, *localParty, outCh, endCh)

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a longer timeout for resharing operations (15 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	// Create request for storage
	req := &ResharingRequest{
		OperationID:     operationID,
		KeyID:           keyID,
		NewThreshold:    newThreshold,
		NewParties:      len(newParticipants),
		OldParticipants: oldParticipants,
		NewParticipants: newParticipants,
	}

	operation := &Operation{
		ID:           operationID,
		Type:         OperationResharing,
		SessionID:    sessionID,
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
	s.operations[operationID] = operation
	s.mutex.Unlock()

	// Start operation in a goroutine
	go s.runResharingOperation(operationCtx, operation, endCh)

	s.logger.Info("Started resharing operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.String("key_id", keyID))

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
			s.handleOperationFailure(ctx, operation, err)
		}
	}()

	// Handle outgoing messages
	go s.handleOutgoingMessages(ctx, operation)

	// Wait for completion or cancellation
	select {
	case result := <-endCh:
		// Save result
		if err := s.saveResharingResult(ctx, operation, result); err != nil {
			s.logger.Error("Failed to save resharing result", zap.Error(err))
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
					s.logger.Error("Failed to move completed resharing operation to persistent storage",
						zap.Error(err),
						zap.String("operation_id", operation.ID))
				}
			}()
		}
	case <-ctx.Done():
		s.logger.Info("Resharing operation canceled", zap.String("operation_id", operation.ID))
	}
}

// saveResharingResult saves resharing result
func (s *Service) saveResharingResult(ctx context.Context, operation *Operation, result *keygen.LocalPartySaveData) error {
	// This would update the existing key with new shares
	// For simplicity, we'll just store as a new key
	return s.saveKeygenResult(ctx, operation, result)
}
