package p2p

import (
	"context"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"go.uber.org/zap"
)

// mdnsNet is a wrapper around the MDNS service
type mdnsNet struct {
	h          host.Host
	peerChan   chan peer.AddrInfo
	logger     *zap.Logger
}

// HandlePeerFound is called when a new peer is found
func (n *mdnsNet) HandlePeerFound(pi peer.AddrInfo) {
	n.peerChan <- pi
}

// NewMDNS initializes the MDNS service and returns a MdnsNet
func NewMDNS(peerhost host.Host,logger *zap.Logger) PeerDiscovery {
	// register with service so that we get notified about peer discovery
	return &mdnsNet{
		h:          peerhost,
		peerChan:   make(chan peer.AddrInfo),
		logger:     logger,
	}
}

// Start starts the MDNS service
func (n *mdnsNet) Start() error {
	ser := mdns.NewMdnsService(n.h, DiscoveryRendezvous, n)
	if err := ser.Start(); err != nil {
		return err
	}

	go func() {
		for {
			peer := <-n.peerChan
			n.logger.Info("Found new peer", zap.String("peer", peer.ID.String()))
			if peer.ID == n.h.ID() {
				continue // don't connect to ourselves.
			}

			ctx := context.Background()
			// Connect to the peer if not already connected.
			if n.h.Network().Connectedness(peer.ID) != network.Connected {
				n.logger.Info("Found new peer, attempting to connect", zap.String("peer", peer.ID.String()))
				if err := n.h.Connect(ctx, peer); err != nil {
					n.logger.Warn("Failed to connect to discovered peer", zap.String("peer", peer.ID.String()), zap.Error(err))
				} else {
					n.logger.Info("Successfully connected to discovered peer", zap.String("peer", peer.ID.String()))
				}
			}
		}
	}()
	return nil
}

// Stop stops the MDNS service
func (n *mdnsNet) Stop() {
	close(n.peerChan)
}