package security

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"go.uber.org/zap"
)

// messageEncryption implements the unified encryption interface
type messageEncryption struct {
	logger         *zap.Logger
	peerEncryption PeerEncryption
}

// NewMessageEncryption creates a new unified message encryption instance
func NewMessageEncryption(config *EncryptionConfig, logger *zap.Logger) (MessageEncryption, error) {
	if config.PrivateKey.Type() != crypto.Secp256k1 {
		return nil, fmt.Errorf("unsupported key type for peer encryption: %s", config.PrivateKey.Type())
	}

	me := &messageEncryption{
		logger:         logger,
		peerEncryption: NewSecp256k1PeerEncryption(config.PrivateKey, config.Peerstore),
	}

	logger.Info("Secp256k1 peer encryption initialized")
	return me, nil
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
