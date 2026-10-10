package consensus

import (
	"testing"

	"github.com/ipfs/go-cid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestDecodeAndCheckBlockParents(t *testing.T) {
	parent := mock.TipSet(mock.MkBlock(nil, 1, 1))
	siblings := make([]cid.Cid, types.MaxTipSetSize+1)
	for i := range siblings {
		siblings[i] = mock.MkBlock(parent, 1, uint64(i+2)).Cid()
	}

	check := func(parents []cid.Cid) (string, error) {
		hdr := mock.MkBlock(parent, 1, 100)
		hdr.Parents = parents
		data, err := (&types.BlockMsg{Header: hdr}).Serialize()
		require.NoError(t, err)
		_, what, err := decodeAndCheckBlock(&pubsub.Message{Message: &pubsub_pb.Message{Data: data}})
		return what, err
	}

	_, err := check(siblings[:types.MaxTipSetSize])
	require.NoError(t, err)

	for name, parents := range map[string][]cid.Cid{
		"empty":    nil,
		"too wide": siblings,
		"repeated": {siblings[0], siblings[0]},
	} {
		t.Run(name, func(t *testing.T) {
			what, err := check(parents)
			require.Error(t, err)
			require.Equal(t, "invalid_parents", what)
		})
	}
}
