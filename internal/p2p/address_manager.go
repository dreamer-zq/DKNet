package p2p

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"
)

// AddressManager manages NodeID to PeerID mappings
type AddressManager struct {
	mu          sync.RWMutex
	addressBook *AddressBook
	filePath    string
	nodeID      string  // this node's NodeID
	peerID      peer.ID // this node's PeerID
	moniker     string
	logger      *zap.Logger

	// Broadcast callback
	broadcastCallback func(*AddressBook) error
}

// NewAddressManager creates a new address manager
func NewAddressManager(dataDir, nodeID string, peerID peer.ID, moniker string, logger *zap.Logger) (*AddressManager, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	filePath := filepath.Join(dataDir, "node_addresses.json")

	am := &AddressManager{
		addressBook: &AddressBook{
			Mappings:  make(map[string]*NodeAddressMapping),
			Version:   1,
			UpdatedAt: time.Now(),
		},
		filePath: filePath,
		nodeID:   nodeID,
		peerID:   peerID,
		moniker:  moniker,
		logger:   logger,
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
		zap.String("file_path", filePath),
		zap.String("node_id", nodeID),
		zap.String("peer_id", peerID.String()),
		zap.Int("existing_mappings", len(am.addressBook.Mappings)))

	return am, nil
}

// setBroadcastCallback sets the callback function for broadcasting address book updates
func (am *AddressManager) setBroadcastCallback(callback func(*AddressBook) error) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.broadcastCallback = callback
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
	if exists && existing.PeerID == peerIDStr && existing.Moniker == moniker {
		// No meaningful change, just update timestamp
		existing.Timestamp = now
		am.logger.Debug("Updated timestamp for existing mapping",
			zap.String("node_id", nodeID),
			zap.String("peer_id", peerIDStr))
		return nil
	}

	// Create or update mapping
	mapping := &NodeAddressMapping{
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

// getPeerID returns the PeerID for a given NodeID
func (am *AddressManager) getPeerID(nodeID string) (string, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	mapping, exists := am.addressBook.Mappings[nodeID]
	if !exists {
		return "", false
	}
	return mapping.PeerID, true
}

// getAllMappings returns a copy of all current mappings
func (am *AddressManager) getAllMappings() map[string]*NodeAddressMapping {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make(map[string]*NodeAddressMapping)
	for k, v := range am.addressBook.Mappings {
		// Create a copy to avoid race conditions
		mapping := *v
		result[k] = &mapping
	}
	return result
}

// getAddressBook returns a copy of the current address book
func (am *AddressManager) getAddressBook() *AddressBook {
	am.mu.RLock()
	defer am.mu.RUnlock()

	book := &AddressBook{
		Mappings:  make(map[string]*NodeAddressMapping),
		Version:   am.addressBook.Version,
		UpdatedAt: am.addressBook.UpdatedAt,
	}

	for k, v := range am.addressBook.Mappings {
		mapping := *v
		book.Mappings[k] = &mapping
	}

	return book
}

// mergeAddressBook merges received address book with local one
func (am *AddressManager) mergeAddressBook(remoteBook *AddressBook) error {
	if remoteBook == nil || remoteBook.Mappings == nil {
		return fmt.Errorf("invalid remote address book")
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	updated := false

	for nodeID, remoteMapping := range remoteBook.Mappings {
		if remoteMapping == nil {
			continue
		}

		// Skip our own mapping - we know it best
		if nodeID == am.nodeID {
			continue
		}

		// Validate the remote mapping
		if _, err := peer.Decode(remoteMapping.PeerID); err != nil {
			am.logger.Warn("Invalid peer ID in remote mapping, skipping",
				zap.String("node_id", nodeID),
				zap.String("peer_id", remoteMapping.PeerID),
				zap.Error(err))
			continue
		}

		localMapping, exists := am.addressBook.Mappings[nodeID]

		// Add new mapping or update if remote is newer
		if !exists || remoteMapping.Timestamp.After(localMapping.Timestamp) {
					// Create a copy of the remote mapping
		newMapping := &NodeAddressMapping{
			NodeID:    remoteMapping.NodeID,
			PeerID:    remoteMapping.PeerID,
			Timestamp: remoteMapping.Timestamp,
			Moniker:   remoteMapping.Moniker,
		}

			am.addressBook.Mappings[nodeID] = newMapping
			updated = true

			if exists {
				am.logger.Info("Updated mapping from remote address book",
					zap.String("node_id", nodeID),
					zap.String("old_peer_id", localMapping.PeerID),
					zap.String("new_peer_id", remoteMapping.PeerID),
					zap.Time("remote_timestamp", remoteMapping.Timestamp),
					zap.Time("local_timestamp", localMapping.Timestamp))
			} else {
				am.logger.Info("Added new mapping from remote address book",
					zap.String("node_id", nodeID),
					zap.String("peer_id", remoteMapping.PeerID),
					zap.String("moniker", remoteMapping.Moniker))
			}
		}
	}

	if updated {
		am.addressBook.Version++
		am.addressBook.UpdatedAt = time.Now()

		// Save to file
		if err := am.saveToFile(); err != nil {
			am.logger.Error("Failed to save merged address book to file", zap.Error(err))
		}
	}

	return nil
}

// broadcastOwnMapping broadcasts this node's mapping to the network
func (am *AddressManager) broadcastOwnMapping() error {
	am.mu.RLock()
	ownMapping, exists := am.addressBook.Mappings[am.nodeID]
	am.mu.RUnlock()

	if !exists {
		return fmt.Errorf("own mapping not found")
	}

	// Create a minimal address book with just our mapping
	book := &AddressBook{
		Mappings: map[string]*NodeAddressMapping{
			am.nodeID: ownMapping,
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}

	if am.broadcastCallback != nil {
		return am.broadcastCallback(book)
	}

	return nil
}

// broadcastFullAddressBook broadcasts the full address book to the network
func (am *AddressManager) broadcastFullAddressBook() error {
	book := am.getAddressBook()

	if am.broadcastCallback != nil {
		return am.broadcastCallback(book)
	}

	return nil
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
	validMappings := make(map[string]*NodeAddressMapping)
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

// startPeriodicBroadcast starts periodic broadcasting of the address book
func (am *AddressManager) startPeriodicBroadcast(interval time.Duration) {
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			if err := am.broadcastFullAddressBook(); err != nil {
				am.logger.Warn("Failed to broadcast address book periodically", zap.Error(err))
			} else {
				am.logger.Debug("Periodic address book broadcast completed")
			}
		}
	}()

	am.logger.Info("Started periodic address book broadcast",
		zap.Duration("interval", interval))
}
