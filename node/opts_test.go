package node_test

import (
	"errors"

	"github.com/filecoin-project/go-lotus/node"

	"github.com/filecoin-project/go-lotus/node/modules/lp2p"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
)

func MockHost(mn mocknet.Mocknet) node.Option {
	return node.Options(
		node.ApplyIf(func(s *node.Settings) bool { return !s.Online },
			node.Error(errors.New("MockHost must be specified after Online")),
		),

		node.Override(new(lp2p.RawHost), lp2p.MockHost),
		node.Override(new(mocknet.Mocknet), mn),
	)
}
