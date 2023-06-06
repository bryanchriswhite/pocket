package peer

import (
	"fmt"
	"log"
	"strconv"

	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/spf13/cobra"

	"github.com/pokt-network/pocket/app/client/cli/helpers"
	"github.com/pokt-network/pocket/shared/modules"
)

var (
	printConnectionsHeader = []string{"Peer ID", "Multiaddr", "Opened", "Direction", "NumStreams"}

	connectionsCmd = &cobra.Command{
		Use:   "connections",
		Short: "Print open peer connections",
		RunE:  connectionsRunE,
	}
)

func init() {
	PeerCmd.AddCommand(connectionsCmd)
}

func connectionsRunE(cmd *cobra.Command, _ []string) error {
	bus, ok := helpers.GetValueFromCLIContext[modules.Bus](cmd, helpers.BusCLICtxKey)
	if !ok {
		log.Fatal("unable to get bus from context")
	}

	p2pModule := bus.GetP2PModule()
	if p2pModule == nil {
		return fmt.Errorf("no p2p module found on the bus")
	}

	conns := p2pModule.GetConnections()
	if err := printConnections(conns); err != nil {
		return fmt.Errorf("printing connections: %w", err)
	}

	return nil
}

func peerConnsRowConsumerFactory(conns []libp2pNetwork.Conn) rowConsumer {
	return func(provideRow rowProvider) error {
		for _, conn := range conns {
			err := provideRow(
				conn.RemotePeer().String(),
				conn.RemoteMultiaddr().String(),
				conn.Stat().Opened.String(),
				conn.Stat().Direction.String(),
				strconv.Itoa(conn.Stat().NumStreams),
			)
			if err != nil {
				return err
			}
		}
		return nil
	}
}
func printConnections(conns []libp2pNetwork.Conn) error {
	return printASCIITable(printConnectionsHeader, peerConnsRowConsumerFactory(conns))
}
