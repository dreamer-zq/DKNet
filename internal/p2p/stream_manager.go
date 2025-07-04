package p2p

import (
	"context"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-msgio"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/common"
)

// StreamManager manages reusable streams to peers.
type StreamManager struct {
	host     host.Host
	protocol protocol.ID
	streams  *common.SafeMap[peer.ID, network.Stream]
	logger   *zap.Logger
}

// NewStreamManager creates a new StreamManager.
func NewStreamManager(h host.Host, p protocol.ID) *StreamManager {
	return &StreamManager{
		host:     h,
		protocol: p,
		streams:  common.New[peer.ID, network.Stream](),
		logger:   zap.L().Named("stream-manager"),
	}
}

// getStream gets a cached stream or creates a new one.
func (sm *StreamManager) getStream(ctx context.Context, peerID peer.ID) (network.Stream, error) {
	stream, ok := sm.streams.Get(peerID)
	if ok && !stream.Conn().IsClosed() {
		sm.logger.Debug("Reusing cached stream", zap.String("peer", peerID.String()))
		return stream, nil
	}
	return sm.createStream(ctx, peerID)
}

func (sm *StreamManager) createStream(ctx context.Context, peerID peer.ID) (network.Stream, error) {
	sm.logger.Debug("Creating new stream", zap.String("peer", peerID.String()))
	newStream, err := sm.host.NewStream(ctx, peerID, sm.protocol)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open new stream to %s", peerID)
	}

	sm.streams.Set(peerID, newStream)
	return newStream, nil
}

// sendMessage sends a message to a peer, managing the stream lifecycle.
func (sm *StreamManager) sendMessage(ctx context.Context, peerID peer.ID, msg *Message) error {
	stream, err := sm.getStream(ctx, peerID)
	if err != nil {
		return err
	}

	msgBytes, err := msg.Compresses()
	if err != nil {
		return errors.Wrap(err, "failed to compress message")
	}

	writer := msgio.NewWriter(stream)
	if err := writer.WriteMsg(msgBytes); err != nil {
		_ = stream.Reset()
		sm.streams.Delete(peerID)
		return errors.Wrapf(err, "failed to write message to peer %s", peerID)
	}

	return nil
}
