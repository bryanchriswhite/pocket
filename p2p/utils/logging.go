package utils

import (
	"fmt"
	"net"
	"os"
	"text/tabwriter"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/rs/zerolog"

	"github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/shared/modules"
)

// RowProvider is a function which receives a variadic number of "column" values.
// It is intended to be passed to a `RowConsumer` so that the consumer can operate
// on the column values, row-by-row, without having to know how to produce them.
type RowProvider func(columns ...string) error

// RowConsumer is any function which receives a `RowProvider` in order to consume
// its column values, row-by-row.
type RowConsumer func(RowProvider) error

type scopeCallback func(scope network.ResourceScope) error

var peerListTableHeader = []string{"Peer ID", "Pokt Address", "ServiceURL"}

// LogScopeStatFactory returns a function which prints the given scope stat fields
// to the debug level of the provided logger.
// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p@v0.27.0/core/network#ScopeStat)
// (see: https://pkg.go.dev/github.com/libp2p/go-libp2p@v0.27.0/core/network#ResourceScope)
// TECHDEBT: would prefer receive a pocket logger object instead.
// Typical calls would pass either `logger.Global` or a `*modules.Logger` which
// are disparate types.
func LogScopeStatFactory(logger *zerolog.Logger, msg string) scopeCallback {
	return func(scope network.ResourceScope) error {
		stat := scope.Stat()
		logger.Debug().Fields(map[string]any{
			"InboundConns":    stat.NumConnsInbound,
			"OutboundConns":   stat.NumConnsOutbound,
			"InboundStreams":  stat.NumStreamsInbound,
			"OutboundStreams": stat.NumStreamsOutbound,
		}).Msg(msg)
		return nil
	}
}

func LogOutgoingMsg(logger *modules.Logger, hostname string, peer types.Peer) {
	msg := "OUTGOING MSG"
	logMessage(logger, msg, hostname, peer)
}

func LogIncomingMsg(logger *modules.Logger, hostname string, peer types.Peer) {
	msg := "INCOMING MSG"
	logMessage(logger, msg, hostname, peer)
}

func logMessage(logger *modules.Logger, msg, hostname string, peer types.Peer) {
	remoteHostname, _, err := net.SplitHostPort(peer.GetServiceURL())
	if err != nil {
		logger.Debug().Err(err).
			Str("serviceURL", peer.GetServiceURL()).
			Msg("parsing remote service URL")
		return
	}

	logger.Debug().Fields(map[string]any{
		"local_hostname":  hostname,
		"remote_hostname": remoteHostname,
	}).Msg(msg)
}

func PrintSelfAddress(bus modules.Bus) error {
	p2pModule := bus.GetP2PModule()
	if p2pModule == nil {
		return fmt.Errorf("no p2p module found on the bus")
	}

	selfAddr, err := p2pModule.GetAddress()
	if err != nil {
		return fmt.Errorf("getting self address: %w", err)
	}

	fmt.Printf("Self address: %s\n", selfAddr)
	return nil
}

func PrintASCIITable(header []string, consumeRows RowConsumer) error {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)

	// Print header
	for _, col := range header {
		if _, err := fmt.Fprintf(w, "| %s\t", col); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "|"); err != nil {
		return err
	}

	// Print separator
	for _, col := range header {
		if _, err := fmt.Fprintf(w, "| "); err != nil {
			return err
		}
		for range col {
			if _, err := fmt.Fprintf(w, "-"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "\t"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "|"); err != nil {
		return err
	}

	err := consumeRows(func(row ...string) error {
		for _, col := range row {
			if _, err := fmt.Fprintf(w, "| %s\t", col); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "|"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Flush the buffer and print the table
	return w.Flush()
}

func PeerListRowConsumerFactory(peers types.PeerList) RowConsumer {
	return func(provideRow RowProvider) error {
		for _, peer := range peers {
			libp2pAddrInfo, err := Libp2pAddrInfoFromPeer(peer)
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

func PrintPeerList(peers types.PeerList) error {
	return PrintASCIITable(peerListTableHeader, PeerListRowConsumerFactory(peers))
}
