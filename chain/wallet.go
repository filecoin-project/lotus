package chain

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/filecoin-project/go-bls-sigs"
	"github.com/minio/blake2b-simd"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/crypto"
)

const (
	KNamePrefix = "wallet-"

	KTSecp256k1 = "secp256k1"
	KTBLS       = "bls"
)

type Wallet struct {
	keys     map[address.Address]*Key
	keystore types.KeyStore
}

func NewWallet(keystore types.KeyStore) (*Wallet, error) {
	w := &Wallet{
		keys:     make(map[address.Address]*Key),
		keystore: keystore,
	}

	return w, nil
}

type Signature struct {
	Type string
	Data []byte
}

func SignatureFromBytes(x []byte) (Signature, error) {
	val, nr := binary.Uvarint(x)
	if nr != 1 {
		return Signature{}, fmt.Errorf("signatures with type field longer than one byte are invalid")
	}
	var ts string
	switch val {
	case 1:
		ts = KTSecp256k1
	default:
		return Signature{}, fmt.Errorf("unsupported signature type: %d", val)
	}

	return Signature{
		Type: ts,
		Data: x[1:],
	}, nil
}

func (s *Signature) Verify(addr address.Address, msg []byte) error {
	b2sum := blake2b.Sum256(msg)

	switch s.Type {
	case KTSecp256k1:
		pubk, err := crypto.EcRecover(b2sum[:], s.Data)
		if err != nil {
			return err
		}

		maybeaddr, err := address.NewSecp256k1Address(pubk)
		if err != nil {
			return err
		}

		if addr != maybeaddr {
			return fmt.Errorf("signature did not match")
		}

		return nil
	default:
		return fmt.Errorf("cannot verify signature of unsupported type: %s", s.Type)
	}
}

func (s *Signature) TypeCode() int {
	switch s.Type {
	case KTSecp256k1:
		return 1
	case KTBLS:
		return 2
	default:
		panic("unsupported signature type")
	}
}

func (w *Wallet) Sign(addr address.Address, msg []byte) (*Signature, error) {
	ki, err := w.findKey(addr)
	if err != nil {
		return nil, err
	}

	switch ki.Type {
	case KTSecp256k1:
		b2sum := blake2b.Sum256(msg)
		sig, err := crypto.Sign(ki.PrivateKey, b2sum[:])
		if err != nil {
			return nil, err
		}

		return &Signature{
			Type: KTSecp256k1,
			Data: sig,
		}, nil
	case KTBLS:
		var pk bls.PrivateKey
		copy(pk[:], ki.PrivateKey)
		sig := bls.PrivateKeySign(pk, msg)

		return &Signature{
			Type: KTBLS,
			Data: sig[:],
		}, nil

	default:
		panic("cant do it sir")
	}
}

func (w *Wallet) findKey(addr address.Address) (*Key, error) {
	k, ok := w.keys[addr]
	if ok {
		return k, nil
	}
	ki, err := w.keystore.Get(KNamePrefix + addr.String())
	if err != nil {
		return nil, xerrors.Errorf("getting from keystore: %w", err)
	}
	k, err = NewKey(ki)
	if err != nil {
		return nil, xerrors.Errorf("decoding from keystore: %w", err)
	}
	w.keys[k.Address] = k
	return k, nil
}

func (w *Wallet) Export(addr address.Address) ([]byte, error) {
	panic("nyi")
}

func (w *Wallet) Import(kdata []byte) (address.Address, error) {
	panic("nyi")
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

func (w *Wallet) GenerateKey(typ string) (address.Address, error) {
	var k *Key
	switch typ {
	case KTSecp256k1:
		priv, err := crypto.GenerateKey()
		if err != nil {
			return address.Undef, err
		}
		ki := types.KeyInfo{
			Type:       typ,
			PrivateKey: priv,
		}

		k, err = NewKey(ki)
		if err != nil {
			return address.Undef, err
		}
	case KTBLS:
		priv := bls.PrivateKeyGenerate()
		ki := types.KeyInfo{
			Type:       typ,
			PrivateKey: priv[:],
		}

		var err error
		k, err = NewKey(ki)
		if err != nil {
			return address.Undef, err
		}
	default:
		return address.Undef, xerrors.Errorf("invalid key type: %s", typ)
	}

	err := w.keystore.Put(KNamePrefix+k.Address.String(), k.KeyInfo)
	if err != nil {
		return address.Undef, xerrors.Errorf("saving to keystore: %w", err)
	}
	w.keys[k.Address] = k
	return k.Address, nil
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
	case KTSecp256k1:
		k.PublicKey = crypto.PublicKey(k.PrivateKey)

		var err error
		k.Address, err = address.NewSecp256k1Address(k.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("converting Secp256k1 to address: %w", err)
		}

	case KTBLS:
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
