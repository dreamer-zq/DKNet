package p2p

import (
	"context"
	"encoding/json"
	"time"

	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/dreamer-zq/DKNet/internal/common"
)

const (
	tssKeygenProtocol    = "/tss/keygen/1.0.0"
	tssSigningProtocol   = "/tss/signing/1.0.0"
	tssResharingProtocol = "/tss/resharing/1.0.0"
	tssGossipProtocol    = "/tss/gossip/1.0.0"
	tssBroadcastTopic    = "tss-broadcast"
)

var typeToProtocol = map[string]protocol.ID{
	"keygen":       tssKeygenProtocol,
	"signing":      tssSigningProtocol,
	"resharing":    tssResharingProtocol,
	"gossip_route": tssGossipProtocol,
}

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

// MessageHandler defines the interface for handling received messages
type MessageHandler interface {
	HandleMessage(ctx context.Context, msg *Message) error
}

// RoutedMessage wraps a message with routing information
type RoutedMessage struct {
	*Message
	OriginalSender string   `json:"original_sender"`
	FinalTarget    string   `json:"final_target"`
	Path           []string `json:"path"`
	TTL            int      `json:"ttl"`
	MessageID      string   `json:"message_id"`
}

// Compresses serializes and compresses the routed message
func (rm *RoutedMessage) Compresses() ([]byte, error) {
	raw, err := json.Marshal(rm)
	if err != nil {
		return nil, err
	}
	return common.Gzip(raw)
}

// Decompresses decompresses and deserializes the routed message
func (rm *RoutedMessage) Decompresses(data []byte) error {
	decompressed, err := common.UnGzip(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(decompressed, rm)
}
