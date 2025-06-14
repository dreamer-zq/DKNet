package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"
)

// GossipRouter implements gossip-based routing for point-to-point messages
type GossipRouter struct {
	network *Network
	logger  *zap.Logger
	
	// Routing table: target -> next hop
	routingTable map[string]peer.ID
	mu           sync.RWMutex
	
	// Message tracking to prevent loops
	seenMessages map[string]time.Time
	seenMu       sync.RWMutex
	
	// Configuration
	maxTTL       int
	cleanupTicker *time.Ticker
}

// RoutedMessage wraps a message with routing information
type RoutedMessage struct {
	*Message
	OriginalSender string   `json:"original_sender"`
	FinalTarget    string   `json:"final_target"`
	Path           []string `json:"path"`
	TTL            int      `json:"ttl"`
	MessageID      string   `json:"message_id"`
}

// NewGossipRouter creates a new gossip router
func NewGossipRouter(network *Network, logger *zap.Logger) *GossipRouter {
	router := &GossipRouter{
		network:      network,
		logger:       logger,
		routingTable: make(map[string]peer.ID),
		seenMessages: make(map[string]time.Time),
		maxTTL:       10, // Maximum hops
	}
	
	// Start cleanup routine for seen messages
	router.cleanupTicker = time.NewTicker(5 * time.Minute)
	go router.cleanupSeenMessages()
	
	return router
}

// SendWithGossip sends a message using gossip routing if direct connection fails
func (gr *GossipRouter) SendWithGossip(ctx context.Context, msg *Message) error {
	if msg.IsBroadcast {
		// Broadcast messages use PubSub gossip (already implemented)
		return gr.network.SendMessage(ctx, msg)
	}
	
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

// sendToTarget attempts direct delivery to target
func (gr *GossipRouter) sendToTarget(ctx context.Context, msg *Message, target string) error {
	targetPeer, err := peer.Decode(target)
	if err != nil {
		return fmt.Errorf("invalid target peer ID: %w", err)
	}
	
	// Check if we're directly connected
	if gr.network.host.Network().Connectedness(targetPeer) == 1 { // Connected
		// Try direct send
		directMsg := *msg
		directMsg.To = []string{target}
		// Ensure ProtocolID is set based on message type
		if directMsg.ProtocolID == "" {
			if protocolID, exists := typeToProtocol[directMsg.Type]; exists {
				directMsg.ProtocolID = protocolID
			} else {
				directMsg.ProtocolID = tssGossipProtocol // fallback
			}
		}
		return gr.network.sendDirectMessage(ctx, &directMsg)
	}
	
	return fmt.Errorf("not directly connected to target")
}

// sendViaGossip sends message via gossip routing
func (gr *GossipRouter) sendViaGossip(ctx context.Context, msg *Message, target string) error {
	// Create routed message
	routedMsg := &RoutedMessage{
		Message:        msg,
		OriginalSender: gr.network.host.ID().String(),
		FinalTarget:    target,
		Path:           []string{gr.network.host.ID().String()},
		TTL:            gr.maxTTL,
		MessageID:      fmt.Sprintf("%s-%d", msg.SessionID, time.Now().UnixNano()),
	}
	
	// Mark as seen to prevent loops
	gr.markAsSeen(routedMsg.MessageID)
	
	// Find best next hops (connected peers)
	connectedPeers := gr.network.getConnectedPeers()
	if len(connectedPeers) == 0 {
		return fmt.Errorf("no connected peers for gossip routing")
	}
	
	// Send to all connected peers (they will decide whether to forward)
	for _, peerID := range connectedPeers {
		if peerID.String() == target {
			// Direct connection found!
			directMsg := *msg
			directMsg.To = []string{target}
			// Ensure ProtocolID is set based on message type
			if directMsg.ProtocolID == "" {
				if protocolID, exists := typeToProtocol[directMsg.Type]; exists {
					directMsg.ProtocolID = protocolID
				} else {
					directMsg.ProtocolID = tssGossipProtocol // fallback
				}
			}
			return gr.network.sendDirectMessage(ctx, &directMsg)
		}
		
		// Send routed message to peer
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
	data, err := json.Marshal(routedMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal routed message: %w", err)
	}
	
	// Create a special gossip message
	gossipMsg := &Message{
		SessionID:   routedMsg.SessionID,
		Type:        "gossip_route",
		From:        gr.network.host.ID().String(),
		To:          []string{peerID.String()},
		Data:        data,
		IsBroadcast: false,
		Timestamp:   time.Now(),
		ProtocolID:  "/tss/gossip/1.0.0",
	}
	
	return gr.network.sendDirectMessage(ctx, gossipMsg)
}

// HandleRoutedMessage handles incoming routed messages
func (gr *GossipRouter) HandleRoutedMessage(ctx context.Context, msg *Message) error {
	var routedMsg RoutedMessage
	if err := json.Unmarshal(msg.Data, &routedMsg); err != nil {
		return fmt.Errorf("failed to unmarshal routed message: %w", err)
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
	if routedMsg.FinalTarget == gr.network.host.ID().String() {
		gr.logger.Info("Received message via gossip routing",
			zap.String("original_sender", routedMsg.OriginalSender),
			zap.Strings("path", routedMsg.Path))
		
		// Deliver to local handler
		return gr.network.messageHandler.HandleMessage(ctx, routedMsg.Message)
	}
	
	// Check if we know the target directly
	targetPeer, err := peer.Decode(routedMsg.FinalTarget)
	if err == nil && gr.network.host.Network().Connectedness(targetPeer) == 1 {
		// Forward directly to target
		gr.logger.Info("Forwarding message directly to target",
			zap.String("target", routedMsg.FinalTarget),
			zap.String("message_id", routedMsg.MessageID))
		
		directMsg := *routedMsg.Message
		directMsg.To = []string{routedMsg.FinalTarget}
		// Ensure ProtocolID is set based on message type
		if directMsg.ProtocolID == "" {
			if protocolID, exists := typeToProtocol[directMsg.Type]; exists {
				directMsg.ProtocolID = protocolID
			} else {
				directMsg.ProtocolID = tssGossipProtocol // fallback
			}
		}
		return gr.network.sendDirectMessage(ctx, &directMsg)
	}
	
	// Continue gossip forwarding
	routedMsg.TTL--
	routedMsg.Path = append(routedMsg.Path, gr.network.host.ID().String())
	
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

// cleanupSeenMessages periodically cleans up old seen messages
func (gr *GossipRouter) cleanupSeenMessages() {
	for range gr.cleanupTicker.C {
		gr.seenMu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
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