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
	NewParticipants []string
}

// StartResharing starts a new resharing operation
func (s *Service) StartResharing(
	ctx context.Context,
	operationID,
	keyID string,
	newThreshold int,
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

	// Load key metadata to get old participants
	keyData, err := s.LoadKeyMetadata(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key metadata: %w", err)
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
		NewParticipants: newParticipants,
	})
	if err != nil {
		return nil, err
	}

	// Broadcast resharing operation sync message to other participants
	go s.broadcastResharingOperation(
		operationID,
		sessionID,
		keyID,
		keyData.Threshold,
		newThreshold,
		keyData.Participants,
		newParticipants,
	)

	s.logger.Info("Started resharing operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.String("key_id", keyID))

	return operation, nil
}

func (s *Service) broadcastResharingOperation(
	operationID, sessionID string,
	keyID string,
	oldThreshold int,
	newThreshold int,
	oldParticipants, newParticipants []string,
) {
	s.logger.Info("Broadcast resharing operation",
		zap.String("operation_id", operationID),
		zap.String("session_id", sessionID),
		zap.String("key_id", keyID),
		zap.Int("old_threshold", oldThreshold),
		zap.Int("new_threshold", newThreshold),
		zap.Int("old_parties", len(oldParticipants)),
		zap.Int("new_parties", len(newParticipants)),
	)

	syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	syncData := &ResharingSyncData{
		OperationSyncData: OperationSyncData{
			OperationID:   operationID,
			OperationType: "resharing",
			SessionID:     sessionID,
			Threshold:     newThreshold,
			Parties:       len(newParticipants),
			Participants:  newParticipants,
		},
		OldThreshold:    oldThreshold,
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
	// Load key metadata to get old participants and threshold
	keyMetadata, err := s.LoadKeyMetadata(ctx, params.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key metadata: %w", err)
	}

	// Check if this node is an old participant (has existing key data)
	isOldParticipant := false
	for _, oldParticipant := range keyMetadata.Participants {
		if oldParticipant == s.nodeID {
			isOldParticipant = true
			break
		}
	}

	// Load key data only if this node is an old participant
	var localParty *keygen.LocalPartySaveData
	if isOldParticipant {
		_, loadedParty, err := s.loadKeyData(ctx, params.KeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to load key data for old participant: %w", err)
		}
		localParty = loadedParty
		s.logger.Info("Loaded existing key data for old participant",
			zap.String("node_id", s.nodeID),
			zap.String("key_id", params.KeyID))
	} else {
		// New participant - no existing key data
		localParty = nil
		s.logger.Info("New participant joining resharing",
			zap.String("node_id", s.nodeID),
			zap.String("key_id", params.KeyID))
	}

	// Create old participant list from key metadata (not from params)
	oldParticipantList, err := s.createParticipantList(keyMetadata.Participants)
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

	// Additional validation for TSS parameters
	if params.NewThreshold < 0 {
		return nil, fmt.Errorf("new threshold cannot be negative: %d", params.NewThreshold)
	}
	if params.NewThreshold >= len(newParticipantList) {
		return nil, fmt.Errorf("new threshold (%d) must be less than new party count (%d)",
			params.NewThreshold, len(newParticipantList))
	}
	if keyMetadata.Threshold < 0 {
		return nil, fmt.Errorf("old threshold cannot be negative: %d", keyMetadata.Threshold)
	}
	if keyMetadata.Threshold >= len(oldParticipantList) {
		return nil, fmt.Errorf("old threshold (%d) must be less than old party count (%d)",
			keyMetadata.Threshold, len(oldParticipantList))
	}

	// Create TSS parameters for resharing
	oldCtx := tss.NewPeerContext(oldParticipantList)
	newCtx := tss.NewPeerContext(newParticipantList)

	// Based on BNB Chain TSS library v2.0.2 documentation and test cases:
	// NewReSharingParameters(curve, oldCtx, newCtx, partyID, oldPartyCount, oldThreshold, newPartyCount, newThreshold)
	// Critical: oldThreshold and newThreshold are the actual threshold values, not counts
	// Use threshold from key metadata (the actual old threshold) instead of params
	tssParams := tss.NewReSharingParameters(
		tss.S256(),              // curve
		oldCtx,                  // old peer context
		newCtx,                  // new peer context
		ourPartyID,              // our party ID
		len(oldParticipantList), // old party count
		keyMetadata.Threshold,   // old threshold from key metadata (t, not t+1)
		len(newParticipantList), // new party count
		params.NewThreshold,     // new threshold (t, not t+1)
	)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create resharing party with additional validation
	s.logger.Info("Creating resharing party",
		zap.String("operation_id", params.OperationID),
		zap.String("key_id", params.KeyID),
		zap.Int("old_parties", len(oldParticipantList)),
		zap.Int("new_parties", len(newParticipantList)),
		zap.Int("old_threshold", keyMetadata.Threshold),
		zap.Int("new_threshold", params.NewThreshold),
		zap.String("our_party_id", ourPartyID.Id))

	// Create resharing party - pass nil for new participants, actual data for old participants
	var party tss.Party
	if localParty != nil {
		// Old participant with existing key data
		party = resharing.NewLocalParty(tssParams, *localParty, outCh, endCh)
	} else {
		// New participant - pass empty LocalPartySaveData according to TSS lib documentation
		emptyData := keygen.LocalPartySaveData{}
		party = resharing.NewLocalParty(tssParams, emptyData, outCh, endCh)
	}

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a longer timeout for resharing operations (15 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	// Create request for storage
	req := &ResharingRequest{
		OperationID:     params.OperationID,
		KeyID:           params.KeyID,
		NewThreshold:    params.NewThreshold,
		NewParties:      len(params.NewParticipants),
		OldParticipants: keyMetadata.Participants, // Use participants from key metadata
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

	// Check if this node is an old participant (has existing key data)
	isOldParticipant := false
	for _, oldParticipant := range syncData.OldParticipants {
		if oldParticipant == s.nodeID {
			isOldParticipant = true
			break
		}
	}

	// Load key data only if this node is an old participant
	var localParty *keygen.LocalPartySaveData
	var keyMetadata *keyData
	if isOldParticipant {
		// Old participant - load existing key data
		var err error
		keyMetadata, localParty, err = s.loadKeyData(ctx, syncData.KeyID)
		if err != nil {
			return fmt.Errorf("failed to load key data for old participant: %w", err)
		}
		s.logger.Info("Loaded existing key data for old participant",
			zap.String("node_id", s.nodeID),
			zap.String("key_id", syncData.KeyID))
	} else {
		// New participant - create mock key metadata from sync data
		keyMetadata = &keyData{
			Participants: syncData.OldParticipants,
			Threshold:    syncData.OldThreshold,
		}
		localParty = nil
		s.logger.Info("New participant joining resharing",
			zap.String("node_id", s.nodeID),
			zap.String("key_id", syncData.KeyID))
	}

	// Create old participant list from sync data
	oldParticipantList, err := s.createParticipantList(syncData.OldParticipants)
	if err != nil {
		return fmt.Errorf("failed to create old participant list: %w", err)
	}

	newParticipantList, err := s.createParticipantList(syncData.NewParticipants)
	if err != nil {
		return fmt.Errorf("failed to create new participant list: %w", err)
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
		return fmt.Errorf("this node (%s) is not in either old or new participant lists", s.nodeID)
	}

	// Create TSS parameters for resharing
	oldCtx := tss.NewPeerContext(oldParticipantList)
	newCtx := tss.NewPeerContext(newParticipantList)

	tssParams := tss.NewReSharingParameters(
		tss.S256(),              // curve
		oldCtx,                  // old peer context
		newCtx,                  // new peer context
		ourPartyID,              // our party ID
		len(oldParticipantList), // old party count
		keyMetadata.Threshold,   // old threshold from metadata or sync data
		len(newParticipantList), // new party count
		syncData.NewThreshold,   // new threshold
	)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create resharing party
	var party tss.Party
	if localParty != nil {
		// Old participant with existing key data
		party = resharing.NewLocalParty(tssParams, *localParty, outCh, endCh)
	} else {
		// New participant - pass empty LocalPartySaveData
		emptyData := keygen.LocalPartySaveData{}
		party = resharing.NewLocalParty(tssParams, emptyData, outCh, endCh)
	}

	// Create operation context with cancellation
	operationCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

	// Create request for storage
	req := &ResharingRequest{
		OperationID:     syncData.OperationID,
		KeyID:           syncData.KeyID,
		NewThreshold:    syncData.NewThreshold,
		NewParties:      len(syncData.NewParticipants),
		OldParticipants: syncData.OldParticipants,
		NewParticipants: syncData.NewParticipants,
	}

	operation := &Operation{
		ID:           syncData.OperationID,
		Type:         OperationResharing,
		SessionID:    syncData.SessionID,
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
	s.operations[syncData.OperationID] = operation
	s.mutex.Unlock()

	// Start operation in a goroutine
	go s.runResharingOperation(operationCtx, operation, endCh)

	s.logger.Info("Started synced resharing operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID))

	return nil
}
