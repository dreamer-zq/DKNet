package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

// PeerEncryption provides end-to-end encryption between peers
type PeerEncryption interface {
	// EncryptForPeer encrypts data that only the target peer can decrypt
	EncryptForPeer(targetPeerID string, data []byte) ([]byte, error)
	// DecryptFromPeer decrypts data encrypted for this peer
	DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error)
	// EncryptForMultiplePeers encrypts data for multiple recipients
	EncryptForMultiplePeers(targetPeerIDs []string, data []byte) (map[string][]byte, error)
}

// peerEncryption implements PeerEncryption interface
type peerEncryption struct {
	privateKey crypto.PrivKey
	peerstore  peerstore.Peerstore
}

// EncryptedEnvelope contains the encrypted symmetric key and data
type EncryptedEnvelope struct {
	EncryptedKey  []byte `json:"encrypted_key"`  // RSA encrypted AES key
	EncryptedData []byte `json:"encrypted_data"` // AES encrypted data
	Nonce         []byte `json:"nonce"`          // AES-GCM nonce
}

// NewPeerEncryption creates a new PeerEncryption instance
func NewPeerEncryption(privateKey crypto.PrivKey, ps peerstore.Peerstore) PeerEncryption {
	return &peerEncryption{
		privateKey: privateKey,
		peerstore:  ps,
	}
}

// EncryptForPeer encrypts data using hybrid encryption (RSA + AES)
func (pe *peerEncryption) EncryptForPeer(targetPeerID string, data []byte) ([]byte, error) {
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

	// Generate random AES key (32 bytes for AES-256)
	aesKey := make([]byte, 32)
	if _, rndErr := rand.Read(aesKey); rndErr != nil {
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

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, ioErr := io.ReadFull(rand.Reader, nonce); ioErr != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	encryptedData := gcm.Seal(nil, nonce, data, nil)

	// Encrypt AES key with recipient's RSA public key
	encryptedKey, err := pe.encryptKeyWithRSA(targetPubKey, aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	// Create envelope
	envelope := EncryptedEnvelope{
		EncryptedKey:  encryptedKey,
		EncryptedData: encryptedData,
		Nonce:         nonce,
	}

	// Serialize envelope
	return json.Marshal(envelope)
}

// DecryptFromPeer decrypts data encrypted for this peer
func (pe *peerEncryption) DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error) {
	// Parse envelope
	var envelope EncryptedEnvelope
	if err := json.Unmarshal(encryptedData, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted envelope: %w", err)
	}

	// Decrypt AES key with our private key
	aesKey, err := pe.decryptKeyWithRSA(pe.privateKey, envelope.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// Decrypt data with AES-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the data
	decryptedData, err := gcm.Open(nil, envelope.Nonce, envelope.EncryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return decryptedData, nil
}

// EncryptForMultiplePeers encrypts data for multiple recipients
func (pe *peerEncryption) EncryptForMultiplePeers(targetPeerIDs []string, data []byte) (map[string][]byte, error) {
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

// encryptKeyWithRSA encrypts the AES key using RSA public key
func (pe *peerEncryption) encryptKeyWithRSA(pubKey crypto.PubKey, aesKey []byte) ([]byte, error) {
	// For RSA keys
	if pubKey.Type() == crypto.RSA {
		// Extract the raw RSA public key bytes
		rawKey, err := pubKey.Raw()
		if err != nil {
			return nil, fmt.Errorf("failed to get raw public key: %w", err)
		}

		// Parse the DER-encoded RSA public key using standard library
		stdRSAPubKey, pkErr := x509.ParsePKIXPublicKey(rawKey)
		if pkErr != nil {
			return nil, fmt.Errorf("failed to parse RSA public key: %w", err)
		}

		// Type assert to RSA public key
		rsaPubKey, ok := stdRSAPubKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("key is not an RSA public key")
		}

		return rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPubKey, aesKey, nil)
	}

	// For Ed25519 or other key types, we need to implement alternative encryption
	// For now, return error for unsupported key types
	return nil, fmt.Errorf("unsupported key type for encryption: %s", pubKey.Type())
}

// decryptKeyWithRSA decrypts the AES key using RSA private key
func (pe *peerEncryption) decryptKeyWithRSA(privKey crypto.PrivKey, encryptedKey []byte) ([]byte, error) {
	// Convert libp2p private key to RSA private key
	if privKey.Type() == crypto.RSA {
		rawKey, err := privKey.Raw()
		if err != nil {
			return nil, fmt.Errorf("failed to get raw private key: %w", err)
		}

		// Parse the DER-encoded RSA private key using standard library
		stdRSAPrivKey, err := x509.ParsePKCS1PrivateKey(rawKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse RSA private key: %w", err)
		}

		return rsa.DecryptOAEP(sha256.New(), rand.Reader, stdRSAPrivKey, encryptedKey, nil)
	}

	return nil, fmt.Errorf("unsupported key type for decryption: %s", privKey.Type())
}

// unimplementedPeerEncryption provides a no-op implementation
type unimplementedPeerEncryption struct{}

func (u *unimplementedPeerEncryption) EncryptForPeer(targetPeerID string, data []byte) ([]byte, error) {
	return data, nil
}

func (u *unimplementedPeerEncryption) DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error) {
	return encryptedData, nil
}

func (u *unimplementedPeerEncryption) EncryptForMultiplePeers(targetPeerIDs []string, data []byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for _, peerID := range targetPeerIDs {
		result[peerID] = data
	}
	return result, nil
}

// NewUnimplementedPeerEncryption creates a no-op peer encryption
func NewUnimplementedPeerEncryption() PeerEncryption {
	return &unimplementedPeerEncryption{}
}
