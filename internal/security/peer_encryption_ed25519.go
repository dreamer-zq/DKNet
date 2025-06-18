package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"golang.org/x/crypto/hkdf"
)

// Ed25519EncryptedEnvelope contains the encrypted data for Ed25519-based encryption
type Ed25519EncryptedEnvelope struct {
	EncryptedData []byte `json:"encrypted_data"` // AES-GCM encrypted data
	Nonce         []byte `json:"nonce"`          // AES-GCM nonce
	SenderID      string `json:"sender_id"`      // Sender peer ID for key derivation
}

// ed25519PeerEncryption implements PeerEncryption for Ed25519 keys using shared key derivation
type ed25519PeerEncryption struct {
	privateKey crypto.PrivKey
	peerstore  peerstore.Peerstore
	ownPeerID  peer.ID
}

// NewEd25519PeerEncryption creates a new Ed25519-based PeerEncryption instance
func NewEd25519PeerEncryption(privateKey crypto.PrivKey, ps peerstore.Peerstore) PeerEncryption {
	ownPeerID, _ := peer.IDFromPrivateKey(privateKey)
	return &ed25519PeerEncryption{
		privateKey: privateKey,
		peerstore:  ps,
		ownPeerID:  ownPeerID,
	}
}

// EncryptForPeer encrypts data using shared key derivation with Ed25519 keys
func (pe *ed25519PeerEncryption) EncryptForPeer(targetPeerID string, data []byte) ([]byte, error) {
	// Parse target peer ID
	peerID, err := peer.Decode(targetPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID: %w", err)
	}

	// Get target peer's public key
	targetPubKey := pe.peerstore.PubKey(peerID)
	if targetPubKey == nil {
		return nil, fmt.Errorf("public key not found for peer %s", targetPeerID)
	}

	// Verify it's an Ed25519 key
	if targetPubKey.Type() != crypto.Ed25519 {
		return nil, fmt.Errorf("unsupported key type for Ed25519 encryption: %s", targetPubKey.Type())
	}

	// Derive shared key using a unified method
	sharedKey, err := pe.deriveUnifiedKey(pe.ownPeerID.String(), targetPeerID, pe.privateKey.GetPublic(), targetPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive shared key: %w", err)
	}

	// Encrypt data with AES-GCM
	encryptedData, nonce, err := encryptWithAESGCM(data, sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Create envelope
	envelope := Ed25519EncryptedEnvelope{
		EncryptedData: encryptedData,
		Nonce:         nonce,
		SenderID:      pe.ownPeerID.String(),
	}

	// Serialize envelope
	return json.Marshal(envelope)
}

// DecryptFromPeer decrypts data encrypted with Ed25519 shared key derivation
func (pe *ed25519PeerEncryption) DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error) {
	// Parse envelope
	var envelope Ed25519EncryptedEnvelope
	if err := json.Unmarshal(encryptedData, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted envelope: %w", err)
	}

	// Verify our key is Ed25519
	if pe.privateKey.Type() != crypto.Ed25519 {
		return nil, fmt.Errorf("unsupported key type for Ed25519 decryption: %s", pe.privateKey.Type())
	}

	// Get sender's public key
	senderPeer, err := peer.Decode(senderPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid sender peer ID: %w", err)
	}

	senderPubKey := pe.peerstore.PubKey(senderPeer)
	if senderPubKey == nil {
		return nil, fmt.Errorf("sender public key not found for peer %s", senderPeerID)
	}

	// Derive the same shared key using the unified method
	// Note: use the same order as encryption (sender first, receiver second)
	sharedKey, err := pe.deriveUnifiedKey(senderPeerID, pe.ownPeerID.String(), senderPubKey, pe.privateKey.GetPublic())
	if err != nil {
		return nil, fmt.Errorf("failed to derive shared key: %w", err)
	}

	// Decrypt data with AES-GCM
	decryptedData, err := decryptWithAESGCM(envelope.EncryptedData, envelope.Nonce, sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return decryptedData, nil
}

// EncryptForMultiplePeers encrypts data for multiple Ed25519 peers
func (pe *ed25519PeerEncryption) EncryptForMultiplePeers(targetPeerIDs []string, data []byte) (map[string][]byte, error) {
	result := make(map[string][]byte)

	for _, peerID := range targetPeerIDs {
		encryptedData, err := pe.EncryptForPeer(peerID, data)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt for peer %s: %w", peerID, err)
		}
		result[peerID] = encryptedData
	}

	return result, nil
}

// Helper functions

// deriveUnifiedKey derives a shared AES key from peer IDs and keys
func (pe *ed25519PeerEncryption) deriveUnifiedKey(peerID1, peerID2 string, key1, key2 crypto.PubKey) ([]byte, error) {
	// Get raw key material
	key1Raw, err := key1.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get key1: %w", err)
	}

	key2Raw, err := key2.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get key2: %w", err)
	}

	// For Ed25519 keys, normalize to 32 bytes
	key1Material := normalizeEd25519Key(key1Raw)
	key2Material := normalizeEd25519Key(key2Raw)

	// Create deterministic key material by combining in a consistent order
	keyMaterial := append(key1Material, key2Material...)
	keyMaterial = append(keyMaterial, []byte(peerID1)...)
	keyMaterial = append(keyMaterial, []byte(peerID2)...)

	// Use HKDF to derive a 32-byte AES key
	salt := sha256.Sum256([]byte("DKNet-Ed25519-P2P-Salt"))
	info := []byte("DKNet-Ed25519-P2P-Encryption-v1")
	
	hkdf := hkdf.New(sha256.New, keyMaterial, salt[:], info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, key); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	
	return key, nil
}

// normalizeEd25519Key normalizes Ed25519 key material to 32 bytes
func normalizeEd25519Key(keyRaw []byte) []byte {
	if len(keyRaw) == 64 {
		// libp2p Ed25519 private key - use first 32 bytes (seed)
		return keyRaw[:32]
	}
	return keyRaw
}

// encryptWithAESGCM encrypts data using AES-GCM
func encryptWithAESGCM(data, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encrypted := gcm.Seal(nil, nonce, data, nil)
	return encrypted, nonce, nil
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
