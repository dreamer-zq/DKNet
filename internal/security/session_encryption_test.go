package security

import (
	"bytes"
	"testing"
)

func TestSessionEncryption(t *testing.T) {
	// Test seed key (32 bytes)
	seedKey := []byte("12345678901234567890123456789012")
	
	sessionEncryption := NewSessionEncryption(seedKey)
	
	// Test data
	originalData := []byte("Hello, this is test TSS data for encryption!")
	sessionID := "test-session-123"
	
	// Test encryption
	encryptedData, err := sessionEncryption.EncryptData(sessionID, originalData)
	if err != nil {
		t.Fatalf("Failed to encrypt data: %v", err)
	}
	
	// Encrypted data should be different from original
	if bytes.Equal(originalData, encryptedData) {
		t.Fatal("Encrypted data should be different from original data")
	}
	
	// Test decryption
	decryptedData, err := sessionEncryption.DecryptData(sessionID, encryptedData)
	if err != nil {
		t.Fatalf("Failed to decrypt data: %v", err)
	}
	
	// Decrypted data should match original
	if !bytes.Equal(originalData, decryptedData) {
		t.Fatalf("Decrypted data does not match original. Expected: %s, Got: %s", 
			string(originalData), string(decryptedData))
	}
}

func TestSessionEncryptionWithDifferentSessions(t *testing.T) {
	seedKey := []byte("12345678901234567890123456789012")
	sessionEncryption := NewSessionEncryption(seedKey)
	
	originalData := []byte("Test data")
	sessionID1 := "session-1"
	sessionID2 := "session-2"
	
	// Encrypt with session 1
	encrypted1, err := sessionEncryption.EncryptData(sessionID1, originalData)
	if err != nil {
		t.Fatalf("Failed to encrypt with session 1: %v", err)
	}
	
	// Encrypt with session 2
	encrypted2, err := sessionEncryption.EncryptData(sessionID2, originalData)
	if err != nil {
		t.Fatalf("Failed to encrypt with session 2: %v", err)
	}
	
	// Different sessions should produce different encrypted data
	if bytes.Equal(encrypted1, encrypted2) {
		t.Fatal("Different sessions should produce different encrypted data")
	}
	
	// Decrypt with correct session
	decrypted1, err := sessionEncryption.DecryptData(sessionID1, encrypted1)
	if err != nil {
		t.Fatalf("Failed to decrypt with session 1: %v", err)
	}
	
	if !bytes.Equal(originalData, decrypted1) {
		t.Fatal("Decrypted data should match original")
	}
	
	// Try to decrypt with wrong session (should fail)
	_, err = sessionEncryption.DecryptData(sessionID2, encrypted1)
	if err == nil {
		t.Fatal("Decryption should fail with wrong session ID")
	}
}

func TestSessionEncryptionEmptySessionID(t *testing.T) {
	seedKey := []byte("12345678901234567890123456789012")
	sessionEncryption := NewSessionEncryption(seedKey)
	
	originalData := []byte("Test data")
	
	// Empty session ID should fail
	_, err := sessionEncryption.EncryptData("", originalData)
	if err == nil {
		t.Fatal("Encryption should fail with empty session ID")
	}
	
	_, err = sessionEncryption.DecryptData("", []byte("dummy"))
	if err == nil {
		t.Fatal("Decryption should fail with empty session ID")
	}
}

func TestSessionEncryptionSameSeedKeySameResult(t *testing.T) {
	seedKey := []byte("12345678901234567890123456789012")
	
	// Create two instances with same seed key
	encryption1 := NewSessionEncryption(seedKey)
	encryption2 := NewSessionEncryption(seedKey)
	
	originalData := []byte("Test data for same seed key")
	sessionID := "same-session"
	
	// Encrypt with first instance
	encrypted1, err := encryption1.EncryptData(sessionID, originalData)
	if err != nil {
		t.Fatalf("Failed to encrypt with first instance: %v", err)
	}
	
	// Decrypt with second instance (should work with same seed key)
	decrypted, err := encryption2.DecryptData(sessionID, encrypted1)
	if err != nil {
		t.Fatalf("Failed to decrypt with second instance: %v", err)
	}
	
	if !bytes.Equal(originalData, decrypted) {
		t.Fatal("Decrypted data should match original when using same seed key")
	}
} 