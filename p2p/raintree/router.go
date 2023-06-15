package raintree

import (
	"context"
	"fmt"
	"github.com/pokt-network/pocket/p2p/unicast"
	"net/http"
	"strings"

	libp2pHost "github.com/libp2p/go-libp2p/core/host"
	"google.golang.org/protobuf/proto"

	"github.com/pokt-network/pocket/logger"
	"github.com/pokt-network/pocket/p2p/config"
	"github.com/pokt-network/pocket/p2p/protocol"
	"github.com/pokt-network/pocket/p2p/providers"
	rpcCHP "github.com/pokt-network/pocket/p2p/providers/current_height_provider/rpc"
	"github.com/pokt-network/pocket/p2p/providers/peerstore_provider"
	rpcPSP "github.com/pokt-network/pocket/p2p/providers/peerstore_provider/rpc"
	typesP2P "github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/p2p/utils"
	"github.com/pokt-network/pocket/rpc"
	"github.com/pokt-network/pocket/shared/codec"
	cryptoPocket "github.com/pokt-network/pocket/shared/crypto"
	"github.com/pokt-network/pocket/shared/mempool"
	"github.com/pokt-network/pocket/shared/messaging"
	"github.com/pokt-network/pocket/shared/modules"
	"github.com/pokt-network/pocket/shared/modules/base_modules"
	sharedUtils "github.com/pokt-network/pocket/shared/utils"
	"github.com/pokt-network/pocket/telemetry"
)

var (
	_ typesP2P.Router            = &rainTreeRouter{}
	_ modules.IntegratableModule = &rainTreeRouter{}
	_ rainTreeFactory            = &rainTreeRouter{}
)

type rainTreeFactory = modules.FactoryWithConfig[typesP2P.Router, *config.RainTreeConfig]

type rainTreeRouter struct {
	base_modules.IntegratableModule
	unicast.UnicastRouter

	logger *modules.Logger
	// handler is the function to call when a message is received.
	handler typesP2P.MessageHandler
	// host represents a libp2p libp2pNetwork node, it encapsulates a libp2p peerstore
	// & connection manager. `libp2p.New` configures and starts listening
	// according to options.
	// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p#section-readme)
	host libp2pHost.Host
	// selfAddr is the pocket address representing this host.
	selfAddr              cryptoPocket.Address
	peersManager          *rainTreePeersManager
	pstoreProvider        peerstore_provider.PeerstoreProvider
	currentHeightProvider providers.CurrentHeightProvider
	nonceDeduper          *mempool.GenericFIFOSet[uint64, uint64]
	bootstrapNodes        []string
}

func NewRainTreeRouter(bus modules.Bus, cfg *config.RainTreeConfig) (typesP2P.Router, error) {
	return new(rainTreeRouter).Create(bus, cfg)
}

func (*rainTreeRouter) Create(bus modules.Bus, cfg *config.RainTreeConfig) (typesP2P.Router, error) {
	// TODO_THIS_COMMIT: rename logger to support multiple routers
	routerLogger := logger.Global.CreateLoggerForModule("router")
	routerLogger.Info().Msg("Initializing rainTreeRouter")

	if err := cfg.IsValid(); err != nil {
		return nil, err
	}

	rtr := &rainTreeRouter{
		host:                  cfg.Host,
		selfAddr:              cfg.Addr,
		pstoreProvider:        cfg.PeerstoreProvider,
		currentHeightProvider: cfg.CurrentHeightProvider,
		logger:                routerLogger,
		handler:               cfg.Handler,
		bootstrapNodes:        cfg.BootstrapNodes,
	}
	rtr.SetBus(bus)

	if err := rtr.setupDependencies(); err != nil {
		return nil, err
	}

	return typesP2P.Router(rtr), nil
}

// NetworkBroadcast implements the respective member of `typesP2P.Router`.
func (rtr *rainTreeRouter) Broadcast(data []byte) error {
	return rtr.broadcastAtLevel(data, rtr.peersManager.GetMaxNumLevels())
}

// broadcastAtLevel recursively sends to both left and right target peers
// from the starting level, demoting until level == 0.
// (see: https://github.com/pokt-network/pocket-network-protocol/tree/main/p2p)
func (rtr *rainTreeRouter) broadcastAtLevel(data []byte, level uint32) error {
	// This is handled either by the cleanup layer or redundancy layer
	if level == 0 {
		return nil
	}
	msg := &typesP2P.RainTreeMessage{
		Level: level,
		Data:  data,
	}
	msgBz, err := codec.GetCodec().Marshal(msg)
	if err != nil {
		return err
	}

	for _, target := range rtr.getTargetsAtLevel(level) {
		if shouldSendToTarget(target) {
			if err = rtr.sendInternal(msgBz, target.address); err != nil {
				rtr.logger.Error().Err(err).Msg("sending to peer during broadcast")
			}
		}
	}

	if err = rtr.demote(msg); err != nil {
		rtr.logger.Error().Err(err).Msg("demoting self during RainTree message propagation")
	}

	return nil
}

// demote broadcasts to the decremented level's targets.
// (see: https://github.com/pokt-network/pocket-network-protocol/tree/main/p2p)
func (rtr *rainTreeRouter) demote(rainTreeMsg *typesP2P.RainTreeMessage) error {
	if rainTreeMsg.Level > 0 {
		if err := rtr.broadcastAtLevel(rainTreeMsg.Data, rainTreeMsg.Level-1); err != nil {
			return err
		}
	}
	return nil
}

// NetworkSend implements the respective member of `typesP2P.Router`.
func (rtr *rainTreeRouter) Send(data []byte, address cryptoPocket.Address) error {
	msg := &typesP2P.RainTreeMessage{
		Level: 0, // Direct send that does not need to be propagated
		Data:  data,
	}

	bz, err := codec.GetCodec().Marshal(msg)
	if err != nil {
		return err
	}

	return rtr.sendInternal(bz, address)
}

// sendInternal sends `data` to the peer at pokt `address` if not self.
func (rtr *rainTreeRouter) sendInternal(data []byte, address cryptoPocket.Address) error {
	// TODO: How should we handle this?
	if rtr.selfAddr.Equals(address) {
		rtr.logger.Debug().Str("pokt_addr", address.String()).Msg("attempted to send to self")
		return nil
	}

	peer := rtr.peersManager.GetPeerstore().GetPeer(address)
	if peer == nil {
		return fmt.Errorf("no known peer with pokt address %s", address)
	}

	// debug logging
	hostname := rtr.getHostname()
	utils.LogOutgoingMsg(rtr.logger, hostname, peer)

	if err := utils.Libp2pSendToPeer(rtr.host, protocol.RaintreeProtocolID, data, peer); err != nil {
		return err
	}

	// A bus is not available In client debug mode
	bus := rtr.GetBus()
	if bus == nil {
		return nil
	}

	bus.
		GetTelemetryModule().
		GetEventMetricsAgent().
		EmitEvent(
			telemetry.P2P_EVENT_METRICS_NAMESPACE,
			telemetry.P2P_RAINTREE_MESSAGE_EVENT_METRIC_NAME,
			telemetry.P2P_RAINTREE_MESSAGE_EVENT_METRIC_SEND_LABEL, "send",
		)

	return nil
}

// handleRainTreeMsg deserializes a RainTree message to extract the `PocketEnvelope`
// bytes contained within, continues broadcast propagation, if applicable, and
// passes them off to the application by calling the configured `rtr.handler`.
// Intended to be called in a go routine.
func (rtr *rainTreeRouter) handleRainTreeMsg(rainTreeMsgBz []byte) error {
	blockHeightInt := rtr.GetBus().GetConsensusModule().CurrentHeight()
	blockHeight := fmt.Sprintf("%d", blockHeightInt)

	rtr.GetBus().
		GetTelemetryModule().
		GetEventMetricsAgent().
		EmitEvent(
			telemetry.P2P_EVENT_METRICS_NAMESPACE,
			telemetry.P2P_RAINTREE_MESSAGE_EVENT_METRIC_NAME,
			telemetry.P2P_RAINTREE_MESSAGE_EVENT_METRIC_HEIGHT_LABEL, blockHeight,
		)

	var rainTreeMsg typesP2P.RainTreeMessage
	if err := proto.Unmarshal(rainTreeMsgBz, &rainTreeMsg); err != nil {
		return err
	}

	// TECHDEBT(#763): refactor as "pre-propagation validation"
	networkMessage := messaging.PocketEnvelope{}
	if err := proto.Unmarshal(rainTreeMsg.Data, &networkMessage); err != nil {
		rtr.logger.Error().Err(err).Msg("Error decoding network message")
		return err
	}
	// --

	// Continue RainTree propagation
	if rainTreeMsg.Level > 0 {
		if err := rtr.broadcastAtLevel(rainTreeMsg.Data, rainTreeMsg.Level-1); err != nil {
			return err
		}
	}

	// There was no error, but we don't need to forward this to the app-specific bus.
	// For example, the message has already been handled by the application.
	if rainTreeMsg.Data == nil {
		return nil
	}

	// call configured handler to forward to app-specific bus
	if err := rtr.handler(rainTreeMsg.Data); err != nil {
		rtr.logger.Error().Err(err).Msg("handling raintree message")
	}
	return nil
}

// GetPeerstore implements the respective member of `typesP2P.Router`.
func (rtr *rainTreeRouter) GetPeerstore() typesP2P.Peerstore {
	return rtr.peersManager.GetPeerstore()
}

// AddPeer implements the respective member of `typesP2P.Router`.
func (rtr *rainTreeRouter) AddPeer(peer typesP2P.Peer) error {
	// Noop if peer with the same pokt address exists in the peerstore.
	// TECHDEBT: add method(s) to update peers.
	if p := rtr.peersManager.GetPeerstore().GetPeer(peer.GetAddress()); p != nil {
		return nil
	}

	if err := utils.AddPeerToLibp2pHost(rtr.host, peer); err != nil {
		return err
	}

	rtr.peersManager.HandleEvent(
		typesP2P.PeerManagerEvent{
			EventType: typesP2P.AddPeerEventType,
			Peer:      peer,
		},
	)
	return nil
}

func (rtr *rainTreeRouter) RemovePeer(peer typesP2P.Peer) error {
	rtr.peersManager.HandleEvent(
		typesP2P.PeerManagerEvent{
			EventType: typesP2P.RemovePeerEventType,
			Peer:      peer,
		},
	)
	return nil
}

// Size returns the number of peers the network is aware of and would attempt to
// broadcast to.
func (rtr *rainTreeRouter) Size() int {
	return rtr.peersManager.GetPeerstore().Size()
}

func (rtr *rainTreeRouter) Close() error {
	return nil
}

func (rtr *rainTreeRouter) Bootstrap(ctx context.Context, maxConcurrency uint32) error {
	limiter := sharedUtils.NewLimiter(int(maxConcurrency))

	for _, serviceURL := range rtr.bootstrapNodes {
		// concurrent bootstrapping
		// TECHDEBT(#595): add ctx to interface methods and propagate down.
		limiter.Go(ctx, func() {
			rtr.bootstrapFromRPC(strings.Clone(serviceURL))
		})
	}

	limiter.Close()
	return nil
}

// shouldSendToTarget returns false if target is self.
func shouldSendToTarget(target target) bool {
	return !target.isSelf
}

func (rtr *rainTreeRouter) setupUnicastRouter() error {
	unicastRouterCfg := config.UnicastRouterConfig{
		Logger:         rtr.logger,
		Host:           rtr.host,
		ProtocolID:     protocol.RaintreeProtocolID,
		MessageHandler: rtr.handleRainTreeMsg,
		PeerHandler:    rtr.AddPeer,
	}

	unicastRouter, err := unicast.Create(rtr.GetBus(), &unicastRouterCfg)
	if err != nil {
		return fmt.Errorf("setting up unicast router: %w", err)
	}

	rtr.UnicastRouter = *unicastRouter
	return nil
}

func (rtr *rainTreeRouter) setupDependencies() error {
	if err := rtr.setupUnicastRouter(); err != nil {
		return err
	}

	pstore, err := rtr.pstoreProvider.GetStakedPeerstoreAtHeight(rtr.currentHeightProvider.CurrentHeight())
	if err != nil {
		return fmt.Errorf("getting staked peerstore: %w", err)
	}

	if err := rtr.setupPeerManager(pstore); err != nil {
		return fmt.Errorf("setting up peer manager: %w", err)
	}

	if err := utils.PopulateLibp2pHost(rtr.host, pstore); err != nil {
		return err
	}
	return nil
}

func (rtr *rainTreeRouter) setupPeerManager(pstore typesP2P.Peerstore) (err error) {
	rtr.peersManager, err = newPeersManager(rtr.selfAddr, pstore, true)
	return err
}

func (rtr *rainTreeRouter) getHostname() string {
	return rtr.GetBus().GetRuntimeMgr().GetConfig().P2P.Hostname
}

// bootstrapFromRPC fetches the peerstore of the peer at `serviceURL` via RPC
// and adds it to this host's peerstore after performing a health check.
// TECHDEBT(SOLID): refactor; this method has more than one reason to change
func (rtr *rainTreeRouter) bootstrapFromRPC(serviceURL string) {
	rtr.logger.Info().Str("endpoint", serviceURL).Msg("Attempting to bootstrap from bootstrap node")

	client, err := rpc.NewClientWithResponses(serviceURL)
	if err != nil {
		return
	}
	healthCheck, err := client.GetV1Health(context.TODO())
	if err != nil || healthCheck == nil || healthCheck.StatusCode != http.StatusOK {
		rtr.logger.Warn().Str("serviceURL", serviceURL).Msg("Error getting a green health check from bootstrap node")
		return
	}

	// fetch `serviceURL`'s  peerstore
	pstoreProvider := rpcPSP.Create(
		rpcPSP.WithP2PConfig(
			rtr.GetBus().GetRuntimeMgr().GetConfig().P2P,
		),
		rpcPSP.WithCustomRPCURL(serviceURL),
	)

	currentHeightProvider := rpcCHP.NewRPCCurrentHeightProvider(rpcCHP.WithCustomRPCURL(serviceURL))

	pstore, err := pstoreProvider.GetStakedPeerstoreAtHeight(currentHeightProvider.CurrentHeight())
	if err != nil {
		rtr.logger.Warn().Err(err).Str("endpoint", serviceURL).Msg("Error getting address book from bootstrap node")
		return
	}

	// add `serviceURL`'s peers to this node's peerstore
	for _, peer := range pstore.GetPeerList() {
		rtr.logger.Debug().Str("address", peer.GetAddress().String()).Msg("Adding peer to router")
		if err := rtr.AddPeer(peer); err != nil {
			rtr.logger.Error().Err(err).
				Str("pokt_address", peer.GetAddress().String()).
				Msg("adding peer")
		}
	}
}
