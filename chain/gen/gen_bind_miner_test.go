package gen_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/genesis"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/gen"
	. "github.com/filecoin-project/lotus/chain/stmgr"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/specs-actors/actors/abi"

	logging "github.com/ipfs/go-log"
)

func TestBindMiners(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	ctx := context.TODO()

	bminer1, err := address.NewIDAddress(2021)
	if err != nil {
		t.Fatal(err)
	}
	bminer2, err := address.NewIDAddress(2022)
	assert.NoError(t, err)

	cg, err := gen.NewGeneratorWithBindMiners([]genesis.BindMiner{
		{Address: bminer1, SealProof: abi.RegisteredSealProof_StackedDrg2KiBV1},
		{Address: bminer2, SealProof: abi.RegisteredSealProof_StackedDrg2KiBV1},
	})

	assert.NoError(t, err)

	sm := NewStateManager(cg.ChainStore())

	cg.SetStateManager(sm)

	ts := sm.ChainStore().GetHeaviestTipSet()
	_, err = StateMinerInfo(ctx, sm, ts, bminer1)
	assert.NoError(t, err)
	_, err = StateMinerInfo(ctx, sm, ts, bminer2)
	assert.NoError(t, err)

}
