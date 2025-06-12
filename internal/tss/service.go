package tss

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/p2p"
	"github.com/dreamer-zq/DKNet/internal/storage"
)

// Service provides TSS operations
type Service struct {
	config  *Config
	storage storage.Storage
	network *p2p.Network
	logger  *zap.Logger

	// Active operations
	operations map[string]*Operation
	mutex      sync.RWMutex

	// Node identity
	nodeID  string
	moniker string
	partyID *tss.PartyID

	// Node ID to P2P peer ID mapping
	nodeIDToPeerID map[string]string
	mappingMutex   sync.RWMutex

	// Validation service client (optional)
	validationService ValidationService
}

// NewService creates a new TSS service
func NewService(cfg *Config, storage storage.Storage, network *p2p.Network, logger *zap.Logger) (*Service, error) {
	// Create party ID from node information
	nodeKey := big.NewInt(0)
	nodeKey.SetString(cfg.NodeID, 16)

	partyID := tss.NewPartyID(cfg.NodeID, cfg.Moniker, nodeKey)
	s := &Service{
		config:            cfg,
		storage:           storage,
		network:           network,
		logger:            logger,
		operations:        make(map[string]*Operation),
		nodeID:            cfg.NodeID,
		moniker:           cfg.Moniker,
		partyID:           partyID,
		nodeIDToPeerID:    make(map[string]string),
	}
	
	// Check if validation service is configured and enabled
	if cfg.ValidationService != nil && cfg.ValidationService.Enabled {
		s.validationService = NewHTTPValidationService(cfg.ValidationService, cfg.NodeID, logger)
	}

	return s, nil
}

// HandleMessage handles incoming TSS messages from the P2P network
func (s *Service) HandleMessage(ctx context.Context, msg *p2p.Message) error {
	s.logger.Info("Received incoming P2P message",
		zap.String("session_id", msg.SessionID),
		zap.String("type", msg.Type),
		zap.String("from", msg.From),
		zap.Bool("is_broadcast", msg.IsBroadcast),
		zap.Int("data_len", len(msg.Data)))

	// Handle operation synchronization messages
	if msg.Type == "operation_sync" {
		return s.handleOperationSync(ctx, msg)
	}

	// Handle regular TSS messages
	// Find operation by session ID
	s.mutex.RLock()
	var operation *Operation
	for _, op := range s.operations {
		if op.SessionID == msg.SessionID {
			operation = op
			break
		}
	}
	s.mutex.RUnlock()

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

	// Find sender party ID
	var fromParty *tss.PartyID
	for _, p := range operation.Participants {
		if p.Id == msg.From {
			fromParty = p
			break
		}
	}

	if fromParty == nil {
		s.logger.Error("Unknown sender",
			zap.String("from", msg.From),
			zap.String("session_id", msg.SessionID))
		return fmt.Errorf("unknown sender: %s", msg.From)
	}

	s.logger.Info("Found sender party",
		zap.String("session_id", msg.SessionID),
		zap.String("operation_id", operation.ID),
		zap.String("from", msg.From),
		zap.String("from_party_id", fromParty.Id))

	// Send to party's UpdateFromBytes channel
	go func() {
		s.logger.Info("Sending message to TSS party",
			zap.String("session_id", msg.SessionID),
			zap.String("operation_id", operation.ID),
			zap.String("from", msg.From))

		ok, err := operation.Party.UpdateFromBytes(msg.Data, fromParty, msg.IsBroadcast)
		if err != nil {
			s.logger.Error("Failed to update party with message",
				zap.Error(err),
				zap.String("session_id", msg.SessionID),
				zap.String("operation_id", operation.ID),
				zap.String("from", msg.From))
		} else if !ok {
			s.logger.Warn("Message was not processed by party",
				zap.String("session_id", msg.SessionID),
				zap.String("operation_id", operation.ID),
				zap.String("from", msg.From))
		} else {
			s.logger.Info("Successfully updated TSS party with message",
				zap.String("session_id", msg.SessionID),
				zap.String("operation_id", operation.ID),
				zap.String("from", msg.From))
		}
	}()

	return nil
}

// GetOperation returns an operation by ID
// It first checks active operations in memory, then persistent storage for completed operations
func (s *Service) GetOperation(operationID string) (*Operation, bool) {
	// First check active operations in memory
	s.mutex.RLock()
	op, exists := s.operations[operationID]
	s.mutex.RUnlock()

	if exists {
		return op, true
	}

	// If not found in memory, check persistent storage for completed operations
	// Note: This returns a different data structure (OperationData),
	// but we can check if the operation exists
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

// CancelOperation cancels an operation
func (s *Service) CancelOperation(operationID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	operation, exists := s.operations[operationID]
	if !exists {
		return fmt.Errorf("operation not found: %s", operationID)
	}

	if operation.Status == StatusCompleted || operation.Status == StatusFailed {
		return fmt.Errorf("operation already finished: %s", operation.Status)
	}

	// Cancel the operation
	operation.cancel()
	operation.Status = StatusCancelled
	now := time.Now()
	operation.CompletedAt = &now

	s.logger.Info("Cancelled operation", zap.String("operation_id", operationID))

	// Move cancelled operation to persistent storage
	go func() {
		ctx := context.Background()
		if err := s.moveCompletedOperationToStorage(ctx, operationID); err != nil {
			s.logger.Error("Failed to move cancelled operation to persistent storage",
				zap.Error(err),
				zap.String("operation_id", operationID))
		}
	}()

	return nil
}

// handleOperationSync handles operation synchronization messages
func (s *Service) handleOperationSync(ctx context.Context, msg *p2p.Message) error {
	// Build mapping from sender node ID to P2P peer ID first
	if msg.SenderPeerID != "" {
		s.mappingMutex.Lock()
		s.nodeIDToPeerID[msg.From] = msg.SenderPeerID
		s.mappingMutex.Unlock()
		s.logger.Info("Updated node ID to peer ID mapping from operation sync",
			zap.String("node_id", msg.From),
			zap.String("peer_id", msg.SenderPeerID))
	}

	// Parse operation sync data from message data
	var baseData OperationSyncData
	if err := json.Unmarshal(msg.Data, &baseData); err != nil {
		s.logger.Error("Failed to unmarshal operation sync data", zap.Error(err))
		return fmt.Errorf("failed to unmarshal operation sync data: %w", err)
	}

	s.logger.Info("Received operation sync message",
		zap.String("operation_id", baseData.OperationID),
		zap.String("operation_type", baseData.OperationType),
		zap.String("session_id", baseData.SessionID),
		zap.String("from", msg.From),
		zap.String("sender_peer_id", msg.SenderPeerID),
		zap.Strings("participants", baseData.Participants))

	// Handle mapping-only messages (just for peer ID exchange)
	if baseData.OperationType == "mapping" {
		s.logger.Info("Received mapping-only message, mapping table updated")
		return nil // Mapping was already updated above
	}

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
	case "keygen":
		return s.createSyncedKeygenOperation(ctx, msg)
	case "signing":
		return s.createSyncedSigningOperation(ctx, msg)
	case "resharing":
		// TODO: implement resharing operation sync
		s.logger.Warn("Resharing operation sync not implemented yet")
		return nil
	default:
		return fmt.Errorf("unknown operation type: %s", baseData.OperationType)
	}
}

// broadcastOwnMapping broadcasts this node's mapping information to other nodes
func (s *Service) broadcastOwnMapping(ctx context.Context, sessionID string) {
	// Create a simple operation sync message containing only our mapping info
	go func() {
		syncData := &KeygenSyncData{
			OperationSyncData: OperationSyncData{
				OperationID:   "mapping_only",
				OperationType: "mapping",
				SessionID:     sessionID,
				Participants:  []string{s.nodeID},
			},
		}

		// Create a dummy operation sync message just to share our peer mapping
		if err := s.broadcastOperationSync(ctx, syncData); err != nil {
			s.logger.Error("Failed to broadcast own mapping",
				zap.Error(err),
				zap.String("node_id", s.nodeID))
		}
	}()
}

// handleOutgoingMessages handles outgoing TSS messages
func (s *Service) handleOutgoingMessages(ctx context.Context, operation *Operation) {
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
				continue
			}

			s.logger.Info("Processing message routing",
				zap.String("operation_id", operation.ID),
				zap.Bool("is_broadcast", routing.IsBroadcast),
				zap.Int("wire_bytes_len", len(wireBytes)),
				zap.Int("routing_to_count", len(routing.To)))

			// Create p2p message
			p2pMsg := &p2p.Message{
				SessionID:   operation.SessionID,
				Type:        string(operation.Type), // Set the message type based on operation type
				From:        s.nodeID,
				Data:        wireBytes,
				IsBroadcast: routing.IsBroadcast,
				Timestamp:   time.Now(),
			}

			// Send message through network
			if routing.IsBroadcast {
				s.logger.Info("Sending broadcast message",
					zap.String("operation_id", operation.ID),
					zap.String("session_id", operation.SessionID))
				if err := s.network.SendMessage(ctx, p2pMsg); err != nil {
					s.logger.Error("Failed to broadcast message",
						zap.Error(err),
						zap.String("operation_id", operation.ID))
				} else {
					s.logger.Info("Broadcast message sent successfully",
						zap.String("operation_id", operation.ID))
				}
			} else {
				for _, to := range routing.To {
					// Convert node ID to P2P peer ID
					s.mappingMutex.RLock()
					peerID, exists := s.nodeIDToPeerID[to.Id]
					s.mappingMutex.RUnlock()

					if !exists {
						// If we don't have the mapping, try using the node ID directly as fallback
						s.logger.Warn("No P2P peer ID mapping found for node ID, using node ID as fallback",
							zap.String("node_id", to.Id))
						peerID = to.Id
					}

					p2pMsg.To = []string{peerID}
					s.logger.Info("Sending direct message",
						zap.String("operation_id", operation.ID),
						zap.String("to_node_id", to.Id),
						zap.String("to_peer_id", peerID),
						zap.String("session_id", operation.SessionID))
					if err := s.network.SendMessage(ctx, p2pMsg); err != nil {
						s.logger.Error("Failed to send message",
							zap.String("to_node_id", to.Id),
							zap.String("to_peer_id", peerID),
							zap.Error(err),
							zap.String("operation_id", operation.ID))
					} else {
						s.logger.Info("Direct message sent successfully",
							zap.String("operation_id", operation.ID),
							zap.String("to_node_id", to.Id),
							zap.String("to_peer_id", peerID))
					}
				}
			}
		case <-ctx.Done():
			s.logger.Info("Outgoing message handler stopped",
				zap.String("operation_id", operation.ID),
				zap.Error(ctx.Err()))
			return
		}
	}
}

// loadKeyData loads key data from storage
func (s *Service) loadKeyData(ctx context.Context, keyID string) (*keygen.LocalPartySaveData, error) {
	data, err := s.storage.Load(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	var keyDataStruct KeyData
	if err := json.Unmarshal(data, &keyDataStruct); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key data struct: %w", err)
	}

	var saveData keygen.LocalPartySaveData
	if err := json.Unmarshal(keyDataStruct.KeyData, &saveData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal save data: %w", err)
	}

	return &saveData, nil
}

// loadKeyDataStruct loads the full KeyData structure from storage
func (s *Service) loadKeyDataStruct(ctx context.Context, keyID string) (*KeyData, error) {
	data, err := s.storage.Load(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load key data: %w", err)
	}

	var keyDataStruct KeyData
	if err := json.Unmarshal(data, &keyDataStruct); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key data struct: %w", err)
	}

	return &keyDataStruct, nil
}

// createParticipantList creates a list of party IDs from peer IDs
func (s *Service) createParticipantList(peerIDs []string) ([]*tss.PartyID, error) {
	var participants []*tss.PartyID

	// Sort peer IDs first for consistent ordering
	sortedPeerIDs := make([]string, len(peerIDs))
	copy(sortedPeerIDs, peerIDs)
	sort.Strings(sortedPeerIDs)

	// Use a large base number to avoid any issues with small integers
	baseKey := big.NewInt(1000000)

	for i, peerID := range sortedPeerIDs {
		// Create a unique key by adding index to base
		// This ensures keys are: 1000001, 1000002, 1000003, etc.
		key := new(big.Int).Add(baseKey, big.NewInt(int64(i+1)))

		// Use empty moniker for remote peers, or actual moniker if it's this node
		moniker := ""
		if peerID == s.nodeID {
			moniker = s.moniker
		}

		party := tss.NewPartyID(peerID, moniker, key)
		participants = append(participants, party)
	}
	return tss.SortPartyIDs(participants), nil
}

// broadcastOperationSync broadcasts operation synchronization message to all peers
func (s *Service) broadcastOperationSync(ctx context.Context, syncData Message) error {
	// Serialize sync data
	data, err := json.Marshal(syncData)
	if err != nil {
		return fmt.Errorf("failed to marshal sync data: %w", err)
	}

	msg := &p2p.Message{
		SessionID:    syncData.ID(),
		Type:         "operation_sync",
		From:         s.nodeID,
		To:           []string{},
		IsBroadcast:  true,
		Data:         data, // Serialized operation sync data
		Timestamp:    time.Now(),
		SenderPeerID: s.network.GetPeerID().String(),
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

	// Extract participant IDs
	for i, p := range operation.Participants {
		opData.Participants[i] = p.Id
	}

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

// handleOperationFailure handles operation failure by setting status and moving to persistent storage
func (s *Service) handleOperationFailure(ctx context.Context, operation *Operation, err error) {
	operation.Lock()
	operation.Status = StatusFailed
	operation.Error = err
	now := time.Now()
	operation.CompletedAt = &now
	operation.Unlock()

	// Move failed operation to persistent storage
	go func() {
		if moveErr := s.moveCompletedOperationToStorage(ctx, operation.ID); moveErr != nil {
			s.logger.Error("Failed to move failed operation to persistent storage",
				zap.Error(moveErr),
				zap.String("operation_id", operation.ID))
		}
	}()
}
