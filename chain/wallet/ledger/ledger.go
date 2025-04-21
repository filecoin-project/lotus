package ledgerwallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	ledgerfil "github.com/zondax/ledger-filecoin-go"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("wallet-ledger")

type LedgerWallet struct {
	ds datastore.Datastore
}

func NewWallet(ds dtypes.MetadataDS) *LedgerWallet {
	return &LedgerWallet{ds}
}

type LedgerKeyInfo struct {
	Address address.Address
	Path    []uint32
}

var _ api.Wallet = (*LedgerWallet)(nil)

func (lw LedgerWallet) WalletSign(ctx context.Context, signer address.Address, toSign []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	ki, err := lw.getKeyInfo(ctx, signer)
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

	sig, err := fl.Sign(ki.Path, meta.Extra, ledgerfil.SECP256K1)
	if err != nil {
		return nil, err
	}

	return &crypto.Signature{
		Type: crypto.SigTypeSecp256k1,
		Data: sig.SignatureBytes(),
	}, nil
}

func (lw LedgerWallet) getKeyInfo(ctx context.Context, addr address.Address) (*LedgerKeyInfo, error) {
	kib, err := lw.ds.Get(ctx, keyForAddr(addr))
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
	return lw.ds.Delete(ctx, keyForAddr(k))
}

func (lw LedgerWallet) WalletExport(ctx context.Context, k address.Address) (*types.KeyInfo, error) {
	return nil, fmt.Errorf("cannot export keys from ledger wallets")
}

func (lw LedgerWallet) WalletHas(ctx context.Context, k address.Address) (bool, error) {
	_, err := lw.ds.Get(ctx, keyForAddr(k))
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
	return lw.importKey(ctx, ki)
}

func (lw LedgerWallet) importKey(ctx context.Context, ki LedgerKeyInfo) (address.Address, error) {
	if ki.Address == address.Undef {
		return address.Undef, fmt.Errorf("no address given in imported key info")
	}
	if len(ki.Path) != filHdPathLen {
		return address.Undef, fmt.Errorf("bad hd path len: %d, expected: %d", len(ki.Path), filHdPathLen)
	}
	bb, err := json.Marshal(ki)
	if err != nil {
		return address.Undef, xerrors.Errorf("marshaling key info: %w", err)
	}

	if err := lw.ds.Put(ctx, keyForAddr(ki.Address), bb); err != nil {
		return address.Undef, err
	}

	return ki.Address, nil
}

func (lw LedgerWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	res, err := lw.ds.Query(ctx, query.Query{Prefix: dsLedgerPrefix})
	if err != nil {
		return nil, err
	}
	defer res.Close() // nolint:errcheck

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

const hdHard = 0x80000000

var filHDBasePath = []uint32{hdHard | 44, hdHard | 461, hdHard, 0}
var filHdPathLen = 5

func (lw LedgerWallet) WalletNew(ctx context.Context, t types.KeyType) (address.Address, error) {
	if t != types.KTSecp256k1Ledger {
		return address.Undef, fmt.Errorf("unsupported key type: '%s', only '%s' supported",
			t, types.KTSecp256k1Ledger)
	}

	res, err := lw.ds.Query(ctx, query.Query{Prefix: dsLedgerPrefix})
	if err != nil {
		return address.Undef, err
	}
	defer res.Close() // nolint:errcheck

	var maxi int64 = -1
	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		var ki LedgerKeyInfo
		if err := json.Unmarshal(res.Value, &ki); err != nil {
			return address.Undef, err
		}
		if i := ki.Path[filHdPathLen-1]; maxi == -1 || maxi < int64(i) {
			maxi = int64(i)
		}
	}

	fl, err := ledgerfil.FindLedgerFilecoinApp()
	if err != nil {
		return address.Undef, xerrors.Errorf("finding ledger: %w", err)
	}
	defer fl.Close() // nolint:errcheck

	path := append(append([]uint32(nil), filHDBasePath...), uint32(maxi+1))
	_, _, addr, err := fl.GetAddressPubKey(path, ledgerfil.SECP256K1)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting public key from ledger: %w", err)
	}

	log.Warnf("creating key: %s, accept the key in ledger device", addr)
	_, _, addr, err = fl.ShowAddressPubKey(path, ledgerfil.SECP256K1)
	if err != nil {
		return address.Undef, xerrors.Errorf("verifying public key with ledger: %w", err)
	}

	a, err := address.NewFromString(addr)
	if err != nil {
		return address.Undef, fmt.Errorf("parsing address: %w", err)
	}

	var lki LedgerKeyInfo
	lki.Address = a
	lki.Path = path

	return lw.importKey(ctx, lki)
}

func (lw *LedgerWallet) Get() api.Wallet {
	if lw == nil {
		return nil
	}

	return lw
}

var dsLedgerPrefix = "/ledgerkey/"

func keyForAddr(addr address.Address) datastore.Key {
	return datastore.NewKey(dsLedgerPrefix + addr.String())
}
