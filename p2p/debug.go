package p2p

import (
	"fmt"
	"github.com/pokt-network/pocket/p2p/providers/current_height_provider"
	"github.com/pokt-network/pocket/p2p/providers/peerstore_provider"
	"github.com/pokt-network/pocket/shared/modules"
	"os"

	"github.com/pokt-network/pocket/p2p/debug"
	typesP2P "github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/shared/messaging"
)

func (m *p2pModule) handleDebugMessage(msg *messaging.DebugMessage) error {
	switch msg.Action {
	case messaging.DebugMessageAction_DEBUG_P2P_PEER_LIST:
		if !m.cfg.EnablePeerDiscoveryDebugRpc {
			return typesP2P.ErrPeerDiscoveryDebugRPCDisabled
		}
	default:
		// This debug message isn't intended for the P2P module, ignore it.
		return nil
	}

	switch msg.Action {
	case messaging.DebugMessageAction_DEBUG_P2P_PEER_LIST:
		routerType := debug.RouterType(msg.Message.Value)
		return PrintPeerList(m.GetBus(), routerType)
	default:
		return fmt.Errorf("unsupported P2P debug message action: %s", msg.Action)
	}
}

func PrintPeerList(bus modules.Bus, routerType debug.RouterType) error {
	var (
		peers           typesP2P.PeerList
		pstorePlurality = ""
	)

	// TECHDEBT(#810, #811): use `bus.GetCurrentHeightProvider()` after current
	// height provider is retrievable as a proper submodule.
	currentHeightProviderModule, err := bus.GetModulesRegistry().
		GetModule(current_height_provider.CurrentHeightProviderSubmoduleName)
	if err != nil {
		return fmt.Errorf("getting current height provider: %w", err)
	}
	currentHeightProvider, ok := currentHeightProviderModule.(current_height_provider.CurrentHeightProvider)
	if !ok {
		return fmt.Errorf("unknown current height provider type: %T", currentHeightProviderModule)
	}
	//--

	// TECHDEBT(#810, #811): use `bus.GetPeerstoreProvider()` after peerstore provider
	// is retrievable as a proper submodule.
	pstoreProviderModule, err := bus.GetModulesRegistry().
		GetModule(peerstore_provider.PeerstoreProviderSubmoduleName)
	if err != nil {
		return fmt.Errorf("getting peerstore provider: %w", err)
	}
	pstoreProvider, ok := pstoreProviderModule.(peerstore_provider.PeerstoreProvider)
	if !ok {
		return fmt.Errorf("unknown peerstore provider type: %T", pstoreProviderModule)
	}
	//--

	switch routerType {
	case debug.StakedRouterType:
		// TECHDEBT: add `PeerstoreProvider#GetStakedPeerstoreAtCurrentHeight()`
		// interface method.
		currentHeight := currentHeightProvider.CurrentHeight()
		pstore, err := pstoreProvider.GetStakedPeerstoreAtHeight(currentHeight)
		if err != nil {
			return fmt.Errorf("getting unstaked peerstore: %v", err)
		}

		peers = pstore.GetPeerList()
	case debug.UnstakedRouterType:
		pstore, err := pstoreProvider.GetUnstakedPeerstore()
		if err != nil {
			return fmt.Errorf("getting unstaked peerstore: %v", err)
		}

		peers = pstore.GetPeerList()
	case debug.AllRouterTypes:
		pstorePlurality = "s"

		// TECHDEBT: add `PeerstoreProvider#GetStakedPeerstoreAtCurrentHeight()`
		currentHeight := currentHeightProvider.CurrentHeight()
		stakedPStore, err := pstoreProvider.GetStakedPeerstoreAtHeight(currentHeight)
		if err != nil {
			return fmt.Errorf("getting unstaked peerstore: %v", err)
		}
		unstakedPStore, err := pstoreProvider.GetUnstakedPeerstore()
		if err != nil {
			return fmt.Errorf("getting unstaked peerstore: %v", err)
		}

		unstakedPeers := unstakedPStore.GetPeerList()
		stakedPeers := stakedPStore.GetPeerList()
		additionalPeers, _ := unstakedPeers.Delta(stakedPeers)

		// NB: there should never be any "additional" peers. This would represent
		// a staked actor who is not participating in background gossip for some
		// reason. It's possible that a staked actor node which has restarted
		// recently and hasn't yet completed background router bootstrapping may
		// result in peers experiencing this state.
		if len(additionalPeers) == 0 {
			return debug.PrintPeerListTable(unstakedPeers)
		}

		var allPeers typesP2P.PeerList
		for _, peer := range additionalPeers {
			allPeers = append(unstakedPeers, peer)
		}
		peers = allPeers
	default:
		return fmt.Errorf("unsupported router type: %s", routerType)
	}

	if err := debug.LogSelfAddress(bus); err != nil {
		return fmt.Errorf("printing self address: %w", err)
	}

	// NB: Intentionally printing with `fmt` instead of the logger to match
	// `utils.PrintPeerListTable` which does not use the logger due to
	// incompatibilities with the tabwriter.
	if _, err := fmt.Fprintf(
		os.Stdout,
		"%s router peerstore%s:\n",
		routerType,
		pstorePlurality,
	); err != nil {
		return fmt.Errorf("printing to stdout: %w", err)
	}

	if err := debug.PrintPeerListTable(peers); err != nil {
		return fmt.Errorf("printing peer list: %w", err)
	}
	return nil
}
