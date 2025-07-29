package e2e

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// hashMessageForEthereum creates an Ethereum-compatible hash that can be verified with ecrecover
func hashMessageForEthereum(message []byte) []byte {
	// Ethereum message prefix format: "\x19Ethereum Signed Message:\n" + len(message) + message
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	prefixedMessage := append([]byte(prefix), message...)

	// Use Keccak256 (not SHA3-256) as required by Ethereum
	hash := sha3.NewLegacyKeccak256()
	hash.Write(prefixedMessage)
	return hash.Sum(nil)
}

// verifySignature verifies an Ethereum-compatible signature using secp256k1 ECDSA
func verifySignature(publicKeyHex, messageHex, signatureHex string) error {
	// Remove "0x" prefix if present
	publicKeyHex = strings.TrimPrefix(publicKeyHex, "0x")
	signatureHex = strings.TrimPrefix(signatureHex, "0x")
	messageHex = strings.TrimPrefix(messageHex, "0x")

	// Decode public key
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode public key: %w", err)
	}

	// Handle 64-byte public key (add 0x04 prefix for uncompressed format)
	if len(pubKeyBytes) == 64 {
		// Add uncompressed public key prefix (0x04)
		fullPubKey := make([]byte, 65)
		fullPubKey[0] = 0x04
		copy(fullPubKey[1:], pubKeyBytes)
		pubKeyBytes = fullPubKey
	}

	// Parse secp256k1 public key
	pubKey, err := btcec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// Decode signature (should be 65 bytes: R(32) + S(32) + V(1))
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length: expected 65 bytes, got %d", len(sigBytes))
	}

	// Extract R, S, and V components
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:64])
	v := int(sigBytes[64])

	// Validate V value (should be 27 or 28 for Ethereum)
	if v != 27 && v != 28 {
		return fmt.Errorf("invalid V value: %d (expected 27 or 28)", v)
	}

	// Decode message - messageHex should be the hex-encoded message
	var message []byte
	if messageHex != "" {
		message, err = hex.DecodeString(messageHex)
		if err != nil {
			return fmt.Errorf("failed to decode message: %w", err)
		}
	} else {
		return fmt.Errorf("message hex cannot be empty")
	}

	// Hash the message using Ethereum format
	messageHash := hashMessageForEthereum(message)

	// Convert btcec public key to ecdsa public key
	ecdsaPubKey := pubKey.ToECDSA()

	// Verify signature using ECDSA
	if !ecdsa.Verify(ecdsaPubKey, messageHash, r, s) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

func runSigningTests(t *testing.T, keyIDs *sync.Map, keyInfos *sync.Map) {
	// Helper function for signing tests
	testSigning := func(t *testing.T, keyName string, participants []*Node, message string, testDesc string) {
		t.Helper()

		// Get key info from the map
		rawKeyInfo, ok := keyInfos.Load(keyName)
		require.True(t, ok, "KeyInfo not found for "+keyName)
		keyInfo := rawKeyInfo.(*KeyInfo)

		var participantIDs []string
		for _, p := range participants {
			participantIDs = append(participantIDs, p.PeerID)
		}

		t.Logf(`

--- TEST: %s ---
Request:
  - Key ID:       %s
  - Public Key:   %s
  - Participants: %v
  - Message:      "%s"`, testDesc, keyInfo.KeyID, keyInfo.PublicKey, participantIDs, message)

		// Use the first participant's node to start the operation
		nodeToStart := participants[0]

		msgBytes := []byte(message)

		resp, err := nodeToStart.Client.StartSigning(context.Background(), &tssv1.StartSigningRequest{
			KeyId:        keyInfo.KeyID,
			Message:      msgBytes,
			Participants: participantIDs,
		})
		require.NoError(t, err)

		opResp := waitForOperation(t, nodeToStart, resp.OperationId)
		signature, err := findSignatureInResponse(opResp)
		require.NoError(t, err)
		assert.NotEmpty(t, signature)

		t.Logf(`Result:
  - Signature: %s`, signature)

		// Verify the signature using the original message bytes
		t.Logf("Verifying signature...")
		err = verifySignatureWithMessage(keyInfo.PublicKey, msgBytes, signature)
		if err != nil {
			t.Logf("Direct verification failed: %v", err)
			t.Logf("Attempting verification with hex-encoded message...")
			// Try with hex-encoded message as fallback
			messageHex := hex.EncodeToString(msgBytes)
			err = verifySignature(keyInfo.PublicKey, messageHex, signature)
		}
		
		require.NoError(t, err, "Signature verification failed")

		t.Logf(`Verification:
  - Status: SIGNATURE VALID ✓
Status: SUCCESS
---------------------------`)
	}

	// Run signing tests
	t.Run("2-of-3 with 2 participants", func(t *testing.T) {
		participants := []*Node{TestConfig["node1"], TestConfig["node2"]}
		testSigning(t, "2-of-3", participants, "Hello World Test 1", "2-of-3 with 2 participants")
	})

	t.Run("2-of-3 with different 2 participants", func(t *testing.T) {
		participants := []*Node{TestConfig["node2"], TestConfig["node3"]}
		testSigning(t, "2-of-3", participants, "Hello World Test 2", "2-of-3 with different 2 participants")
	})

	t.Run("3-of-3 with all participants", func(t *testing.T) {
		participants := []*Node{TestConfig["node1"], TestConfig["node2"], TestConfig["node3"]}
		testSigning(t, "3-of-3", participants, "Hello World Test 3", "3-of-3 with all participants")
	})

	t.Run("2-of-2 with both participants", func(t *testing.T) {
		participants := []*Node{TestConfig["node1"], TestConfig["node2"]}
		testSigning(t, "2-of-2", participants, "Hello World Test 4", "2-of-2 with both participants")
	})

	t.Run("Transaction signing test", func(t *testing.T) {
		participants := []*Node{TestConfig["node1"], TestConfig["node2"]}
		testSigning(t, "2-of-3", participants, "Transaction: Send 100 ETH to 0x123...", "Transaction signing test")
	})

	t.Run("JSON message signing test", func(t *testing.T) {
		participants := []*Node{TestConfig["node2"], TestConfig["node3"]}
		jsonMessage := `{"type":"transfer","amount":"100","to":"0x123","nonce":42}`
		testSigning(t, "2-of-3", participants, jsonMessage, "JSON message signing test")
	})
}

// verifySignatureWithMessage verifies signature using raw message bytes directly
func verifySignatureWithMessage(publicKeyHex string, message []byte, signatureHex string) error {
	// Remove "0x" prefix if present
	publicKeyHex = strings.TrimPrefix(publicKeyHex, "0x")
	signatureHex = strings.TrimPrefix(signatureHex, "0x")

	// Decode public key
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode public key: %w", err)
	}

	// Handle 64-byte public key (add 0x04 prefix for uncompressed format)
	if len(pubKeyBytes) == 64 {
		// Add uncompressed public key prefix (0x04)
		fullPubKey := make([]byte, 65)
		fullPubKey[0] = 0x04
		copy(fullPubKey[1:], pubKeyBytes)
		pubKeyBytes = fullPubKey
	}

	// Parse secp256k1 public key
	pubKey, err := btcec.ParsePubKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// Decode signature (should be 65 bytes: R(32) + S(32) + V(1))
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length: expected 65 bytes, got %d", len(sigBytes))
	}

	// Extract R, S, and V components
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:64])
	v := int(sigBytes[64])

	// Validate V value (should be 27 or 28 for Ethereum)
	if v != 27 && v != 28 {
		return fmt.Errorf("invalid V value: %d (expected 27 or 28)", v)
	}

	// Hash the message using Ethereum format
	messageHash := hashMessageForEthereum(message)

	// Convert btcec public key to ecdsa public key
	ecdsaPubKey := pubKey.ToECDSA()

	// Verify signature using ECDSA
	if !ecdsa.Verify(ecdsaPubKey, messageHash, r, s) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
