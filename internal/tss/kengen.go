package tss

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v2/tss"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"

	"github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/p2p"
)

// keygenOperationParams contains parameters for creating a keygen operation
type keygenOperationParams struct {
	OperationID  string
	SessionID    string
	Threshold    int
	Participants []string
	UsePreParams bool // Whether to use pre-computed parameters for faster keygen
}

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

	// Create the keygen operation using common logic
	operation, err := s.createAndStartKeygenOperation(&keygenOperationParams{
		OperationID:  operationID,
		SessionID:    sessionID,
		Threshold:    threshold,
		Participants: participants,
		UsePreParams: false, // Don't use pre-computed parameters for standard keygen
	})
	if err != nil {
		return nil, err
	}

	// Broadcast keygen operation sync message to other participants
	common.SafeGo(operation.EndCh, func() any {
		return s.syncKeygenOperation(operationID, sessionID, threshold, participants)
	})

	return operation, nil
}

// createAndStartKeygenOperation creates a keygen operation with shared logic
func (s *Service) createAndStartKeygenOperation(params *keygenOperationParams) (*Operation, error) {
	// Create participant list
	participantList, err := s.createParticipantList(params.Participants)
	if err != nil {
		return nil, fmt.Errorf("failed to create participant list: %w", err)
	}

	// Find our party ID in the participants list
	ourPartyIndex := slices.IndexFunc(participantList, func(p *tss.PartyID) bool {
		return p.Id == s.nodeID
	})
	if ourPartyIndex == -1 {
		return nil, fmt.Errorf("this node (%s) is not in the participant list", s.nodeID)
	}

	ourPartyID := participantList[ourPartyIndex]

	// Log party ID for sync operations
	if params.UsePreParams {
		s.logger.Info("Found our party ID for synced operation", zap.String("party_id", ourPartyID.Id))
	}

	// Create TSS parameters
	peerCtx := tss.NewPeerContext(participantList)
	tssParams := tss.NewParameters(tss.S256(), peerCtx, ourPartyID, len(params.Participants), params.Threshold)

	// Create channels
	outCh := make(chan tss.Message, 100)
	endCh := make(chan *keygen.LocalPartySaveData, 1)

	// Create keygen party - with or without pre-computed parameters
	var party tss.Party
	if params.UsePreParams {
		// Pre-compute parameters for faster keygen (used in sync operations)
		preParams, err := keygen.GeneratePreParams(1 * time.Minute)
		if err != nil {
			s.logger.Error("Failed to generate pre-params for synced operation", zap.Error(err))
			return nil, fmt.Errorf("failed to generate pre-params: %w", err)
		}
		party = keygen.NewLocalParty(tssParams, outCh, endCh, *preParams)
	} else {
		// Standard keygen party without pre-computed parameters
		party = keygen.NewLocalParty(tssParams, outCh, endCh)
	}

	// Create operation context with cancellation - use background context to avoid HTTP timeout
	// Set a longer timeout for keygen operations (10 minutes)
	operationCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Create request for storage
	req := &KeygenRequest{
		OperationID:  params.OperationID,
		Threshold:    params.Threshold,
		Participants: params.Participants,
	}

	operation := &Operation{
		ID:           params.OperationID,
		Type:         OperationKeygen,
		SessionID:    params.SessionID,
		Participants: participantList,
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

	// Log for sync operations
	if params.UsePreParams {
		s.logger.Info("Synced operation stored, starting keygen goroutine",
			zap.String("operation_id", params.OperationID),
			zap.String("session_id", params.SessionID))
	}

	// Wait for operation completion or cancellation
	go s.watchOperation(operationCtx, operation)
	// Start operation in a goroutine
	go s.runOperation(operationCtx, operation)

	return operation, nil
}

func (s *Service) syncKeygenOperation(
	operationID, sessionID string,
	threshold int,
	participants []string,
) error {
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

	if err := s.syncOperation(syncCtx, syncData); err != nil {
		s.logger.Error("Failed to broadcast keygen operation sync",
			zap.Error(err),
			zap.String("operation_id", operationID))
		return err
	}
	s.logger.Info("Keygen operation sync broadcasted successfully",
		zap.String("operation_id", operationID))
	return nil
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
		zap.Int("original_size", len(keyDataBytes)),
	)

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

	// Create the keygen operation using common logic with pre-computed parameters
	_, err := s.createAndStartKeygenOperation(&keygenOperationParams{
		OperationID:  syncData.OperationID,
		SessionID:    syncData.SessionID,
		Threshold:    syncData.Threshold,
		Participants: syncData.Participants,
		UsePreParams: false, // Use pre-computed parameters for sync operations
	})
	if err != nil {
		s.logger.Error("Failed to create synced keygen operation", zap.Error(err))
		return fmt.Errorf("failed to create synced keygen operation: %w", err)
	}

	s.logger.Info("Started synced keygen operation",
		zap.String("operation_id", syncData.OperationID),
		zap.String("session_id", syncData.SessionID))

	return nil
}
