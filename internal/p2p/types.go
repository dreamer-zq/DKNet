package p2p

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/common"
)

const (
	// TssPartyProtocolID is the protocol ID for TSS party
	TssPartyProtocolID = "/tss/party/0.0.1"
)

// Message represents a generic message sent over the network
type Message struct {
	ProtocolID              protocol.ID `json:"protocol_id"`
	SessionID               string      `json:"session_id"`
	Type                    string      `json:"type"` // Message type for routing
	From                    string      `json:"from"` // sender node ID
	To                      []string    `json:"to"`   // recipient node IDs (empty for broadcast)
	IsBroadcast             bool        `json:"is_broadcast"`
	Data                    []byte      `json:"data"`                     // message payload
	Encrypted               bool        `json:"encrypted"`                // indicates if data is encrypted
	PeerEncrypted           bool        `json:"peer_encrypted,omitempty"` // indicates if data is encrypted for specific peer(s)
	Timestamp               time.Time   `json:"timestamp"`
	IsToOldCommittee        bool        `json:"is_to_old_committee,omitempty"`
	IsToOldAndNewCommittees bool        `json:"is_to_old_and_new_committees,omitempty"`

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

// Clone creates a deep copy of the message
func (m *Message) Clone() *Message {
	clone := *m
	clone.Data = make([]byte, len(m.Data))
	copy(clone.Data, m.Data)
	return &clone
}

// MessageHandler defines the interface for handling incoming P2P messages and events.
// This interface is implemented by the application layer (e.g., TSS service) to process
// messages and enforce security policies.
type MessageHandler interface {
	// HandleMessage processes an incoming message from a peer.
	HandleMessage(ctx context.Context, msg *Message) error
	// Stop gracefully stops the message handler.
	Stop()
}

// PeerDiscovery is the interface for the network layer
type PeerDiscovery interface {
	// Start starts the peer discovery
	Start() error
	// Stop stops the peer discovery
	Stop()
}

// NewPeerDiscovery creates a new peer discovery instance based on the configuration
func NewPeerDiscovery(h host.Host, logger *zap.Logger, conf *Config) PeerDiscovery {
	mod := strings.ToLower(conf.NetMod)
	if mod == "dht" {
		return NewDHT(h, conf.BootstrapPeers, logger)
	}
	return NewMDNS(h, logger)
}
