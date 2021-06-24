package node

import (
	"errors"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"

	"github.com/filecoin-project/lotus/node/modules/lp2p"
)

func MockHost(mn mocknet.Mocknet) Option {
	return Options(
		ApplyIf(func(s *Settings) bool { return !s.Base },
			Error(errors.New("MockHost must be specified after Base")),
		),

		Override(new(lp2p.RawHost), lp2p.MockHost),
		Override(new(mocknet.Mocknet), mn),
	)
}
