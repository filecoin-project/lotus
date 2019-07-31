package actors_test

import (
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
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
}

func TestMultiSigOps(t *testing.T) {
	var creatorAddr, sig1Addr, sig2Addr, outsideAddr address.Address
	var multSigAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&creatorAddr, 10000),
		HarnessAddr(&sig1Addr, 10000),
		HarnessAddr(&sig2Addr, 10000),
		HarnessAddr(&outsideAddr, 0),
		HarnessActor(&multSigAddr, &creatorAddr, actors.MultisigActorCodeCid,
			func() interface{} {
				return actors.MultiSigConstructorParams{
					Signers:  []address.Address{creatorAddr, sig1Addr, sig2Addr},
					Required: 2,
				}
			}),
	}

	h := NewHarness2(t, opts...)
	{
		sendVal := types.NewInt(2000)
		ret, state := h.SendFunds(t, creatorAddr, multSigAddr, sendVal)
		ApplyOK(t, ret)
		ms, err := state.GetActor(multSigAddr)
		assert.NoError(t, err)
		assert.Equal(t, sendVal, ms.Balance)
	}

	{
		sendVal := types.NewInt(100)
		ret, _ := h.Invoke(t, creatorAddr, multSigAddr, actors.MultiSigMethods.Propose,
			actors.MultiSigProposeParams{
				To:    outsideAddr,
				Value: sendVal,
			})
		ApplyOK(t, ret)
		var txIdParam actors.MultiSigTxID
		err := cbor.DecodeInto(ret.Return, &txIdParam.TxID)
		assert.NoError(t, err, "decoding txid")
	}

}
