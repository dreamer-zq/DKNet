package p2p

import (
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	multiaddr "github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

var _ connmgr.ConnectionGater = (*connectionGater)(nil)

type connectionGater struct {
	logger       *zap.Logger
	allowedPeers map[string]bool
	enabled      bool
}

// InterceptAccept implements connmgr.ConnectionGater.
// This is called when a transport listener receives an inbound connection request.
func (c *connectionGater) InterceptAccept(network.ConnMultiaddrs) (allow bool) {
	// We allow the connection to proceed to the next stage where we can check the peer ID.
	// The actual peer-based filtering will happen in InterceptSecured.
	return true
}

// InterceptAddrDial implements connmgr.ConnectionGater.
// This is called when dialing a specific address for a peer.
func (c *connectionGater) InterceptAddrDial(peerID peer.ID, addr multiaddr.Multiaddr) (allow bool) {
	// Always allow outbound connections - we don't restrict who the current node can connect to
	// This is necessary for DHT discovery, bootstrap peers, etc.
	return true
}

// InterceptPeerDial implements connmgr.ConnectionGater.
// This is called when dialing a peer (before resolving addresses).
func (c *connectionGater) InterceptPeerDial(peerID peer.ID) (allow bool) {
	// Always allow outbound connections - we don't restrict who the current node can connect to
	// This is necessary for DHT discovery, bootstrap peers, etc.
	return true
}

// InterceptSecured implements connmgr.ConnectionGater.
// This is called after the security handshake, when we have authenticated the peer.
func (c *connectionGater) InterceptSecured(dir network.Direction, peerID peer.ID, connMultiaddrs network.ConnMultiaddrs) (allow bool) {
	// If access control is disabled, allow all connections
	if !c.enabled {
		return true
	}

	allowed := c.allowedPeers[peerID.String()]
	c.logger.Debug("Try to interceptSecured", zap.String("peer", peerID.String()), zap.Bool("allow", allowed))

	// Check if the peer is in the allowed list
	return allowed
}

// InterceptUpgraded implements connmgr.ConnectionGater.
// This is called when a connection has been fully upgraded (secure + multiplexed).
func (c *connectionGater) InterceptUpgraded(conn network.Conn) (allow bool, reason control.DisconnectReason) {
	c.logger.Debug("Try to interceptUpgraded", zap.String("peer", conn.RemotePeer().String()))
	// If access control is disabled, allow all connections
	if !c.enabled {
		return true, 0
	}

	// Check if the peer is in the allowed list
	peerID := conn.RemotePeer()
	allowed := c.allowedPeers[peerID.String()]
	c.logger.Debug("Try to interceptSecured", zap.String("peer", peerID.String()), zap.Bool("allow", allowed))

	if allowed {
		return true, 0
	}
	// If not allowed, disconnect with appropriate reason
	return false, control.DisconnectReason(0) // Use default disconnect reason
}

// NewConnectionGater creates a new connection gater
func NewConnectionGater(allowedPeers []string, enabled bool, logger *zap.Logger) connmgr.ConnectionGater {
	allowedPeersMap := make(map[string]bool)
	for _, peer := range allowedPeers {
		allowedPeersMap[peer] = true
	}
	return &connectionGater{
		allowedPeers: allowedPeersMap,
		enabled:      enabled,
		logger:       logger,
	}
}
