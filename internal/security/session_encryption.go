package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// SessionEncryption provides encryption/decryption functionality based on sessionID and seed key
type SessionEncryption struct {
	seedKey []byte // Seed key shared among participants
}

// SessionEncryptionInterface defines the interface for session-based encryption
type SessionEncryptionInterface interface {
	EncryptData(sessionID string, data []byte) ([]byte, error)
	DecryptData(sessionID string, encryptedData []byte) ([]byte, error)
}

// NewSessionEncryption creates a new SessionEncryption instance
func NewSessionEncryption(seedKey []byte) *SessionEncryption {
	return &SessionEncryption{
		seedKey: seedKey,
	}
}

// deriveKey derives encryption key from seed key and session ID using SHA256
// Key derivation: SHA256(seedKey + sessionID) -> 32 bytes for AES-256
func (se *SessionEncryption) deriveKey(sessionID string) []byte {
	hasher := sha256.New()
	hasher.Write(se.seedKey)
	hasher.Write([]byte(sessionID))
	return hasher.Sum(nil) // 32 bytes, perfect for AES-256
}

// EncryptData encrypts data using AES-256-GCM with session-derived key
func (se *SessionEncryption) EncryptData(sessionID string, data []byte) ([]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session ID cannot be empty")
	}
	
	key := se.deriveKey(sessionID)
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	
	// Encrypt data: nonce + encrypted_data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// DecryptData decrypts data using AES-256-GCM with session-derived key
func (se *SessionEncryption) DecryptData(sessionID string, encryptedData []byte) ([]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session ID cannot be empty")
	}
	
	key := se.deriveKey(sessionID)
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	
	// Extract nonce and ciphertext
	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
} 