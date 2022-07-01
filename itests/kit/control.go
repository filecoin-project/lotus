package kit

import (
	"context"

	"github.com/stretchr/testify/require"

	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (tm *TestMiner) SetControlAddresses(addrs ...addr.Address) {
	ctx := context.TODO()

	mi, err := tm.FullNode.StateMinerInfo(ctx, tm.ActorAddr, types.EmptyTSK)
	require.NoError(tm.t, err)

	cwp := &miner2.ChangeWorkerAddressParams{
		NewWorker:       mi.Worker,
		NewControlAddrs: addrs,
	}

	sp, err := actors.SerializeParams(cwp)
	require.NoError(tm.t, err)

	smsg, err := tm.FullNode.MpoolPushMessage(ctx, &types.Message{
		From:   mi.Owner,
		To:     tm.ActorAddr,
		Method: builtin.MethodsMiner.ChangeWorkerAddress,

		Value:  big.Zero(),
		Params: sp,
	}, nil)
	require.NoError(tm.t, err)

	tm.FullNode.WaitMsg(ctx, smsg.Cid())
}
