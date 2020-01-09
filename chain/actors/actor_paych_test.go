package actors_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

func TestPaychCreate(t *testing.T) {
	var creatorAddr, targetAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&creatorAddr, 100000),
		HarnessAddr(&targetAddr, 100000),
	}

	h := NewHarness(t, opts...)
	ret, _ := h.CreateActor(t, creatorAddr, actors.PaymentChannelCodeCid,
		&actors.PCAConstructorParams{
			To: targetAddr,
		})
	ApplyOK(t, ret)
}

func signVoucher(t *testing.T, w *wallet.Wallet, addr address.Address, sv *types.SignedVoucher) {
	vb, err := sv.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}

	sig, err := w.Sign(context.TODO(), addr, vb)
	if err != nil {
		t.Fatal(err)
	}

	sv.Signature = sig
}

func TestPaychUpdate(t *testing.T) {
	var creatorAddr, targetAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&creatorAddr, 100000),
		HarnessAddr(&targetAddr, 100000),
	}

	h := NewHarness(t, opts...)
	ret, _ := h.CreateActor(t, creatorAddr, actors.PaymentChannelCodeCid,
		&actors.PCAConstructorParams{
			To: targetAddr,
		})
	ApplyOK(t, ret)
	pch, err := address.NewFromBytes(ret.Return)
	if err != nil {
		t.Fatal(err)
	}

	ret, _ = h.SendFunds(t, creatorAddr, pch, types.NewInt(5000))
	ApplyOK(t, ret)

	sv := &types.SignedVoucher{
		Amount: types.NewInt(100),
		Nonce:  1,
	}
	signVoucher(t, h.w, creatorAddr, sv)

	ret, _ = h.Invoke(t, targetAddr, pch, actors.PCAMethods.UpdateChannelState, &actors.PCAUpdateChannelStateParams{
		Sv: *sv,
	})
	ApplyOK(t, ret)

	ret, _ = h.Invoke(t, targetAddr, pch, actors.PCAMethods.GetToSend, nil)
	ApplyOK(t, ret)

	bi := types.BigFromBytes(ret.Return)
	if bi.String() != "100" {
		t.Fatal("toSend amount was wrong: ", bi.String())
	}

	ret, _ = h.Invoke(t, targetAddr, pch, actors.PCAMethods.Close, nil)
	ApplyOK(t, ret)

	// now we have to 'wait' for the chain to advance.
	h.BlockHeight = 1000

	ret, _ = h.Invoke(t, targetAddr, pch, actors.PCAMethods.Collect, nil)
	ApplyOK(t, ret)

	h.AssertBalanceChange(t, targetAddr, 100)
	h.AssertBalanceChange(t, creatorAddr, -100)
}
