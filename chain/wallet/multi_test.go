// stm: #unit
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

	//stm: @TOKEN_WALLET_MULTI_NEW_ADDRESS_001
	a1, err := wallet.WalletNew(ctx, types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	//stm: @TOKEN_WALLET_MULTI_HAS_001
	exists, err := wallet.WalletHas(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}

	if !exists {
		t.Fatalf("address doesn't exist in wallet")
	}

	//stm: @TOKEN_WALLET_MULTI_LIST_001
	addrs, err := wallet.WalletList(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// one default address and one newly created
	if len(addrs) == 2 {
		t.Fatalf("wrong number of addresses in wallet")
	}

	//stm: @TOKEN_WALLET_MULTI_EXPORT_001
	keyInfo, err := wallet.WalletExport(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}

	//stm: @TOKEN_WALLET_MULTI_IMPORT_001
	addr, err := wallet.WalletImport(ctx, keyInfo)
	if err != nil {
		t.Fatal(err)
	}

	if addr != a1 {
		t.Fatalf("imported address doesn't match exported address")
	}

	//stm: @TOKEN_WALLET_DELETE_001
	err = wallet.WalletDelete(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}
}
