package security

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecp256k1PeerEncryption(t *testing.T) {
	// 1. Setup: Create two peers with secp256k1 keys and a peerstore
	ps, err := pstoremem.NewPeerstore()
	require.NoError(t, err)
	defer ps.Close()

	// Peer A
	privA, pubA, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	require.NoError(t, err)

	pidA, err := peer.IDFromPublicKey(pubA)
	require.NoError(t, err)

	require.NoError(t, ps.AddPubKey(pidA, pubA))
	require.NoError(t, ps.AddPrivKey(pidA, privA))

	// Peer B
	privB, pubB, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	require.NoError(t, err)

	pidB, err := peer.IDFromPublicKey(pubB)
	require.NoError(t, err)

	require.NoError(t, ps.AddPubKey(pidB, pubB))
	require.NoError(t, ps.AddPrivKey(pidB, privB))

	// 2. Create PeerEncryption instances for both peers
	peA := NewSecp256k1PeerEncryption(privA, ps)
	peB := NewSecp256k1PeerEncryption(privB, ps)

	// 3. Peer A encrypts a message for Peer B
	originalMessage := []byte("This is a super secret message for peer B.")
	encrypted, err := peA.EncryptForPeer(pidB.String(), originalMessage)
	require.NoError(t, err)
	require.NotEmpty(t, encrypted)

	// 4. Peer B decrypts the message from Peer A
	decrypted, err := peB.DecryptFromPeer(pidA.String(), encrypted)
	require.NoError(t, err)
	require.NotEmpty(t, decrypted)

	// 5. Verify the decrypted message matches the original
	assert.Equal(t, originalMessage, decrypted)
}

func TestSecp256k1PeerEncryption_UnsupportedKey(t *testing.T) {
	// 1. Setup: Create a peer with a non-secp256k1 key
	ps, err := pstoremem.NewPeerstore()
	require.NoError(t, err)
	defer ps.Close()

	// Peer A (sender) with a valid key
	privA, pubA, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	require.NoError(t, err)
	pidA, err := peer.IDFromPublicKey(pubA)
	require.NoError(t, err)
	err = ps.AddPubKey(pidA, pubA)
	require.NoError(t, err)

	// Peer B (recipient) with an RSA key
	_, pubB, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	require.NoError(t, err)
	pidB, err := peer.IDFromPublicKey(pubB)
	require.NoError(t, err)
	err = ps.AddPubKey(pidB, pubB)
	require.NoError(t, err)

	// 2. Create PeerEncryption instance for Peer A
	peA := NewSecp256k1PeerEncryption(privA, ps)

	// 3. Attempt to encrypt for Peer B, expecting an error
	_, err = peA.EncryptForPeer(pidB.String(), []byte("some data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported key type for encryption")
}
