package full

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"testing"
)

func init() {
	miner.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	power.ConsensusMinerMinPower = big.NewInt(2048)
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}

func TestStateVerifiedClientStatus(t *testing.T) {
	ctx := context.TODO()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	sm := stmgr.NewStateManager(cg.ChainStore())
	store := cbornode.NewCborStore(sm.ChainStore().Blockstore())

	sapi := &StateAPI{
		In:            fx.In{},
		Wallet:        nil,
		ProofVerifier: nil,
		StateManager:  sm,
		Chain:         cg.ChainStore(),
		Beacon:        nil,
	}

	// Request for non-existent address should return nil data cap
	addr, err := address.NewFromString("t00")
	require.NoError(t, err)
	dcap, err := sapi.StateVerifiedClientStatus(ctx, addr, types.EmptyTSK)
	require.NoError(t, err)
	require.Nil(t, dcap)

	// Get the verified registry actor state
	act, err := sapi.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	var st verifreg.State
	err = store.Get(ctx, act.Head, &st)
	require.NoError(t, err)

	// Add an address with a data cap
	st.PutVerifiedClient(ctxStore{store}, addr, big.NewInt(10))

	// Store verified registry actor state
	stateC, err := store.Put(ctx, &st)
	require.NoError(t, err)

	// Get the state tree
	stree, err := getStateTree(ctx, sm)
	require.NoError(t, err)

	// Update the verified registry actor with the new state
	stree.MutateActor(builtin.VerifiedRegistryActorAddr, func(actor *types.Actor) error {
		actor.Head = stateC
		return nil
	})

	// Flush the state
	root, err := stree.Flush(ctx)
	require.NoError(t, err)

	// Replace the head block's ParentStateRoot with the new state and add it
	// to the chain
	ts := sapi.Chain.GetHeaviestTipSet()
	blks := ts.Blocks()
	blks[0].ParentStateRoot = root
	ts, err = types.NewTipSet(blks)
	require.NoError(t, err)
	err = sapi.Chain.AddBlock(ctx, blks[0])
	require.NoError(t, err)

	// Request for address should give back the expected data cap
	dcap, err = sapi.StateVerifiedClientStatus(ctx, addr, ts.Key())
	require.NoError(t, err)
	require.NotNil(t, dcap)
	require.Equal(t, (*dcap).Int64(), 10)
}

func getStateTree(ctx context.Context, sm *stmgr.StateManager) (*state.StateTree, error) {
	ts := sm.ChainStore().GetHeaviestTipSet()

	st, _, err := sm.TipSetState(ctx, ts)
	if err != nil {
		return nil, err
	}

	cst := cbornode.NewCborStore(sm.ChainStore().Blockstore())
	return state.LoadStateTree(cst, st)
}

type ctxStore struct {
	*cbornode.BasicIpldStore
}

func (ctxStore) Context() context.Context {
	return context.TODO()
}
