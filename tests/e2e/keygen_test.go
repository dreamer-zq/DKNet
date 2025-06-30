package e2e

import (
	"context"
	"sync"
	"testing"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runKeygenTests(t *testing.T, keyIDs *sync.Map) {
	t.Run("2-of-3", func(t *testing.T) {
		participants := []string{TestConfig["node1"].PeerID, TestConfig["node2"].PeerID, TestConfig["node3"].PeerID}
		threshold := int32(1)

		t.Logf(`

--- TEST: Keygen (2-of-3) ---
Request:
  - Threshold:    %d
  - Participants: %v`, threshold, participants)

		// Start keygen on node1
		resp, err := TestConfig["node1"].Client.StartKeygen(context.Background(), &tssv1.StartKeygenRequest{
			Threshold:    threshold,
			Participants: participants,
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.OperationId)

		// Wait for operation to complete
		opResp := waitForOperation(t, TestConfig["node1"], resp.OperationId)

		// Verify result
		pubKey, err := findPubKeyInResponse(opResp)
		require.NoError(t, err)
		assert.NotEmpty(t, pubKey)

		keyID := opResp.GetKeygenResult().GetKeyId()
		require.NotEmpty(t, keyID)
		keyIDs.Store("2-of-3", keyID)
		t.Logf(`Result:
  - Key ID:     %s
  - Public Key: %s
Status: SUCCESS
---------------------------`, keyID, pubKey)
	})

	t.Run("3-of-3", func(t *testing.T) {
		participants := []string{TestConfig["node1"].PeerID, TestConfig["node2"].PeerID, TestConfig["node3"].PeerID}
		threshold := int32(2)

		t.Logf(`

--- TEST: Keygen (3-of-3) ---
Request:
  - Threshold:    %d
  - Participants: %v`, threshold, participants)

		resp, err := TestConfig["node2"].Client.StartKeygen(context.Background(), &tssv1.StartKeygenRequest{
			Threshold:    threshold,
			Participants: participants,
		})
		require.NoError(t, err)
		opResp := waitForOperation(t, TestConfig["node2"], resp.OperationId)

		pubKey, err := findPubKeyInResponse(opResp)
		require.NoError(t, err)
		assert.NotEmpty(t, pubKey)

		keyID := opResp.GetKeygenResult().GetKeyId()
		require.NotEmpty(t, keyID)
		keyIDs.Store("3-of-3", keyID)
		t.Logf(`Result:
  - Key ID:     %s
  - Public Key: %s
Status: SUCCESS
---------------------------`, keyID, pubKey)
	})

	t.Run("2-of-2", func(t *testing.T) {
		participants := []string{TestConfig["node1"].PeerID, TestConfig["node2"].PeerID}
		threshold := int32(1)

		t.Logf(`

--- TEST: Keygen (2-of-2) ---
Request:
  - Threshold:    %d
  - Participants: %v`, threshold, participants)

		resp, err := TestConfig["node2"].Client.StartKeygen(context.Background(), &tssv1.StartKeygenRequest{
			Threshold:    threshold,
			Participants: participants,
		})
		require.NoError(t, err)
		opResp := waitForOperation(t, TestConfig["node2"], resp.OperationId)

		pubKey, err := findPubKeyInResponse(opResp)
		require.NoError(t, err)
		assert.NotEmpty(t, pubKey)

		keyID := opResp.GetKeygenResult().GetKeyId()
		require.NotEmpty(t, keyID)
		keyIDs.Store("2-of-2", keyID)
		t.Logf(`Result:
  - Key ID:     %s
  - Public Key: %s
Status: SUCCESS
---------------------------`, keyID, pubKey)
	})
} 