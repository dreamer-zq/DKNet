package security

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"golang.org/x/crypto/hkdf"
)

const (
	secp256k1Salt = "DKNet-Secp256k1-P2P-Salt"
	secp256k1Info = "DKNet-Secp256k1-P2P-Encryption-v1"
)

type secp256k1PeerEncryption struct {
	privateKey crypto.PrivKey
	peerstore  peerstore.Peerstore
}

// NewSecp256k1PeerEncryption creates a new Secp256k1-based PeerEncryption instance.
func NewSecp256k1PeerEncryption(privateKey crypto.PrivKey, ps peerstore.Peerstore) PeerEncryption {
	return &secp256k1PeerEncryption{
		privateKey: privateKey,
		peerstore:  ps,
	}
}

// EncryptForPeer encrypts data using hybrid encryption (ECIES with Secp256k1 + AES-GCM)
func (pe *secp256k1PeerEncryption) EncryptForPeer(targetPeerID string, data []byte) ([]byte, error) {
	// 1. Get recipient's public key
	pid, err := peer.Decode(targetPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid target peer ID: %w", err)
	}

	// 2. Get recipient's public key
	targetPubKey := pe.peerstore.PubKey(pid)
	if targetPubKey == nil {
		return nil, fmt.Errorf("public key not found for peer %s", targetPeerID)
	}
	if targetPubKey.Type() != crypto.Secp256k1 {
		return nil, fmt.Errorf("unsupported key type for encryption: %s", targetPubKey.Type())
	}

	// Convert libp2p public key to btcec public key
	rawTargetPubKey, err := targetPubKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw target public key: %w", err)
	}
	targetBtcecPub, err := btcec.ParsePubKey(rawTargetPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key to btcec: %w", err)
	}

	// Use our private key for ECDH instead of an ephemeral one
	rawOurPrivKey, err := pe.privateKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw private key: %w", err)
	}
	ourBtcecPriv, _ := btcec.PrivKeyFromBytes(rawOurPrivKey)

	// Perform ECDH to derive a shared secret
	sharedSecret := btcec.GenerateSharedSecret(ourBtcecPriv, targetBtcecPub)

	// 4. Use HKDF to derive a symmetric key for AES
	salt := sha256.Sum256([]byte(secp256k1Salt))
	info := []byte(secp256k1Info)
	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt[:], info)
	aesKey := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(hkdfReader, aesKey); err != nil {
		return nil, fmt.Errorf("failed to derive AES key: %w", err)
	}

	// 5. Encrypt data with AES-GCM
	ciphertext, nonce, err := encryptWithAESGCM(data, aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt with AES-GCM: %w", err)
	}

	// 6. Create and serialize the envelope
	envelope := EncryptedEnvelope{
		Nonce:           nonce,
		Ciphertext:      ciphertext,
	}

	return json.Marshal(envelope)
}

// DecryptFromPeer decrypts data encrypted for this peer
func (pe *secp256k1PeerEncryption) DecryptFromPeer(senderPeerID string, encryptedData []byte) ([]byte, error) {
	// 1. Parse envelope
	var envelope EncryptedEnvelope
	if err := json.Unmarshal(encryptedData, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted envelope: %w", err)
	}

	// 2. Get sender's public key from peerstore
	senderPID, err := peer.Decode(senderPeerID)
	if err != nil {
		return nil, fmt.Errorf("invalid sender peer ID: %w", err)
	}

	senderPubKey := pe.peerstore.PubKey(senderPID)
	if senderPubKey == nil {
		return nil, fmt.Errorf("public key not found for peer %s", senderPeerID)
	}

	if senderPubKey.Type() != crypto.Secp256k1 {
		return nil, fmt.Errorf("unsupported sender key type for decryption: %s", senderPubKey.Type())
	}
	
	rawSenderPubKey, err := senderPubKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw sender public key: %w", err)
	}
	senderBtcecPub, err := btcec.ParsePubKey(rawSenderPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sender public key: %w", err)
	}

	// 3. Get our private key
	rawOurPrivKey, err := pe.privateKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw private key: %w", err)
	}
	ourBtcecPriv, _ := btcec.PrivKeyFromBytes(rawOurPrivKey)

	// 4. Perform ECDH to derive the same shared secret
	sharedSecret := btcec.GenerateSharedSecret(ourBtcecPriv, senderBtcecPub)

	// 5. Use HKDF to derive the same symmetric key
	salt := sha256.Sum256([]byte(secp256k1Salt))
	info := []byte(secp256k1Info)
	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt[:], info)
	aesKey := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(hkdfReader, aesKey); err != nil {
		return nil, fmt.Errorf("failed to derive AES key: %w", err)
	}

	// 6. Decrypt data with AES-GCM
	return decryptWithAESGCM(envelope.Ciphertext, envelope.Nonce, aesKey)
}