package stmgr_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

// A call is traced; preparing its state is not, and only builds a VM when there is
// something to prepare.
func TestCallVMs(t *testing.T) {
	ctx := context.Background()

	cg, err := gen.NewGenerator()
	require.NoError(t, err)
	mts, err := cg.NextTipSet()
	require.NoError(t, err)
	head := mts.TipSet.TipSet()

	tsMsgs, err := cg.ChainStore().MessagesForTipset(ctx, head)
	require.NoError(t, err)
	require.NotEmpty(t, tsMsgs)

	sm := cg.StateManager()
	headState, _, err := sm.TipSetState(ctx, head)
	require.NoError(t, err)

	var tracing []bool
	sm.SetVMConstructor(func(ctx context.Context, opts *vm.VMOpts) (vm.Interface, error) {
		tracing = append(tracing, opts.Tracing)
		return vm.NewVM(ctx, opts)
	})

	msg := &types.Message{
		From:       cg.Banker(),
		To:         cg.Banker(),
		Value:      big.Zero(),
		GasLimit:   buildconstants.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}

	t.Run("replaying tipset messages", func(t *testing.T) {
		tracing = nil
		ret, err := sm.CallWithGas(ctx, msg, nil, head, true)
		require.NoError(t, err)
		require.True(t, ret.MsgRct.ExitCode.IsSuccess())
		require.Equal(t, msg.To, ret.ExecutionTrace.Msg.To)
		require.Equal(t, []bool{false, true}, tracing)
	})

	t.Run("on a computed state", func(t *testing.T) {
		tracing = nil
		ret, err := sm.ApplyOnStateWithGas(ctx, headState, msg, head)
		require.NoError(t, err)
		require.True(t, ret.MsgRct.ExitCode.IsSuccess())
		require.Equal(t, msg.To, ret.ExecutionTrace.Msg.To)
		require.Equal(t, []bool{true}, tracing)
	})
}
