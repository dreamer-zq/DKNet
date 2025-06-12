package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

// Network provides secure P2P communication for TSS operations
type Network struct {
	host   host.Host
	dht    *dht.IpfsDHT
	pubsub *pubsub.PubSub
	logger *zap.Logger

	// Message handling
	handler MessageHandler

	// Connection management
	peers     map[peer.ID]bool
	peerMutex sync.RWMutex

	// PubSub topics
	discoveryTopic     *pubsub.Topic
	broadcastTopic     *pubsub.Topic
	nodeDiscoveryTopic *pubsub.Topic // New topic for node address discovery
	topicMutex         sync.RWMutex

	// Address management
	addressManager *AddressManager
}

// Config holds P2P network configuration
type Config struct {
	ListenAddrs    []string
	BootstrapPeers []string
	PrivateKeyFile string
	MaxPeers       int
	DataDir        string // Directory for storing node address mappings
	NodeID         string // This node's NodeID for TSS
	Moniker        string // Human-readable node name
}

// NewNetwork creates a new P2P network instance
func NewNetwork(cfg *Config, logger *zap.Logger) (*Network, error) {
	// Load or generate private key
	privKey, err := loadOrGeneratePrivateKey(cfg.PrivateKeyFile, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	// Parse listen addresses
	var listenAddrs []multiaddr.Multiaddr
	for _, addr := range cfg.ListenAddrs {
		multiAddr, parseErr := multiaddr.NewMultiaddr(addr)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid multiaddr %s: %w", addr, parseErr)
		}
		listenAddrs = append(listenAddrs, multiAddr)
	}

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	logger.Info("P2P node created",
		zap.String("peer_id", h.ID().String()),
		zap.Strings("listen_addrs", cfg.ListenAddrs))

	// Create DHT for peer discovery
	kademliaDHT, err := dht.New(context.Background(), h)
	if err != nil {
		if closeErr := h.Close(); closeErr != nil {
			logger.Error("Failed to close host during cleanup", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}

	// Create PubSub for broadcasting
	ps, err := pubsub.NewGossipSub(context.Background(), h)
	if err != nil {
		if closeErr := h.Close(); closeErr != nil {
			logger.Error("Failed to close host during cleanup", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("failed to create pubsub: %w", err)
	}

	n := &Network{
		host:   h,
		dht:    kademliaDHT,
		pubsub: ps,
		logger: logger,
		peers:  make(map[peer.ID]bool),
	}

	// Initialize address manager if NodeID is provided
	if cfg.NodeID != "" {
		addressManager, err := NewAddressManager(cfg.DataDir, cfg.NodeID, h.ID(), cfg.Moniker, logger.Named("address-manager"))
		if err != nil {
			if closeErr := h.Close(); closeErr != nil {
				logger.Error("Failed to close host during cleanup", zap.Error(closeErr))
			}
			return nil, fmt.Errorf("failed to create address manager: %w", err)
		}
		n.addressManager = addressManager

		// Set up broadcast callback for address manager
		addressManager.setBroadcastCallback(n.broadcastAddressBook)
	}

	// Set up protocol handlers
	n.setupProtocolHandlers()

	return n, nil
}

// Start starts the P2P network
func (n *Network) Start(ctx context.Context, bootstrapPeers []string) error {
	n.logger.Info("Starting P2P network",
		zap.Strings("bootstrap_peers", bootstrapPeers),
		zap.String("local_peer_id", n.host.ID().String()))

	// Bootstrap the DHT
	n.logger.Info("Bootstrapping DHT")
	if err := n.dht.Bootstrap(ctx); err != nil {
		n.logger.Error("Failed to bootstrap DHT", zap.Error(err))
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}
	n.logger.Info("DHT bootstrap completed")

	// Connect to bootstrap peers
	n.logger.Info("Connecting to bootstrap peers", zap.Int("peer_count", len(bootstrapPeers)))
	if err := n.connectToBootstrapPeers(ctx, bootstrapPeers); err != nil {
		n.logger.Warn("Failed to connect to some bootstrap peers", zap.Error(err))
	}

	// Subscribe to discovery topic
	n.logger.Info("Subscribing to topics")
	if err := n.subscribeToDiscovery(ctx); err != nil {
		n.logger.Error("Failed to subscribe to discovery", zap.Error(err))
		return fmt.Errorf("failed to subscribe to discovery: %w", err)
	}

	// Start address discovery if address manager is available
	if n.addressManager != nil {
		if err := n.StartAddressDiscovery(ctx); err != nil {
			n.logger.Error("Failed to start address discovery", zap.Error(err))
			return fmt.Errorf("failed to start address discovery: %w", err)
		}
	}

	n.logger.Info("P2P network started successfully")
	return nil
}

// Stop stops the P2P network
func (n *Network) Stop() error {
	if err := n.host.Close(); err != nil {
		return fmt.Errorf("failed to close host: %w", err)
	}
	n.logger.Info("P2P network stopped")
	return nil
}

// SetMessageHandler sets the message handler
func (n *Network) SetMessageHandler(handler MessageHandler) {
	n.handler = handler
}

// SendMessage sends a message to specific peers or broadcasts it
func (n *Network) SendMessage(ctx context.Context, msg *Message) error {
	if msg.IsBroadcast {
		return n.broadcastMessage(ctx, msg)
	}
	return n.sendDirectMessage(ctx, msg)
}

// GetPeerID returns the local peer ID
func (n *Network) GetPeerID() peer.ID {
	return n.host.ID()
}

// GetConnectedPeers returns the list of connected peers
func (n *Network) GetConnectedPeers() []peer.ID {
	n.peerMutex.RLock()
	defer n.peerMutex.RUnlock()

	var peers []peer.ID
	for peerID := range n.peers {
		peers = append(peers, peerID)
	}
	return peers
}

// setupProtocolHandlers sets up handlers for TSS protocols
func (n *Network) setupProtocolHandlers() {
	protocols := []protocol.ID{
		tssKeygenProtocol,
		tssSigningProtocol,
		tssResharingProtocol,
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

	// Read message
	data, err := io.ReadAll(stream)
	if err != nil {
		n.logger.Error("Failed to read stream", zap.Error(err))
		return
	}

	// Parse message
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		n.logger.Error("Failed to unmarshal message", zap.Error(err))
		return
	}

	// Extract and update sender's address mapping at network layer
	n.extractAndUpdateSenderMapping(&msg, stream.Conn().RemotePeer().String())

	// Handle message at business layer
	if n.handler != nil {
		ctx := context.Background()
		if err := n.handler.HandleMessage(ctx, &msg); err != nil {
			n.logger.Error("Failed to handle message", zap.Error(err))
		}
	}
}

// sendDirectMessage sends a message directly to specific peers
func (n *Network) sendDirectMessage(ctx context.Context, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Determine protocol based on message type
	var protocolID protocol.ID
	switch msg.Type {
	case "keygen":
		protocolID = tssKeygenProtocol
	case "signing":
		protocolID = tssSigningProtocol
	case "resharing":
		protocolID = tssResharingProtocol
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}

	// Send to each recipient
	for _, recipient := range msg.To {
		peerID, err := peer.Decode(recipient)
		if err != nil {
			n.logger.Error("Invalid peer ID", zap.String("peer_id", recipient), zap.Error(err))
			continue
		}

		stream, err := n.host.NewStream(ctx, peerID, protocolID)
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
	n.logger.Info("Starting broadcast message",
		zap.String("session_id", msg.SessionID),
		zap.String("type", msg.Type),
		zap.String("from", msg.From),
		zap.String("sender_peer_id", msg.SenderPeerID),
	)

	data, err := json.Marshal(msg)
	if err != nil {
		n.logger.Error("Failed to marshal broadcast message", zap.Error(err))
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Get the stored broadcast topic
	n.topicMutex.RLock()
	topic := n.broadcastTopic
	n.topicMutex.RUnlock()

	if topic == nil {
		n.logger.Error("Broadcast topic not initialized")
		return fmt.Errorf("broadcast topic not initialized")
	}

	n.logger.Debug("Using existing broadcast topic", zap.String("topic", tssBroadcastTopic))

	// Get current connected peers
	connectedPeers := n.GetConnectedPeers()
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

// connectToBootstrapPeers connects to bootstrap peers
func (n *Network) connectToBootstrapPeers(ctx context.Context, bootstrapPeers []string) error {
	if len(bootstrapPeers) == 0 {
		n.logger.Info("No bootstrap peers configured")
		return nil
	}

	n.logger.Info("Starting bootstrap peer connections", zap.Int("peer_count", len(bootstrapPeers)))

	var wg sync.WaitGroup
	for _, peerAddr := range bootstrapPeers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			n.logger.Info("Attempting to connect to bootstrap peer", zap.String("addr", addr))

			maddr, err := multiaddr.NewMultiaddr(addr)
			if err != nil {
				n.logger.Error("Invalid bootstrap peer address", zap.String("addr", addr), zap.Error(err))
				return
			}

			peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
			if err != nil {
				n.logger.Error("Failed to parse peer info", zap.String("addr", addr), zap.Error(err))
				return
			}

			n.logger.Info("Parsed peer info",
				zap.String("addr", addr),
				zap.String("peer_id", peerInfo.ID.String()))

			connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()

			if err := n.host.Connect(connectCtx, *peerInfo); err != nil {
				n.logger.Error("Failed to connect to bootstrap peer",
					zap.String("peer_id", peerInfo.ID.String()),
					zap.String("addr", addr),
					zap.Error(err))
				return
			}

			n.logger.Info("Connected to bootstrap peer",
				zap.String("peer_id", peerInfo.ID.String()),
				zap.String("addr", addr))

			// Update connected peers
			n.peerMutex.Lock()
			n.peers[peerInfo.ID] = true
			n.peerMutex.Unlock()
		}(peerAddr)
	}
	wg.Wait()

	// Log final connection status
	connectedPeers := n.GetConnectedPeers()
	n.logger.Debug("Bootstrap connection completed",
		zap.Int("connected_peers", len(connectedPeers)),
		zap.Int("attempted_peers", len(bootstrapPeers)))

	return nil
}

// subscribeToDiscovery subscribes to the discovery topic
func (n *Network) subscribeToDiscovery(ctx context.Context) error {
	// Subscribe to discovery topic
	discoveryTopic, err := n.pubsub.Join(tssDiscoveryTopic)
	if err != nil {
		return fmt.Errorf("failed to join discovery topic: %w", err)
	}

	// Store topic reference
	n.topicMutex.Lock()
	n.discoveryTopic = discoveryTopic
	n.topicMutex.Unlock()

	discoverySub, err := discoveryTopic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to discovery topic: %w", err)
	}

	n.logger.Info("Subscribed to discovery topic", zap.String("topic", tssDiscoveryTopic))

	// Handle discovery messages in a separate goroutine
	go func() {
		defer discoverySub.Cancel()
		for {
			message, msgErr := discoverySub.Next(ctx)
			if msgErr != nil {
				if ctx.Err() != nil {
					return // Context canceled
				}
				n.logger.Error("Error reading discovery message", zap.Error(msgErr))
				continue
			}

			// Process discovery message
			n.logger.Debug("Received discovery message",
				zap.String("from", message.ReceivedFrom.String()))
		}
	}()

	// Subscribe to broadcast topic for TSS messages
	broadcastTopic, err := n.pubsub.Join(tssBroadcastTopic)
	if err != nil {
		return fmt.Errorf("failed to join broadcast topic: %w", err)
	}

	// Store topic reference
	n.topicMutex.Lock()
	n.broadcastTopic = broadcastTopic
	n.topicMutex.Unlock()

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
			if err := json.Unmarshal(msg.Data, &tssMsg); err != nil {
				n.logger.Error("Failed to unmarshal broadcast message", zap.Error(err))
				continue
			}

			n.logger.Info("Parsed broadcast TSS message",
				zap.String("session_id", tssMsg.SessionID),
				zap.String("type", tssMsg.Type),
				zap.String("from", tssMsg.From),
				zap.Bool("is_broadcast", tssMsg.IsBroadcast))

			// Extract and update sender's address mapping at network layer
			n.extractAndUpdateSenderMapping(&tssMsg, msg.ReceivedFrom.String())

			// Handle the message if we have a handler
			if n.handler != nil {
				if err := n.handler.HandleMessage(ctx, &tssMsg); err != nil {
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

// loadOrGeneratePrivateKey loads a private key from file or generates a new one
func loadOrGeneratePrivateKey(keyFile string, logger *zap.Logger) (crypto.PrivKey, error) {
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

// broadcastAddressBook broadcasts an address book to the network
func (n *Network) broadcastAddressBook(book *AddressBook) error {
	data, err := json.Marshal(book)
	if err != nil {
		return fmt.Errorf("failed to marshal address book: %w", err)
	}

	// Get the node discovery topic
	n.topicMutex.RLock()
	topic := n.nodeDiscoveryTopic
	n.topicMutex.RUnlock()

	if topic == nil {
		return fmt.Errorf("node discovery topic not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := topic.Publish(ctx, data); err != nil {
		return fmt.Errorf("failed to publish address book: %w", err)
	}

	n.logger.Debug("Address book broadcasted successfully",
		zap.Int("mappings_count", len(book.Mappings)),
		zap.Int64("version", book.Version))

	return nil
}

// GetNodePeerID returns the PeerID for a given NodeID using the address manager
func (n *Network) GetNodePeerID(nodeID string) (string, bool) {
	if n.addressManager == nil {
		return "", false
	}
	return n.addressManager.getPeerID(nodeID)
}

// UpdateNodeMapping updates a node mapping temporarily (used during TSS operations)
func (n *Network) UpdateNodeMapping(nodeID, peerID, moniker string) error {
	if n.addressManager == nil {
		return fmt.Errorf("address manager not initialized")
	}

	// Update the mapping in address manager
	return n.addressManager.updateMapping(nodeID, peerID, moniker)
}

// GetAllNodeMappings returns all node address mappings
func (n *Network) GetAllNodeMappings() map[string]*NodeAddressMapping {
	if n.addressManager == nil {
		return make(map[string]*NodeAddressMapping)
	}
	
	return n.addressManager.getAllMappings()
}

// StartAddressDiscovery starts the address discovery and broadcasting process
func (n *Network) StartAddressDiscovery(ctx context.Context) error {
	if n.addressManager == nil {
		return fmt.Errorf("address manager not initialized")
	}

	// Subscribe to node discovery topic
	nodeDiscoveryTopic, err := n.pubsub.Join(nodeDiscoveryTopic)
	if err != nil {
		return fmt.Errorf("failed to join node discovery topic: %w", err)
	}

	// Store topic reference
	n.topicMutex.Lock()
	n.nodeDiscoveryTopic = nodeDiscoveryTopic
	n.topicMutex.Unlock()

	nodeDiscoverySub, err := nodeDiscoveryTopic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to node discovery topic: %w", err)
	}

	n.logger.Info("Subscribed to node discovery topic", zap.String("topic", "node-discovery"))

	// Handle node discovery messages in a separate goroutine
	go func() {
		defer nodeDiscoverySub.Cancel()
		for {
			message, msgErr := nodeDiscoverySub.Next(ctx)
			if msgErr != nil {
				if ctx.Err() != nil {
					return // Context canceled
				}
				n.logger.Error("Error reading node discovery message", zap.Error(msgErr))
				continue
			}

			// Skip our own messages
			if message.ReceivedFrom == n.host.ID() {
				continue
			}

			// Parse and merge address book
			var addressBook AddressBook
			if err := json.Unmarshal(message.Data, &addressBook); err != nil {
				n.logger.Warn("Failed to unmarshal address book from peer",
					zap.String("from", message.ReceivedFrom.String()),
					zap.Error(err))
				continue
			}

			if err := n.addressManager.mergeAddressBook(&addressBook); err != nil {
				n.logger.Warn("Failed to merge address book from peer",
					zap.String("from", message.ReceivedFrom.String()),
					zap.Error(err))
			}
		}
	}()

	// Broadcast our own mapping immediately upon startup
	if err := n.addressManager.broadcastOwnMapping(); err != nil {
		n.logger.Warn("Failed to broadcast own mapping on startup", zap.Error(err))
	}

	// Start periodic broadcasting of full address book
	n.addressManager.startPeriodicBroadcast(addressBroadcastInterval)

	n.logger.Info("Address discovery started successfully")
	return nil
}

// extractAndUpdateSenderMapping extracts and updates the sender's address mapping at the network layer
func (n *Network) extractAndUpdateSenderMapping(msg *Message, peerID string) {
	// Only update mapping if we have sender peer ID information
	if msg.SenderPeerID == "" || msg.From == "" {
		return
	}

	// Skip our own messages
	if msg.SenderPeerID == n.host.ID().String() {
		return
	}

	// Update the sender's address mapping in the address manager
	if n.addressManager != nil {
		n.logger.Debug("Updating node mapping at network layer",
			zap.String("sender_node_id", msg.From),
			zap.String("sender_peer_id", msg.SenderPeerID))

		if err := n.addressManager.updateMapping(msg.From, msg.SenderPeerID, ""); err != nil {
			n.logger.Warn("Failed to update sender's address mapping at network layer",
				zap.String("sender_node_id", msg.From),
				zap.String("sender_peer_id", msg.SenderPeerID),
				zap.Error(err))
		}
	}
}
