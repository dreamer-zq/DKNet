package tss

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/google/uuid"
	"go.uber.org/zap"

	dkcommon "github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/dreamer-zq/DKNet/internal/plugin"
	"github.com/dreamer-zq/DKNet/internal/storage"
)

// Service provides TSS operations
type Service struct {
	logger            *zap.Logger
	storage           storage.Storage
	network           *p2p.Network
	encryption        *plugin.KeyCipher
	validationService plugin.ValidationService // optional

	operations map[string]*Operation
	mutex      sync.RWMutex
	nodeID     string
	moniker    string
}

// NewService creates a new TSS service
func NewService(
	cfg *Config,
	store storage.Storage,
	network *p2p.Network,
	logger *zap.Logger,
	encryptionPassword string,
) (*Service, error) {
	// Initialize key encryption
	keyEncryption, err := plugin.NewKeyCipher(encryptionPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize key encryption: %w", err)
	}

	service := &Service{
		storage:    store,
		network:    network,
		logger:     logger,
		encryption: keyEncryption,
		operations: make(map[string]*Operation),
		nodeID:     cfg.PeerID,
		moniker:    cfg.Moniker,
	}

	// Check if validation service is configured and enabled
	if cfg.ValidationService != nil && cfg.ValidationService.Enabled {
		service.validationService = plugin.NewHTTPValidationService(cfg.ValidationService, cfg.PeerID, logger)
	}

	// Set this service as the message handler for the network
	network.SetMessageHandler(service)

	logger.Info("TSS service initialized",
		zap.String("peer_id", cfg.PeerID),
		zap.String("moniker", cfg.Moniker))

	return service, nil
}

// Stop is part of the MessageHandler interface.
// Currently a no-op as operation lifecycles are tied to contexts.
func (s *Service) Stop() {
	s.logger.Info("TSS Service stopping.")
}

// HandleMessage handles incoming TSS messages from the P2P network
func (s *Service) HandleMessage(ctx context.Context, msg *p2p.Message) error {
	s.logger.Info("Received incoming P2P message",
		zap.String("session_id", msg.SessionID),
		zap.String("type", msg.Type),
		zap.String("from", msg.From),
		zap.Bool("is_broadcast", msg.IsBroadcast),
		zap.Int("data_len", len(msg.Data)))

	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Recovered from panic in HandleMessage", zap.Any("error", r))
		}
	}()

	// Handle operation synchronization messages
	if msg.Type == string(OperationSync) {
		return s.handleOperationSync(ctx, msg)
	}

	// Handle regular TSS messages
	// Find operation by session ID
	operation := s.getOperation(msg.SessionID)
	if operation == nil {
		s.logger.Warn("No operation found for session ID",
			zap.String("session_id", msg.SessionID),
			zap.String("from", msg.From))
		return fmt.Errorf("no operation found for session ID: %s", msg.SessionID)
	}

	s.logger.Info("Found operation for incoming message",
		zap.String("session_id", msg.SessionID),
		zap.String("operation_id", operation.ID),
		zap.String("from", msg.From))

	// Skip messages from ourselves to avoid self-processing
	if msg.From == s.nodeID {
		s.logger.Debug("Skipping message from self",
			zap.String("session_id", msg.SessionID),
			zap.String("operation_id", operation.ID),
			zap.String("from", msg.From),
			zap.String("our_node_id", s.nodeID))
		return nil
	}

	// Find sender party ID
	idx := slices.IndexFunc(operation.Participants, func(op *tss.PartyID) bool {
		return op.Id == msg.From
	})
	if idx == -1 {
		s.logger.Error("Unknown sender",
			zap.String("from", msg.From),
			zap.String("session_id", msg.SessionID))
		return fmt.Errorf("unknown sender: %s", msg.From)
	}
	fromParty := operation.Participants[idx]

	s.logger.Info("Found sender party",
		zap.String("session_id", msg.SessionID),
		zap.String("operation_id", operation.ID),
		zap.String("from", msg.From),
		zap.String("from_party_id", fromParty.Id))

	// Send to party's UpdateFromBytes channel
	dkcommon.SafeGo(operation.EndCh, func() any {
		s.logger.Info("Sending message to TSS party",
			zap.String("session_id", msg.SessionID),
			zap.String("operation_id", operation.ID),
			zap.Bool("isToOldCommittee", msg.IsToOldCommittee),
			zap.Bool("isToOldAndNewCommittees", msg.IsToOldAndNewCommittees),
			zap.String("from", msg.From))

		if operation.Type == OperationResharing {
			switch {
			case operation.isNewParticipant() && msg.IsToOldCommittee:
				s.logger.Info("Skipping message to old participant",
					zap.String("session_id", msg.SessionID),
					zap.String("operation_id", operation.ID),
					zap.String("from", msg.From))
				return nil
			case !operation.isNewParticipant() && !msg.IsToOldCommittee:
				s.logger.Info("Skipping message to new participant",
					zap.String("session_id", msg.SessionID),
					zap.String("operation_id", operation.ID),
					zap.String("from", msg.From))
				return nil
			default:
			}
		}

		ok, err := operation.Party.UpdateFromBytes(msg.Data, fromParty, msg.IsBroadcast)
		if err != nil {
			s.logger.Error("Failed to update party with message",
				zap.Error(err),
				zap.String("session_id", msg.SessionID),
				zap.String("operation_id", operation.ID),
				zap.String("from", msg.From))
			return err
		} else if !ok {
			s.logger.Warn("Message was not processed by party",
				zap.String("session_id", msg.SessionID),
				zap.String("operation_id", operation.ID),
				zap.String("from", msg.From))
			return fmt.Errorf("message was not processed by party")
		}

		s.logger.Info("Successfully updated TSS party with message",
			zap.String("session_id", msg.SessionID),
			zap.String("operation_id", operation.ID),
			zap.String("from", msg.From))
		return nil
	})

	return nil
}

// GetOperation returns an operation by ID
func (s *Service) GetOperation(operationID string) (*Operation, bool) {
	// First check active operations in memory
	s.mutex.RLock()
	op, exists := s.operations[operationID]
	s.mutex.RUnlock()

	if exists {
		return op, true
	}
	return nil, false
}

// GetOperationData returns operation data by ID, checking both memory and persistent storage
func (s *Service) GetOperationData(ctx context.Context, operationID string) (*OperationData, error) {
	// First check active operations in memory
	s.mutex.RLock()
	op, exists := s.operations[operationID]
	s.mutex.RUnlock()

	if exists {
		// Convert active operation to OperationData
		opData := &OperationData{
			ID:           op.ID,
			Type:         op.Type,
			SessionID:    op.SessionID,
			Status:       op.Status,
			Participants: make([]string, len(op.Participants)),
			CreatedAt:    op.CreatedAt,
			CompletedAt:  op.CompletedAt,
			Result:       op.Result,
		}

		// Extract participant IDs
		for i, p := range op.Participants {
			opData.Participants[i] = p.Id
		}

		// Set error if present
		if op.Error != nil {
			opData.Error = op.Error.Error()
		}

		return opData, nil
	}

	// If not found in memory, check persistent storage
	return s.loadOperation(ctx, operationID)
}

// handleOperationSync handles operation synchronization messages
func (s *Service) handleOperationSync(ctx context.Context, msg *p2p.Message) error {
	// Parse operation sync data from message data
	var baseData OperationSyncData
	if err := json.Unmarshal(msg.Data, &baseData); err != nil {
		s.logger.Error("Failed to unmarshal operation sync data", zap.Error(err))
		return fmt.Errorf("failed to unmarshal operation sync data: %w", err)
	}

	s.logger.Info("Received operation sync message",
		zap.String("operation_id", baseData.OperationID),
		zap.String("operation_type", string(baseData.OperationType)),
		zap.String("session_id", baseData.SessionID),
		zap.String("from", msg.From),
		zap.Strings("participants", baseData.Participants))

	// Check if we are one of the participants
	isParticipant := slices.Contains(baseData.Participants, s.nodeID)

	if !isParticipant {
		s.logger.Info("Ignoring operation sync - not a participant",
			zap.String("operation_id", baseData.OperationID),
			zap.String("node_id", s.nodeID))
		return nil
	}

	// Check if we already have this operation
	s.mutex.RLock()
	_, exists := s.operations[baseData.OperationID]
	s.mutex.RUnlock()

	if exists {
		s.logger.Info("Operation already exists, ignoring sync message",
			zap.String("operation_id", baseData.OperationID))
		return nil
	}

	// Create the operation based on the sync message
	switch baseData.OperationType {
	case OperationKeygen:
		return s.createSyncedKeygenOperation(ctx, msg)
	case OperationSigning:
		return s.createSyncedSigningOperation(ctx, msg)
	case OperationResharing:
		return s.createSyncedResharingOperation(ctx, msg)
	default:
		return fmt.Errorf("unknown operation type: %s", baseData.OperationType)
	}
}

// handleOutgoingMessages handles outgoing TSS messages
func (s *Service) handleOutgoingMessages(ctx context.Context, operation *Operation) error {
	s.logger.Info("Starting outgoing message handler", zap.String("operation_id", operation.ID))

	for {
		select {
		case msg := <-operation.OutCh:
			s.logger.Info("Received outgoing TSS message",
				zap.String("operation_id", operation.ID),
				zap.String("msg_type", fmt.Sprintf("%T", msg)))

			// Get wire bytes and routing info
			wireBytes, routing, err := msg.WireBytes()
			if err != nil {
				s.logger.Error("Failed to get wire bytes",
					zap.Error(err),
					zap.String("operation_id", operation.ID))
				return err
			}

			s.logger.Info("Processing message routing",
				zap.String("operation_id", operation.ID),
				zap.Bool("is_broadcast", routing.IsBroadcast),
				zap.Int("wire_bytes_len", len(wireBytes)),
				zap.Int("routing_to_count", len(routing.To)))
			// Create p2p message
			p2pMsg := &p2p.Message{
				ProtocolID:              p2p.TssPartyProtocolID,
				SessionID:               operation.SessionID,
				Type:                    string(operation.Type), // Set the message type based on operation type
				From:                    s.nodeID,
				To:                      make([]string, 0, len(routing.To)),
				Data:                    wireBytes,
				IsBroadcast:             routing.IsBroadcast,
				Timestamp:               time.Now(),
				IsToOldCommittee:        msg.IsToOldCommittee(),
				IsToOldAndNewCommittees: msg.IsToOldAndNewCommittees(),
			}

			to, err := s.toParticipants(operation, msg, routing)
			if err != nil {
				s.logger.Error("get participants failed", zap.Error(err))
				return err
			}

			p2pMsg.To = to
			s.logger.Info("Sending point-to-point message",
				zap.String("operation_id", operation.ID),
				zap.String("session_id", operation.SessionID),
				zap.Strings("targets", p2pMsg.To),
				zap.Bool("IsToOldCommittee", p2pMsg.IsToOldCommittee),
				zap.Bool("IsToOldAndNewCommittees", p2pMsg.IsToOldAndNewCommittees),
			)

			if err := s.network.SendMessage(ctx, p2pMsg); err != nil {
				s.logger.Error("Failed to send message",
					zap.Error(err),
					zap.String("operation_id", operation.ID),
					zap.Strings("targets", p2pMsg.To))
				return err
			}
		case <-ctx.Done():
			s.logger.Info("Outgoing message handler stopped",
				zap.String("operation_id", operation.ID),
				zap.Error(ctx.Err()))
			return ctx.Err()
		}
	}
}

// loadKeyData loads and decrypts key data from storage
func (s *Service) loadKeyData(ctx context.Context, keyID string) (*keyData, *keygen.LocalPartySaveData, error) {
	data, err := s.storage.Load(ctx, keyID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load key data: %w", err)
	}

	var keyDataStruct keyData
	if unmarshalErr := json.Unmarshal(data, &keyDataStruct); unmarshalErr != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal key data struct: %w", unmarshalErr)
	}

	// Decrypt the key data
	decryptedKeyData, err := s.encryption.Decrypt(keyDataStruct.KeyData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt key data: %w", err)
	}

	var saveData keygen.LocalPartySaveData
	if err := json.Unmarshal(decryptedKeyData, &saveData); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal save data: %w", err)
	}

	s.logger.Debug("Successfully loaded and decrypted key data",
		zap.String("key_id", keyID),
		zap.Int("encrypted_size", len(keyDataStruct.KeyData)),
		zap.Int("decrypted_size", len(decryptedKeyData)))

	return &keyDataStruct, &saveData, nil
}

// LoadKeyMetadata loads key metadata from storage
func (s *Service) LoadKeyMetadata(ctx context.Context, keyID string) (*keyData, error) {
	data, err := s.storage.Load(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	var keyDataStruct keyData
	if unmarshalErr := json.Unmarshal(data, &keyDataStruct); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal key data struct: %w", unmarshalErr)
	}

	return &keyDataStruct, nil
}

// createParticipantList creates a list of party IDs from peer IDs
func (s *Service) createParticipantList(peerIDs []string) ([]*tss.PartyID, error) {
	participants := dkcommon.Map(peerIDs, func(peerID string) *tss.PartyID {
		// Generate a deterministic key based on the peer ID itself
		// This ensures the same node always gets the same key across different operations
		key := s.generateDeterministicKey(peerID)

		// Use empty moniker for remote peers, or actual moniker if it's this node
		moniker := ""
		if peerID == s.nodeID {
			moniker = s.moniker
		}

		return tss.NewPartyID(peerID, moniker, key)
	})
	return tss.SortPartyIDs(participants), nil
}

// generateDeterministicKey generates a deterministic big.Int key from a peer ID
// This ensures the same peer always gets the same key across different operations
// Uses the same method as bnb-chain/tss library for compatibility
func (s *Service) generateDeterministicKey(peerID string) *big.Int {
	// Use TSS library's SHA512_256 function for consistency with bnb-chain/tss
	hash := common.SHA512_256([]byte(peerID))

	// Convert hash to big.Int
	key := new(big.Int).SetBytes(hash)

	// Ensure key is never zero (add 1 if it is)
	if key.Cmp(big.NewInt(0)) == 0 {
		key.SetInt64(1)
	}

	return key
}

// syncOperation broadcasts operation synchronization message to all peers
func (s *Service) syncOperation(ctx context.Context, syncData Message) error {
	// Serialize sync data
	data, err := json.Marshal(syncData)
	if err != nil {
		return fmt.Errorf("failed to marshal sync data: %w", err)
	}

	// Remove self from the list of participants
	to := slices.DeleteFunc(syncData.To(), func(to string) bool {
		return to == s.nodeID
	})

	if len(to) == 0 {
		s.logger.Warn("No participants to sync with", zap.String("SessionID", syncData.ID()))
		return nil
	}

	msg := &p2p.Message{
		ProtocolID:  p2p.TssPartyProtocolID,
		SessionID:   syncData.ID(),
		Type:        string(OperationSync),
		From:        s.nodeID,
		To:          to,
		IsBroadcast: true,
		Data:        data, // Serialized operation sync data
		Timestamp:   time.Now(),
	}
	return s.network.SendMessage(ctx, msg)
}

// saveOperation saves an operation to persistent storage
func (s *Service) saveOperation(ctx context.Context, operation *Operation) error {
	// Convert Operation to OperationData for persistence
	opData := &OperationData{
		ID:           operation.ID,
		Type:         operation.Type,
		SessionID:    operation.SessionID,
		Status:       operation.Status,
		Participants: make([]string, len(operation.Participants)),
		CreatedAt:    operation.CreatedAt,
		CompletedAt:  operation.CompletedAt,
		Request:      operation.Request,
		Result:       operation.Result,
	}

	if !opData.IsCompleted() {
		return fmt.Errorf("operation %s is not completed (status: %s)", opData.ID, opData.Status)
	}

	opData.Participants = dkcommon.Map(operation.Participants, func(p *tss.PartyID) string {
		return p.Id
	})

	// Set error if present
	if operation.Error != nil {
		opData.Error = operation.Error.Error()
	}

	// Serialize operation data to JSON
	data, err := json.Marshal(opData)
	if err != nil {
		return fmt.Errorf("failed to marshal operation data: %w", err)
	}

	// Save to storage with operation key prefix
	key := fmt.Sprintf("operation:%s", operation.ID)
	return s.storage.Save(ctx, key, data)
}

// loadOperation loads an operation from persistent storage
func (s *Service) loadOperation(ctx context.Context, operationID string) (*OperationData, error) {
	key := fmt.Sprintf("operation:%s", operationID)
	data, err := s.storage.Load(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load operation data: %w", err)
	}

	var opData OperationData
	if err := json.Unmarshal(data, &opData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal operation data: %w", err)
	}

	// Reconstruct typed Result from generic interface{} based on operation type
	if opData.Result != nil {
		resultBytes, err := json.Marshal(opData.Result)
		if err == nil {
			switch opData.Type {
			case OperationKeygen:
				var result KeygenResult
				if err := json.Unmarshal(resultBytes, &result); err == nil {
					opData.Result = &result
				}
			case OperationSigning:
				var result SigningResult
				if err := json.Unmarshal(resultBytes, &result); err == nil {
					opData.Result = &result
				}
			case OperationResharing:
				var result KeygenResult // Resharing result uses same type as keygen
				if err := json.Unmarshal(resultBytes, &result); err == nil {
					opData.Result = &result
				}
			}
		}
	}

	// Reconstruct typed Request from generic interface{} based on operation type
	if opData.Request != nil {
		requestBytes, err := json.Marshal(opData.Request)
		if err == nil {
			switch opData.Type {
			case OperationKeygen:
				var request KeygenRequest
				if err := json.Unmarshal(requestBytes, &request); err == nil {
					opData.Request = &request
				}
			case OperationSigning:
				var request SigningRequest
				if err := json.Unmarshal(requestBytes, &request); err == nil {
					opData.Request = &request
				}
			case OperationResharing:
				var request ResharingRequest
				if err := json.Unmarshal(requestBytes, &request); err == nil {
					opData.Request = &request
				}
			}
		}
	}

	return &opData, nil
}

// moveCompletedOperationToStorage moves a completed operation from memory to persistent storage
func (s *Service) moveCompletedOperationToStorage(ctx context.Context, operationID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	operation, exists := s.operations[operationID]
	if !exists {
		return fmt.Errorf("operation not found in memory: %s", operationID)
	}

	// Save to persistent storage
	if err := s.saveOperation(ctx, operation); err != nil {
		return fmt.Errorf("failed to save operation to storage: %w", err)
	}

	// Remove from memory
	delete(s.operations, operationID)
	return nil
}

// checkIdempotency checks if an operation with the given ID already exists
// Returns the existing operation if found, nil if not found, and an error if there's an issue
func (s *Service) checkIdempotency(ctx context.Context, operationID string) (*Operation, error) {
	if operationID == "" {
		return nil, nil // No operation ID provided, proceed with new operation
	}

	// Check if operation already exists in memory
	s.mutex.RLock()
	if existingOp, exists := s.operations[operationID]; exists {
		s.mutex.RUnlock()
		s.logger.Info("Operation already exists in memory",
			zap.String("operation_id", operationID),
			zap.String("status", string(existingOp.Status)))
		return existingOp, nil
	}
	s.mutex.RUnlock()

	// Check if operation exists in persistent storage
	opData, err := s.loadOperation(ctx, operationID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("Operation found in persistent storage",
		zap.String("operation_id", operationID),
		zap.String("status", string(opData.Status)))

	// Convert OperationData to Operation for consistency
	// Note: This is a read-only operation since it's completed
	operation := &Operation{
		ID:          opData.ID,
		Type:        opData.Type,
		SessionID:   opData.SessionID,
		Status:      opData.Status,
		CreatedAt:   opData.CreatedAt,
		CompletedAt: opData.CompletedAt,
		Request:     opData.Request,
		Result:      opData.Result,
	}
	if opData.Error != "" {
		operation.Error = fmt.Errorf("%s", opData.Error)
	}
	return operation, nil
}

// generateOrUseOperationID generates a new operation ID if not provided, or returns the provided one
func (s *Service) generateOrUseOperationID(providedID string) string {
	if providedID != "" {
		return providedID
	}
	return uuid.New().String()
}

func (s *Service) toParticipants(operation *Operation, msg tss.Message, routing *tss.MessageRouting) ([]string, error) {
	var (
		to           []*tss.PartyID
		participants []string
	)

	switch {
	case msg.IsToOldCommittee():
		req, ok := operation.Request.(*ResharingRequest)
		if !ok {
			return nil, fmt.Errorf("invalid resharing request")
		}
		return req.OldParticipants, nil
	case msg.IsToOldAndNewCommittees():
		req, ok := operation.Request.(*ResharingRequest)
		if !ok {
			return nil, fmt.Errorf("invalid resharing request")
		}
		participants = append(participants, req.OldParticipants...)
		participants = append(participants, req.NewParticipants...)
		participants = dkcommon.Distinct(participants)
		return participants, nil
	case len(routing.To) > 0:
		to = routing.To
	case routing.IsBroadcast:
		to = operation.Participants
	default:
		return nil, fmt.Errorf("invalid routing")
	}

	participants = dkcommon.Map(to, func(to *tss.PartyID) string {
		return to.Id
	})
	return participants, nil
}

func (s *Service) watchOperation(ctx context.Context, op *Operation) {
	s.logger.Info("Waiting for operation completion or cancellation", zap.String("operation_id", op.ID))

	// Always move completed operation to persistent storage for cleanup
	defer func() {
		if err := s.moveCompletedOperationToStorage(ctx, op.ID); err != nil {
			s.logger.Error("Failed to move operation to persistent storage during cleanup",
				zap.Error(err),
				zap.String("operation_id", op.ID),
				zap.String("type", string(op.Type)))
		}
		s.logger.Info("Operation completed",
			zap.String("operation_id", op.ID),
			zap.String("type", string(op.Type)),
			zap.String("status", string(op.Status)),
		)
	}()

	// Wait for operation completion or cancellation
	select {
	case <-ctx.Done():
		s.logger.Info("Operation canceled or timed out", zap.String("operation_id", op.ID), zap.Error(ctx.Err()))
		op.Status = StatusCancelled
		op.CompletedAt = dkcommon.Now()
	case result := <-op.EndCh:
		op.CompletedAt = dkcommon.Now()
		switch r := result.(type) {
		case error:
			op.Error = r
			op.Status = StatusFailed
			s.logger.Error("Operation failed", zap.String("operation_id", op.ID), zap.Error(r))
		case *keygen.LocalPartySaveData:
			op.Status = StatusCompleted
			if err := s.saveKeygenResult(ctx, op, r); err != nil {
				s.logger.Error("Failed to save signing result", zap.Error(err))
				op.Error = err
				op.Status = StatusFailed
			}
		case *common.SignatureData:
			op.Status = StatusCompleted
			if err := s.saveSigningResult(ctx, op, r); err != nil {
				s.logger.Error("Failed to save signing result", zap.Error(err))
				op.Error = err
				op.Status = StatusFailed
			}
		default:
			s.logger.Error("Unknown operation result type", zap.Any("result", result))
			op.Status = StatusFailed
		}
	}
}

// runOperation runs a TSS operation
func (s *Service) runOperation(ctx context.Context, operation *Operation) {
	s.logger.Info("Starting TSS operation goroutine", zap.String("operation_id", operation.ID))

	// Update status
	operation.Lock()
	operation.Status = StatusInProgress
	operation.Unlock()

	// Start the party
	dkcommon.SafeGo(operation.EndCh, func() any {
		s.logger.Info("Starting TSS party", zap.String("operation_id", operation.ID))
		if err := operation.Party.Start(); err != nil {
			return err
		}
		s.logger.Info("TSS party started successfully", zap.String("operation_id", operation.ID))
		return nil
	})

	// Handle outgoing messages
	dkcommon.SafeGo(operation.EndCh, func() any {
		return s.handleOutgoingMessages(ctx, operation)
	})
}

func (s *Service) getOperation(sessionID string) *Operation {
	find := func() *Operation {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		for _, op := range s.operations {
			if op.SessionID == sessionID {
				return op
			}
		}
		return nil
	}

	return dkcommon.Retry(find, 1, 10)
}
