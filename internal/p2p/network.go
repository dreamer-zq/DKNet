package p2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

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
	host          host.Host
	pubsub        *pubsub.PubSub
	logger        *zap.Logger
	connectTicker *time.Ticker

	// Message handling
	messageHandler MessageHandler
	mu             sync.RWMutex

	// Connected peers tracking
	connectedPeers map[peer.ID]bool
	bootstrapPeers []string
	// Gossip routing for point-to-point messages
	gossipRouter *GossipRouter
	// Access control
	accessController security.AccessController
	// Unified message encryption
	messageEncryption security.MessageEncryption
}

// Config holds P2P network configuration
type Config struct {
	ListenAddrs    []string
	BootstrapPeers []string
	PrivateKeyFile string
	MaxPeers       int

	// Access control configuration
	AccessControl *config.AccessControlConfig

	// Encryption configuration
	SessionEncryption *config.SessionEncryptionConfig
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

	// Initialize unified message encryption
	encryptionConfig := &security.EncryptionConfig{
		EnableEncryption: cfg.SessionEncryption.Enabled,
		PrivateKey:       privKey,
		Peerstore:        h.Peerstore(),
	}

	// Configure session encryption if enabled
	if cfg.SessionEncryption.Enabled {
		seedKey, err := hex.DecodeString(cfg.SessionEncryption.SeedKey)
		if err != nil {
			return nil, fmt.Errorf("invalid session encryption seed key: %w", err)
		}
		encryptionConfig.SessionSeedKey = seedKey
	}

	messageEncryption := security.NewMessageEncryption(encryptionConfig, logger)

	n := &Network{
		host:              h,
		pubsub:            ps,
		logger:            logger,
		connectedPeers:    make(map[peer.ID]bool),
		accessController:  security.NewController(cfg.AccessControl, logger.Named("access_control")),
		messageEncryption: messageEncryption,
		bootstrapPeers:    cfg.BootstrapPeers,
		connectTicker:     time.NewTicker(time.Second * 10),
	}

	// Set up protocol handlers
	n.setupProtocolHandlers()

	// Initialize gossip router
	n.gossipRouter = NewGossipRouter(n, logger.Named("gossip"))

	return n, nil
}

// Start starts the P2P network
func (n *Network) Start(ctx context.Context) error {
	n.logger.Info("Starting P2P network")

	// Connect to bootstrap peers
	go n.tryConnect()

	n.logger.Info("P2P network started successfully")
	return nil
}

// Stop stops the P2P network
func (n *Network) Stop() error {
	// Stop gossip router
	if n.gossipRouter != nil {
		n.gossipRouter.Stop()
	}
	n.connectTicker.Stop()

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

// SendWithGossip sends a message using gossip routing as fallback
func (n *Network) SendWithGossip(ctx context.Context, msg *Message) error {
	return n.gossipRouter.SendWithGossip(ctx, msg)
}

// send sends a message to specific peers
func (n *Network) send(ctx context.Context, msg *Message) error {
	// Apply encryption before sending
	if err := n.encryptMessage(msg); err != nil {
		return fmt.Errorf("failed to encrypt message: %w", err)
	}

	return n.sendDirect(ctx, msg)
}

// encryptMessage applies encryption to a message using unified encryption interface
func (n *Network) encryptMessage(msg *Message) error {
	// Create encryption context
	ctx := &security.MessageEncryptionContext{
		Data:         msg.Data,
		Encrypted:    msg.Encrypted,
		IsBroadcast:  msg.IsBroadcast,
		Recipients:   msg.To,
		SessionID:    msg.SessionID,
		SenderPeerID: msg.SenderPeerID,

		// Set callback functions to update the message
		SetData: func(data []byte) {
			msg.Data = data
		},
		SetEncrypted: func(encrypted bool) {
			msg.Encrypted = encrypted
		},
	}

	// Use unified encryption interface
	return n.messageEncryption.Encrypt(ctx)
}

// decryptMessage applies decryption to a message using unified encryption interface
func (n *Network) decryptMessage(msg *Message) error {
	// Create decryption context
	ctx := &security.MessageEncryptionContext{
		Data:         msg.Data,
		Encrypted:    msg.Encrypted,
		IsBroadcast:  msg.IsBroadcast,
		Recipients:   msg.To,
		SessionID:    msg.SessionID,
		SenderPeerID: msg.SenderPeerID,

		// Set callback functions to update the message
		SetData: func(data []byte) {
			msg.Data = data
		},
		SetEncrypted: func(encrypted bool) {
			msg.Encrypted = encrypted
		},
	}

	// Use unified decryption interface
	return n.messageEncryption.Decrypt(ctx)
}

// sendDirect sends a message directly to specific peers
func (n *Network) sendDirect(ctx context.Context, msg *Message) error {
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

// getConnectedPeers returns the list of connected peers
func (n *Network) getConnectedPeers() []peer.ID {
	// Get connected peers directly from libp2p host
	return n.host.Network().Peers()
}

// setupProtocolHandlers sets up handlers for TSS protocols
func (n *Network) setupProtocolHandlers() {
	protocols := []protocol.ID{
		TssPartyProtocolId,
		TssGossipProtocol,
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

	// Apply decryption if needed
	if err := n.decryptMessage(&msg); err != nil {
		n.logger.Error("Failed to decrypt stream message",
			zap.String("peer_id", remotePeerID.String()),
			zap.Error(err))
		return
	}

	// Handle gossip routing messages
	if msg.ProtocolID == TssGossipProtocol {
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

// GetHostID implements NetworkTransport interface
func (n *Network) GetHostID() string {
	return n.host.ID().String()
}

func (n *Network) tryConnect() {
	// Connect to bootstrap peers
	connect := func() {
		for _, peerAddr := range n.bootstrapPeers {
			go func(addr string) {
				peerInfo, err := peer.AddrInfoFromString(addr)
				if err != nil {
					n.logger.Error("Invalid bootstrap peer address", zap.String("addr", addr), zap.Error(err))
					return
				}

				if err := n.host.Connect(context.Background(), *peerInfo); err != nil {
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
	}
	for {
		<-n.connectTicker.C
		connect()
	}
}

func (n *Network) connected(peerID peer.ID) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.connectedPeers[peerID]
}
