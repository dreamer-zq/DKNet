package p2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

// mdnsNet is a wrapper around the MDNS service
type mdnsNet struct {
	h        host.Host
	peerChan chan peer.AddrInfo
	logger   *zap.Logger
	service  mdns.Service
	ticker   *time.Ticker
	ctx      context.Context
	cancel   context.CancelFunc
}

// HandlePeerFound is called when a new peer is found
func (n *mdnsNet) HandlePeerFound(pi peer.AddrInfo) {
	select {
	case n.peerChan <- pi:
	case <-n.ctx.Done():
		return
	default:
		// Non-blocking send, drop if channel is full
		n.logger.Debug("Peer channel full, dropping discovered peer", zap.String("peer", pi.ID.String()))
	}
}

// NewMDNS initializes the MDNS service and returns a MdnsNet
func NewMDNS(peerhost host.Host, logger *zap.Logger) PeerDiscovery {
	// register with service so that we get notified about peer discovery
	return &mdnsNet{
		h:        peerhost,
		peerChan: make(chan peer.AddrInfo, 10), // Buffered channel to prevent blocking
		logger:   logger,
	}
}

// Start starts the MDNS service
func (n *mdnsNet) Start() error {
	n.ctx, n.cancel = context.WithCancel(context.Background())

	// Create MDNS service
	n.service = mdns.NewMdnsService(n.h, DiscoveryRendezvous, n)
	if err := n.service.Start(); err != nil {
		n.cancel()
		return err
	}

	// Start the peer connection handler
	go n.handlePeers()

	// Start periodic rediscovery to ensure early nodes can find later joining nodes
	n.startPeriodicRediscovery()

	n.logger.Info("MDNS service started with periodic rediscovery")
	return nil
}

// startPeriodicRediscovery starts a periodic rediscovery mechanism
func (n *mdnsNet) startPeriodicRediscovery() {
	// Set up periodic rediscovery every 30 seconds
	n.ticker = time.NewTicker(30 * time.Second)

	go func() {
		defer n.ticker.Stop()

		for {
			select {
			case <-n.ticker.C:
				n.logger.Debug("Performing periodic MDNS rediscovery")
				// Trigger a new round of discovery
				n.triggerRediscovery()

			case <-n.ctx.Done():
				return
			}
		}
	}()
}

// triggerRediscovery triggers a new round of MDNS discovery by actively browsing for services
func (n *mdnsNet) triggerRediscovery() {
	// Create a context with timeout for this discovery round
	ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
	defer cancel()

	// Create a resolver for active discovery
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		n.logger.Debug("Failed to create zeroconf resolver", zap.Error(err))
		return
	}

	// Create a channel to receive discovered services
	entries := make(chan *zeroconf.ServiceEntry)

	// Start browsing for services in a separate goroutine
	go func() {
		err := resolver.Browse(ctx, DiscoveryRendezvous, "local.", entries)
		if err != nil {
			n.logger.Debug("Failed to browse for MDNS services", zap.Error(err))
			return
		}
	}()

	// Process discovered services
	discoveredCount := 0
	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				// Channel closed by zeroconf library, no more entries
				n.logger.Debug("Finished active MDNS discovery round", zap.Int("discovered", discoveredCount))
				return
			}

			if entry == nil {
				continue
			}

			discoveredCount++
			n.logger.Debug("Actively discovered MDNS service",
				zap.String("instance", entry.Instance),
				zap.String("hostname", entry.HostName),
				zap.Int("port", entry.Port),
				zap.Strings("txt", entry.Text))

			// Parse the service entry to extract peer information
			n.processDiscoveredService(entry)

		case <-ctx.Done():
			n.logger.Debug("Active MDNS discovery round timed out", zap.Int("discovered", discoveredCount))
			return
		}
	}
}

// processDiscoveredService processes a discovered zeroconf service entry
func (n *mdnsNet) processDiscoveredService(entry *zeroconf.ServiceEntry) {
	// Extract peer ID from TXT records
	// libp2p MDNS stores the peer ID in TXT records as "dnsaddr=<multiaddr>"
	var peerID peer.ID
	var found bool

	for _, txt := range entry.Text {
		// Look for dnsaddr TXT record that contains the peer ID
		addrStr, ok := strings.CutPrefix(txt, "dnsaddr=")
		if !ok {
			continue
		}
		
		// Parse the multiaddr to extract peer ID
		maddr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			n.logger.Debug("Failed to parse dnsaddr from TXT record",
				zap.String("txt", txt),
				zap.Error(err))
			continue
		}

		// Extract peer ID from the multiaddr
		extractedPeerID, err := peer.IDFromP2PAddr(maddr)
		if err != nil {
			n.logger.Debug("Failed to extract peer ID from multiaddr",
				zap.String("multiaddr", addrStr),
				zap.Error(err))
			continue
		}

		peerID = extractedPeerID
		found = true
		break
	}

	if !found {
		n.logger.Debug("No peer ID found in TXT records",
			zap.Strings("txt", entry.Text))
		return
	}

	// Skip if it's ourselves
	if peerID == n.h.ID() {
		n.logger.Debug("Skipping self peer discovered via active MDNS",
			zap.String("peer", peerID.String()))
		return
	}

	// Build multiaddresses from the service entry
	var addrs []multiaddr.Multiaddr

	// Add IPv4 addresses
	for _, ip := range entry.AddrIPv4 {
		addrStr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ip.String(), entry.Port, peerID.String())
		maddr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			n.logger.Debug("Failed to parse IPv4 multiaddr",
				zap.String("addr", addrStr),
				zap.Error(err))
			continue
		}
		addrs = append(addrs, maddr)
	}

	// Add IPv6 addresses
	for _, ip := range entry.AddrIPv6 {
		addrStr := fmt.Sprintf("/ip6/%s/tcp/%d/p2p/%s", ip.String(), entry.Port, peerID.String())
		maddr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			n.logger.Debug("Failed to parse IPv6 multiaddr",
				zap.String("addr", addrStr),
				zap.Error(err))
			continue
		}
		addrs = append(addrs, maddr)
	}

	if len(addrs) == 0 {
		n.logger.Debug("Service entry has no valid addresses")
		return
	}

	// Create peer.AddrInfo from the discovered service
	peerInfo := peer.AddrInfo{
		ID:    peerID,
		Addrs: addrs,
	}

	// Send the discovered peer to our handler
	n.logger.Debug("Converted zeroconf service to peer info",
		zap.String("peer", peerID.String()),
		zap.Int("addrs", len(peerInfo.Addrs)))

	select {
	case n.peerChan <- peerInfo:
	case <-n.ctx.Done():
		return
	default:
		n.logger.Debug("Peer channel full, dropping actively discovered peer",
			zap.String("peer", peerID.String()))
	}
}

// handlePeers handles discovered peers in a separate goroutine
func (n *mdnsNet) handlePeers() {
	for {
		select {
		case p := <-n.peerChan:
			n.logger.Info("Found new peer via MDNS", zap.String("peer", p.ID.String()))

			// Skip ourselves
			if p.ID == n.h.ID() {
				n.logger.Debug("Skipping self peer", zap.String("peer", p.ID.String()))
				continue
			}

			// Check if already connected
			if n.h.Network().Connectedness(p.ID) == network.Connected {
				n.logger.Debug("Already connected to peer", zap.String("peer", p.ID.String()))
				continue
			}

			// Attempt to connect
			n.logger.Info("Attempting to connect to discovered peer", zap.String("peer", p.ID.String()))
			ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second)
			if err := n.h.Connect(ctx, p); err != nil {
				n.logger.Warn("Failed to connect to discovered peer",
					zap.String("peer", p.ID.String()),
					zap.Error(err))
			} else {
				n.logger.Info("Successfully connected to discovered peer",
					zap.String("peer", p.ID.String()))
			}
			cancel()

		case <-n.ctx.Done():
			return
		}
	}
}

// Stop stops the MDNS service
func (n *mdnsNet) Stop() {
	if n.cancel != nil {
		n.cancel()
	}

	if n.ticker != nil {
		n.ticker.Stop()
	}

	if n.service != nil {
		if err := n.service.Close(); err != nil {
			n.logger.Warn("Error closing MDNS service", zap.Error(err))
		}
	}

	close(n.peerChan)
	n.logger.Info("MDNS service stopped")
}
