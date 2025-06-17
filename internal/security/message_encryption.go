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
	// IsEncryptionAvailable returns true if encryption is available
	IsEncryptionAvailable() bool
}

// MessageEncryptionContext contains message context for encryption decisions
type MessageEncryptionContext struct {
	// Message data
	Data      []byte
	Encrypted bool

	// Routing information
	IsBroadcast  bool
	Recipients   []string // peer IDs for point-to-point
	SessionID    string   // for session-based encryption
	SenderPeerID string

	// Callback functions to update the original message
	SetData      func([]byte)
	SetEncrypted func(bool)
}

// EncryptionStrategy represents different encryption strategies
type EncryptionStrategy int

const (
	// StrategyNone indicates that no encryption is available
	StrategyNone EncryptionStrategy = iota
	// StrategySession indicates that session encryption is used
	StrategySession
	// StrategyPeerToPeer indicates that peer-to-peer encryption is used
	StrategyPeerToPeer
	// StrategyMultiRecipient indicates that multi-recipient encryption is used
	StrategyMultiRecipient
)

// EncryptionConfig contains configuration for the encryption manager
type EncryptionConfig struct {
	// Enable encryption (both peer-to-peer and session encryption)
	EnableEncryption bool
	// Session encryption seed key (required if EnableEncryption is true)
	SessionSeedKey []byte
	// Private key for peer encryption
	PrivateKey crypto.PrivKey
	// Peerstore for peer public keys
	Peerstore peerstore.Peerstore
}

// messageEncryption implements the unified encryption interface
type messageEncryption struct {
	sessionEncryption SessionEncryption
	peerEncryption    PeerEncryption
	logger            *zap.Logger
}

// NewMessageEncryption creates a new unified message encryption instance
func NewMessageEncryption(config *EncryptionConfig, logger *zap.Logger) MessageEncryption {
	me := &messageEncryption{
		logger:            logger.Named("message_encryption"),
		sessionEncryption: NewUnimplementedSessionEncryption(),
		peerEncryption:    NewUnimplementedPeerEncryption(),
	}

	// Initialize encryption modules based on EnableEncryption flag
	if config.EnableEncryption {
		// Initialize session encryption if seed key is provided
		if len(config.SessionSeedKey) > 0 {
			me.sessionEncryption = NewSessionEncryption(config.SessionSeedKey)
			logger.Info("Session encryption enabled for unified encryption")
		}

		// Initialize peer encryption if private key and peerstore are available
		if config.PrivateKey != nil && config.Peerstore != nil {
			if config.PrivateKey.Type() == crypto.Ed25519 {
				me.peerEncryption = NewEd25519PeerEncryption(config.PrivateKey, config.Peerstore)
				logger.Info("Peer encryption enabled (Ed25519 optimized) for unified encryption")
			} else {
				me.peerEncryption = NewPeerEncryption(config.PrivateKey, config.Peerstore)
				logger.Info("Peer encryption enabled (RSA) for unified encryption")
			}
		}
	} else {
		logger.Info("Unified encryption disabled")
	}
	return me
}

// Encrypt encrypts a message using the appropriate strategy
func (me *messageEncryption) Encrypt(msg *MessageEncryptionContext) error {
	// Skip if already encrypted
	if msg.Encrypted {
		return nil
	}

	// Determine encryption strategy
	strategy := me.determineEncryptionStrategy(msg)
	me.logger.Debug("Determined encryption strategy",
		zap.Int("strategy", int(strategy)),
		zap.Bool("is_broadcast", msg.IsBroadcast),
		zap.Int("recipients_count", len(msg.Recipients)),
		zap.String("session_id", msg.SessionID))

	switch strategy {
	case StrategyPeerToPeer:
		me.logger.Debug("Using peer-to-peer encryption strategy")
		return me.encryptForPeers(msg)
	case StrategySession:
		me.logger.Debug("Using session encryption strategy")
		return me.encryptWithSession(msg)
	case StrategyNone:
		me.logger.Debug("No encryption applied - no strategy available")
		return nil
	default:
		return fmt.Errorf("unsupported encryption strategy: %d", strategy)
	}
}

// Decrypt decrypts a message using the appropriate strategy
func (me *messageEncryption) Decrypt(msg *MessageEncryptionContext) error {
	// Skip if not encrypted
	if !msg.Encrypted {
		return nil
	}

	// Determine the likely encryption strategy used for this message
	strategy := me.determineEncryptionStrategy(msg)
	me.logger.Debug("Determined decryption strategy",
		zap.Int("strategy", int(strategy)),
		zap.Bool("is_broadcast", msg.IsBroadcast),
		zap.Int("recipients_count", len(msg.Recipients)),
		zap.String("session_id", msg.SessionID))

	// Try decryption based on the determined strategy
	switch strategy {
	case StrategyPeerToPeer:
		// Try peer decryption first for point-to-point messages
		if err := me.decryptFromPeer(msg); err == nil {
			return nil
		}
		// Fallback to session decryption if peer decryption fails
		me.logger.Debug("Peer decryption failed, trying session decryption as fallback")
		return me.decryptWithSession(msg)

	case StrategySession, StrategyMultiRecipient:
		// Try session decryption first
		if err := me.decryptWithSession(msg); err == nil {
			return nil
		}
		// Fallback to peer decryption if session decryption fails
		me.logger.Debug("Session decryption failed, trying peer decryption as fallback")
		return me.decryptFromPeer(msg)

	default:
		// Try both methods for unknown strategy
		if err := me.decryptFromPeer(msg); err == nil {
			return nil
		}
		if err := me.decryptWithSession(msg); err == nil {
			return nil
		}

		me.logger.Warn("Unable to decrypt message with any available method")
		return fmt.Errorf("unable to decrypt message: all decryption methods failed")
	}
}

// IsEncryptionAvailable returns true if any encryption method is available
func (me *messageEncryption) IsEncryptionAvailable() bool {
	return me.sessionEncryption != nil || me.peerEncryption != nil
}

// determineEncryptionStrategy determines the best encryption strategy for a message
func (me *messageEncryption) determineEncryptionStrategy(msg *MessageEncryptionContext) EncryptionStrategy {
	// For broadcast messages, only session encryption is suitable
	if msg.IsBroadcast {
		if msg.SessionID != "" {
			return StrategySession
		}
		return StrategyNone
	}

	// For point-to-point messages, prefer peer encryption
	if len(msg.Recipients) == 1 {
		return StrategyPeerToPeer
	}

	// For multiple recipients, use session encryption
	if len(msg.Recipients) > 1 && msg.SessionID != "" {
		return StrategyMultiRecipient // This will fallback to session encryption
	}

	// Fallback to session encryption if available
	if msg.SessionID != "" {
		return StrategySession
	}

	return StrategyNone
}

// encryptForPeers encrypts message for specific peers
func (me *messageEncryption) encryptForPeers(msg *MessageEncryptionContext) error {
	if len(msg.Recipients) != 1 {
		return fmt.Errorf("peer encryption only supports single recipient")
	}

	targetPeerID := msg.Recipients[0]
	encryptedData, err := me.peerEncryption.EncryptForPeer(targetPeerID, msg.Data)
	if err != nil {
		// Fallback to session encryption if peer encryption fails
		me.logger.Warn("Peer encryption failed, falling back to session encryption",
			zap.String("target_peer", targetPeerID),
			zap.Error(err))

		if msg.SessionID != "" {
			return me.encryptWithSession(msg)
		}
		return fmt.Errorf("failed to encrypt for peer %s: %w", targetPeerID, err)
	}

	// Update message through callbacks
	msg.SetData(encryptedData)
	msg.SetEncrypted(true)

	me.logger.Debug("Message encrypted for peer",
		zap.String("target_peer", targetPeerID))

	return nil
}

// encryptWithSession encrypts message using session encryption
func (me *messageEncryption) encryptWithSession(msg *MessageEncryptionContext) error {
	encryptedData, err := me.sessionEncryption.Encrypt(msg.SessionID, msg.Data)
	if err != nil {
		return fmt.Errorf("failed to encrypt with session encryption: %w", err)
	}

	// Update message through callbacks
	msg.SetData(encryptedData)
	msg.SetEncrypted(true)

	me.logger.Debug("Message encrypted with session encryption",
		zap.String("session_id", msg.SessionID))

	return nil
}

// decryptFromPeer decrypts message from a specific peer
func (me *messageEncryption) decryptFromPeer(msg *MessageEncryptionContext) error {
	decryptedData, err := me.peerEncryption.DecryptFromPeer(msg.SenderPeerID, msg.Data)
	if err != nil {
		return fmt.Errorf("failed to decrypt from peer: %w", err)
	}

	// Update message through callbacks
	msg.SetData(decryptedData)
	msg.SetEncrypted(false)

	me.logger.Debug("Message decrypted from peer",
		zap.String("sender_peer", msg.SenderPeerID))

	return nil
}

// decryptWithSession decrypts message using session encryption
func (me *messageEncryption) decryptWithSession(msg *MessageEncryptionContext) error {
	decryptedData, err := me.sessionEncryption.Decrypt(msg.SessionID, msg.Data)
	if err != nil {
		return fmt.Errorf("failed to decrypt with session encryption: %w", err)
	}

	// Update message through callbacks
	msg.SetData(decryptedData)
	msg.SetEncrypted(false)

	me.logger.Debug("Message decrypted with session encryption",
		zap.String("session_id", msg.SessionID))

	return nil
}
