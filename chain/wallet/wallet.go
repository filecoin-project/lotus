package wallet

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/filecoin-project/go-bls-sigs"
	"github.com/filecoin-project/go-lotus/node/repo"

	"github.com/minio/blake2b-simd"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/crypto"
)

const (
	KNamePrefix = "wallet-"
)

type Wallet struct {
	keys     map[address.Address]*Key
	keystore types.KeyStore

	lk sync.Mutex
}

func NewWallet(keystore types.KeyStore) (*Wallet, error) {
	w := &Wallet{
		keys:     make(map[address.Address]*Key),
		keystore: keystore,
	}

	return w, nil
}

func (w *Wallet) Sign(ctx context.Context, addr address.Address, msg []byte) (*types.Signature, error) {
	ki, err := w.findKey(addr)
	if err != nil {
		return nil, err
	}
	if ki == nil {
		return nil, xerrors.Errorf("signing using key '%s': %w", addr.String(), repo.ErrKeyNotFound)
	}

	switch ki.Type {
	case types.KTSecp256k1:
		b2sum := blake2b.Sum256(msg)
		sig, err := crypto.Sign(ki.PrivateKey, b2sum[:])
		if err != nil {
			return nil, err
		}

		return &types.Signature{
			Type: types.KTSecp256k1,
			Data: sig,
		}, nil
	case types.KTBLS:
		var pk bls.PrivateKey
		copy(pk[:], ki.PrivateKey)
		sig := bls.PrivateKeySign(pk, msg)

		return &types.Signature{
			Type: types.KTBLS,
			Data: sig[:],
		}, nil

	default:
		panic("cant do it sir")
	}
}

func (w *Wallet) findKey(addr address.Address) (*Key, error) {
	w.lk.Lock()
	defer w.lk.Unlock()

	k, ok := w.keys[addr]
	if ok {
		return k, nil
	}
	ki, err := w.keystore.Get(KNamePrefix + addr.String())
	if err != nil {
		if xerrors.Is(err, repo.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, xerrors.Errorf("getting from keystore: %w", err)
	}
	k, err = NewKey(ki)
	if err != nil {
		return nil, xerrors.Errorf("decoding from keystore: %w", err)
	}
	w.keys[k.Address] = k
	return k, nil
}

func (w *Wallet) Export(addr address.Address) (*types.KeyInfo, error) {
	k, err := w.findKey(addr)
	if err != nil {
		return nil, xerrors.Errorf("failed to find key to export: %w", err)
	}

	return &k.KeyInfo, nil
}

func (w *Wallet) Import(ki *types.KeyInfo) (address.Address, error) {
	w.lk.Lock()
	defer w.lk.Unlock()

	k, err := NewKey(*ki)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to make key: %w", err)
	}

	if err := w.keystore.Put(KNamePrefix+k.Address.String(), k.KeyInfo); err != nil {
		return address.Undef, xerrors.Errorf("saving to keystore: %w", err)
	}

	return k.Address, nil
}

func (w *Wallet) ListAddrs() ([]address.Address, error) {
	all, err := w.keystore.List()
	if err != nil {
		return nil, xerrors.Errorf("listing keystore: %w", err)
	}

	sort.Strings(all)

	out := make([]address.Address, 0, len(all))
	for _, a := range all {
		if strings.HasPrefix(a, KNamePrefix) {
			name := strings.TrimPrefix(a, KNamePrefix)
			addr, err := address.NewFromString(name)
			if err != nil {
				return nil, xerrors.Errorf("converting name to address: %w", err)
			}
			out = append(out, addr)
		}
	}

	return out, nil
}

func GenerateKey(typ string) (*Key, error) {
	switch typ {
	case types.KTSecp256k1:
		priv, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		ki := types.KeyInfo{
			Type:       typ,
			PrivateKey: priv,
		}

		return NewKey(ki)
	case types.KTBLS:
		priv := bls.PrivateKeyGenerate()
		ki := types.KeyInfo{
			Type:       typ,
			PrivateKey: priv[:],
		}

		return NewKey(ki)
	default:
		return nil, xerrors.Errorf("invalid key type: %s", typ)
	}
}

func (w *Wallet) GenerateKey(typ string) (address.Address, error) {
	w.lk.Lock()
	defer w.lk.Unlock()

	k, err := GenerateKey(typ)
	if err != nil {
		return address.Undef, err
	}

	if err := w.keystore.Put(KNamePrefix+k.Address.String(), k.KeyInfo); err != nil {
		return address.Undef, xerrors.Errorf("saving to keystore: %w", err)
	}
	w.keys[k.Address] = k
	return k.Address, nil
}

func (w *Wallet) HasKey(addr address.Address) (bool, error) {
	k, err := w.findKey(addr)
	if err != nil {
		return false, err
	}
	return k != nil, nil
}

type Key struct {
	types.KeyInfo

	PublicKey []byte
	Address   address.Address
}

func NewKey(keyinfo types.KeyInfo) (*Key, error) {
	k := &Key{
		KeyInfo: keyinfo,
	}

	switch k.Type {
	case types.KTSecp256k1:
		k.PublicKey = crypto.PublicKey(k.PrivateKey)

		var err error
		k.Address, err = address.NewSecp256k1Address(k.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("converting Secp256k1 to address: %w", err)
		}

	case types.KTBLS:
		var pk bls.PrivateKey
		copy(pk[:], k.PrivateKey)
		pub := bls.PrivateKeyPublicKey(pk)
		k.PublicKey = pub[:]

		var err error
		k.Address, err = address.NewBLSAddress(k.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("converting BLS to address: %w", err)
		}

	default:
		return nil, xerrors.Errorf("unknown key type")
	}
	return k, nil

}
