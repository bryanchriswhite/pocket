package utils

import (
	"context"
	"fmt"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/pokt-network/pocket/logger"
	"time"

	libp2pHost "github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/multierr"

	"github.com/pokt-network/pocket/p2p/protocol"
	typesP2P "github.com/pokt-network/pocket/p2p/types"
)

const (
	week = time.Hour * 24 * 7
	// TECHDEBT(#629): consider more carefully and parameterize.
	defaultPeerTTL = 2 * week
	// TECHDEBT(#629): configure timeouts. Consider security exposure vs. real-world conditions.
	// TECHDEBT(#629): parameterize and expose via config.
	// writeStreamTimeout is the duration to wait for a read operation on a
	// stream to complete, after which the stream is closed ("timed out").
	writeStreamTimeout = time.Second * 1
)

// PopulateLibp2pHost iterates through peers in given `pstore`, converting peer
// info for use with libp2p and adding it to the underlying libp2p host's peerstore.
// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p@v0.26.2/core/host#Host)
// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p@v0.26.2/core/peerstore#Peerstore)
func PopulateLibp2pHost(host libp2pHost.Host, pstore typesP2P.Peerstore) (err error) {
	for _, peer := range pstore.GetPeerList() {
		if addErr := AddPeerToLibp2pHost(host, peer); addErr != nil {
			err = multierr.Append(err, addErr)
		}
	}
	return err
}

// AddPeerToLibp2pHost covnerts the given pocket peer for use with libp2p and adds
// it to the given libp2p host's underlying peerstore.
func AddPeerToLibp2pHost(host libp2pHost.Host, peer typesP2P.Peer) error {
	pubKey, err := Libp2pPublicKeyFromPeer(peer)
	if err != nil {
		return fmt.Errorf(
			"converting peer public key, pokt address: %s: %w",
			peer.GetAddress(),
			err,
		)
	}

	libp2pPeer, err := Libp2pAddrInfoFromPeer(peer)
	if err != nil {
		return fmt.Errorf(
			"converting peer info, pokt address: %s: %w",
			peer.GetAddress(),
			err,
		)
	}

	host.Peerstore().AddAddrs(libp2pPeer.ID, libp2pPeer.Addrs, defaultPeerTTL)
	if err := host.Peerstore().AddPubKey(libp2pPeer.ID, pubKey); err != nil {
		return fmt.Errorf(
			"adding peer public key, pokt address: %s: %w",
			peer.GetAddress(),
			err,
		)
	}
	return nil
}

// RemovePeerFromLibp2pHost removes the given peer's libp2p public keys and
// protocols from the libp2p host's underlying peerstore.
func RemovePeerFromLibp2pHost(host libp2pHost.Host, peer typesP2P.Peer) error {
	peerInfo, err := Libp2pAddrInfoFromPeer(peer)
	if err != nil {
		return err
	}

	host.Peerstore().RemovePeer(peerInfo.ID)
	return host.Peerstore().RemoveProtocols(peerInfo.ID)
}

// Libp2pSendToPeer sends data to the given pocket peer from the given libp2p host.
func Libp2pSendToPeer(host libp2pHost.Host, data []byte, peer typesP2P.Peer) error {
	// TECHDEBT(#595): add ctx to interface methods and propagate down.
	ctx := context.TODO()

	peerInfo, err := Libp2pAddrInfoFromPeer(peer)
	if err != nil {
		return err
	}

	// TODO: remove or cleanup
	err = host.Network().ResourceManager().ViewTransient(func(scope libp2pNetwork.ResourceScope) error {
		stat := scope.Stat()
		logger.Global.Debug().Fields(map[string]any{
			"InboundConns":    stat.NumConnsInbound,
			"OutboundConns":   stat.NumConnsOutbound,
			"InboundStreams":  stat.NumStreamsInbound,
			"OutboundStreams": stat.NumStreamsOutbound,
		}).Msg("host transient resource scope")
		return nil
	})
	if err != nil {
		logger.Global.Debug().Err(err).Msg("interrogating resource manager")
	}
	// ---

	stream, err := host.NewStream(ctx, peerInfo.ID, protocol.PoktProtocolID)
	if err != nil {
		return fmt.Errorf("opening stream: %w", err)
	}

	if _, err = stream.Write(data); err != nil {
		return multierr.Append(
			fmt.Errorf("writing to stream: %w", err),
			stream.Reset(),
		)
	}

	// MUST USE `streamClose` NOT `stream.CloswWrite`; otherwise, outbound streams
	// will accumulate until resource limits are hit (e.g.):
	// > "opening stream: stream-3478: transient: cannot reserve outbound stream: resource limit exceeded"
	return stream.Close()
}

// TODO: remove me - used for debugging only
func PrintPStore(pstore typesP2P.Peerstore) {
	mapPStore, ok := pstore.(typesP2P.PeerAddrMap)
	if !ok {
		panic(fmt.Sprintf("unexpected pstore type: %T", pstore))
	}

	count := 0
	for poktAddrStr, peer := range mapPStore {
		logger.Global.Debug().Fields(map[string]any{
			"poktAddr":   poktAddrStr,
			"serviceURL": peer.GetServiceURL(),
		}).Msgf("peer %d", count)
		count++
	}

}

// newWriteStreamDeadline returns a future deadline
// based on the read stream timeout duration.
func newWriteStreamDeadline() time.Time {
	return time.Now().Add(writeStreamTimeout)
}
