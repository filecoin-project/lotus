package ledgerwallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	ledgerfil "github.com/whyrusleeping/ledger-filecoin-go"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type LedgerWallet struct {
	ds datastore.Datastore
}

func NewWallet(ds datastore.Datastore) *LedgerWallet {
	return &LedgerWallet{ds}
}

type LedgerKeyInfo struct {
	Address address.Address
	Path    []uint32
}

var _ api.WalletAPI = (*LedgerWallet)(nil)

func (lw LedgerWallet) WalletSign(ctx context.Context, signer address.Address, toSign []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	ki, err := lw.getKeyInfo(signer)
	if err != nil {
		return nil, err
	}

	fl, err := ledgerfil.FindLedgerFilecoinApp()
	if err != nil {
		return nil, err
	}
	defer fl.Close() // nolint:errcheck
	if meta.Type != api.MTChainMsg {
		return nil, fmt.Errorf("ledger can only sign chain messages")
	}

	{
		var cmsg types.Message
		if err := cmsg.UnmarshalCBOR(bytes.NewReader(meta.Extra)); err != nil {
			return nil, xerrors.Errorf("unmarshalling message: %w", err)
		}

		_, bc, err := cid.CidFromBytes(toSign)
		if err != nil {
			return nil, xerrors.Errorf("getting cid from signing bytes: %w", err)
		}

		if !cmsg.Cid().Equals(bc) {
			return nil, xerrors.Errorf("cid(meta.Extra).bytes() != toSign")
		}
	}

	sig, err := fl.SignSECP256K1(ki.Path, meta.Extra)
	if err != nil {
		return nil, err
	}

	return &crypto.Signature{
		Type: crypto.SigTypeSecp256k1,
		Data: sig.SignatureBytes(),
	}, nil
}

func (lw LedgerWallet) getKeyInfo(addr address.Address) (*LedgerKeyInfo, error) {
	kib, err := lw.ds.Get(keyForAddr(addr))
	if err != nil {
		return nil, err
	}

	var out LedgerKeyInfo
	if err := json.Unmarshal(kib, &out); err != nil {
		return nil, xerrors.Errorf("unmarshalling ledger key info: %w", err)
	}

	return &out, nil
}

func (lw LedgerWallet) WalletDelete(ctx context.Context, k address.Address) error {
	return lw.ds.Delete(keyForAddr(k))
}

func (lw LedgerWallet) WalletExport(ctx context.Context, k address.Address) (*types.KeyInfo, error) {
	return nil, fmt.Errorf("cannot export keys from ledger wallets")
}

func (lw LedgerWallet) WalletHas(ctx context.Context, k address.Address) (bool, error) {
	_, err := lw.ds.Get(keyForAddr(k))
	if err == nil {
		return true, nil
	}
	if err == datastore.ErrNotFound {
		return false, nil
	}
	return false, err
}

func (lw LedgerWallet) WalletImport(ctx context.Context, kinfo *types.KeyInfo) (address.Address, error) {
	var ki LedgerKeyInfo
	if err := json.Unmarshal(kinfo.PrivateKey, &ki); err != nil {
		return address.Undef, err
	}

	if ki.Address == address.Undef {
		return address.Undef, fmt.Errorf("no address given in imported key info")
	}

	if err := lw.ds.Put(keyForAddr(ki.Address), kinfo.PrivateKey); err != nil {
		return address.Undef, err
	}

	return ki.Address, nil
}

func (lw LedgerWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	res, err := lw.ds.Query(query.Query{Prefix: "ledgerkey/"})
	if err != nil {
		return nil, err
	}

	var out []address.Address
	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		var ki LedgerKeyInfo
		if err := json.Unmarshal(res.Value, &ki); err != nil {
			return nil, err
		}

		out = append(out, ki.Address)
	}
	return out, nil
}

func (lw LedgerWallet) WalletNew(ctx context.Context, t crypto.SigType) (address.Address, error) {
	return address.Undef, fmt.Errorf("cannot create new address on ledger")
}

func keyForAddr(addr address.Address) datastore.Key {
	return datastore.NewKey("ledgerkey/" + addr.String())
}
