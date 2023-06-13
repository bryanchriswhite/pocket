package peer

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/pokt-network/pocket/app/client/cli/helpers"
	"github.com/pokt-network/pocket/p2p/providers/peerstore_provider/persistence"
	"github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/p2p/utils"
	"github.com/pokt-network/pocket/shared/modules"
)

var (
	printPeerListHeader = []string{"Peer ID", "Pokt Address", "ServiceURL"}

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

	p2pPStoreProvider, err := persistence.Create(bus)
	if err != nil {
		log.Fatalf("creatating p2p peerstore provider: %v", err)
	}

	// TODO_THIS_COMMIT: add & react to `--all`, `--staked`, `--unstaked`
	// persistent flags

	pstore, err := p2pPStoreProvider.GetUnstakedPeerstore()
	if err != nil {
		log.Fatalf("getting unstaked peerstore: %v", err)
	}

	if err := printSelfAddress(bus); err != nil {
		return err
	}

	log.Println("Unstaked router peerstore:")
	if err := printPeerList(pstore.GetPeerList()); err != nil {
		return fmt.Errorf("printing peer list: %w", err)
	}

	//if p2pModule.IsBootstrapped() {
	//
	//}

	// TECHDEBT(workaround): polling for correct peerstore size until there's a
	// better way to know when the p2pModule's routers are bootstrapped.

	return nil
}

func peerListRowConsumerFactory(peers types.PeerList) rowConsumer {
	return func(provideRow rowProvider) error {
		for _, peer := range peers {
			libp2pAddrInfo, err := utils.Libp2pAddrInfoFromPeer(peer)
			if err != nil {
				return fmt.Errorf("converting peer to libp2p addr info: %w", err)
			}

			err = provideRow(
				libp2pAddrInfo.ID.String(),
				peer.GetAddress().String(),
				peer.GetServiceURL(),
			)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func printPeerList(peers types.PeerList) error {
	return printASCIITable(printPeerListHeader, peerListRowConsumerFactory(peers))
}
