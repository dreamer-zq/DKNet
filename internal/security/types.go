package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

// PeerEncryption provides end-to-end encryption between peers
type PeerEncryption interface {
	// EncryptForPeer encrypts data that only the target peer can decrypt
	EncryptForPeer(targetPeerID string, data []byte) ([]byte, error)
	// DecryptFromPeer decrypts data encrypted for this peer
	DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error)
}

// EncryptedEnvelope contains the encrypted symmetric key and data
type EncryptedEnvelope struct {
	Nonce           []byte `json:"nonce"`             // AES-GCM nonce
	Ciphertext      []byte `json:"ciphertext"`        // AES-GCM encrypted data
}

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

// encryptWithAESGCM encrypts data using AES-GCM
func encryptWithAESGCM(data, key []byte) (encryptedData, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encryptedData = gcm.Seal(nil, nonce, data, nil)
	return encryptedData, nonce, nil
}

// decryptWithAESGCM decrypts data using AES-GCM
func decryptWithAESGCM(encryptedData, nonce, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	decrypted, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return decrypted, nil
}