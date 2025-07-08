package e2e

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// nolint:unused
func runSharingTests(t *testing.T, keyIDs *sync.Map) {
	t.Run("reshare-2-of-3-to-2-of-2", func(t *testing.T) {
		keyID, ok := keyIDs.Load("2-of-3")
		require.True(t, ok, "Key '2-of-3' not found in keyIDs map")

		newParticipants := []string{TestConfig["node1"].PeerID, TestConfig["node2"].PeerID}
		newThreshold := int32(1)

		t.Logf(`

--- TEST: Reshare (2-of-3 to 2-of-2) ---
Request:
  - Key ID:         %s
  - New Threshold:    %d
  - New Participants: %v`, keyID, newThreshold, newParticipants)

		// Start resharing on node1
		resp, err := TestConfig["node1"].Client.StartResharing(context.Background(), &tssv1.StartResharingRequest{
			KeyId:           keyID.(string),
			NewThreshold:    newThreshold,
			NewParticipants: newParticipants,
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.OperationId)

		// Wait for operation to complete
		opResp := waitForOperation(t, TestConfig["node1"], resp.OperationId)

		// Verify result
		reshareKeyID := opResp.GetResharingResult().GetKeyId()
		require.NotEmpty(t, reshareKeyID)

		pubKey, err := findPubKeyInResponse(opResp)
		require.NoError(t, err)
		assert.NotEmpty(t, pubKey)

		keyIDs.Store("reshared-2-of-2", reshareKeyID)
		t.Logf(`Result:
  - New Key ID: %s
  - Public Key: %s
Status: SUCCESS
---------------------------`, reshareKeyID, pubKey)
	})
}
