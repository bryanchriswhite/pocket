package p2p

import (
	"fmt"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/pokt-network/pocket/logger"
	"github.com/pokt-network/pocket/shared/modules"
)

func WithDebugNotifee() modules.ModuleOption {
	return func(m modules.InjectableModule) {
		p2pModule, ok := m.(*p2pModule)
		if !ok {
			getP2PLogger().Error().
				Err(fmt.Errorf("unexpected p2pModule type %T", m)).
				Msg("casting p2pModule")
		}
		p2pModule.appendStartOption(startWithDebugNotifee())
	}
}

func getP2PLogger() *modules.Logger {
	return logger.Global.CreateLoggerForModule(modules.P2PModuleName)
}

func startWithDebugNotifee() modules.ModuleOption {
	return func(m modules.InjectableModule) {
		p2pModule, ok := m.(*p2pModule)
		if !ok {
			getP2PLogger().Error().
				Err(fmt.Errorf("unexpected p2pModule type %T", m)).
				Msg("casting p2pModule")
		}

		debugNotifee := &libp2pNetwork.NotifyBundle{
			ConnectedF: func(_ libp2pNetwork.Network, conn libp2pNetwork.Conn) {
				getP2PLogger().Debug().
					Fields(connectionLogFields(conn)).
					Msg("connected")
			},
			DisconnectedF: func(_ libp2pNetwork.Network, conn libp2pNetwork.Conn) {
				getP2PLogger().Debug().
					Fields(connectionLogFields(conn)).
					Msg("disconnected")
			},
		}

		p2pModule.host.Network().Notify(debugNotifee)
	}
}

func connectionLogFields(conn libp2pNetwork.Conn) map[string]any {
	return map[string]any{
		"local_id":  conn.LocalPeer().String(),
		"remote_id": conn.RemotePeer().String(),
	}
}
