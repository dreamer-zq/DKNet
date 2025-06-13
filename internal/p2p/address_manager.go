package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"
)

// transport defines the interface for low-level network operations
type transport interface {
	// JoinTopic joins a pubsub topic and returns the topic handle
	JoinTopic(topicName string) (*pubsub.Topic, error)
	// GetHostID returns the local peer ID
	GetHostID() peer.ID
}

// AddressManager manages NodeID to PeerID mappings
type AddressManager struct {
	mu          sync.RWMutex
	addressBook *AddressBook
	filePath    string
	nodeID      string  // this node's NodeID
	peerID      peer.ID // this node's PeerID
	logger      *zap.Logger

	// Broadcast configuration and control
	broadcastTicker *time.Ticker
	stopBroadcast   chan struct{}

	// Network transport for low-level operations
	transport transport

	// PubSub topic for address book discovery
	discoveryTopic *pubsub.Topic
	subscription   *pubsub.Subscription

	// Context for managing goroutines
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAddressManager creates a new address manager
func NewAddressManager(
	dataDir, nodeID string,
	peerID peer.ID,
	moniker string,
	broadcastIntervalStr string,
	transport transport,
	logger *zap.Logger,
) (*AddressManager, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	// Parse broadcast interval
	broadcastInterval, err := time.ParseDuration(broadcastIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid address book broadcast interval: %w", err)
	}

	// Set default broadcast interval if not specified and broadcasting is enabled
	if broadcastInterval <= 0 {
		broadcastInterval = 5 * time.Minute
	}

	// Create context for managing goroutines
	ctx, cancel := context.WithCancel(context.Background())

	am := &AddressManager{
		addressBook: &AddressBook{
			Mappings:  make(map[string]*NodeMapping),
			Version:   1,
			UpdatedAt: time.Now(),
		},
		filePath:        filepath.Join(dataDir, "node_addresses.json"),
		nodeID:          nodeID,
		peerID:          peerID,
		logger:          logger,
		broadcastTicker: time.NewTicker(broadcastInterval),
		stopBroadcast:   make(chan struct{}),
		transport:       transport,
		ctx:             ctx,
		cancel:          cancel,
	}

	// Load existing address book if it exists
	if err := am.loadFromFile(); err != nil {
		logger.Warn("Failed to load existing address book, starting fresh", zap.Error(err))
	}

	// Add/update this node's mapping
	if err := am.updateMapping(nodeID, peerID.String(), moniker); err != nil {
		return nil, fmt.Errorf("failed to add own mapping: %w", err)
	}

	logger.Info("Address manager initialized",
		zap.String("node_id", nodeID),
		zap.String("peer_id", peerID.String()),
		zap.Int("existing_mappings", len(am.addressBook.Mappings)),
		zap.Duration("broadcast_interval", broadcastInterval))

	return am, nil
}

// Start initializes the address manager's network operations
func (am *AddressManager) Start() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Join the discovery topic
	topic, err := am.transport.JoinTopic(addressDiscoveryTopic)
	if err != nil {
		return fmt.Errorf("failed to join discovery topic: %w", err)
	}
	am.discoveryTopic = topic

	// Subscribe to the topic
	subscription, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to discovery topic: %w", err)
	}
	am.subscription = subscription

	// Start listening for address book updates
	go am.handleIncomingAddressBooks()

	// Start broadcasting
	am.startBroadcasting()
	return nil
}

// Stop gracefully shuts down the address manager
func (am *AddressManager) Stop() {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Cancel context to stop all goroutines
	if am.cancel != nil {
		am.cancel()
	}

	// Stop broadcasting
	am.stopBroadcasting()

	// Close subscription
	if am.subscription != nil {
		am.subscription.Cancel()
		am.subscription = nil
	}

	// Close topic
	if am.discoveryTopic != nil {
		if err := am.discoveryTopic.Close(); err != nil {
			am.logger.Warn("Failed to close discovery topic", zap.Error(err))
		}
		am.discoveryTopic = nil
	}

	am.logger.Info("Address manager stopped")
}

// handleIncomingAddressBooks processes incoming address book updates
func (am *AddressManager) handleIncomingAddressBooks() {
	for {
		select {
		case <-am.ctx.Done():
			return
		default:
			msg, err := am.subscription.Next(am.ctx)
			if err != nil {
				if am.ctx.Err() != nil {
					// Context canceled, normal shutdown
					return
				}
				am.logger.Error("Failed to receive message from discovery topic", zap.Error(err))
				continue
			}

			// Skip messages from ourselves
			if msg.ReceivedFrom == am.transport.GetHostID() {
				continue
			}

			// Parse the address book
			var receivedBook AddressBook
			if err := receivedBook.Decompresses(msg.Data); err != nil {
				am.logger.Warn("Failed to unmarshal received address book", zap.Error(err))
				continue
			}

			// Merge the received address book
			am.mergeAddressBook(&receivedBook)
		}
	}
}

// broadcastAddressBook sends the current address book to the network
func (am *AddressManager) broadcastAddressBook() error {
	am.mu.RLock()
	book := am.getAddressBookCopy()
	am.mu.RUnlock()

	data, err := book.Compresses()
	if err != nil {
		return fmt.Errorf("failed to marshal address book: %w", err)
	}

	if am.discoveryTopic == nil {
		return fmt.Errorf("discovery topic not initialized")
	}

	if err := am.discoveryTopic.Publish(am.ctx, data); err != nil {
		return fmt.Errorf("failed to publish address book: %w", err)
	}

	am.logger.Info("Address book broadcasted")
	return nil
}

// updateMapping updates or adds a node address mapping
func (am *AddressManager) updateMapping(nodeID, peerIDStr, moniker string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Validate peer ID
	_, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("invalid peer ID %s: %w", peerIDStr, err)
	}

	now := time.Now()
	existing, exists := am.addressBook.Mappings[nodeID]

	// Check if this is a meaningful update
	if exists && existing.PeerID == peerIDStr {
		// No meaningful change, just update timestamp
		existing.Timestamp = now
		am.logger.Debug("Updated timestamp for existing mapping",
			zap.String("node_id", nodeID),
			zap.String("peer_id", peerIDStr))
		return nil
	}

	// Create or update mapping
	mapping := &NodeMapping{
		NodeID:    nodeID,
		PeerID:    peerIDStr,
		Timestamp: now,
		Moniker:   moniker,
	}

	am.addressBook.Mappings[nodeID] = mapping
	am.addressBook.Version++
	am.addressBook.UpdatedAt = now

	// Save to file
	if err := am.saveToFile(); err != nil {
		am.logger.Error("Failed to save address book to file", zap.Error(err))
		// Don't return error, keep the in-memory update
	}

	if exists {
		am.logger.Info("Updated node address mapping",
			zap.String("node_id", nodeID),
			zap.String("old_peer_id", existing.PeerID),
			zap.String("new_peer_id", peerIDStr),
			zap.String("moniker", moniker))
	} else {
		am.logger.Info("Added new node address mapping",
			zap.String("node_id", nodeID),
			zap.String("peer_id", peerIDStr),
			zap.String("moniker", moniker))
	}

	return nil
}

// GetPeerID returns the PeerID for a given NodeID
func (am *AddressManager) GetPeerID(nodeID string) (string, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	mapping, exists := am.addressBook.Mappings[nodeID]
	if !exists {
		return "", false
	}
	return mapping.PeerID, true
}

// GetAllMappings returns a copy of all current mappings
func (am *AddressManager) GetAllMappings() map[string]*NodeMapping {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make(map[string]*NodeMapping)
	for k, v := range am.addressBook.Mappings {
		// Create a copy to avoid race conditions
		mapping := *v
		result[k] = &mapping
	}
	return result
}

// getAddressBookCopy returns a copy of the current address book
func (am *AddressManager) getAddressBookCopy() *AddressBook {
	book := &AddressBook{
		Mappings:  make(map[string]*NodeMapping),
		Version:   am.addressBook.Version,
		UpdatedAt: am.addressBook.UpdatedAt,
	}

	for nodeID, mapping := range am.addressBook.Mappings {
		book.Mappings[nodeID] = &NodeMapping{
			NodeID:    mapping.NodeID,
			PeerID:    mapping.PeerID,
			Moniker:   mapping.Moniker,
			Timestamp: mapping.Timestamp,
		}
	}

	return book
}

// mergeAddressBook merges a received address book with the local one
func (am *AddressManager) mergeAddressBook(receivedBook *AddressBook) {
	am.mu.Lock()
	defer am.mu.Unlock()

	updated := false
	for nodeID, receivedMapping := range receivedBook.Mappings {
		// Skip our own mapping
		if nodeID == am.nodeID {
			continue
		}

		existingMapping, exists := am.addressBook.Mappings[nodeID]
		if !exists || receivedMapping.Timestamp.After(existingMapping.Timestamp) {
			am.addressBook.Mappings[nodeID] = &NodeMapping{
				NodeID:    receivedMapping.NodeID,
				PeerID:    receivedMapping.PeerID,
				Moniker:   receivedMapping.Moniker,
				Timestamp: receivedMapping.Timestamp,
			}
			updated = true
			am.logger.Debug("Updated mapping from received address book",
				zap.String("node_id", nodeID),
				zap.String("peer_id", receivedMapping.PeerID),
				zap.String("moniker", receivedMapping.Moniker))
		}
	}

	if updated {
		am.addressBook.UpdatedAt = time.Now()
		if err := am.saveToFile(); err != nil {
			am.logger.Error("Failed to save address book after merge", zap.Error(err))
		}
	}
}

// loadFromFile loads address book from file
func (am *AddressManager) loadFromFile() error {
	data, err := os.ReadFile(am.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, which is fine
		}
		return fmt.Errorf("failed to read address book file: %w", err)
	}

	var book AddressBook
	if err := json.Unmarshal(data, &book); err != nil {
		return fmt.Errorf("failed to unmarshal address book: %w", err)
	}

	// Validate loaded mappings
	validMappings := make(map[string]*NodeMapping)
	for nodeID, mapping := range book.Mappings {
		if mapping == nil {
			continue
		}

		// Validate peer ID
		if _, err := peer.Decode(mapping.PeerID); err != nil {
			am.logger.Warn("Invalid peer ID in saved mapping, skipping",
				zap.String("node_id", nodeID),
				zap.String("peer_id", mapping.PeerID),
				zap.Error(err))
			continue
		}

		validMappings[nodeID] = mapping
	}

	am.addressBook.Mappings = validMappings
	am.addressBook.Version = book.Version
	am.addressBook.UpdatedAt = book.UpdatedAt

	am.logger.Info("Loaded address book from file",
		zap.String("file_path", am.filePath),
		zap.Int("mappings_count", len(validMappings)),
		zap.Int64("version", book.Version))

	return nil
}

// saveToFile saves address book to file
func (am *AddressManager) saveToFile() error {
	data, err := json.MarshalIndent(am.addressBook, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal address book: %w", err)
	}

	// Write to temporary file first, then rename (atomic operation)
	tempFile := am.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempFile, am.filePath); err != nil {
		// Clean up temp file on error
		if removeErr := os.Remove(tempFile); removeErr != nil {
			am.logger.Warn("Failed to remove temporary file", zap.String("file", tempFile), zap.Error(removeErr))
		}
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	am.logger.Debug("Saved address book to file",
		zap.String("file_path", am.filePath),
		zap.Int("mappings_count", len(am.addressBook.Mappings)))

	return nil
}

// startBroadcasting begins periodic broadcasting of the address book
func (am *AddressManager) startBroadcasting() {
	go func() {
		defer am.broadcastTicker.Stop()

		// Broadcast immediately on start
		if err := am.broadcastAddressBook(); err != nil {
			am.logger.Error("Failed to broadcast address book on start", zap.Error(err))
		}

		for {
			select {
			case <-am.ctx.Done():
				return
			case <-am.stopBroadcast:
				return
			case <-am.broadcastTicker.C:
				if err := am.broadcastAddressBook(); err != nil {
					am.logger.Error("Failed to broadcast address book", zap.Error(err))
				}
			}
		}
	}()
}

// stopBroadcasting stops the periodic broadcasting
func (am *AddressManager) stopBroadcasting() {
	am.broadcastTicker.Stop()
	am.broadcastTicker = nil

	// Signal stop to broadcasting goroutine
	am.stopBroadcast <- struct{}{}

	am.logger.Info("Stopped broadcasting address book")
}
