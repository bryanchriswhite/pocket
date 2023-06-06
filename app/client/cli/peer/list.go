package peer

import (
	"fmt"
	"github.com/pokt-network/pocket/app/client/cli/helpers"
	"github.com/pokt-network/pocket/logger"
	"github.com/pokt-network/pocket/p2p"
	"github.com/pokt-network/pocket/p2p/debug"
	"github.com/pokt-network/pocket/shared/messaging"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"
	"time"
)

// TODO_THIS_COMMIT: convert to a flag
const bootstrapDelay = 5 * time.Second

var (
	listCmd = &cobra.Command{
		Use:   "list",
		Short: "Print addresses and service URLs of known peers",
		RunE:  listRunE,
	}
)

func init() {
	PeerCmd.AddCommand(listCmd)
}

func listRunE(cmd *cobra.Command, _ []string) error {
	var routerType debug.RouterType

	bus, err := helpers.GetBusFromCmd(cmd)
	if err != nil {
		return err
	}

	switch {
	case stakedFlag:
		routerType = debug.StakedRouterType
	case unstakedFlag:
		routerType = debug.UnstakedRouterType
	case allFlag:
		fallthrough
	default:
		routerType = debug.AllRouterTypes
	}

	debugMsg := &messaging.DebugMessage{
		Action: messaging.DebugMessageAction_DEBUG_P2P_PEER_LIST,
		Type:   messaging.DebugMessageRoutingType_DEBUG_MESSAGE_TYPE_BROADCAST,
		Message: &anypb.Any{
			Value: []byte(routerType),
		},
	}
	debugMsgAny, err := anypb.New(debugMsg)
	if err != nil {
		return fmt.Errorf("creating anypb from debug message: %w", err)
	}

	if localFlag {
		if err := p2p.PrintPeerList(bus, routerType); err != nil {
			return fmt.Errorf("printing peer list: %w", err)
		}
		return nil
	}

	if broadcastFlag {
		// TECHDEBT(#810, #811): will need to wait for DHT bootstrapping to complete before
		// this command can be used with unstaked actors.

		bus, err = helpers.GetBusFromCmd(cmd)
		if err != nil {
			return err
		}

		if err := bus.GetP2PModule().Broadcast(debugMsgAny); err != nil {
			return fmt.Errorf("broadcasting debug message: %w", err)
		}
		return nil
	}

	// TECHDEBT(#810, #811): use broadcast instead to reach all peers.
	return sendToStakedPeers(cmd, debugMsgAny)
}

func sendToStakedPeers(cmd *cobra.Command, debugMsgAny *anypb.Any) error {
	bus, err := helpers.GetBusFromCmd(cmd)
	if err != nil {
		return err
	}

	pstore, err := helpers.FetchPeerstore(cmd)
	if err != nil {
		logger.Global.Fatal().Err(err).Msg("Unable to retrieve the pstore")
	}

	if pstore.Size() == 0 {
		logger.Global.Fatal().Msg("No validators found")
	}

	for _, peer := range pstore.GetPeerList() {
		if err := bus.GetP2PModule().Send(peer.GetAddress(), debugMsgAny); err != nil {
			logger.Global.Error().Err(err).Msg("Failed to send debug message")
		}
	}
	return nil
}
