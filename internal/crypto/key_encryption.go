package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// KeyEncryption handles encryption/decryption of TSS keys
type KeyEncryption struct {
	gcm cipher.AEAD
}

// NewKeyEncryption creates a new key encryption service
func NewKeyEncryption(password string) (*KeyEncryption, error) {
	if password == "" {
		return nil, fmt.Errorf("encryption password cannot be empty")
	}

	// Derive key from password using PBKDF2
	salt := []byte("dknet-tss-key-salt-v1") // Fixed salt for deterministic key derivation
	key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &KeyEncryption{
		gcm: gcm,
	}, nil
}

// Encrypt encrypts the given data
func (ke *KeyEncryption) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate random nonce
	nonce := make([]byte, ke.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	ciphertext := ke.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts the given data
func (ke *KeyEncryption) Decrypt(ciphertext []byte) ([]byte, error) {
	// Check minimum size (nonce + at least some data)
	nonceSize := ke.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and encrypted data
	nonce, encryptedData := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt the data
	plaintext, err := ke.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// VerifyPassword verifies if the password can decrypt a test payload
func (ke *KeyEncryption) VerifyPassword(testCiphertext []byte) error {
	_, err := ke.Decrypt(testCiphertext)
	return err
} 