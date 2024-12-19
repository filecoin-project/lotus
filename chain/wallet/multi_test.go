package wallet

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestMultiWallet(t *testing.T) {

	ctx := context.Background()

	local, err := NewWallet(NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	var wallet api.Wallet = MultiWallet{
		Local: local,
	}

	a1, err := wallet.WalletNew(ctx, types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	exists, err := wallet.WalletHas(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatalf("address doesn't exist in wallet")
	}

	addrs, err := wallet.WalletList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// one default address and one newly created
	if len(addrs) == 2 {
		t.Fatalf("wrong number of addresses in wallet")
	}

	keyInfo, err := wallet.WalletExport(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}

	addr, err := wallet.WalletImport(ctx, keyInfo)
	if err != nil {
		t.Fatal(err)
	}

	if addr != a1 {
		t.Fatalf("imported address doesn't match exported address")
	}

	err = wallet.WalletDelete(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}
}
