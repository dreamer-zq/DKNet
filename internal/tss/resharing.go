package tss

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/common"
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
	common.SafeGo(operation.EndCh, func() any {
		return s.broadcastResharingOperation(
			operationID,
			sessionID,
			keyID,
			keyData.Threshold,
			newThreshold,
			keyData.Participants,
			newParticipants,
		)
	})

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
) error {
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
		return err
	}

	s.logger.Info("Resharing operation sync broadcasted successfully",
		zap.String("operation_id", operationID),
		zap.String("key_id", keyID))
	return nil
}

// createResharingOperation creates a resharing operation with common logic
// This function should only be called from old participants who have the key data
func (s *Service) createResharingOperation(ctx context.Context, params *resharingOperationParams) (*Operation, error) {
	// Load key data (this node must be an old participant)
	keyMetadata, localParty, err := s.loadKeyData(ctx, params.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	// Verify this node is an old participant (only old participants can initiate resharing)
	if !slices.Contains(keyMetadata.Participants, s.nodeID) {
		return nil, fmt.Errorf("only old participants can initiate resharing operations, node %s is not in old participants", s.nodeID)
	}

	s.logger.Info("Loaded existing key data for resharing initiation",
		zap.String("node_id", s.nodeID),
		zap.String("key_id", params.KeyID))

	// Create old participant list from key metadata (not from params)
	oldParticipantList, err := s.createParticipantList(keyMetadata.Participants)
	if err != nil {
		return nil, fmt.Errorf("failed to create old participant list: %w", err)
	}

	newParticipantList, err := s.createParticipantList(params.NewParticipants)
	if err != nil {
		return nil, fmt.Errorf("failed to create new participant list: %w", err)
	}

	// Find our party ID in the old participant list (since this is always an old participant)
	idx := slices.IndexFunc(oldParticipantList, func(p *tss.PartyID) bool {
		return p.Id == s.nodeID
	})

	if idx == -1 {
		return nil, fmt.Errorf("this node (%s) is not in old participant list", s.nodeID)
	}
	ourPartyID := oldParticipantList[idx]

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

	// Use ReSharingParameters instead of regular Parameters
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
	channelSize := len(newParticipantList) + len(oldParticipantList)
	outCh := make(chan tss.Message, channelSize)
	endCh := make(chan *keygen.LocalPartySaveData, channelSize)

	// Create resharing party with additional validation
	s.logger.Info("Creating resharing party",
		zap.String("operation_id", params.OperationID),
		zap.String("key_id", params.KeyID),
		zap.Int("old_parties", len(oldParticipantList)),
		zap.Int("new_parties", len(newParticipantList)),
		zap.Int("old_threshold", keyMetadata.Threshold),
		zap.Int("new_threshold", params.NewThreshold),
		zap.String("our_party_id", ourPartyID.Id))

	// Create resharing party with existing key data (this node is always an old participant)
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
		EndCh:        common.ConvertToAnyCh(endCh), // Generic channel for interface{}
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
	go s.runResharingOperation(operationCtx, operation)

	return operation, nil
}

// runResharingOperation runs a resharing operation
func (s *Service) runResharingOperation(ctx context.Context, operation *Operation) {
	// Update status
	operation.Lock()
	operation.Status = StatusInProgress
	operation.Unlock()

	// Start the party
	common.SafeGo(operation.EndCh, func() any {
		s.logger.Info("Starting TSS party", zap.String("operation_id", operation.ID))
		if err := operation.Party.Start(); err != nil {
			return err
		}
		s.logger.Info("TSS party started successfully", zap.String("operation_id", operation.ID))
		return nil
	})

	// Handle outgoing messages
	common.SafeGo(operation.EndCh, func() any {
		return s.handleOutgoingMessages(ctx, operation)
	})
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
	isOldParticipant := slices.Contains(syncData.OldParticipants, s.nodeID)

	// Load key data only if this node is an old participant
	var localParty keygen.LocalPartySaveData

	if isOldParticipant {
		// Old participant - load existing key data
		_, party, err := s.loadKeyData(ctx, syncData.KeyID)
		if err != nil {
			return fmt.Errorf("failed to load key data for old participant: %w", err)
		}

		localParty = *party

		s.logger.Info("Loaded existing key data for old participant",
			zap.String("node_id", s.nodeID),
			zap.String("key_id", syncData.KeyID))
	} else {
		localParty = keygen.NewLocalPartySaveData(len(syncData.NewParticipants))
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
	if isOldParticipant {
		idx := slices.IndexFunc(oldParticipantList, func(p *tss.PartyID) bool {
			return p.Id == s.nodeID
		})
		if idx == -1 {
			return fmt.Errorf("this node (%s) is not in old participant list", s.nodeID)
		}
		ourPartyID = oldParticipantList[idx]
	} else {
		idx := slices.IndexFunc(newParticipantList, func(p *tss.PartyID) bool {
			return p.Id == s.nodeID
		})
		if idx == -1 {
			return fmt.Errorf("this node (%s) is not in new participant list", s.nodeID)
		}
		ourPartyID = newParticipantList[idx]
	}

	// Create TSS parameters for resharing
	oldCtx := tss.NewPeerContext(oldParticipantList)
	newCtx := tss.NewPeerContext(newParticipantList)

	// Use ReSharingParameters instead of regular Parameters
	tssParams := tss.NewReSharingParameters(
		tss.S256(),              // curve
		oldCtx,                  // old peer context
		newCtx,                  // new peer context
		ourPartyID,              // our party ID
		len(oldParticipantList), // old party count
		syncData.OldThreshold,   // old threshold from sync data
		len(newParticipantList), // new party count
		syncData.NewThreshold,   // new threshold
	)

	channelSize := len(newParticipantList) + len(oldParticipantList)
	// Create channels
	outCh := make(chan tss.Message, channelSize)
	endCh := make(chan *keygen.LocalPartySaveData, channelSize)

	// Create resharing party
	party := resharing.NewLocalParty(tssParams, localParty, outCh, endCh)
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
		EndCh:        common.ConvertToAnyCh(endCh), // Generic channel for interface{}
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Request:      req, // Store the request for persistence
		cancel:       cancel,
	}

	// Store operation
	s.mutex.Lock()
	s.operations[syncData.OperationID] = operation
	s.mutex.Unlock()

	// Wait for operation completion or cancellation
	go s.watchOperation(operationCtx, operation)
	// Start operation in a goroutine
	go s.runResharingOperation(operationCtx, operation)

	s.logger.Info("Started synced resharing operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID))

	return nil
}
