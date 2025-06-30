package security

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"go.uber.org/zap"
)

// MessageEncryption provides unified encryption interface for all message types
type MessageEncryption interface {
	// Encrypt encrypts a message based on its routing strategy
	Encrypt(msg *MessageEncryptionContext) error
	// Decrypt decrypts a message
	Decrypt(msg *MessageEncryptionContext) error
}

// MessageEncryptionContext contains message context for encryption decisions
type MessageEncryptionContext struct {
	// Message data
	Data         []byte
	Encrypted    bool
	Recipient    string // peer IDs for point-to-point
	SenderPeerID string

	// Callback functions to update the original message
	SetData      func([]byte)
	SetEncrypted func(bool)
}

// EncryptionConfig contains configuration for the encryption manager
type EncryptionConfig struct {
	// Private key for peer encryption
	PrivateKey crypto.PrivKey
	// Peerstore for peer public keys
	Peerstore peerstore.Peerstore
}

// messageEncryption implements the unified encryption interface
type messageEncryption struct {
	logger         *zap.Logger
	peerEncryption PeerEncryption
}

// NewMessageEncryption creates a new unified message encryption instance
func NewMessageEncryption(config *EncryptionConfig, logger *zap.Logger) MessageEncryption {
	me := &messageEncryption{
		logger: logger,
	}

	// Initialize peer encryption if enabled and private key provided
	// Choose the appropriate peer encryption based on key type
	switch config.PrivateKey.Type() {
	case crypto.RSA:
		me.peerEncryption = NewPeerEncryption(config.PrivateKey, config.Peerstore)
		logger.Info("RSA peer encryption initialized")
	case crypto.Ed25519:
		me.peerEncryption = NewEd25519PeerEncryption(config.PrivateKey, config.Peerstore)
		logger.Info("Ed25519 peer encryption initialized")
	default:
		logger.Warn("Unsupported key type for peer encryption, using no-op implementation",
			zap.String("key_type", config.PrivateKey.Type().String()))
		me.peerEncryption = NewUnimplementedPeerEncryption()
	}
	return me
}

// Encrypt encrypts a message using the appropriate strategy
func (me *messageEncryption) Encrypt(msg *MessageEncryptionContext) error {
	// Skip if already encrypted
	if msg.Encrypted {
		return nil
	}

	me.logger.Debug("Attempting peer encryption",
		zap.String("target_peer", msg.Recipient),
		zap.String("sender_peer", msg.SenderPeerID),
		zap.Int("data_len", len(msg.Data)))

	encryptedData, err := me.peerEncryption.EncryptForPeer(msg.Recipient, msg.Data)
	if err != nil {
		// Fallback to session encryption if peer encryption fails
		me.logger.Warn("Peer encryption failed, falling back to session encryption",
			zap.String("target_peer", msg.Recipient),
			zap.String("sender_peer", msg.SenderPeerID),
			zap.Error(err))
		return fmt.Errorf("failed to encrypt for peer %s: %w", msg.Recipient, err)
	}

	// Update message through callbacks
	msg.SetData(encryptedData)
	msg.SetEncrypted(true)

	me.logger.Debug("Message encrypted for peer",
		zap.String("target_peer", msg.Recipient),
		zap.String("sender_peer", msg.SenderPeerID))

	return nil
}

// Decrypt decrypts a message using the appropriate strategy
func (me *messageEncryption) Decrypt(msg *MessageEncryptionContext) error {
	// Skip if not encrypted
	if !msg.Encrypted {
		return nil
	}

	me.logger.Debug("Attempting peer decryption",
		zap.String("sender_peer", msg.SenderPeerID),
		zap.Int("data_len", len(msg.Data)))

	if me.peerEncryption == nil {
		return fmt.Errorf("peer encryption not available")
	}

	if msg.SenderPeerID == "" {
		me.logger.Warn("Empty sender peer ID for peer decryption")
		return fmt.Errorf("empty sender peer ID")
	}

	decryptedData, err := me.peerEncryption.DecryptFromPeer(msg.SenderPeerID, msg.Data)
	if err != nil {
		me.logger.Debug("Peer decryption failed",
			zap.String("sender_peer", msg.SenderPeerID),
			zap.Error(err))
		return fmt.Errorf("failed to decrypt from peer: %w", err)
	}

	// Update message through callbacks
	msg.SetData(decryptedData)
	msg.SetEncrypted(false)

	me.logger.Debug("Message decrypted from peer",
		zap.String("sender_peer", msg.SenderPeerID))

	return nil
}
