package p2p

import (
	"context"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	"go.uber.org/zap"
)

// dhtNet is a wrapper around the DHT service
type dhtNet struct {
	h              host.Host
	bootstrapPeers []string
	logger         *zap.Logger
	ticker         *time.Ticker
	dhtInstance    *dht.IpfsDHT
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewDHT initializes the DHT service and returns a DhtNet
func NewDHT(h host.Host, bootstrapPeers []string, logger *zap.Logger) PeerDiscovery {
	return &dhtNet{h: h, bootstrapPeers: bootstrapPeers, logger: logger}
}

// Start starts the DHT service
func (n *dhtNet) Start() error {
	// Create context for the DHT service
	n.ctx, n.cancel = context.WithCancel(context.Background())

	// Parse bootstrap peers
	bootstrapPeers := make([]peer.AddrInfo, 0, len(n.bootstrapPeers))
	for _, addr := range n.bootstrapPeers {
		peerinfo, err := peer.AddrInfoFromString(addr)
		if err != nil {
			n.logger.Warn("Failed to parse bootstrap peer", zap.String("addr", addr), zap.Error(err))
			continue
		}
		bootstrapPeers = append(bootstrapPeers, *peerinfo)
	}

	if len(bootstrapPeers) == 0 {
		bootstrapPeers = dht.GetDefaultBootstrapPeerAddrInfos()
	}

	// Create DHT instance
	var err error
	n.dhtInstance, err = dht.New(
		n.ctx, n.h,
		dht.BootstrapPeers(bootstrapPeers...),
		dht.Mode(dht.ModeAuto),
		dht.RoutingTableRefreshPeriod(30*time.Second),
	)
	if err != nil {
		n.cancel()
		return err
	}

	// Bootstrap the DHT
	if err := n.dhtInstance.Bootstrap(n.ctx); err != nil {
		n.cancel()
		return err
	}

	// Start peer discovery
	n.startPeerDiscovery()
	n.logger.Info("DHT service started successfully")
	return nil
}

func (n *dhtNet) startPeerDiscovery() {
	routingDiscovery := drouting.NewRoutingDiscovery(n.dhtInstance)

	// Create context with timeout for this discovery round
	ctx, cancel := context.WithTimeout(n.ctx, 20*time.Second)
	defer cancel()

	// Advertise our presence (util.Advertise already runs in goroutine internally)
	n.logger.Debug("Advertising ourselves under rendezvous point", zap.String("rendezvous", DiscoveryRendezvous))
	util.Advertise(ctx, routingDiscovery, DiscoveryRendezvous)

	// Then start periodic discovery
	n.ticker = time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-n.ticker.C:
				n.discoverPeers(routingDiscovery)
			case <-n.ctx.Done():
				return
			}
		}
	}()
}

func (n *dhtNet) discoverPeers(routingDiscovery *drouting.RoutingDiscovery) {
	// Create context with timeout for this discovery round
	ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
	defer cancel()

	peerChan, err := routingDiscovery.FindPeers(ctx, DiscoveryRendezvous)
	if err != nil {
		n.logger.Error("Failed to find peers", zap.Error(err))
		return
	}

	// Process discovered peers
	for {
		select {
		case p, ok := <-peerChan:
			if !ok {
				n.logger.Debug("No more peers discovered in this round")
				return
			}

			n.logger.Debug("Discovered peer", zap.String("peer", p.ID.String()))

			// Skip if it's ourselves
			if p.ID == n.h.ID() {
				n.logger.Debug("Skipping self peer", zap.String("peer", p.ID.String()))
				continue
			}

			// Skip if no addresses
			if len(p.Addrs) == 0 {
				n.logger.Debug("Peer has no addresses", zap.String("peer", p.ID.String()))
				continue
			}

			// Check if already connected
			if n.h.Network().Connectedness(p.ID) == network.Connected {
				n.logger.Debug("Already connected to peer", zap.String("peer", p.ID.String()))
				continue
			}

			// Attempt to connect
			n.connectToPeer(p)

		case <-ctx.Done():
			n.logger.Debug("Discovery round timed out")
			return
		}
	}
}

func (n *dhtNet) connectToPeer(peerInfo peer.AddrInfo) {
	// Use a separate context with timeout for connection
	connectCtx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
	defer cancel()

	n.logger.Info("Attempting to connect to peer",
		zap.String("peer", peerInfo.ID.String()),
		zap.Strings("addrs", func() []string {
			addrs := make([]string, len(peerInfo.Addrs))
			for i, addr := range peerInfo.Addrs {
				addrs[i] = addr.String()
			}
			return addrs
		}()))

	if err := n.h.Connect(connectCtx, peerInfo); err != nil {
		n.logger.Warn("Failed to connect to discovered peer",
			zap.String("peer", peerInfo.ID.String()),
			zap.Error(err))
	}
	n.logger.Info("Successfully connected to discovered peer",
		zap.String("peer", peerInfo.ID.String()))
}

func (n *dhtNet) Stop() {
	if n.ticker != nil {
		n.ticker.Stop()
	}

	if n.cancel != nil {
		n.cancel()
	}

	if n.dhtInstance != nil {
		if err := n.dhtInstance.Close(); err != nil {
			n.logger.Warn("Error closing DHT instance", zap.Error(err))
		}
	}

	n.logger.Info("DHT service stopped")
}
