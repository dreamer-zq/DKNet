package e2e

import (
	"context"
	"sync"
	"testing"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runSigningTests(t *testing.T, keyIDs *sync.Map) {
	// Helper function for signing tests
	testSigning := func(t *testing.T, keyName string, participants []*Node, message string, testDesc string) {
		t.Helper()

		rawID, ok := keyIDs.Load(keyName)
		require.True(t, ok, "KeyID not found for "+keyName)
		keyID := rawID.(string)

		var participantIDs []string
		for _, p := range participants {
			participantIDs = append(participantIDs, p.PeerID)
		}

		t.Logf(`

--- TEST: %s ---
Request:
  - Key ID:       %s
  - Participants: %v
  - Message:      "%s"`, testDesc, keyID, participantIDs, message)

		// Use the first participant's node to start the operation
		nodeToStart := participants[0]

		msgBytes := []byte(message)

		resp, err := nodeToStart.Client.StartSigning(context.Background(), &tssv1.StartSigningRequest{
			KeyId:        keyID,
			Message:      msgBytes,
			Participants: participantIDs,
		})
		require.NoError(t, err)

		opResp := waitForOperation(t, nodeToStart, resp.OperationId)
		signature, err := findSignatureInResponse(opResp)
		require.NoError(t, err)
		assert.NotEmpty(t, signature)

		t.Logf(`Result:
  - Signature: %s
Status: SUCCESS
---------------------------`, signature)
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