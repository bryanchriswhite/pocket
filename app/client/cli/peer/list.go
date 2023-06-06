package peer

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pokt-network/pocket/app/client/cli/helpers"
	"github.com/pokt-network/pocket/p2p/providers/peerstore_provider/persistence"
	"github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/shared/modules"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Print addresses and service URLs of known peers",
	RunE:  listRunE,
}

func init() {
	PeerCmd.AddCommand(listCmd)
}

func listRunE(cmd *cobra.Command, _ []string) error {
	bus, ok := helpers.GetValueFromCLIContext[modules.Bus](cmd, helpers.BusCLICtxKey)
	if !ok {
		log.Fatal("unable to get bus from context")
	}

	p2pPStoreProvider, err := persistence.NewP2PPeerstoreProvider(bus)
	if err != nil {
		log.Fatalf("creatating p2p peerstore provider: %v", err)
	}

	// TODO_THIS_COMMIT: add & react to `--all`, `--staked`, `--unstaked`
	// persistent flags

	pstore, err := p2pPStoreProvider.GetUnstakedPeerstore()
	if err != nil {
		log.Fatalf("getting unstaked peerstore: %v", err)
	}

	selfAddr, err := p2pModule.GetAddress()
	if err != nil {
		log.Fatalf("getting self address: %v", err)
	}
	log.Printf("Self address: %s\n", selfAddr)
	log.Println("Unstaked router peerstore:")

	printPeerList(pstore.GetPeerList())

	//if p2pModule.IsBootstrapped() {
	//
	//}

	// TECHDEBT(workaround): polling for correct peerstore size until there's a
	// better way to know when the p2pModule's routers are bootstrapped.

	return nil
}

var printPeerListHeader = []string{"Peer address", "Peer ServiceURL"}

func printPeerList(peers types.PeerList) {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)

	// Print header
	for _, h := range printPeerListHeader {
		fmt.Fprintf(w, "| %s\t", h)
	}
	fmt.Fprintln(w, "|")

	// Print separator
	for _, header := range printPeerListHeader {
		fmt.Fprintf(w, "| ")
		for range header {
			fmt.Fprintf(w, "-")
		}
		fmt.Fprintf(w, "\t")
	}
	fmt.Fprintln(w, "|")

	for _, peer := range peers {
		fmt.Fprintf(w, "| %s\t", peer.GetAddress())
		fmt.Fprintf(w, "| %s\t", peer.GetServiceURL())
		fmt.Fprintln(w, "|")
	}

	// Flush the buffer and print the table
	w.Flush()
}
