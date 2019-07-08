package chain

import (
	"encoding/binary"
	"fmt"
	"github.com/filecoin-project/go-lotus/chain/address"

	"github.com/minio/blake2b-simd"

	"github.com/filecoin-project/go-lotus/lib/crypto"
	"github.com/filecoin-project/go-lotus/lib/bls-signatures"
)

const (
	KTSecp256k1 = "secp256k1"
	KTBLS       = "bls"
)

type Wallet struct {
	keys map[address.Address]*KeyInfo
}

func NewWallet() *Wallet {
	return &Wallet{keys: make(map[address.Address]*KeyInfo)}
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

func (w *Wallet) findKey(addr address.Address) (*KeyInfo, error) {
	ki, ok := w.keys[addr]
	if !ok {
		return nil, fmt.Errorf("key not for given address not found in wallet")
	}
	return ki, nil
}

func (w *Wallet) Export(addr address.Address) ([]byte, error) {
	panic("nyi")
}

func (w *Wallet) Import(kdata []byte) (address.Address, error) {
	panic("nyi")
}

func (w *Wallet) GenerateKey(typ string) (address.Address, error) {
	switch typ {
	case KTSecp256k1:
		k, err := crypto.GenerateKey()
		if err != nil {
			return address.Undef, err
		}
		ki := &KeyInfo{
			PrivateKey: k,
			Type:       typ,
		}

		addr := ki.Address()
		w.keys[addr] = ki
		return addr, nil
	case KTBLS:
		priv := bls.PrivateKeyGenerate()

		ki := &KeyInfo{
			PrivateKey: priv[:],
			Type:       KTBLS,
		}

		addr := ki.Address()
		w.keys[addr] = ki
		return addr, nil
	default:
		return address.Undef, fmt.Errorf("invalid key type: %s", typ)
	}
}

type KeyInfo struct {
	PrivateKey []byte

	Type string
}

func (ki *KeyInfo) Address() address.Address {
	switch ki.Type {
	case KTSecp256k1:
		pub := crypto.PublicKey(ki.PrivateKey)
		addr, err := address.NewSecp256k1Address(pub)
		if err != nil {
			panic(err)
		}

		return addr
	case KTBLS:
		var pk bls.PrivateKey
		copy(pk[:], ki.PrivateKey)
		pub := bls.PrivateKeyPublicKey(pk)
		a, err := address.NewBLSAddress(pub[:])
		if err != nil {
			panic(err)
		}
		return a
	default:
		panic("unsupported key type")
	}
}
