package current_height_provider

//go:generate mockgen -package=mock_types -destination=../../types/mocks/current_height_provider_mock.go github.com/pokt-network/pocket/p2p/providers/current_height_provider CurrentHeightProvider

import "github.com/pokt-network/pocket/shared/modules"

const CurrentHeightProviderSubmoduleName = "current_height_provider"

type CurrentHeightProvider interface {
	modules.Submodule

	CurrentHeight() uint64
}
