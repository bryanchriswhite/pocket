package peer

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/pokt-network/pocket/shared/modules"
)

type rowProvider func(...string) error
type rowConsumer func(rowProvider) error

func printSelfAddress(bus modules.Bus) error {
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

func printASCIITable(header []string, consumeRows rowConsumer) error {
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
