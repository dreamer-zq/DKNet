package e2e

import (
	"sync"
	"testing"
)

func TestE2ESuite(t *testing.T) {
	// Setup gRPC clients for all nodes, this runs once for the suite.
	setupClients(t)
	defer teardownClients(t)

	// Use a map to store generated key IDs to pass between test phases.
	var keyIDs sync.Map

	// === Phase 1: Keygen Tests ===
	t.Run("Keygen", func(t *testing.T) {
		runKeygenTests(t, &keyIDs)
	})

	// === Phase 2: Signing Tests ===
	t.Run("Signing", func(t *testing.T) {
		runSigningTests(t, &keyIDs)
	})

	// === Phase 3: Resharing Tests ===
	t.Run("Resharing", func(t *testing.T) {
		// TODO: Implement resharing tests
		// runSharingTests(t, &keyIDs)
	})
}
