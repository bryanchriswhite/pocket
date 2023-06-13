package p2p

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	rpcCHP "github.com/pokt-network/pocket/p2p/providers/current_height_provider/rpc"
	rpcABP "github.com/pokt-network/pocket/p2p/providers/peerstore_provider/rpc"
	"github.com/pokt-network/pocket/rpc"
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
	var wg sync.WaitGroup

	for _, serviceURL := range m.bootstrapNodes {
		wg.Add(1)

		// concurrent bootstrapping
		// TECHDEBT: add maximum bootstrapping concurrency to P2P config
		go m.bootstrapFromServiceURL(serviceURL)
	}
	wg.Wait()

	if m.network.GetPeerstore().Size() == 0 {
		return fmt.Errorf("bootstrap failed")
	}
	return nil
}

// boostrapFromServiceURL fetches the peerstore of the peer at `serviceURL and
// adds it to this host's peerstore after performing a health check.
// TECHDEBT(SOLID): refactor; this method has more than one reason to change
func (m *p2pModule) bootstrapFromServiceURL(serviceURL string) {
	// NB: no need closure `serviceURL` as long as it remains a value type
	m.logger.Info().Str("endpoint", serviceURL).Msg("Attempting to bootstrap from bootstrap node")

	client, err := rpc.NewClientWithResponses(serviceURL)
	if err != nil {
		return
	}
	healthCheck, err := client.GetV1Health(context.TODO())
	if err != nil || healthCheck == nil || healthCheck.StatusCode != http.StatusOK {
		m.logger.Warn().Str("serviceURL", serviceURL).Msg("Error getting a green health check from bootstrap node")
		return
	}

	// fetch `serviceURL`'s  peerstore
	pstoreProvider := rpcABP.Create(
		rpcABP.WithP2PConfig(
			m.GetBus().GetRuntimeMgr().GetConfig().P2P,
		),
		rpcABP.WithCustomRPCURL(serviceURL),
	)

	currentHeightProvider := rpcCHP.NewRPCCurrentHeightProvider(rpcCHP.WithCustomRPCURL(serviceURL))

	pstore, err := pstoreProvider.GetStakedPeerstoreAtHeight(currentHeightProvider.CurrentHeight())
	if err != nil {
		m.logger.Warn().Err(err).Str("endpoint", serviceURL).Msg("Error getting address book from bootstrap node")
		return
	}

	// add `serviceURL`'s peers to this node's peerstore
	for _, peer := range pstore.GetPeerList() {
		m.logger.Debug().Str("address", peer.GetAddress().String()).Msg("Adding peer to router")
		if err := m.stakedActorRouter.AddPeer(peer); err != nil {
			m.logger.Error().Err(err).
				Str("pokt_address", peer.GetAddress().String()).
				Msg("adding peer")
		}
	}
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
