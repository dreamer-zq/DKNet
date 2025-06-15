package p2p

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/security"
)

// Network handles P2P networking for TSS operations
type Network struct {
	host   host.Host
	pubsub *pubsub.PubSub
	logger *zap.Logger

	// Message handling
	messageHandler MessageHandler
	mu             sync.RWMutex

	// Topics for different purposes
	topics map[string]*pubsub.Topic

	// Connected peers tracking
	connectedPeers map[peer.ID]bool

	// Gossip routing for point-to-point messages
	gossipRouter *GossipRouter

	// Access control
	accessController security.AccessController
}

// Config holds P2P network configuration
type Config struct {
	ListenAddrs    []string
	BootstrapPeers []string
	PrivateKeyFile string
	MaxPeers       int

	// Access control configuration
	AccessControl *config.AccessControlConfig
}

// NewNetwork creates a new P2P network instance
func NewNetwork(cfg *Config, logger *zap.Logger) (*Network, error) {
	// Create libp2p host
	privKey, err := loadPrivateKey(cfg.PrivateKeyFile, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(cfg.ListenAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Create PubSub
	ps, err := pubsub.NewGossipSub(context.Background(), h)
	if err != nil {
		if closeErr := h.Close(); closeErr != nil {
			logger.Error("Failed to close host during cleanup", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("failed to create pubsub: %w", err)
	}

	n := &Network{
		host:             h,
		pubsub:           ps,
		logger:           logger,
		topics:           make(map[string]*pubsub.Topic),
		connectedPeers:   make(map[peer.ID]bool),
		accessController: security.NewController(cfg.AccessControl, logger.Named("access_control")),
	}

	// Set up protocol handlers
	n.setupProtocolHandlers()

	// Initialize gossip router
	n.gossipRouter = NewGossipRouter(n, logger.Named("gossip"))

	return n, nil
}

// Start starts the P2P network
func (n *Network) Start(ctx context.Context, bootstrapPeers []string) error {
	n.logger.Info("Starting P2P network", zap.Strings("bootstrap_peers", bootstrapPeers))

	// Connect to bootstrap peers
	for _, peerAddr := range bootstrapPeers {
		go func(addr string) {
			peerInfo, err := peer.AddrInfoFromString(addr)
			if err != nil {
				n.logger.Error("Invalid bootstrap peer address", zap.String("addr", addr), zap.Error(err))
				return
			}

			if err := n.host.Connect(ctx, *peerInfo); err != nil {
				n.logger.Warn("Failed to connect to bootstrap peer", zap.String("addr", addr), zap.Error(err))
				return
			}

			n.logger.Info("Connected to bootstrap peer", zap.String("peer", peerInfo.ID.String()))

			// Update connected peers
			n.mu.Lock()
			n.connectedPeers[peerInfo.ID] = true
			n.mu.Unlock()
		}(peerAddr)
	}

	// Subscribe to discovery and broadcast topics
	if err := n.subscribeToTopics(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to topics: %w", err)
	}

	n.logger.Info("P2P network started successfully")
	return nil
}

// Stop stops the P2P network
func (n *Network) Stop() error {
	// Stop gossip router
	if n.gossipRouter != nil {
		n.gossipRouter.Stop()
	}

	if err := n.host.Close(); err != nil {
		return fmt.Errorf("failed to close host: %w", err)
	}
	n.logger.Info("P2P network stopped")
	return nil
}

// SetMessageHandler sets the message handler
func (n *Network) SetMessageHandler(handler MessageHandler) {
	n.messageHandler = handler
}

// SendMessage sends a message to specific peers or broadcasts it
func (n *Network) SendMessage(ctx context.Context, msg *Message) error {
	if msg.IsBroadcast {
		return n.broadcastMessage(ctx, msg)
	}
	msg.ProtocolID = typeToProtocol[msg.Type]
	return n.sendDirectMessage(ctx, msg)
}

// SendMessageWithGossip sends a message using gossip routing as fallback
func (n *Network) SendMessageWithGossip(ctx context.Context, msg *Message) error {
	return n.gossipRouter.SendWithGossip(ctx, msg)
}

// getConnectedPeers returns the list of connected peers
func (n *Network) getConnectedPeers() []peer.ID {
	// Get connected peers directly from libp2p host
	return n.host.Network().Peers()
}

// setupProtocolHandlers sets up handlers for TSS protocols
func (n *Network) setupProtocolHandlers() {
	protocols := []protocol.ID{
		tssKeygenProtocol,
		tssSigningProtocol,
		tssResharingProtocol,
		tssGossipProtocol,
	}

	for _, p := range protocols {
		n.host.SetStreamHandler(p, n.handleStream)
	}
}

// handleStream handles incoming streams
func (n *Network) handleStream(stream network.Stream) {
	defer func() {
		if err := stream.Close(); err != nil {
			n.logger.Warn("Failed to close incoming stream", zap.Error(err))
		}
	}()

	// Get the remote peer ID
	remotePeerID := stream.Conn().RemotePeer()

	// Access control check
	if !n.accessController.IsAuthorized(remotePeerID.String()) {
		n.logger.Warn("Rejected stream from unauthorized peer",
			zap.String("peer_id", remotePeerID.String()),
			zap.String("protocol", string(stream.Protocol())))
		return
	}

	data, err := io.ReadAll(stream)
	if err != nil {
		n.logger.Error("Failed to read stream", zap.Error(err))
		return
	}

	var msg Message
	if err := msg.Decompresses(data); err != nil {
		n.logger.Error("Failed to unmarshal message", zap.Error(err))
		return
	}

	// Handle gossip routing messages
	if msg.Type == "gossip_route" {
		if err := n.gossipRouter.HandleRoutedMessage(context.Background(), &msg); err != nil {
			n.logger.Error("Failed to handle gossip routed message", zap.Error(err))
		}
		return
	}

	// Handle regular messages at business layer
	if n.messageHandler != nil {
		ctx := context.Background()
		if err := n.messageHandler.HandleMessage(ctx, &msg); err != nil {
			n.logger.Error("Failed to handle message", zap.Error(err))
		}
	}
}

// sendDirectMessage sends a message directly to specific peers
func (n *Network) sendDirectMessage(ctx context.Context, msg *Message) error {
	// Fill in the sender's actual PeerID
	msg.SenderPeerID = n.host.ID().String()

	// Validate all peer IDs first
	for _, recipient := range msg.To {
		if _, err := peer.Decode(recipient); err != nil {
			return fmt.Errorf("invalid peer ID format: %s, error: %w", recipient, err)
		}
	}

	data, err := msg.Compresses()
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send to each recipient
	for _, recipient := range msg.To {
		peerID, err := peer.Decode(recipient)
		if err != nil {
			// This should not happen since we validated above, but keep for safety
			n.logger.Error("Invalid peer ID", zap.String("peer_id", recipient), zap.Error(err))
			continue
		}

		stream, err := n.host.NewStream(ctx, peerID, msg.ProtocolID)
		if err != nil {
			n.logger.Error("Failed to create stream", zap.String("peer_id", recipient), zap.Error(err))
			continue
		}

		if _, err := stream.Write(data); err != nil {
			n.logger.Error("Failed to write to stream", zap.String("peer_id", recipient), zap.Error(err))
		}

		if err := stream.Close(); err != nil {
			n.logger.Warn("Failed to close stream", zap.String("peer_id", recipient), zap.Error(err))
		}
	}

	return nil
}

// broadcastMessage broadcasts a message using PubSub
func (n *Network) broadcastMessage(ctx context.Context, msg *Message) error {
	// Fill in the sender's actual PeerID
	msg.SenderPeerID = n.host.ID().String()

	data, err := msg.Compresses()
	if err != nil {
		n.logger.Error("Failed to marshal broadcast message", zap.Error(err))
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Get the stored broadcast topic
	n.mu.RLock()
	topic := n.topics[tssBroadcastTopic]
	n.mu.RUnlock()

	if topic == nil {
		n.logger.Error("Broadcast topic not initialized")
		return fmt.Errorf("broadcast topic not initialized")
	}

	n.logger.Debug("Using existing broadcast topic", zap.String("topic", tssBroadcastTopic))

	// Get current connected peers
	connectedPeers := n.getConnectedPeers()
	n.logger.Debug("Broadcasting to connected peers",
		zap.Int("connected_peer_count", len(connectedPeers)),
		zap.String("session_id", msg.SessionID))

	if err := topic.Publish(ctx, data); err != nil {
		n.logger.Error("Failed to publish broadcast message",
			zap.Error(err),
			zap.String("session_id", msg.SessionID))
		return fmt.Errorf("failed to publish message: %w", err)
	}

	n.logger.Info("Broadcast message published successfully",
		zap.String("session_id", msg.SessionID),
		zap.Int("message_size", len(data)))

	return nil
}

// loadPrivateKey loads a private key from file
func loadPrivateKey(keyFile string, logger *zap.Logger) (crypto.PrivKey, error) {
	// Log the key file path
	logger.Info("Attempting to load private key from", zap.String("key_file", keyFile))

	// Try to load existing key
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return nil, err
	}

	// Load existing key
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		logger.Error("Failed to read key file", zap.Error(err))
		return nil, err
	}

	privKey, err := crypto.UnmarshalPrivateKey(keyBytes)
	if err != nil {
		logger.Error("Failed to unmarshal private key", zap.Error(err))
		return nil, err
	}

	logger.Info("Successfully loaded private key from file")
	return privKey, nil
}

// JoinTopic implements networkTransport interface
func (n *Network) JoinTopic(topicName string) (*pubsub.Topic, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if topic already exists
	if topic, exists := n.topics[topicName]; exists && topic != nil {
		return topic, nil
	}

	// Join the topic
	topic, err := n.pubsub.Join(topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to join topic %s: %w", topicName, err)
	}

	// Store the topic
	n.topics[topicName] = topic

	n.logger.Debug("Joined topic", zap.String("topic", topicName))
	return topic, nil
}

// GetHostID implements NetworkTransport interface
func (n *Network) GetHostID() peer.ID {
	return n.host.ID()
}

// subscribeToTopics subscribes to the necessary topics for TSS operations
func (n *Network) subscribeToTopics(ctx context.Context) error {
	// Subscribe to broadcast topic for TSS messages
	if err := n.subscribeToBroadcast(ctx); err != nil {
		return fmt.Errorf("failed to subscribe to broadcast: %w", err)
	}

	return nil
}

// subscribeToBroadcast subscribes to the TSS broadcast topic
func (n *Network) subscribeToBroadcast(ctx context.Context) error {
	// Subscribe to broadcast topic for TSS messages
	broadcastTopic, err := n.pubsub.Join(tssBroadcastTopic)
	if err != nil {
		return fmt.Errorf("failed to join broadcast topic: %w", err)
	}

	// Store topic reference
	n.mu.Lock()
	n.topics[tssBroadcastTopic] = broadcastTopic
	n.mu.Unlock()

	broadcastSub, err := broadcastTopic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to broadcast topic: %w", err)
	}

	n.logger.Info("Subscribed to broadcast topic", zap.String("topic", tssBroadcastTopic))

	// Handle broadcast messages in a separate goroutine
	go func() {
		defer broadcastSub.Cancel()
		for {
			msg, err := broadcastSub.Next(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return // Context canceled
				}
				n.logger.Error("Error reading broadcast message", zap.Error(err))
				continue
			}

			n.logger.Info("Received broadcast message",
				zap.String("from", msg.ReceivedFrom.String()),
				zap.Int("data_len", len(msg.Data)))

			// Parse the message
			var tssMsg Message
			if err := tssMsg.Decompresses(msg.Data); err != nil {
				n.logger.Error("Failed to unmarshal broadcast message", zap.Error(err))
				continue
			}

			n.logger.Info("Parsed broadcast TSS message",
				zap.String("session_id", tssMsg.SessionID),
				zap.String("type", tssMsg.Type),
				zap.String("from", tssMsg.From),
				zap.Bool("is_broadcast", tssMsg.IsBroadcast))

			// Handle the message if we have a handler
			if n.messageHandler != nil {
				if err := n.messageHandler.HandleMessage(ctx, &tssMsg); err != nil {
					n.logger.Error("Failed to handle broadcast message",
						zap.Error(err),
						zap.String("session_id", tssMsg.SessionID))
				}
			} else {
				n.logger.Warn("No message handler set for broadcast message",
					zap.String("session_id", tssMsg.SessionID))
			}
		}
	}()

	return nil
}
