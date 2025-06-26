package tss

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/tss"
	"golang.org/x/crypto/sha3"

	"github.com/dreamer-zq/DKNet/internal/config"
)

// OperationType defines the type of TSS operation
type OperationType string

const (
	// OperationKeygen is the type for key generation operations
	OperationKeygen OperationType = "keygen"
	// OperationSigning is the type for signing operations
	OperationSigning OperationType = "signing"
	// OperationResharing is the type for resharing operations
	OperationResharing OperationType = "resharing"
	// OperationSync is the type for operation broadcast
	OperationSync OperationType = "operation_sync"
)

// Config holds TSS service configuration
type Config struct {
	PeerID  string
	Moniker string
	// Validation service configuration (optional)
	ValidationService *config.ValidationServiceConfig `json:"validation_service,omitempty"`
}

// Operation represents an active TSS operation
type Operation struct {
	ID           string
	Type         OperationType
	SessionID    string
	Participants []*tss.PartyID
	Party        tss.Party
	OutCh        chan tss.Message
	EndCh        chan any
	Status       OperationStatus
	CreatedAt    time.Time
	CompletedAt  *time.Time
	Result       any
	Error        error
	Request      any // Store the original request (KeygenRequest, SigningRequest, etc.)

	// Synchronization
	mutex  sync.RWMutex
	cancel context.CancelFunc
}

// Lock locks the operation
func (o *Operation) Lock() {
	o.mutex.Lock()
}

// Unlock unlocks the operation
func (o *Operation) Unlock() {
	o.mutex.Unlock()
}

// RLock locks the operation for reading
func (o *Operation) RLock() {
	o.mutex.RLock()
}

// RUnlock unlocks the operation for reading
func (o *Operation) RUnlock() {
	o.mutex.RUnlock()
}

func (o *Operation) isOldParticipant() bool {
	req, ok := o.Request.(*ResharingRequest)
	if !ok {
		return false
	}
	return slices.IndexFunc(req.OldParticipants, func(p string) bool {
		return p == o.Party.PartyID().GetId()
	}) != -1
}

func (o *Operation) isNewParticipant() bool {
	req, ok := o.Request.(*ResharingRequest)
	if !ok {
		return false
	}
	return slices.IndexFunc(req.NewParticipants, func(p string) bool {
		return p == o.Party.PartyID().GetId()
	}) != -1
}

// OperationStatus defines operation status
type OperationStatus string

const (
	// StatusPending is the status for pending operations
	StatusPending OperationStatus = "pending"
	// StatusInProgress is the status for in progress operations
	StatusInProgress OperationStatus = "in_progress"
	// StatusCompleted is the status for completed operations
	StatusCompleted OperationStatus = "completed"
	// StatusFailed is the status for failed operations
	StatusFailed OperationStatus = "failed"
	// StatusCancelled is the status for canceled operations
	StatusCancelled OperationStatus = "canceled"
)

// KeygenRequest represents a keygen request
type KeygenRequest struct {
	OperationID  string   `json:"operation_id,omitempty"` // Optional operation ID for idempotency
	Threshold    int      `json:"threshold"`
	Participants []string `json:"participants"` // peer IDs
}

// KeygenResult represents keygen result
type KeygenResult struct {
	PublicKey string `json:"public_key"`
	KeyID     string `json:"key_id"`
}

// SigningRequest represents a signing request
type SigningRequest struct {
	OperationID  string   `json:"operation_id,omitempty"` // Optional operation ID for idempotency
	Message      []byte   `json:"message"`
	KeyID        string   `json:"key_id"`
	Participants []string `json:"participants"` // peer IDs
}

// SigningResult represents signing result
type SigningResult struct {
	Signature string `json:"signature"`
	R         string `json:"r"`
	S         string `json:"s"`
	V         int    `json:"v"`
}

// ResharingRequest represents a resharing request
type ResharingRequest struct {
	OperationID     string   `json:"operation_id,omitempty"` // Optional operation ID for idempotency
	KeyID           string   `json:"key_id"`
	NewThreshold    int      `json:"new_threshold"`
	NewParties      int      `json:"new_parties"`
	OldParticipants []string `json:"old_participants"`
	NewParticipants []string `json:"new_participants"`
}

// Message is the interface for all operation sync data
type Message interface {
	ID() string
	To() []string
}

// OperationSyncData defines the base structure for operation sync data
type OperationSyncData struct {
	OperationID   string        `json:"operation_id"`
	OperationType OperationType `json:"operation_type"`
	SessionID     string        `json:"session_id"`
	Threshold     int           `json:"threshold"`
	Parties       int           `json:"parties"`
	Participants  []string      `json:"participants"`
}

// ID implement Message.ID
func (o *OperationSyncData) ID() string {
	return o.OperationID
}

// KeygenSyncData contains keygen-specific sync data
type KeygenSyncData struct {
	OperationSyncData
	// Add keygen-specific fields if needed in the future
}

// To implement Message.To
func (k *KeygenSyncData) To() []string {
	return k.Participants
}

// SigningSyncData contains signing-specific sync data
type SigningSyncData struct {
	OperationSyncData
	KeyID   string `json:"key_id"`
	Message []byte `json:"message"`
}

// To implement Message.To
func (s *SigningSyncData) To() []string {
	return s.Participants
}

// ResharingSyncData contains resharing-specific sync data
type ResharingSyncData struct {
	OperationSyncData
	OldThreshold    int      `json:"old_threshold"`
	NewThreshold    int      `json:"new_threshold"`
	OldParticipants []string `json:"old_participants"`
	NewParticipants []string `json:"new_participants"`
	KeyID           string   `json:"key_id"`
}

// To implement Message.To
func (r *ResharingSyncData) To() []string {
	return r.NewParticipants
}

// OperationData represents operation data for persistence
type OperationData struct {
	ID           string          `json:"id"`
	Type         OperationType   `json:"type"`
	SessionID    string          `json:"session_id"`
	Status       OperationStatus `json:"status"`
	Participants []string        `json:"participants"` // peer IDs
	Request      interface{}     `json:"request"`      // KeygenRequest, SigningRequest, or ResharingRequest
	Result       interface{}     `json:"result"`       // KeygenResult, SigningResult, etc.
	Error        string          `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// IsCompleted returns true if the operation has completed (success, failure, or cancellation)
func (o *OperationData) IsCompleted() bool {
	return o.Status == StatusCompleted || o.Status == StatusFailed || o.Status == StatusCancelled
}

// IsActive returns true if the operation is still active (pending or in progress)
func (o *OperationData) IsActive() bool {
	return o.Status == StatusPending || o.Status == StatusInProgress
}

// keyData represents the TSS key data that needs to be stored
type keyData struct {
	Moniker      string   `json:"moniker"`
	KeyData      []byte   `json:"key_data"`
	Threshold    int      `json:"threshold"`
	Participants []string `json:"participants"` // peer IDs
}

// hashMessageForEthereum creates an Ethereum-compatible hash that can be verified with ecrecover
func hashMessageForEthereum(message []byte) []byte {
	// Ethereum message prefix format: "\x19Ethereum Signed Message:\n" + len(message) + message
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	prefixedMessage := append([]byte(prefix), message...)

	// Use Keccak256 (not SHA3-256) as required by Ethereum
	hash := sha3.NewLegacyKeccak256()
	hash.Write(prefixedMessage)
	return hash.Sum(nil)
}
