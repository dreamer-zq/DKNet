package p2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-msgio"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/common"
	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/security"
)

// Network handles P2P networking for TSS operations
type Network struct {
	host   host.Host
	logger *zap.Logger

	// Message handling
	messageHandler MessageHandler
	// Stream management for direct messages
	streamCache *common.SafeMap[peer.ID, network.Stream]
	// Connected peers tracking
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

	// Create a connection manager
	lowWater := len(cfg.BootstrapPeers)
	highWater := cfg.MaxPeers
	if highWater == 0 {
		highWater = lowWater + 20 // Default if not set
	}
	if highWater < lowWater {
		logger.Warn("MaxPeers configuration is less than the number of bootstrap peers, setting highWater to lowWater",
			zap.Int("max_peers", highWater),
			zap.Int("bootstrap_peers", lowWater),
		)
		highWater = lowWater
	}

	cm, err := connmgr.NewConnManager(lowWater, highWater, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		return nil, fmt.Errorf("failed to create connection manager: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(cfg.ListenAddrs...),
		libp2p.ConnectionManager(cm),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
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
		logger:            logger,
		streamCache:       common.New[peer.ID, network.Stream](),
		accessController:  security.NewController(cfg.AccessControl, logger.Named("access_control")),
		messageEncryption: messageEncryption,
		bootstrapPeers:    cfg.BootstrapPeers,
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

	// Connect to bootstrap peers, the connection manager will handle retries.
	go n.bootstrapConnect()

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

// SendMessage sends a message using gossip routing as fallback
func (n *Network) SendMessage(ctx context.Context, msg *Message) error {
	return n.gossipRouter.SendMessage(ctx, msg)
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

	// Apply encryption before sending
	if err := n.encryptMessage(msg); err != nil {
		return fmt.Errorf("failed to encrypt message: %w", err)
	}

	data, err := msg.Compresses()
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	n.logger.Info("Sending message directly to peers",
		zap.String("sender", msg.SenderPeerID),
		zap.Strings("to", msg.To),
		zap.String("protocol", string(msg.ProtocolID)),
		zap.String("session_id", msg.SessionID),
		zap.String("type", msg.Type),
	)

	// Send to each recipient
	for _, recipient := range msg.To {
		peerID, err := peer.Decode(recipient)
		if err != nil {
			n.logger.Error("Invalid peer ID, skipping", zap.String("peer_id", recipient), zap.Error(err))
			continue
		}
		if err := n.sendToPeer(ctx, peerID, msg.ProtocolID, data); err != nil {
			n.logger.Error("Failed to send message to peer", zap.String("peer_id", recipient), zap.Error(err))
		}
	}

	return nil
}

// sendToPeer handles sending data to a single peer, reusing streams.
func (n *Network) sendToPeer(ctx context.Context, peerID peer.ID, proto protocol.ID, data []byte) error {
	stream, err := n.getStream(ctx, peerID, proto)
	if err != nil {
		return fmt.Errorf("failed to get stream for peer %s: %w", peerID, err)
	}

	// Use msgio to write length-prefixed message
	writer := msgio.NewWriter(stream)
	if err := writer.WriteMsg(data); err != nil {
		// If write fails, the stream is likely broken. Reset it, so a new one is created next time.
		_ = stream.Reset()
		n.streamCache.Delete(peerID)
		return fmt.Errorf("failed to write message to stream for peer %s: %w", peerID, err)
	}
	return nil
}

// getStream gets a cached stream for a peer or creates a new one.
func (n *Network) getStream(ctx context.Context, peerID peer.ID, proto protocol.ID) (network.Stream, error) {
	n.logger.Info("Getting stream for peer",
		zap.String("peer_id", peerID.String()),
		zap.String("protocol", string(proto)),
	)

	createStream := func() (network.Stream, error) {
		n.logger.Debug("Creating new stream for peer",
			zap.String("peer_id", peerID.String()),
			zap.String("protocol", string(proto)),
		)

		stream, err := n.host.NewStream(ctx, peerID, proto)
		if err != nil {
			return nil, err
		}
		n.streamCache.Set(peerID, stream)
		return stream, nil
	}

	// Check if the stream is in the cache
	stream, ok := n.streamCache.Get(peerID)
	if !ok {
		n.logger.Debug("Stream not found in cache, creating new one",
			zap.String("peer_id", peerID.String()),
			zap.String("protocol", string(proto)),
		)
		return createStream()
	}

	// If the stream is not closed, return it
	if !stream.Conn().IsClosed() {
		n.logger.Debug("Stream found in cache, and it is not closed, returning it",
			zap.String("peer_id", peerID.String()),
			zap.String("protocol", string(proto)),
		)
		return stream, nil
	}

	n.logger.Debug("Stream found in cache, but it is closed, resetting it",
		zap.String("peer_id", peerID.String()),
		zap.String("protocol", string(proto)),
	)

	// If the stream is closed, reset it and create a new one
	_ = stream.Reset()
	n.streamCache.Delete(peerID)
	return createStream()
}

// getConnectedPeers returns the list of connected peers
func (n *Network) getConnectedPeers() []peer.ID {
	// Get connected peers directly from libp2p host
	return n.host.Network().Peers()
}

// setupProtocolHandlers sets up handlers for TSS protocols
func (n *Network) setupProtocolHandlers() {
	protocols := []protocol.ID{
		TssPartyProtocolID,
		TssGossipProtocol,
	}

	for _, p := range protocols {
		n.host.SetStreamHandler(p, n.handleStream)
	}
}

// handleStream handles incoming streams, reading multiple messages in a loop.
func (n *Network) handleStream(stream network.Stream) {
	defer func() {
		if err := stream.Close(); err != nil {
			n.logger.Warn("Failed to close incoming stream", zap.Error(err))
		}
	}()

	remotePeerID := stream.Conn().RemotePeer()

	// Access control check once per stream
	if !n.accessController.IsAuthorized(remotePeerID.String()) {
		n.logger.Warn("Rejected stream from unauthorized peer",
			zap.String("peer_id", remotePeerID.String()),
			zap.String("protocol", string(stream.Protocol())))
		_ = stream.Reset()
		return
	}

	reader := msgio.NewReader(stream)

	for {
		data, err := reader.ReadMsg()
		if err != nil {
			if err != io.EOF && err.Error() != "stream reset" {
				n.logger.Debug("Stream read error", zap.Error(err), zap.String("peer", remotePeerID.String()))
			}
			// Stream is closed, exit the loop.
			return
		}

		// Process the message in a new goroutine to avoid blocking the read loop.
		// A copy of the data is created because the buffer is recycled by ReleaseMsg.
		msgData := make([]byte, len(data))
		copy(msgData, data)
		go n.processIncomingMessage(msgData, remotePeerID)

		reader.ReleaseMsg(data)
	}
}

// processIncomingMessage handles the logic for a single received message.
func (n *Network) processIncomingMessage(data []byte, remotePeerID peer.ID) {
	var msg Message
	if err := msg.Decompresses(data); err != nil {
		n.logger.Error("Failed to unmarshal message", zap.Error(err), zap.String("peer", remotePeerID.String()))
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

// bootstrapConnect performs a single connection attempt to each bootstrap peer.
// This is to seed the peerstore, so the connection manager can take over.
func (n *Network) bootstrapConnect() {
	// Connect to bootstrap peers
	for _, peerAddr := range n.bootstrapPeers {
		go func(addr string) {
			peerInfo, err := peer.AddrInfoFromString(addr)
			if err != nil {
				n.logger.Error("Invalid bootstrap peer address", zap.String("addr", addr), zap.Error(err))
				return
			}
			// Use a timeout for the initial connection attempt
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			if err := n.host.Connect(ctx, *peerInfo); err != nil {
				n.logger.Warn("Failed to connect to bootstrap peer, connection manager will retry", zap.String("addr", addr), zap.Error(err))
				return
			}
			n.logger.Info("Connected to bootstrap peer", zap.String("peer", peerInfo.ID.String()))
		}(peerAddr)
	}
}

// connected checks if the network is directly connected to a given peer.
func (n *Network) connected(peerID peer.ID) bool {
	return n.host.Network().Connectedness(peerID) == network.Connected
}
