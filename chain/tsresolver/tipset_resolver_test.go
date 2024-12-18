package tsresolver_test

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	f3mock "github.com/filecoin-project/lotus/chain/lf3/mock"
	"github.com/filecoin-project/lotus/chain/tsresolver"
	"github.com/filecoin-project/lotus/chain/types"
)

var dummyCid = cid.MustParse("bafkqaaa")

func TestResolveEthBlockSelector(t *testing.T) {
	ctx := context.Background()

	t.Run("basic selectors", func(t *testing.T) {
		req := require.New(t)

		parent := makeTestTipSet(t, 99, nil)
		head := makeTestTipSet(t, 100, parent.Cids())

		loader := &mockTipSetLoader{
			head: head,
			tipsets: map[types.TipSetKey]*types.TipSet{
				parent.Key(): parent,
			},
		}

		resolver := tsresolver.NewTipSetResolver(loader, &f3mock.MockF3API{})

		ts, err := resolver.ResolveEthBlockSelector(ctx, "pending", true)
		req.NoError(err)
		req.Equal(head, ts)

		ts, err = resolver.ResolveEthBlockSelector(ctx, "latest", true)
		req.NoError(err)
		req.Equal(parent, ts)

		ts, err = resolver.ResolveEthBlockSelector(ctx, "earliest", true)
		req.ErrorContains(err, "not supported")
		req.Nil(ts)
	})

	for _, f3Status := range []string{"disabled", "stopped", "nonfinalizing"} {
		t.Run("safe/finalized with F3 "+f3Status, func(t *testing.T) {
			req := require.New(t)

			head := makeTestTipSet(t, 1000, nil)
			safe := makeTestTipSet(t, 969, nil) // head - 31
			final := makeTestTipSet(t, 99, nil) // head - 901
			// setup f3 such that if the checks for disabled/running/non-finalized slip through
			// that it will return a tipset that `"finalized"` would return - we should never see
			// this in these tests
			f3finalzedParent := makeTestTipSet(t, 960, nil)
			f3finalzed := makeTestTipSet(t, 961, f3finalzedParent.Cids())

			loader := &mockTipSetLoader{
				head: head,
				byHeight: map[abi.ChainEpoch]*types.TipSet{
					969: safe,
					99:  final,
				},
				tipsets: map[types.TipSetKey]*types.TipSet{
					f3finalzedParent.Key(): f3finalzedParent,
					f3finalzed.Key():       f3finalzed,
				},
			}

			f3 := &f3mock.MockF3API{}
			// setup f3 as if it's operational, then selectively turn off what we are testing
			f3.SetManifest(manifest.LocalDevnetManifest())
			f3.SetLatestCert(&certs.FinalityCertificate{
				ECChain: gpbft.ECChain{
					gpbft.TipSet{Key: f3finalzed.Key().Bytes()},
				},
			})

			switch f3Status {
			case "disabled":
				f3.SetEnabled(false)
			case "stopped":
				f3.SetLatestCert(nil) // no cert as a proxy for not running
			case "nonfinalizing":
				m := manifest.LocalDevnetManifest()
				m.EC.Finalize = false
				f3.SetManifest(m)
			}

			resolver := tsresolver.NewTipSetResolver(loader, f3)

			ts, err := resolver.ResolveEthBlockSelector(ctx, "safe", true)
			req.NoError(err)
			req.Equal(safe, ts)

			ts, err = resolver.ResolveEthBlockSelector(ctx, "finalized", true)
			req.NoError(err)
			req.Equal(final, ts)
		})
	}

	t.Run("safe/finalized with F3 enabled and close to head", func(t *testing.T) {
		// normal F3 operation, closer to head than default "safe"

		req := require.New(t)

		head := makeTestTipSet(t, 1000, nil)
		finalzedParent := makeTestTipSet(t, 994, nil)
		finalzed := makeTestTipSet(t, 995, finalzedParent.Cids())

		loader := &mockTipSetLoader{
			head: head,
			tipsets: map[types.TipSetKey]*types.TipSet{
				finalzedParent.Key(): finalzedParent,
				finalzed.Key():       finalzed,
			},
		}

		f3 := &f3mock.MockF3API{}
		f3.SetEnabled(true)
		f3.SetRunning(true)
		f3.SetManifest(manifest.LocalDevnetManifest())
		f3.SetLatestCert(&certs.FinalityCertificate{
			ECChain: gpbft.ECChain{
				gpbft.TipSet{Key: finalzed.Key().Bytes()},
			},
		})

		resolver := tsresolver.NewTipSetResolver(loader, f3)

		ts, err := resolver.ResolveEthBlockSelector(ctx, "safe", true)
		req.NoError(err)
		req.Equal(finalzedParent, ts)

		ts, err = resolver.ResolveEthBlockSelector(ctx, "finalized", true)
		req.NoError(err)
		req.Equal(finalzedParent, ts)
	})

	t.Run("safe/finalized with F3 enabled and far from head", func(t *testing.T) {
		// F3 is running, but delayed longer than EC, so expect fall-back to EC behaviour

		req := require.New(t)

		head := makeTestTipSet(t, 1000, nil)
		safe := makeTestTipSet(t, 969, nil)    // head - 31
		final := makeTestTipSet(t, 99, nil)    // head - 901
		finalzed := makeTestTipSet(t, 10, nil) // head - 990

		loader := &mockTipSetLoader{
			head: head,
			tipsets: map[types.TipSetKey]*types.TipSet{
				finalzed.Key(): finalzed,
			},
			byHeight: map[abi.ChainEpoch]*types.TipSet{
				969: safe,
				99:  final,
			},
		}

		f3 := &f3mock.MockF3API{}
		f3.SetEnabled(true)
		f3.SetRunning(true)
		f3.SetManifest(manifest.LocalDevnetManifest())
		f3.SetLatestCert(&certs.FinalityCertificate{
			ECChain: gpbft.ECChain{
				gpbft.TipSet{Key: finalzed.Key().Bytes()},
			},
		})

		resolver := tsresolver.NewTipSetResolver(loader, f3)

		ts, err := resolver.ResolveEthBlockSelector(ctx, "safe", true)
		req.NoError(err)
		req.Equal(safe, ts)

		ts, err = resolver.ResolveEthBlockSelector(ctx, "finalized", true)
		req.NoError(err)
		req.Equal(final, ts)
	})

	t.Run("block number resolution", func(t *testing.T) {
		req := require.New(t)

		head := makeTestTipSet(t, 100, nil)
		target := makeTestTipSet(t, 42, nil)

		loader := &mockTipSetLoader{
			head: head,
			byHeight: map[abi.ChainEpoch]*types.TipSet{
				42: target,
			},
		}

		resolver := tsresolver.NewTipSetResolver(loader, &f3mock.MockF3API{})

		ts, err := resolver.ResolveEthBlockSelector(ctx, "42", true)
		req.NoError(err)
		req.Equal(target, ts)

		ts, err = resolver.ResolveEthBlockSelector(ctx, "0x2a", true)
		req.NoError(err)
		req.Equal(target, ts)
	})
}

func makeTestTipSet(t *testing.T, height int64, parents []cid.Cid) *types.TipSet {
	if parents == nil {
		parents = []cid.Cid{dummyCid, dummyCid}
	}
	ts, err := types.NewTipSet([]*types.BlockHeader{{
		Miner:                 builtin.SystemActorAddr,
		Height:                abi.ChainEpoch(height),
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		Parents:               parents,
	}})
	require.NoError(t, err)
	return ts
}

type mockTipSetLoader struct {
	head     *types.TipSet
	tipsets  map[types.TipSetKey]*types.TipSet
	byHeight map[abi.ChainEpoch]*types.TipSet
}

func (m *mockTipSetLoader) GetHeaviestTipSet() *types.TipSet { return m.head }
func (m *mockTipSetLoader) LoadTipSet(_ context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	return m.tipsets[tsk], nil
}
func (m *mockTipSetLoader) GetTipsetByHeight(_ context.Context, h abi.ChainEpoch, _ *types.TipSet, _ bool) (*types.TipSet, error) {
	return m.byHeight[h], nil
}
