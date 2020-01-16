package actors_test

import (
	"testing"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func TestMultiSigCreate(t *testing.T) {
	var creatorAddr, sig1Addr, sig2Addr, outsideAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&creatorAddr, 100000),
		HarnessAddr(&sig1Addr, 100000),
		HarnessAddr(&sig2Addr, 100000),
		HarnessAddr(&outsideAddr, 100000),
	}

	h := NewHarness(t, opts...)
	ret, _ := h.CreateActor(t, creatorAddr, actors.MultisigCodeCid,
		&actors.MultiSigConstructorParams{
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
	var creatorAddr, sig1Addr, sig2Addr, outsideAddr address.Address
	var multSigAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&creatorAddr, 100000),
		HarnessAddr(&sig1Addr, 100000),
		HarnessAddr(&sig2Addr, 100000),
		HarnessAddr(&outsideAddr, 100000),
		HarnessActor(&multSigAddr, &creatorAddr, actors.MultisigCodeCid,
			func() cbg.CBORMarshaler {
				return &actors.MultiSigConstructorParams{
					Signers:  []address.Address{creatorAddr, sig1Addr, sig2Addr},
					Required: 2,
				}
			}),
	}

	h := NewHarness(t, opts...)
	{
		const chargeVal = 2000
		// Send funds into the multisig
		ret, _ := h.SendFunds(t, creatorAddr, multSigAddr, types.NewInt(chargeVal))
		ApplyOK(t, ret)
		h.AssertBalanceChange(t, creatorAddr, -chargeVal)
		h.AssertBalanceChange(t, multSigAddr, chargeVal)
	}

	{
		// Transfer funds outside of multsig
		const sendVal = 1000
		ret, _ := h.Invoke(t, creatorAddr, multSigAddr, actors.MultiSigMethods.Propose,
			&actors.MultiSigProposeParams{
				To:    outsideAddr,
				Value: types.NewInt(sendVal),
			})
		ApplyOK(t, ret)
		var txIDParam actors.MultiSigTxID
		err := cbor.DecodeInto(ret.Return, &txIDParam.TxID)
		assert.NoError(t, err, "decoding txid")

		ret, _ = h.Invoke(t, outsideAddr, multSigAddr, actors.MultiSigMethods.Approve,
			&txIDParam)
		assert.Equal(t, uint8(1), ret.ExitCode, "outsideAddr should not approve")
		h.AssertBalanceChange(t, multSigAddr, 0)

		ret2, _ := h.Invoke(t, sig1Addr, multSigAddr, actors.MultiSigMethods.Approve,
			&txIDParam)
		ApplyOK(t, ret2)

		h.AssertBalanceChange(t, outsideAddr, sendVal)
		h.AssertBalanceChange(t, multSigAddr, -sendVal)
	}

}
