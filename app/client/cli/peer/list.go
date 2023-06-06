package peer

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pokt-network/pocket/app/client/cli/helpers"
	"github.com/pokt-network/pocket/shared/messaging"
	"github.com/pokt-network/pocket/shared/modules"
)

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
	bus, ok := helpers.GetValueFromCLIContext[modules.Bus](cmd, helpers.BusCLICtxKey)
	if !ok {
		log.Fatal("unable to get bus from context")
	}

	debugMsg := &messaging.DebugMessage{
		Action: messaging.DebugMessageAction_DEBUG_P2P_PEER_LIST,
		Type:   messaging.DebugMessageRoutingType_DEBUG_MESSAGE_TYPE_BROADCAST,
	}
	debugMsgAny, err := anypb.New(debugMsg)
	if err != nil {
		return fmt.Errorf("creating anypb from debug message: %w", err)
	}

	if localFlag {
		// call common behavior -- debug message handler

	}

	// TECHDEBT: will need to wait for DHT bootstrapping to complete before
	// this command can be used with unstaked actors.

	if err := bus.GetP2PModule().Broadcast(debugMsgAny); err != nil {
		return fmt.Errorf("broadcasting debug message: %w", err)
	}

	return nil
}
