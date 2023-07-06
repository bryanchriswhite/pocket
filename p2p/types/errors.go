package types

import (
	"errors"
	"fmt"
)

var (
	ErrUnknownPeer                   = errors.New("unknown peer")
	ErrPeerDiscoveryDebugRPCDisabled = errors.New("peer discovery debug RPC disabled")
)

func ErrUnknownEventType(msg any) error {
	return fmt.Errorf("unknown event type: %v", msg)
}
