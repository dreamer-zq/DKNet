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
	"golang.org/x/crypto/nacl/box"
)

// ed25519PeerEncryption implements PeerEncryption for Ed25519 keys using NaCl box
type ed25519PeerEncryption struct {
	privateKey crypto.PrivKey
	peerstore  peerstore.Peerstore
}

// Ed25519EncryptedEnvelope contains the encrypted data for Ed25519 keys
type Ed25519EncryptedEnvelope struct {
	EncryptedData []byte `json:"encrypted_data"` // NaCl box encrypted data
	Nonce         []byte `json:"nonce"`          // NaCl box nonce (24 bytes)
	EphemeralPub  []byte `json:"ephemeral_pub"`  // Ephemeral public key for this message
}

// NewEd25519PeerEncryption creates a peer encryption instance optimized for Ed25519 keys
func NewEd25519PeerEncryption(privateKey crypto.PrivKey, ps peerstore.Peerstore) PeerEncryption {
	return &ed25519PeerEncryption{
		privateKey: privateKey,
		peerstore:  ps,
	}
}

// EncryptForPeer encrypts data for a specific peer using Ed25519 keys and NaCl box
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

	// Check if it's Ed25519 key
	if targetPubKey.Type() != crypto.Ed25519 {
		// Fall back to hybrid encryption for non-Ed25519 keys
		return pe.fallbackEncrypt(targetPeerID, data)
	}

	// Generate ephemeral key pair for this message
	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Convert target public key to NaCl format
	targetPubKeyBytes, err := targetPubKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw public key: %w", err)
	}

	// Ed25519 public keys are 32 bytes, NaCl box keys are also 32 bytes
	// We can use Ed25519 keys directly with NaCl box
	var targetBoxPub [32]byte
	copy(targetBoxPub[:], targetPubKeyBytes)

	// Generate nonce
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt using NaCl box
	encryptedData := box.Seal(nil, data, &nonce, &targetBoxPub, ephemeralPriv)

	// Create envelope
	envelope := Ed25519EncryptedEnvelope{
		EncryptedData: encryptedData,
		Nonce:         nonce[:],
		EphemeralPub:  ephemeralPub[:],
	}

	return json.Marshal(envelope)
}

// DecryptFromPeer decrypts data encrypted for this peer
func (pe *ed25519PeerEncryption) DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error) {
	// Try Ed25519 decryption first
	if data, err := pe.decryptEd25519(encryptedData); err == nil {
		return data, nil
	}

	// Fall back to hybrid decryption
	return pe.fallbackDecrypt(senderPeerID, encryptedData)
}

// decryptEd25519 decrypts Ed25519-encrypted data
func (pe *ed25519PeerEncryption) decryptEd25519(encryptedData []byte) ([]byte, error) {
	// Check if our key is Ed25519
	if pe.privateKey.Type() != crypto.Ed25519 {
		return nil, fmt.Errorf("not an Ed25519 key")
	}

	// Parse envelope
	var envelope Ed25519EncryptedEnvelope
	if err := json.Unmarshal(encryptedData, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse Ed25519 envelope: %w", err)
	}

	// Convert our private key to NaCl format
	privKeyBytes, err := pe.privateKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw private key: %w", err)
	}

	var ourPriv [32]byte
	copy(ourPriv[:], privKeyBytes)

	// Reconstruct ephemeral public key
	var ephemeralPub [32]byte
	copy(ephemeralPub[:], envelope.EphemeralPub)

	// Reconstruct nonce
	var nonce [24]byte
	copy(nonce[:], envelope.Nonce)

	// Decrypt using NaCl box
	decryptedData, ok := box.Open(nil, envelope.EncryptedData, &nonce, &ephemeralPub, &ourPriv)
	if !ok {
		return nil, fmt.Errorf("failed to decrypt with NaCl box")
	}

	return decryptedData, nil
}

// fallbackEncrypt falls back to hybrid encryption for non-Ed25519 keys
func (pe *ed25519PeerEncryption) fallbackEncrypt(targetPeerID string, data []byte) ([]byte, error) {
	// Use AES encryption with a derived key
	// Generate a random AES key
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	// Encrypt data with AES-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, readErr := io.ReadFull(rand.Reader, nonce); readErr != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", readErr)
	}

	encryptedData := gcm.Seal(nil, nonce, data, nil)

	// For simplicity, we'll derive a key from both peer IDs
	// In a real implementation, you might want to use ECDH or similar
	keyDerivationData := fmt.Sprintf("%s-%s", pe.getOwnPeerID(), targetPeerID)
	derivedKey := sha256.Sum256([]byte(keyDerivationData))

	// Encrypt the AES key with the derived key
	keyBlock, err := aes.NewCipher(derivedKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create key cipher: %w", err)
	}

	keyGCM, err := cipher.NewGCM(keyBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to create key GCM: %w", err)
	}

	keyNonce := make([]byte, keyGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, keyNonce); err != nil {
		return nil, fmt.Errorf("failed to generate key nonce: %w", err)
	}

	encryptedKey := keyGCM.Seal(nil, keyNonce, aesKey, nil)

	// Create a fallback envelope
	envelope := map[string]interface{}{
		"type":          "fallback",
		"encrypted_key": encryptedKey,
		"key_nonce":     keyNonce,
		"data":          encryptedData,
		"nonce":         nonce,
	}

	return json.Marshal(envelope)
}

// fallbackDecrypt decrypts using fallback method
func (pe *ed25519PeerEncryption) fallbackDecrypt(senderPeerID string, encryptedData []byte) ([]byte, error) {
	var envelope map[string]interface{}
	if err := json.Unmarshal(encryptedData, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse fallback envelope: %w", err)
	}

	if envelope["type"] != "fallback" {
		return nil, fmt.Errorf("not a fallback envelope")
	}

	// Derive the same key used for encryption
	keyDerivationData := fmt.Sprintf("%s-%s", senderPeerID, pe.getOwnPeerID())
	derivedKey := sha256.Sum256([]byte(keyDerivationData))

	// Extract components
	encryptedKey := envelope["encrypted_key"].([]byte)
	keyNonce := envelope["key_nonce"].([]byte)
	encryptedDataBytes := envelope["data"].([]byte)
	nonce := envelope["nonce"].([]byte)

	// Decrypt the AES key
	keyBlock, err := aes.NewCipher(derivedKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create key cipher: %w", err)
	}

	keyGCM, err := cipher.NewGCM(keyBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to create key GCM: %w", err)
	}

	aesKey, err := keyGCM.Open(nil, keyNonce, encryptedKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// Decrypt the data
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return gcm.Open(nil, nonce, encryptedDataBytes, nil)
}

// getOwnPeerID gets our own peer ID
func (pe *ed25519PeerEncryption) getOwnPeerID() string {
	peerID, _ := peer.IDFromPrivateKey(pe.privateKey)
	return peerID.String()
}

// EncryptForMultiplePeers encrypts data for multiple recipients
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
