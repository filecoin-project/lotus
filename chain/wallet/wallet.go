package wallet

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	bls "github.com/filecoin-project/filecoin-ffi"

	logging "github.com/ipfs/go-log/v2"
	"github.com/minio/blake2b-simd"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-crypto"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("wallet")

const (
	KNamePrefix = "wallet-"
	KDefault    = "default"
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

func KeyWallet(keys ...*Key) *Wallet {
	m := make(map[address.Address]*Key)
	for _, key := range keys {
		m[key.Address] = key
	}

	return &Wallet{
		keys: m,
	}
}

func (w *Wallet) Sign(ctx context.Context, addr address.Address, msg []byte) (*types.Signature, error) {
	ki, err := w.findKey(addr)
	if err != nil {
		return nil, err
	}
	if ki == nil {
		return nil, xerrors.Errorf("signing using key '%s': %w", addr.String(), types.ErrKeyInfoNotFound)
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
		return nil, fmt.Errorf("cannot sign with unsupported key type: %q", ki.Type)
	}
}

func (w *Wallet) findKey(addr address.Address) (*Key, error) {
	w.lk.Lock()
	defer w.lk.Unlock()

	k, ok := w.keys[addr]
	if ok {
		return k, nil
	}
	if w.keystore == nil {
		log.Warn("findKey didn't find the key in in-memory wallet")
		return nil, nil
	}

	ki, err := w.keystore.Get(KNamePrefix + addr.String())
	if err != nil {
		if xerrors.Is(err, types.ErrKeyInfoNotFound) {
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

func (w *Wallet) GetDefault() (address.Address, error) {
	w.lk.Lock()
	defer w.lk.Unlock()

	ki, err := w.keystore.Get(KDefault)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get default key: %w", err)
	}

	k, err := NewKey(ki)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to read default key from keystore: %w", err)
	}

	return k.Address, nil
}

func (w *Wallet) SetDefault(a address.Address) error {
	w.lk.Lock()
	defer w.lk.Unlock()

	ki, err := w.keystore.Get(KNamePrefix + a.String())
	if err != nil {
		return err
	}

	if err := w.keystore.Delete(KDefault); err != nil {
		if !xerrors.Is(err, types.ErrKeyInfoNotFound) {
			log.Warnf("failed to unregister current default key: %s", err)
		}
	}

	if err := w.keystore.Put(KDefault, ki); err != nil {
		return err
	}

	return nil
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

	_, err = w.keystore.Get(KDefault)
	if err != nil {
		if !xerrors.Is(err, types.ErrKeyInfoNotFound) {
			return address.Undef, err
		}

		if err := w.keystore.Put(KDefault, k.KeyInfo); err != nil {
			return address.Undef, xerrors.Errorf("failed to set new key as default: %w", err)
		}
	}

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
