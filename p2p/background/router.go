// TECHDEBT(olshansky): Delete this once we are fully comfortable with RainTree moving forward.

package background

import (
	"context"
	"fmt"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"io"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pHost "github.com/libp2p/go-libp2p/core/host"
	"google.golang.org/protobuf/proto"

	"github.com/pokt-network/pocket/logger"
	"github.com/pokt-network/pocket/p2p/config"
	"github.com/pokt-network/pocket/p2p/protocol"
	"github.com/pokt-network/pocket/p2p/providers"
	typesP2P "github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/p2p/utils"
	cryptoPocket "github.com/pokt-network/pocket/shared/crypto"
	"github.com/pokt-network/pocket/shared/modules"
	"github.com/pokt-network/pocket/shared/modules/base_modules"
)

var (
	_ typesP2P.Router            = &backgroundRouter{}
	_ modules.IntegratableModule = &backgroundRouter{}
	_ backgroundRouterFactory    = &backgroundRouter{}
)

type backgroundRouterFactory = modules.FactoryWithConfig[typesP2P.Router, *config.BackgroundConfig]

// backgroundRouter implements `typesP2P.Router` for use with all P2P participants.
type backgroundRouter struct {
	base_modules.IntegratableModule

	logger *modules.Logger
	// handler is the function to call when a message is received.
	handler typesP2P.MessageHandler
	// host represents a libp2p network node, it encapsulates a libp2p peerstore
	// & connection manager. `libp2p.New` configures and starts listening
	// according to options.
	// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p#section-readme)
	host libp2pHost.Host

	// Fields below are assigned during creation via `#setupDependencies()`.

	// gossipSub is used for broadcast communication
	// (i.e. multiple, unidentified receivers)
	// TECHDEBT: investigate diff between randomSub and gossipSub
	gossipSub *pubsub.PubSub
	// topic is similar to pubsub but received messages are filtered by a "topic" string.
	// Published messages are also given the respective topic before broadcast.
	topic *pubsub.Topic
	// subscription provides an interface to continuously read messages from.
	subscription *pubsub.Subscription
	// kadDHT is a kademlia distributed hash table used for routing and peer discovery.
	kadDHT *dht.IpfsDHT
	// TECHDEBT: `pstore` will likely be removed in future refactoring / simplification
	// of the `Router` interface.
	// pstore is the background router's peerstore. Assigned in `backgroundRouter#setupPeerstore()`.
	pstore typesP2P.Peerstore
}

// Create returns a `backgroundRouter` as a `typesP2P.Router`
// interface using the given configuration.
func Create(bus modules.Bus, cfg *config.BackgroundConfig) (typesP2P.Router, error) {
	return new(backgroundRouter).Create(bus, cfg)
}

func (*backgroundRouter) Create(bus modules.Bus, cfg *config.BackgroundConfig) (typesP2P.Router, error) {
	// TECHDEBT(#595): add ctx to interface methods and propagate down.
	ctx := context.TODO()

	networkLogger := logger.Global.CreateLoggerForModule("backgroundRouter")
	networkLogger.Info().Msg("Initializing background router")

	if err := cfg.IsValid(); err != nil {
		return nil, err
	}

	rtr := &backgroundRouter{
		logger:  networkLogger,
		handler: cfg.Handler,
		host:    cfg.Host,
	}
	rtr.SetBus(bus)

	if err := rtr.setupDependencies(ctx, cfg); err != nil {
		return nil, err
	}

	rtr.host.SetStreamHandler(protocol.RaintreeProtocolID, rtr.handleStream)

	go rtr.readSubscription(ctx)

	return rtr, nil
}

// Broadcast implements the respective `typesP2P.Router` interface  method.
func (rtr *backgroundRouter) Broadcast(data []byte) error {
	// CONSIDERATION: validate as PocketEnvelopeBz here (?)
	// TODO_THIS_COMMIT: wrap in BackgroundMessage
	backgroundMsg := &typesP2P.BackgroundMessage{
		Data: data,
	}
	backgroundMsgBz, err := proto.Marshal(backgroundMsg)
	if err != nil {
		return err
	}

	// TECHDEBT(#595): add ctx to interface methods and propagate down.
	return rtr.topic.Publish(context.TODO(), backgroundMsgBz)
}

// Send implements the respective `typesP2P.Router` interface  method.
func (rtr *backgroundRouter) Send(data []byte, address cryptoPocket.Address) error {
	rtr.logger.Warn().Str("address", address.String()).Msg("sending background message to peer")

	peer := rtr.pstore.GetPeer(address)
	if peer == nil {
		return fmt.Errorf("peer with address %s not in peerstore", address)
	}

	if err := utils.Libp2pSendToPeer(
		rtr.host,
		protocol.BackgroundProtocolID,
		data,
		peer,
	); err != nil {
		return err
	}
	return nil
}

// GetPeerstore implements the respective `typesP2P.Router` interface  method.
func (rtr *backgroundRouter) GetPeerstore() typesP2P.Peerstore {
	return rtr.pstore
}

// AddPeer implements the respective `typesP2P.Router` interface  method.
func (rtr *backgroundRouter) AddPeer(peer typesP2P.Peer) error {
	// Noop if peer with the pokt address already exists in the peerstore.
	// TECHDEBT: add method(s) to update peers.
	if p := rtr.pstore.GetPeer(peer.GetAddress()); p != nil {
		return nil
	}

	if err := utils.AddPeerToLibp2pHost(rtr.host, peer); err != nil {
		return err
	}

	return rtr.pstore.AddPeer(peer)
}

// RemovePeer implements the respective `typesP2P.Router` interface  method.
func (rtr *backgroundRouter) RemovePeer(peer typesP2P.Peer) error {
	if err := utils.RemovePeerFromLibp2pHost(rtr.host, peer); err != nil {
		return err
	}

	return rtr.pstore.RemovePeer(peer.GetAddress())
}

func (rtr *backgroundRouter) Close() error {
	// TODO_THIS_COMMIT: why is this causing problems?
	//rtr.subscription.Cancel()

	//return multierror.Append(
	//	rtr.topic.Close(),
	//	rtr.kadDHT.Close(),
	//)
	return nil
}

func (rtr *backgroundRouter) setupDependencies(ctx context.Context, cfg *config.BackgroundConfig) error {
	if err := rtr.setupPeerDiscovery(ctx); err != nil {
		return fmt.Errorf("setting up peer discovery: %w", err)
	}

	if err := rtr.setupPubsub(ctx); err != nil {
		return fmt.Errorf("setting up pubsub: %w", err)
	}

	if err := rtr.setupTopic(); err != nil {
		return fmt.Errorf("setting up topic: %w", err)
	}

	if err := rtr.setupSubscription(); err != nil {
		return fmt.Errorf("setting up subscription: %w", err)
	}

	if err := rtr.setupPeerstore(
		cfg.PeerstoreProvider,
		cfg.CurrentHeightProvider,
	); err != nil {
		return fmt.Errorf("setting up peerstore: %w", err)
	}
	return nil
}

func (rtr *backgroundRouter) setupPeerstore(
	pstoreProvider providers.PeerstoreProvider,
	currentHeightProvider providers.CurrentHeightProvider,
) (err error) {
	// seed initial peerstore with current on-chain peer info (i.e. staked actors)
	rtr.pstore, err = pstoreProvider.GetStakedPeerstoreAtHeight(
		currentHeightProvider.CurrentHeight(),
	)
	if err != nil {
		return err
	}

	// CONSIDERATION: add `GetPeers` method to `PeerstoreProvider` interface
	// to avoid this loop.
	for _, peer := range rtr.pstore.GetPeerList() {
		if err := utils.AddPeerToLibp2pHost(rtr.host, peer); err != nil {
			return err
		}

		// TODO: refactor: #bootstrap()
		libp2pPeer, err := utils.Libp2pAddrInfoFromPeer(peer)
		if err != nil {
			return fmt.Errorf(
				"converting peer info, pokt address: %s: %w",
				peer.GetAddress(),
				err,
			)
		}

		// don't attempt to connect to self
		if rtr.host.ID() == libp2pPeer.ID {
			return nil
		}

		// TECHDEBT(#595): add ctx to interface methods and propagate down.
		if err := rtr.host.Connect(context.TODO(), libp2pPeer); err != nil {
			return fmt.Errorf("connecting to peer: %w", err)
		}
	}
	return nil
}

func (rtr *backgroundRouter) setupPeerDiscovery(ctx context.Context) (err error) {
	dhtMode := dht.ModeAutoServer
	// NB: don't act as a bootstrap node in peer discovery in client debug mode
	if isClientDebugMode(rtr.GetBus()) {
		dhtMode = dht.ModeClient
	}

	// TECHDEBT(#595): add ctx to interface methods and propagate down.
	rtr.kadDHT, err = dht.New(ctx, rtr.host, dht.Mode(dhtMode))
	return err
}

func (rtr *backgroundRouter) setupPubsub(ctx context.Context) (err error) {
	// TODO_THIS_COMMIT: remove or refactor
	//
	// TECHDEBT: integrate with go-libp2p-pubsub tracing
	//truncID := host.ID().String()[:20]
	//jsonTracer, err := pubsub.NewJSONTracer(fmt.Sprintf("./pubsub-trace_%s.json", truncID))
	//if err != nil {
	//	return fmt.Errorf("creating json tracer: %w", err)
	//}
	//
	//tracerOpt := pubsub.WithEventTracer(jsonTracer)

	// CONSIDERATION: If switching to `NewRandomSub`, there will be a max size
	//rtr.gossipSub, err = pubsub.NewGossipSub(ctx, host, tracerOpt)
	rtr.gossipSub, err = pubsub.NewGossipSub(ctx, rtr.host)
	return err
}

func (rtr *backgroundRouter) setupTopic() (err error) {
	rtr.topic, err = rtr.gossipSub.Join(protocol.BackgroundTopicStr)
	return err
}

func (rtr *backgroundRouter) setupSubscription() (err error) {
	// INVESTIGATE: `WithBufferSize` `SubOpt`:
	// > WithBufferSize is a Subscribe option to customize the size of the subscribe
	// > output buffer. The default length is 32 but it can be configured to avoid
	// > dropping messages if the consumer is not reading fast enough.
	// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p-pubsub#WithBufferSize)
	rtr.subscription, err = rtr.topic.Subscribe()
	return err
}

// TODO_THIS_COMMIT: consider moving this to `baseRouter` to de-dup
//
// handleStream ensures the peerstore contains the remote peer and then reads
// the incoming stream in a new go routine.
func (rtr *backgroundRouter) handleStream(stream libp2pNetwork.Stream) {
	rtr.logger.Debug().Msg("handling incoming stream")
	peer, err := utils.PeerFromLibp2pStream(stream)
	if err != nil {
		rtr.logger.Error().Err(err).
			Str("address", peer.GetAddress().String()).
			Msg("parsing remote peer identity")

		if err = stream.Reset(); err != nil {
			rtr.logger.Error().Err(err).Msg("resetting stream")
		}
		return
	}

	if err := rtr.AddPeer(peer); err != nil {
		rtr.logger.Error().Err(err).
			Str("address", peer.GetAddress().String()).
			Msg("adding remote peer to router")
	}

	go rtr.readStream(stream)
}

// readStream reads the incoming stream, extracts the serialized `PocketEnvelope`
// data from the incoming `RainTreeMessage`, and passes it to the application by
// calling the configured `rtr.handler`. Intended to be called in a go routine.
func (rtr *backgroundRouter) readStream(stream libp2pNetwork.Stream) {
	// TODO_THIS_COMMIT: cleanup --v

	// Time out if no data is sent to free resources.
	// NB: tests using libp2p's `mocknet` rely on this not returning an error.
	//if err := stream.SetReadDeadline(newReadStreamDeadline()); err != nil {
	//	// `SetReadDeadline` not supported by `mocknet` streams.
	//	rtr.logger.Error().Err(err).Msg("setting stream read deadline")
	//}

	// log incoming stream
	//rtr.logStream(stream)

	// TODO_THIS_COMMIT: cleanup --^

	// read stream
	backgroundMsgBz, err := io.ReadAll(stream)
	if err != nil {
		rtr.logger.Error().Err(err).Msg("reading from stream")
		if err := stream.Reset(); err != nil {
			rtr.logger.Error().Err(err).Msg("resetting stream (read-side)")
		}
		return
	}

	// done reading; reset to signal this to remote peer
	// NB: failing to reset the stream can easily max out the number of available
	// network connections on the receiver's side.
	if err := stream.Reset(); err != nil {
		rtr.logger.Error().Err(err).Msg("resetting stream (read-side)")
	}

	// extract `PocketEnvelope` from `RainTreeMessage` (& continue propagation)
	if err := rtr.handleBackgroundMsg(backgroundMsgBz); err != nil {
		rtr.logger.Error().Err(err).Msg("handling raintree message")
		return
	}
}

func (rtr *backgroundRouter) readSubscription(ctx context.Context) {
	// TODO_THIS_COMMIT: look into "topic validaton"
	// (see: https://github.com/libp2p/specs/tree/master/pubsub#topic-validation)
	for {
		msg, err := rtr.subscription.Next(ctx)
		if ctx.Err() != nil {
			fmt.Printf("error: %s\n", ctx.Err())
			return
		}

		if err != nil {
			rtr.logger.Error().Err(err).
				Msg("error reading from background topic subscription")
			continue
		}

		// TECHDEBT/DISCUSS: telemetry
		if err := rtr.handleBackgroundMsg(msg.Data); err != nil {
			rtr.logger.Error().Err(err).Msg("error handling background message")
			continue
		}
	}
}

func (rtr *backgroundRouter) handleBackgroundMsg(backgroundMsgBz []byte) error {
	var backgroundMsg typesP2P.BackgroundMessage
	if err := proto.Unmarshal(backgroundMsgBz, &backgroundMsg); err != nil {
		return err
	}

	// There was no error, but we don't need to forward this to the app-specific bus.
	// For example, the message has already been handled by the application.
	if backgroundMsg.Data == nil {
		return nil
	}

	return rtr.handler(backgroundMsg.Data)
}

// isClientDebugMode returns the value of `ClientDebugMode` in the base config
func isClientDebugMode(bus modules.Bus) bool {
	return bus.GetRuntimeMgr().GetConfig().ClientDebugMode
}
