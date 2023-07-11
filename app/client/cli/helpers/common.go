package helpers

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pokt-network/pocket/logger"
	"github.com/pokt-network/pocket/p2p/providers/current_height_provider"
	"github.com/pokt-network/pocket/p2p/providers/peerstore_provider"
	"github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/runtime"
	"github.com/pokt-network/pocket/shared/messaging"
	"github.com/pokt-network/pocket/shared/modules"
)

var (
	// TECHDEBT: Accept reading this from `Datadir` and/or as a flag.
	genesisPath = runtime.GetEnv("GENESIS_PATH", "build/config/genesis.json")

	// P2PMod is initialized in order to broadcast a message to the local network
	// TECHDEBT: prefer to retrieve P2P module from the bus instead.
	P2PMod modules.P2PModule
)

// fetchPeerstore retrieves the providers from the CLI context and uses them to retrieve the address book for the current height
func FetchPeerstore(cmd *cobra.Command) (types.Peerstore, error) {
	bus, err := GetBusFromCmd(cmd)
	if err != nil {
		return nil, err
	}

	modulesRegistry := bus.GetModulesRegistry()
	// TECHDEBT(#810, #811): use `bus.GetPeerstoreProvider()` after peerstore provider
	// is retrievable as a proper submodule
	pstoreProvider, err := modulesRegistry.GetModule(peerstore_provider.PeerstoreProviderSubmoduleName)
	if err != nil {
		return nil, errors.New("retrieving peerstore provider")
	}
	currentHeightProvider, err := modulesRegistry.GetModule(current_height_provider.ModuleName)
	if err != nil {
		return nil, errors.New("retrieving currentHeightProvider")
	}

	height := currentHeightProvider.(current_height_provider.CurrentHeightProvider).CurrentHeight()
	pstore, err := pstoreProvider.(peerstore_provider.PeerstoreProvider).GetStakedPeerstoreAtHeight(height)
	if err != nil {
		return nil, fmt.Errorf("retrieving peerstore at height %d", height)
	}
	// Inform the client's main P2P that a the blockchain is at a new height so it can, if needed, update its view of the validator set
	err = sendConsensusNewHeightEventToP2PModule(height, bus)
	if err != nil {
		return nil, errors.New("sending consensus new height event")
	}
	return pstore, nil
}

// sendConsensusNewHeightEventToP2PModule mimicks the consensus module sending a ConsensusNewHeightEvent to the p2p module
// This is necessary because the debug client is not a validator and has no consensus module but it has to update the peerstore
// depending on the changes in the validator set.
// TODO(#613): Make the debug client mimic a full node.
func sendConsensusNewHeightEventToP2PModule(height uint64, bus modules.Bus) error {
	newHeightEvent, err := messaging.PackMessage(&messaging.ConsensusNewHeightEvent{Height: height})
	if err != nil {
		logger.Global.Fatal().Err(err).Msg("Failed to pack consensus new height event")
	}
	return bus.GetP2PModule().HandleEvent(newHeightEvent.Content)
}
