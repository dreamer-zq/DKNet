package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"
)

const (
	// Message types
	gossipRouteMessageType = "gossip_route"
	// Configuration defaults
	defaultMaxTTL          = 10
	defaultCleanupInterval = 5 * time.Minute
	defaultSeenMessageTTL  = 10 * time.Minute
)

// GossipRouter implements gossip-based routing for point-to-point messages
type GossipRouter struct {
	network *Network
	logger  *zap.Logger

	// Message tracking to prevent loops
	seenMessages map[string]time.Time
	seenMu       sync.RWMutex

	// Configuration
	maxTTL        int
	cleanupTicker *time.Ticker
}

// NewGossipRouter creates a new gossip router
func NewGossipRouter(network *Network, logger *zap.Logger) *GossipRouter {
	router := &GossipRouter{
		network:      network,
		logger:       logger,
		seenMessages: make(map[string]time.Time),
		maxTTL:       defaultMaxTTL,
	}

	// Start cleanup routine for seen messages
	router.cleanupTicker = time.NewTicker(defaultCleanupInterval)
	go router.cleanupSeenMessages()

	return router
}

// SendWithGossip sends a message using gossip routing if direct connection fails
func (gr *GossipRouter) SendWithGossip(ctx context.Context, msg *Message) error {
	// For point-to-point messages, try direct first, then gossip
	for _, target := range msg.To {
		if err := gr.sendToTarget(ctx, msg, target); err != nil {
			gr.logger.Warn("Failed to send message to target, trying gossip routing",
				zap.String("target", target),
				zap.Error(err))

			if gossipErr := gr.sendViaGossip(ctx, msg, target); gossipErr != nil {
				gr.logger.Error("Gossip routing also failed",
					zap.String("target", target),
					zap.Error(gossipErr))
				return gossipErr
			}
		}
	}

	return nil
}

// sendToTarget sends message to a specific target peer
func (gr *GossipRouter) sendToTarget(ctx context.Context, msg *Message, target string) error {
	if target == gr.network.GetHostID() {
		gr.logger.Info("Skipping send message to self", zap.String("target", target))
		return nil
	}

	targetPeer, err := peer.Decode(target)
	if err != nil {
		return fmt.Errorf("invalid target peer ID: %w", err)
	}

	// Check if we're directly connected
	if gr.network.connected(targetPeer) {
		// Try direct send
		directMsg := *msg
		directMsg.To = []string{target}
		return gr.network.send(ctx, &directMsg)
	}

	return fmt.Errorf("not directly connected to target")
}

// sendViaGossip sends message via gossip routing
func (gr *GossipRouter) sendViaGossip(ctx context.Context, msg *Message, target string) error {
	if target == gr.network.GetHostID() {
		gr.logger.Info("Skipping send message to self", zap.String("target", target))
		return nil
	}

	// Find connected peers first
	connectedPeers := gr.network.getConnectedPeers()
	if len(connectedPeers) == 0 {
		return fmt.Errorf("no connected peers for gossip routing")
	}

	// Check if target is directly connected before creating routed message
	for _, peerID := range connectedPeers {
		if peerID.String() != target {
			continue
		}
		// Direct connection found! Send directly without gossip overhead
		gr.logger.Info("Found direct connection to target, sending directly",
			zap.String("target", target))

		directMsg := *msg
		directMsg.To = []string{target}
		return gr.network.send(ctx, &directMsg)
	}

	// Target not directly connected, use gossip routing
	gr.logger.Info("Target not directly connected, using gossip routing",
		zap.String("target", target))

	// Create routed message
	routedMsg := &RoutedMessage{
		Message:        msg,
		OriginalSender: gr.network.GetHostID(),
		FinalTarget:    target,
		Path:           []string{gr.network.GetHostID()},
		TTL:            gr.maxTTL,
		MessageID:      gr.generateMessageID(msg.SessionID),
	}

	// Mark as seen to prevent loops
	gr.markAsSeen(routedMsg.MessageID)

	// Send routed message to all connected peers
	for _, peerID := range connectedPeers {
		if err := gr.sendRoutedMessage(ctx, routedMsg, peerID); err != nil {
			gr.logger.Warn("Failed to send routed message to peer",
				zap.String("peer", peerID.String()),
				zap.Error(err))
		}
	}

	gr.logger.Info("Gossip message sent to connected peers",
		zap.String("target", target),
		zap.Int("peer_count", len(connectedPeers)))

	return nil
}

// sendRoutedMessage sends a routed message to a specific peer
func (gr *GossipRouter) sendRoutedMessage(ctx context.Context, routedMsg *RoutedMessage, peerID peer.ID) error {
	data, err := routedMsg.Compresses()
	if err != nil {
		return fmt.Errorf("failed to compress routed message: %w", err)
	}

	// Create a special gossip message
	gossipMsg := &Message{
		SessionID:   routedMsg.SessionID,
		Type:        gossipRouteMessageType,
		From:        gr.network.GetHostID(),
		To:          []string{peerID.String()},
		Data:        data,
		IsBroadcast: false,
		Timestamp:   time.Now(),
		ProtocolID:  TssGossipProtocol,
	}

	return gr.network.send(ctx, gossipMsg)
}

// HandleRoutedMessage handles incoming routed messages
func (gr *GossipRouter) HandleRoutedMessage(ctx context.Context, msg *Message) error {
	var routedMsg RoutedMessage
	if err := routedMsg.Decompresses(msg.Data); err != nil {
		return fmt.Errorf("failed to decompress routed message: %w", err)
	}

	// Check if we've seen this message before
	if gr.hasSeen(routedMsg.MessageID) {
		gr.logger.Debug("Ignoring duplicate routed message",
			zap.String("message_id", routedMsg.MessageID))
		return nil
	}

	// Mark as seen
	gr.markAsSeen(routedMsg.MessageID)

	// Check TTL
	if routedMsg.TTL <= 0 {
		gr.logger.Warn("Message TTL expired",
			zap.String("message_id", routedMsg.MessageID))
		return nil
	}

	// Check if we are the final target
	if routedMsg.FinalTarget == gr.network.GetHostID() {
		gr.logger.Info("Received message via gossip routing",
			zap.String("original_sender", routedMsg.OriginalSender),
			zap.Strings("path", routedMsg.Path))

		// Deliver to local handler
		return gr.network.messageHandler.HandleMessage(ctx, routedMsg.Message)
	}

	// Check if we know the target directly
	targetPeer, err := peer.Decode(routedMsg.FinalTarget)
	if err == nil && gr.network.connected(targetPeer) {
		// Forward directly to target
		gr.logger.Info("Forwarding message directly to target",
			zap.String("target", routedMsg.FinalTarget),
			zap.String("message_id", routedMsg.MessageID))

		directMsg := *routedMsg.Message
		directMsg.To = []string{routedMsg.FinalTarget}
		return gr.network.send(ctx, &directMsg)
	}

	// Continue gossip forwarding
	routedMsg.TTL--
	routedMsg.Path = append(routedMsg.Path, gr.network.GetHostID())

	// Forward to other connected peers (except sender)
	connectedPeers := gr.network.getConnectedPeers()
	senderPeer, _ := peer.Decode(msg.From)

	for _, peerID := range connectedPeers {
		if peerID == senderPeer {
			continue // Don't send back to sender
		}

		if err := gr.sendRoutedMessage(ctx, &routedMsg, peerID); err != nil {
			gr.logger.Warn("Failed to forward routed message",
				zap.String("peer", peerID.String()),
				zap.Error(err))
		}
	}

	return nil
}

// markAsSeen marks a message as seen
func (gr *GossipRouter) markAsSeen(messageID string) {
	gr.seenMu.Lock()
	defer gr.seenMu.Unlock()
	gr.seenMessages[messageID] = time.Now()
}

// hasSeen checks if a message has been seen before
func (gr *GossipRouter) hasSeen(messageID string) bool {
	gr.seenMu.RLock()
	defer gr.seenMu.RUnlock()
	_, exists := gr.seenMessages[messageID]
	return exists
}

// generateMessageID generates a unique message ID
func (gr *GossipRouter) generateMessageID(sessionID string) string {
	return fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
}

// cleanupSeenMessages periodically cleans up old seen messages
func (gr *GossipRouter) cleanupSeenMessages() {
	for range gr.cleanupTicker.C {
		gr.seenMu.Lock()
		cutoff := time.Now().Add(-defaultSeenMessageTTL)
		for msgID, timestamp := range gr.seenMessages {
			if timestamp.Before(cutoff) {
				delete(gr.seenMessages, msgID)
			}
		}
		gr.seenMu.Unlock()
	}
}

// Stop stops the gossip router
func (gr *GossipRouter) Stop() {
	if gr.cleanupTicker != nil {
		gr.cleanupTicker.Stop()
	}
}
