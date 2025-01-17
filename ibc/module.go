package ibc

import (
	"github.com/pokt-network/pocket/logger"
	"github.com/pokt-network/pocket/runtime/configs"
	coreTypes "github.com/pokt-network/pocket/shared/core/types"
	"github.com/pokt-network/pocket/shared/modules"
	"github.com/pokt-network/pocket/shared/modules/base_modules"
)

var _ modules.IBCModule = &ibcModule{}

type ibcModule struct {
	base_modules.IntegrableModule

	cfg    *configs.IBCConfig
	logger *modules.Logger

	// Only a single host is allowed at a time
	host *host
}

func Create(bus modules.Bus, options ...modules.ModuleOption) (modules.Module, error) {
	return new(ibcModule).Create(bus, options...)
}

func (m *ibcModule) Create(bus modules.Bus, options ...modules.ModuleOption) (modules.Module, error) {
	*m = ibcModule{
		cfg:    bus.GetRuntimeMgr().GetConfig().IBC,
		logger: logger.Global.CreateLoggerForModule(modules.IBCModuleName),
	}
	m.logger.Info().Msg("🪐 creating IBC module 🪐")

	for _, option := range options {
		option(m)
	}

	bus.RegisterModule(m)

	// Only validators can be an IBC host due to the need for reliability
	isValidator := false
	if _, err := m.GetBus().GetUtilityModule().GetValidatorModule(); err == nil {
		isValidator = true
	}
	if isValidator && m.cfg.Enabled {
		m.logger.Info().Msg("🛰️ creating IBC host 🛰️")
		if err := m.newHost(); err != nil {
			m.logger.Error().Err(err).Msg("❌ failed to create IBC host")
			return nil, err
		}
	}

	return m, nil
}

func (m *ibcModule) Start() error {
	if !m.cfg.Enabled {
		m.logger.Info().Msg("🚫 IBC module disabled 🚫")
		return nil
	}
	m.logger.Info().Msg("🪐 starting IBC module 🪐")
	// TODO: start the host logic
	return nil
}

func (m *ibcModule) Stop() error {
	return nil
}

func (m *ibcModule) GetHost() modules.IBCHost {
	return m.host
}

func (m *ibcModule) GetModuleName() string {
	return modules.IBCModuleName
}

// newHost returns a new IBC host instance if one is not already created
func (m *ibcModule) newHost() error {
	if m.host != nil {
		return coreTypes.ErrHostAlreadyExists()
	}
	host := &host{
		logger: m.logger,
	}
	m.host = host
	return nil
}
