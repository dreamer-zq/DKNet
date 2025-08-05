package e2e

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPublicKeyFormatHandling tests that our signature verification functions correctly handle 64-byte public keys
func TestPublicKeyFormatHandling(t *testing.T) {
	// Test data: a 64-byte public key (without 0x04 prefix) - this is what DKNet TSS returns
	pubKey64Bytes := "8c9d8b28bc4acc015da055ebe19314aa4d6f9ef3f7c0f86b57b49441f98393e0" +
		"2599723d1cf0dd2963ccc329e68889307849c450c49589cb09ad7186258a66be"

	// Test message
	message := "Hello DKNet TSS!"
	messageBytes := []byte(message)

	// Test signature (65 bytes: R + S + V) - this would come from actual TSS signing
	// Note: This is a dummy signature for testing format handling, not a real signature
	testSignature := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef" +
		"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1b"

	t.Run("64-byte public key handling", func(t *testing.T) {
		// Test that our functions can handle 64-byte public keys without crashing

		// Decode the public key to verify it's 64 bytes
		pubKeyBytes, err := hex.DecodeString(pubKey64Bytes)
		require.NoError(t, err)
		assert.Equal(t, 64, len(pubKeyBytes), "Test public key should be 64 bytes")

		// Test verifySignatureWithMessage function
		err = verifySignatureWithMessage(pubKey64Bytes, messageBytes, testSignature)
		// We expect this to fail because it's a dummy signature, but it should NOT fail due to public key format
		if err != nil {
			// The error should be about signature verification, not public key parsing
			assert.NotContains(t, err.Error(), "malformed public key: invalid length: 64",
				"Should not fail due to 64-byte public key length")
			assert.NotContains(t, err.Error(), "failed to parse public key",
				"Should not fail to parse public key after our fix")
			t.Logf("Expected verification failure (dummy signature): %v", err)
		}

		// Test verifySignature function
		messageHex := hex.EncodeToString(messageBytes)
		err = verifySignature(pubKey64Bytes, messageHex, testSignature)
		// Same expectation: should not fail due to public key format
		if err != nil {
			assert.NotContains(t, err.Error(), "malformed public key: invalid length: 64",
				"Should not fail due to 64-byte public key length")
			assert.NotContains(t, err.Error(), "failed to parse public key",
				"Should not fail to parse public key after our fix")
			t.Logf("Expected verification failure (dummy signature): %v", err)
		}
	})

	t.Run("65-byte public key still works", func(t *testing.T) {
		// Test that 65-byte public keys (with 0x04 prefix) still work
		pubKey65Bytes := "04" + pubKey64Bytes

		// Verify it's 65 bytes
		pubKeyBytes, err := hex.DecodeString(pubKey65Bytes)
		require.NoError(t, err)
		assert.Equal(t, 65, len(pubKeyBytes), "Test public key should be 65 bytes")

		// Test both functions with 65-byte key
		err = verifySignatureWithMessage(pubKey65Bytes, messageBytes, testSignature)
		if err != nil {
			assert.NotContains(t, err.Error(), "failed to parse public key",
				"Should not fail to parse 65-byte public key")
			t.Logf("Expected verification failure (dummy signature): %v", err)
		}

		messageHex := hex.EncodeToString(messageBytes)
		err = verifySignature(pubKey65Bytes, messageHex, testSignature)
		if err != nil {
			assert.NotContains(t, err.Error(), "failed to parse public key",
				"Should not fail to parse 65-byte public key")
			t.Logf("Expected verification failure (dummy signature): %v", err)
		}
	})

	t.Run("hash function consistency", func(t *testing.T) {
		// Test that our hash function produces consistent results
		hash1 := hashMessageForEthereum(messageBytes)
		hash2 := hashMessageForEthereum(messageBytes)

		assert.Equal(t, hash1, hash2, "Hash function should be deterministic")
		assert.Equal(t, 32, len(hash1), "Hash should be 32 bytes (Keccak256)")

		// Test with different messages
		hash3 := hashMessageForEthereum([]byte("Different message"))
		assert.NotEqual(t, hash1, hash3, "Different messages should produce different hashes")

		t.Logf("Message: '%s' -> Hash: %s", message, hex.EncodeToString(hash1))
	})
}
