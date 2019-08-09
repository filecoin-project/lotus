package actors_test

import (
	"context"
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
	"go.opencensus.io/trace"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/tracing"
)

func TestMultiSigCreate(t *testing.T) {
	var creatorAddr, sig1Addr, sig2Addr, outsideAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&creatorAddr, 10000),
		HarnessAddr(&sig1Addr, 10000),
		HarnessAddr(&sig2Addr, 10000),
		HarnessAddr(&outsideAddr, 10000),
	}

	h := NewHarness2(t, opts...)
	ret, _ := h.CreateActor(t, creatorAddr, actors.MultisigActorCodeCid,
		actors.MultiSigConstructorParams{
			Signers:  []address.Address{creatorAddr, sig1Addr, sig2Addr},
			Required: 2,
		})
	ApplyOK(t, ret)
}

func ApplyOK(t testing.TB, ret *vm.ApplyRet) {
	t.Helper()
	if ret.ExitCode != 0 {
		t.Fatalf("exit code should be 0, got %d, actorErr: %+v", ret.ExitCode, ret.ActorErr)
	}
	if ret.ActorErr != nil {
		t.Fatalf("somehow got an error with exit == 0: %s", ret.ActorErr)
	}
}

func TestMultiSigOps(t *testing.T) {
	je := tracing.SetupJaegerTracing("test")
	defer je.Flush()
	ctx, span := trace.StartSpan(context.Background(), "/test-"+t.Name())
	defer span.End()

	var creatorAddr, sig1Addr, sig2Addr, outsideAddr address.Address
	var multSigAddr address.Address
	opts := []HarnessOpt{
		HarnessCtx(ctx),
		HarnessAddr(&creatorAddr, 10000),
		HarnessAddr(&sig1Addr, 10000),
		HarnessAddr(&sig2Addr, 10000),
		HarnessAddr(&outsideAddr, 1000),
		HarnessActor(&multSigAddr, &creatorAddr, actors.MultisigActorCodeCid,
			func() interface{} {
				return actors.MultiSigConstructorParams{
					Signers:  []address.Address{creatorAddr, sig1Addr, sig2Addr},
					Required: 2,
				}
			}),
	}

	curVal := types.NewInt(2000)
	h := NewHarness2(t, opts...)
	{
		// Send funds into the multisig
		ret, state := h.SendFunds(t, creatorAddr, multSigAddr, curVal)
		ApplyOK(t, ret)
		ms, err := state.GetActor(multSigAddr)
		assert.NoError(t, err)
		assert.Equal(t, curVal, ms.Balance)
	}

	{
		// Transfer funds outside of multsig
		sendVal := types.NewInt(100)
		ret, _ := h.Invoke(t, creatorAddr, multSigAddr, actors.MultiSigMethods.Propose,
			actors.MultiSigProposeParams{
				To:    outsideAddr,
				Value: sendVal,
			})
		ApplyOK(t, ret)
		var txIDParam actors.MultiSigTxID
		err := cbor.DecodeInto(ret.Return, &txIDParam.TxID)
		assert.NoError(t, err, "decoding txid")

		ret, _ = h.Invoke(t, outsideAddr, multSigAddr, actors.MultiSigMethods.Approve,
			txIDParam)
		assert.Equal(t, uint8(1), ret.ExitCode, "outsideAddr should not approve")

		ret, state := h.Invoke(t, sig1Addr, multSigAddr, actors.MultiSigMethods.Approve,
			txIDParam)
		outAct, err := state.GetActor(outsideAddr)
		ApplyOK(t, ret)
		curVal = types.BigSub(curVal, sendVal)

		assert.NoError(t, err)
		assert.Equal(t, types.NewInt(1099), outAct.Balance)

		msAct, err := state.GetActor(multSigAddr)
		assert.NoError(t, err)
		assert.Equal(t, curVal, msAct.Balance)
	}

}
