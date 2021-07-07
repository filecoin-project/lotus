package kit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

func SetControlAddresses(t *testing.T, client *TestFullNode, w *TestMiner, addrs ...addr.Address) {
	ctx := context.TODO()

	mi, err := client.StateMinerInfo(ctx, w.ActorAddr, types.EmptyTSK)
	require.NoError(t, err)

	cwp := &miner2.ChangeWorkerAddressParams{
		NewWorker:       mi.Worker,
		NewControlAddrs: addrs,
	}

	sp, err := actors.SerializeParams(cwp)
	require.NoError(t, err)

	smsg, err := client.MpoolPushMessage(ctx, &types.Message{
		From:   mi.Owner,
		To:     w.ActorAddr,
		Method: miner.Methods.ChangeWorkerAddress,

		Value:  big.Zero(),
		Params: sp,
	}, nil)
	require.NoError(t, err)

	WaitMsg(ctx, t, client, smsg.Cid())
}
