package p2p

import (
	"context"
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pokt-network/pocket/runtime/defaults"
)

// configureBootstrapNodes parses the bootstrap nodes from the config and validates them
func (m *p2pModule) configureBootstrapNodes() error {
	p2pCfg := m.GetBus().GetRuntimeMgr().GetConfig().P2P

	bootstrapNodesCsv := strings.Trim(p2pCfg.BootstrapNodesCsv, " ")
	if bootstrapNodesCsv == "" {
		bootstrapNodesCsv = defaults.DefaultP2PBootstrapNodesCsv
	}
	csvReader := csv.NewReader(strings.NewReader(bootstrapNodesCsv))
	bootStrapNodes, err := csvReader.Read()
	if err != nil {
		return fmt.Errorf("error parsing bootstrap nodes: %w", err)
	}

	// validate the bootstrap nodes
	for i, node := range bootStrapNodes {
		bootStrapNodes[i] = strings.Trim(node, " ")
		if !isValidHostnamePort(bootStrapNodes[i]) {
			return fmt.Errorf("invalid bootstrap node: %s", bootStrapNodes[i])
		}
	}
	m.bootstrapNodes = bootStrapNodes
	return nil
}

// bootstrap attempts to bootstrap from a bootstrap node
func (m *p2pModule) bootstrap() error {
	if isStaked, err := m.isStakedActor(); err != nil {
		return err
	} else if isStaked {
		// TECHDEBT(#595): add ctx to interface methods and propagate down.
		if err := m.stakedActorRouter.Bootstrap(
			context.TODO(),
			m.cfg.MaxBootstrapConcurrency,
		); err != nil {
			return err
		}
	}
	return nil
}

func isValidHostnamePort(str string) bool {
	pattern := regexp.MustCompile(`^(https?)://([a-zA-Z0-9.-]+):(\d{1,5})$`)
	matches := pattern.FindStringSubmatch(str)
	if len(matches) != 4 {
		return false
	}
	protocol := matches[1]
	if protocol != "http" && protocol != "https" {
		return false
	}
	port, err := strconv.Atoi(matches[3])
	if err != nil || port < 0 || port > 65535 {
		return false
	}
	return true
}
