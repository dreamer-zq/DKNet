package p2p

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
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

// NodeMapping represents a mapping between NodeID and PeerID
type NodeMapping struct {
	NodeID    string    `json:"node_id"`
	PeerID    string    `json:"peer_id"`
	Timestamp time.Time `json:"timestamp"`
	// Additional node information
	Moniker   string   `json:"moniker,omitempty"`
}

// AddressBook holds all known node address mappings
type AddressBook struct {
	Mappings  map[string]*NodeMapping `json:"mappings"` // key: NodeID
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

	// P2P layer information - records original sender's actual PeerID to avoid mapping confusion from forwarding
	SenderPeerID string `json:"sender_peer_id,omitempty"` // actual P2P peer ID of original sender
}

// Marshal serializes and compresses the message
func (m *Message) Marshal() ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal decompresses and deserializes the message
func (m *Message) Unmarshal(data []byte) error {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, m)
}

// MessageHandler defines the interface for handling received messages
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message) error
}
