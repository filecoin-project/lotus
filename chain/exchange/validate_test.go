package exchange

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func mkTipSetBlocks(t *testing.T, n int) []*types.BlockHeader {
	t.Helper()
	parent := mock.TipSet(mock.MkBlock(nil, 1, 1))
	blks := make([]*types.BlockHeader, n)
	for i := range blks {
		blks[i] = mock.MkBlock(parent, 1, uint64(i+2))
	}
	return blks
}

func cidsOf(blks []*types.BlockHeader) []cid.Cid {
	cids := make([]cid.Cid, len(blks))
	for i, b := range blks {
		cids[i] = b.Cid()
	}
	return cids
}

func TestValidateRequestHead(t *testing.T) {
	blks := mkTipSetBlocks(t, types.MaxTipSetSize+1)
	cids := cidsOf(blks)

	for name, tc := range map[string]struct {
		head []cid.Cid
		ok   bool
	}{
		"single":    {head: cids[:1], ok: true},
		"max width": {head: cids[:types.MaxTipSetSize], ok: true},
		"empty":     {head: nil},
		"too wide":  {head: cids},
		"repeated":  {head: []cid.Cid{cids[0], cids[0]}},
	} {
		t.Run(name, func(t *testing.T) {
			req := &Request{Head: tc.head, Length: 1, Options: Headers}
			_, resp := validateRequest(context.Background(), req)
			if tc.ok {
				require.Nil(t, resp)
			} else {
				require.NotNil(t, resp)
				require.Equal(t, status(BadRequest), resp.Status)
			}
		})
	}
}

func TestDoRequestRejectsInvalidHead(t *testing.T) {
	c := &client{}
	head := cidsOf(mkTipSetBlocks(t, 1))
	_, err := c.doRequest(context.Background(), &Request{Head: []cid.Cid{head[0], head[0]}, Length: 1, Options: Headers}, nil, nil)
	require.ErrorContains(t, err, "duplicate")
}

func TestProcessResponseTipSetBlocks(t *testing.T) {
	blks := mkTipSetBlocks(t, types.MaxTipSetSize+1)
	c := &client{}
	process := func(head []cid.Cid, resBlocks []*types.BlockHeader) error {
		req := &Request{Head: head, Length: 1, Options: Headers}
		res := &Response{Status: Ok, Chain: []*BSTipSet{{Blocks: resBlocks}}}
		_, err := c.processResponse(req, res, nil)
		return err
	}

	t.Run("max width", func(t *testing.T) {
		ts := mock.TipSet(blks[:types.MaxTipSetSize]...)
		require.NoError(t, process(ts.Cids(), blks[:types.MaxTipSetSize]))
	})
	t.Run("too wide", func(t *testing.T) {
		require.ErrorContains(t, process(cidsOf(blks), blks), "more than the maximum")
	})
	t.Run("repeated parents", func(t *testing.T) {
		blk := *blks[0]
		blk.Parents = []cid.Cid{blk.Parents[0], blk.Parents[0]}
		require.ErrorContains(t, process([]cid.Cid{blk.Cid()}, []*types.BlockHeader{&blk}), "duplicates")
	})
	t.Run("genesis parents unchecked", func(t *testing.T) {
		gen := mock.MkBlock(nil, 1, 1)
		gen.Parents = []cid.Cid{cid.MustParse("bafyreiaqpwbbyjo4a42saasj36kkrpv4tsherf2e7bvezkert2a7dhonoi")}
		require.NoError(t, process([]cid.Cid{gen.Cid()}, []*types.BlockHeader{gen}))
	})
	t.Run("repeated block", func(t *testing.T) {
		head := []cid.Cid{blks[0].Cid(), blks[0].Cid()}
		require.ErrorContains(t, process(head, []*types.BlockHeader{blks[0], blks[0]}), "duplicate")
	})
}

func TestCollectChainSegmentRejectsInvalidParents(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemory()
	cs := store.NewChainStore(bs, bs, datastore.NewMapDatastore(), nil, nil)
	defer cs.Close() //nolint:errcheck

	parent := mock.TipSet(mock.MkBlock(nil, 1, 1))
	child := mock.MkBlock(parent, 1, 2)
	child.Parents = []cid.Cid{parent.Cids()[0], parent.Cids()[0]}
	require.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{parent, mock.TipSet(child)}))

	req := &validatedRequest{
		head:    types.NewTipSetKey(child.Cid()),
		length:  2,
		options: parseOptions(Headers),
	}
	_, err := collectChainSegment(ctx, cs, req)
	require.ErrorContains(t, err, "duplicates")
}

// compacted builds a single-tipset response of blocks blocks, each including
// perBlock references to one BLS message.
func compacted(blocks, perBlock int) []*BSTipSet {
	inc := make([][]uint64, blocks)
	for i := range inc {
		inc[i] = make([]uint64, perBlock)
	}
	return []*BSTipSet{{
		Blocks: make([]*types.BlockHeader, blocks),
		Messages: &CompactedMessages{
			Bls:           make([]*types.Message, 1),
			BlsIncludes:   inc,
			SecpkIncludes: make([][]uint64, blocks),
		},
	}}
}

func TestValidateCompressedIndicesBounds(t *testing.T) {
	c := &client{}
	require.NoError(t, c.validateCompressedIndices(compacted(types.MaxTipSetSize, buildconstants.BlockMessageLimit)))
	require.ErrorContains(t, c.validateCompressedIndices(compacted(types.MaxTipSetSize+1, 1)), "blocks")
	require.ErrorContains(t, c.validateCompressedIndices(compacted(1, buildconstants.BlockMessageLimit+1)), "messages")
}

func TestValidateRequestTruncatesMessages(t *testing.T) {
	head := cidsOf(mkTipSetBlocks(t, 1))

	vr, resp := validateRequest(context.Background(), &Request{Head: head, Length: 900, Options: Headers | Messages})
	require.Nil(t, resp)
	require.Equal(t, uint64(MaxMessagesRequestLength), vr.length)
	require.Equal(t, uint64(900), vr.requestedLength)

	vr, resp = validateRequest(context.Background(), &Request{Head: head, Length: 900, Options: Headers})
	require.Nil(t, resp)
	require.Equal(t, uint64(900), vr.length)
}

func TestServeTruncatedRequestIsPartial(t *testing.T) {
	ctx := context.Background()
	bs := blockstore.NewMemory()
	cs := store.NewChainStore(bs, bs, datastore.NewMapDatastore(), nil, nil)
	defer cs.Close() //nolint:errcheck
	parent := mock.TipSet(mock.MkBlock(nil, 1, 1))
	child := mock.TipSet(mock.MkBlock(parent, 1, 2))
	require.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{parent, child}))

	s := NewServer(cs).(*server)
	resp, err := s.serviceRequest(ctx, &validatedRequest{
		head:            child.Key(),
		length:          1,
		requestedLength: 2,
		options:         parseOptions(Headers),
	})
	require.NoError(t, err)
	require.Equal(t, status(Partial), resp.Status)
	require.Len(t, resp.Chain, 1)
}
