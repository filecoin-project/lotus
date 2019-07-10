package node

import (
	"errors"

	"github.com/filecoin-project/go-lotus/node/modules/lp2p"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
)

func MockHost(mn mocknet.Mocknet) Option {
	return Options(
		applyIf(func(s *settings) bool { return !s.online },
			Error(errors.New("MockHost must be specified after Online")),
		),

		Override(new(lp2p.RawHost), lp2p.MockHost),
		Override(new(mocknet.Mocknet), mn),
	)
}
