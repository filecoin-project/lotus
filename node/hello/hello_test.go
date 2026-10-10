package hello

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

// TestHandleStreamRejectsInvalidTipSet checks that a hello carrying a malformed
// heaviest tipset key disconnects the peer before any fetch is attempted. The
// service has no chain store or exchange, so reaching the fetch would panic.
func TestHandleStreamRejectsInvalidTipSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mn, err := mocknet.FullMeshLinked(2)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mn.Close() })
	server, attacker := mn.Hosts()[0], mn.Hosts()[1]

	gen := mock.TipSet(mock.MkBlock(nil, 1, 1))
	hs := &Service{h: server, syncer: &chain.Syncer{Genesis: gen}}
	server.SetStreamHandler(ProtocolID, hs.HandleStream)

	require.NoError(t, mn.ConnectAllButSelf())

	s, err := attacker.NewStream(ctx, server.ID(), ProtocolID)
	require.NoError(t, err)
	local := gen.Cids()[0]
	require.NoError(t, cborutil.WriteCborRPC(s, &HelloMessage{
		HeaviestTipSet:       []cid.Cid{local, local},
		HeaviestTipSetHeight: 1,
		HeaviestTipSetWeight: types.NewInt(1),
		GenesisHash:          local,
	}))

	require.Eventually(t, func() bool {
		return len(attacker.Network().ConnsToPeer(server.ID())) == 0
	}, 5*time.Second, 10*time.Millisecond)
}
