package wallet

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestWallet(t *testing.T) {

	ctx := context.Background()

	w1, err := NewWallet(NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	a1, err := w1.WalletNew(ctx, types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	exists, err := w1.WalletHas(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatalf("address doesn't exist in wallet")
	}

	w2, err := NewWallet(NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	a2, err := w2.WalletNew(ctx, types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	a3, err := w2.WalletNew(ctx, types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	addrs, err := w2.WalletList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(addrs) != 2 {
		t.Fatalf("wrong number of addresses in wallet")
	}

	err = w2.WalletDelete(ctx, a2)
	if err != nil {
		t.Fatal(err)
	}

	exists, err = w2.WalletHas(ctx, a2)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("failed to delete wallet address")
	}

	err = w2.SetDefault(a3)
	if err != nil {
		t.Fatal(err)
	}

	def, err := w2.GetDefault()
	if !assert.Equal(t, a3, def) {
		t.Fatal(err)
	}

	keyInfo, err := w2.WalletExport(ctx, a3)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := w2.WalletImport(ctx, keyInfo)
	if err != nil {
		t.Fatal(err)
	}

	if addr != a3 {
		t.Fatalf("imported address doesn't match exported address")
	}

}
