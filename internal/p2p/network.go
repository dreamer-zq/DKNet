package p2p

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/config"
	"github.com/dreamer-zq/DKNet/internal/security"
)

// Network handles P2P networking for TSS operations
type Network struct {
	host           host.Host
	messageHandler MessageHandler
	streamManager  *StreamManager
	logger         *zap.Logger
	cfg            *Config
	accessControl  security.AccessController
	// Unified message encryption
	messageEncryption security.MessageEncryption
}

// Config holds P2P network configuration
type Config struct {
	ListenAddrs    []string
	BootstrapPeers []string
	PrivateKeyFile string

	// Access control configuration
	AccessControl *config.AccessControlConfig
}

// NewNetwork creates a new P2P network instance
func NewNetwork(cfg *Config, logger *zap.Logger) (*Network, error) {
	privKey, err := loadPrivateKey(cfg.PrivateKeyFile, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load private key")
	}

	listenAddrs, err := convertAddrs(cfg.ListenAddrs)
	if err != nil {
		return nil, errors.Wrap(err, "invalid listen addresses")
	}

	var dhtInstance *dht.IpfsDHT
	h, err := libp2p.New(
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.Identity(privKey),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err error
			dhtInstance, err = dht.New(context.Background(), h, dht.Mode(dht.ModeAuto))
			return dhtInstance, err
		}),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
		libp2p.EnableNATService(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create libp2p host")
	}

	// Initialize unified message encryption
	encryptionConfig := &security.EncryptionConfig{
		PrivateKey: privKey,
		Peerstore:  h.Peerstore(),
	}

	n := &Network{
		host:              h,
		logger:            logger,
		cfg:               cfg,
		streamManager:     NewStreamManager(h, TssPartyProtocolID),
		accessControl:     security.NewController(cfg.AccessControl, logger.Named("access-control")),
		messageEncryption: security.NewMessageEncryption(encryptionConfig, logger),
	}
	h.SetStreamHandler(TssPartyProtocolID, n.handleStream)

	logger.Info("P2P node created", zap.String("peerID", h.ID().String()), zap.Any("addresses", h.Addrs()))

	go n.bootstrapConnect(dhtInstance)

	return n, nil
}

// Start is a placeholder for now.
func (n *Network) Start(ctx context.Context) error {
	n.logger.Info("P2P network started")
	return nil
}

// Stop stops the P2P network
func (n *Network) Stop() error {
	n.messageHandler.Stop()
	if err := n.host.Close(); err != nil {
		return errors.Wrap(err, "failed to close host")
	}
	n.logger.Info("P2P network stopped")
	return nil
}

// SetMessageHandler sets the message handler
func (n *Network) SetMessageHandler(handler MessageHandler) {
	n.messageHandler = handler
}

// SendMessage sends a message to the specified peers.
// It relies on the libp2p host's configured routing (DHT) to find and connect to peers.
func (n *Network) SendMessage(ctx context.Context, msg *Message) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	// Set the original sender's actual PeerID
	msg.SenderPeerID = n.GetHostID()
	sendFn := func(p peer.ID, msg *Message) {
		defer wg.Done()
		if err := n.streamManager.sendMessage(ctx, p, msg); err != nil {
			mu.Lock()
			defer mu.Unlock()

			errs = append(errs, err)
		}
	}

	for _, target := range msg.To {
		if target == n.GetHostID() {
			n.logger.Debug("Skipping sending message to self", zap.String("target", target))
			continue
		}

		targetMsg := msg.Clone()
		targetMsg.To = []string{target}
		// Encrypt the message before sending
		if err := n.encryptMessage(targetMsg); err != nil {
			return errors.Wrap(err, "failed to encrypt message")
		}

		targetPeer, err := peer.Decode(target)
		if err != nil {
			return errors.Wrapf(err, "invalid target peer ID %s", target)
		}

		wg.Add(1)
		go sendFn(targetPeer, targetMsg)
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Wrapf(errs[0], "encountered %d error(s) while sending message", len(errs))
	}
	return nil
}

// encryptMessage applies encryption to a message by calling the handler.
func (n *Network) encryptMessage(msg *Message) error {
	ctx := &security.MessageEncryptionContext{
		Data:         msg.Data,
		Encrypted:    msg.Encrypted,
		Recipient:    msg.To[0],
		SenderPeerID: msg.SenderPeerID,
		SetData:      func(data []byte) { msg.Data = data },
		SetEncrypted: func(encrypted bool) { msg.Encrypted = encrypted },
	}
	return n.messageEncryption.Encrypt(ctx)
}

// decryptMessage applies decryption to a message by calling the handler.
func (n *Network) decryptMessage(msg *Message) error {
	ctx := &security.MessageEncryptionContext{
		Data:         msg.Data,
		Encrypted:    msg.Encrypted,
		Recipient:    msg.To[0],
		SenderPeerID: msg.SenderPeerID,
		SetData:      func(data []byte) { msg.Data = data },
		SetEncrypted: func(encrypted bool) { msg.Encrypted = encrypted },
	}
	return n.messageEncryption.Decrypt(ctx)
}

// handleStream handles incoming streams.
func (n *Network) handleStream(stream network.Stream) {
	defer func() {
		if err := stream.Close(); err != nil {
			n.logger.Warn("Failed to close incoming stream", zap.Error(err))
		}
	}()

	remotePeerID := stream.Conn().RemotePeer()

	if !n.accessControl.IsAuthorized(remotePeerID.String()) {
		n.logger.Warn("Rejected stream from unauthorized peer", zap.String("peer", remotePeerID.String()))
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
			return
		}

		msgData := make([]byte, len(data))
		copy(msgData, data)
		reader.ReleaseMsg(data)

		go n.processIncomingMessage(msgData, remotePeerID)
	}
}

// processIncomingMessage handles the logic for a single received message.
func (n *Network) processIncomingMessage(data []byte, remotePeerID peer.ID) {
	var msg Message
	if err := msg.Decompresses(data); err != nil {
		n.logger.Error("Failed to decompress message", zap.Error(err), zap.String("peer", remotePeerID.String()))
		return
	}

	if err := n.decryptMessage(&msg); err != nil {
		n.logger.Error("Failed to decrypt stream message", zap.String("peer_id", remotePeerID.String()), zap.Error(err))
		return
	}

	if err := n.messageHandler.HandleMessage(context.Background(), &msg); err != nil {
		n.logger.Error("Failed to handle message", zap.Error(err))
	}
}

// loadPrivateKey loads a private key from a file.
func loadPrivateKey(keyFile string, logger *zap.Logger) (crypto.PrivKey, error) {
	logger.Info("Attempting to load private key from", zap.String("key_file", keyFile))
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return nil, err
	}
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	privKey, err := crypto.UnmarshalPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}
	logger.Info("Successfully loaded private key from file")
	return privKey, nil
}

// GetHostID returns the peer ID of the host.
func (n *Network) GetHostID() string {
	return n.host.ID().String()
}

// bootstrapConnect connects to bootstrap peers to join the network.
func (n *Network) bootstrapConnect(dhtInstance *dht.IpfsDHT) {
	ctx := context.Background()
	if err := dhtInstance.Bootstrap(ctx); err != nil {
		n.logger.Error("Failed to bootstrap DHT", zap.Error(err))
		return
	}

	var wg sync.WaitGroup
	for _, addr := range n.cfg.BootstrapPeers {
		if addr == "" {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(multiaddr.StringCast(addr))
		if err != nil {
			n.logger.Warn("Invalid bootstrap peer address", zap.String("addr", addr), zap.Error(err))
			continue
		}

		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			if err := n.host.Connect(ctx, pi); err != nil {
				n.logger.Warn("Failed to connect to bootstrap peer", zap.String("peer", pi.ID.String()), zap.Error(err))
				return
			}
			n.logger.Info("Successfully connected to bootstrap peer", zap.String("peer", pi.ID.String()))
		}(*pi)
	}
	wg.Wait()
}

func convertAddrs(addrs []string) ([]multiaddr.Multiaddr, error) {
	var multiaddrs []multiaddr.Multiaddr
	for _, addr := range addrs {
		maddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
		multiaddrs = append(multiaddrs, maddr)
	}
	return multiaddrs, nil
}
