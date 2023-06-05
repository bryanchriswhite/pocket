package persistence

import (
	"fmt"

	"github.com/pokt-network/pocket/p2p/providers/peerstore_provider"
	typesP2P "github.com/pokt-network/pocket/p2p/types"
	"github.com/pokt-network/pocket/shared/modules"
	"github.com/pokt-network/pocket/shared/modules/base_modules"
)

var (
	_ peerstore_provider.PeerstoreProvider = &p2pPeerstoreProvider{}
	_ p2pPStoreProviderFactory             = &p2pPeerstoreProvider{}
)

type p2pPStoreProviderFactory = modules.FactoryWithConfig[peerstore_provider.PeerstoreProvider, modules.P2PModule]

type p2pPeerstoreProvider struct {
	base_modules.IntegratableModule
	persistencePeerstoreProvider

	p2pModule modules.P2PModule
}

// TODO_THIS_COMMIT: refactor/move (?)
type UnstakedPeerstoreProvider interface {
	GetUnstakedPeerstore() (typesP2P.Peerstore, error)
}

func NewP2PPeerstoreProvider(
	bus modules.Bus,
	p2pModule modules.P2PModule,
) (peerstore_provider.PeerstoreProvider, error) {
	return new(p2pPeerstoreProvider).Create(bus, p2pModule)
}

func (*p2pPeerstoreProvider) Create(
	bus modules.Bus,
	p2pModule modules.P2PModule,
) (peerstore_provider.PeerstoreProvider, error) {
	if bus == nil {
		return nil, fmt.Errorf("bus is required")
	}

	if p2pModule == nil {
		return nil, fmt.Errorf("p2p module is required")
	}

	p2pPSP := &p2pPeerstoreProvider{
		IntegratableModule: *base_modules.NewIntegratableModule(bus),
		p2pModule:          p2pModule,
	}

	return p2pPSP, nil
}

func (p2pPSP *p2pPeerstoreProvider) Start() error {
	return nil
}

func (*p2pPeerstoreProvider) GetModuleName() string {
	return peerstore_provider.ModuleName
}

func (p2pPSP *p2pPeerstoreProvider) GetUnstakedPeerstore() (typesP2P.Peerstore, error) {
	unstakedPStoreProvider, ok := p2pPSP.p2pModule.(UnstakedPeerstoreProvider)
	if !ok {
		return nil, fmt.Errorf("p2p module does not implement UnstakedPeerstoreProvider")
	}
	return unstakedPStoreProvider.GetUnstakedPeerstore()
}
