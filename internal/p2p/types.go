package p2p

import (
	"context"
	"encoding/json"
	"time"

	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/dreamer-zq/DKNet/internal/common"
)

const (
	tssKeygenProtocol     = "/tss/keygen/1.0.0"
	tssSigningProtocol    = "/tss/signing/1.0.0"
	tssResharingProtocol  = "/tss/resharing/1.0.0"
	tssBroadcastTopic     = "tss-broadcast"
	addressDiscoveryTopic = "address-discovery"
)

var typeToProtocol = map[string]protocol.ID{
	"keygen":    tssKeygenProtocol,
	"signing":   tssSigningProtocol,
	"resharing": tssResharingProtocol,
}

// NodeMapping represents a mapping between NodeID and PeerID
type NodeMapping struct {
	NodeID    string    `json:"node_id"`
	PeerID    string    `json:"peer_id"`
	Timestamp time.Time `json:"timestamp"`
	// Additional node information
	Moniker string `json:"moniker,omitempty"`
}

// AddressBook holds all known node address mappings
type AddressBook struct {
	Mappings  map[string]*NodeMapping `json:"mappings"` // key: NodeID
	Version   int64                   `json:"version"`
	UpdatedAt time.Time               `json:"updated_at"`
}

// Compresses compresses and serializes the address book
func (m *AddressBook) Compresses() ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	compressed, err := common.Gzip(raw)
	if err != nil {
		return nil, err
	}
	return compressed, nil
}

// Decompresses decompresses and deserializes the address book
func (m *AddressBook) Decompresses(data []byte) error {
	decompressed, err := common.UnGzip(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(decompressed, m)
}

// Message represents a generic message sent over the network
type Message struct {
	ProtocolID  protocol.ID `json:"protocol_id"`
	SessionID   string      `json:"session_id"`
	Type        string      `json:"type"` // Message type for routing
	From        string      `json:"from"` // sender node ID
	To          []string    `json:"to"`   // recipient node IDs (empty for broadcast)
	IsBroadcast bool        `json:"is_broadcast"`
	Data        []byte      `json:"data"` // message payload
	Timestamp   time.Time   `json:"timestamp"`

	// P2P layer information - records original sender's actual PeerID to avoid mapping confusion from forwarding
	SenderPeerID string `json:"sender_peer_id,omitempty"` // actual P2P peer ID of original sender
}

// Compresses serializes and compresses the message
func (m *Message) Compresses() ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return common.Gzip(raw)
}

// Decompresses decompresses and deserializes the message
func (m *Message) Decompresses(data []byte) error {
	decompressed, err := common.UnGzip(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(decompressed, m)
}

// MessageHandler defines the interface for handling received messages
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message) error
}
