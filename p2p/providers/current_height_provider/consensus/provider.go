package consensus

import (
	"github.com/pokt-network/pocket/p2p/providers"
	"github.com/pokt-network/pocket/p2p/providers/current_height_provider"
	"github.com/pokt-network/pocket/shared/modules"
	"github.com/pokt-network/pocket/shared/modules/base_modules"
)

var _ providers.CurrentHeightProvider = &consensusCurrentHeightProvider{}

type consensusCurrentHeightProvider struct {
	base_modules.IntegrableModule
}

func Create(bus modules.Bus) (providers.CurrentHeightProvider, error) {
	return new(consensusCurrentHeightProvider).Create(bus)
}

func (*consensusCurrentHeightProvider) Create(bus modules.Bus) (providers.CurrentHeightProvider, error) {
	consCHP := &consensusCurrentHeightProvider{}
	bus.RegisterModule(consCHP)
	return consCHP, nil
}

func (consCHP *consensusCurrentHeightProvider) GetModuleName() string {
	return current_height_provider.CurrentHeightProviderSubmoduleName
}

func (consCHP *consensusCurrentHeightProvider) CurrentHeight() uint64 {
	return consCHP.GetBus().GetConsensusModule().CurrentHeight()
}
