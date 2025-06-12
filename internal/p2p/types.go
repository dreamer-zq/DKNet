package p2p

import (
	"context"
	"time"
)

const (
	// Protocol IDs for different TSS operations
	tssKeygenProtocol    = "/tss/keygen/1.0.0"
	tssSigningProtocol   = "/tss/signing/1.0.0"
	tssResharingProtocol = "/tss/resharing/1.0.0"

	// PubSub topics
	tssDiscoveryTopic        = "tss-discovery"
	tssBroadcastTopic        = "tss-broadcast"
	nodeDiscoveryTopic       = "node-discovery" // New topic for node address discovery
	connectTimeout           = 30 * time.Second
	addressBroadcastInterval = 5 * time.Minute // Interval for broadcasting address book
)

// NodeAddressMapping represents a mapping between NodeID and PeerID
type NodeAddressMapping struct {
	NodeID    string    `json:"node_id"`
	PeerID    string    `json:"peer_id"`
	Timestamp time.Time `json:"timestamp"`
	// Additional node information
	Moniker   string   `json:"moniker,omitempty"`
}

// AddressBook holds all known node address mappings
type AddressBook struct {
	Mappings  map[string]*NodeAddressMapping `json:"mappings"` // key: NodeID
	Version   int64                          `json:"version"`
	UpdatedAt time.Time                      `json:"updated_at"`
}

// Message represents a generic message sent over the network
type Message struct {
	SessionID   string    `json:"session_id"`
	Type        string    `json:"type"` // Message type for routing
	From        string    `json:"from"` // sender node ID
	To          []string  `json:"to"`   // recipient node IDs (empty for broadcast)
	IsBroadcast bool      `json:"is_broadcast"`
	Data        []byte    `json:"data"` // message payload
	Timestamp   time.Time `json:"timestamp"`

	// P2P layer information
	SenderPeerID string `json:"sender_peer_id,omitempty"` // actual P2P peer ID of sender
}

// MessageHandler defines the interface for handling received messages
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message) error
}
