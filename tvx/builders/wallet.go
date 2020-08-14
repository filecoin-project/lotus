package builders

import (
	"fmt"
	"math/rand"

	"github.com/minio/blake2b-simd"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-crypto"
	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/wallet"

	"github.com/filecoin-project/lotus/chain/types"
)

type Wallet struct {
	// Private keys by address
	keys map[address.Address]*wallet.Key

	// Seed for deterministic secp key generation.
	secpSeed int64
	// Seed for deterministic bls key generation.
	blsSeed int64 // nolint: structcheck
}

func newWallet() *Wallet {
	return &Wallet{
		keys:     make(map[address.Address]*wallet.Key),
		secpSeed: 0,
	}
}

func (w *Wallet) NewSECP256k1Account() address.Address {
	secpKey := w.newSecp256k1Key()
	w.keys[secpKey.Address] = secpKey
	return secpKey.Address
}

func (w *Wallet) NewBLSAccount() address.Address {
	blsKey := w.newBLSKey()
	w.keys[blsKey.Address] = blsKey
	return blsKey.Address
}

func (w *Wallet) Sign(addr address.Address, data []byte) (acrypto.Signature, error) {
	ki, ok := w.keys[addr]
	if !ok {
		return acrypto.Signature{}, fmt.Errorf("unknown address %v", addr)
	}
	var sigType acrypto.SigType
	if ki.Type == wallet.KTSecp256k1 {
		sigType = acrypto.SigTypeBLS
		hashed := blake2b.Sum256(data)
		sig, err := crypto.Sign(ki.PrivateKey, hashed[:])
		if err != nil {
			return acrypto.Signature{}, err
		}

		return acrypto.Signature{
			Type: sigType,
			Data: sig,
		}, nil
	} else if ki.Type == wallet.KTBLS {
		panic("lotus validator cannot sign BLS messages")
	} else {
		panic("unknown signature type")
	}

}

func (w *Wallet) newSecp256k1Key() *wallet.Key {
	randSrc := rand.New(rand.NewSource(w.secpSeed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		panic(err)
	}
	w.secpSeed++
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       wallet.KTSecp256k1,
		PrivateKey: prv,
	})
	if err != nil {
		panic(err)
	}
	return key
}

func (w *Wallet) newBLSKey() *wallet.Key {
	// FIXME: bls needs deterministic key generation
	//sk := ffi.PrivateKeyGenerate(s.blsSeed)
	// s.blsSeed++
	sk := [32]byte{}
	sk[0] = uint8(w.blsSeed) // hack to keep gas values determinist
	w.blsSeed++
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       wallet.KTBLS,
		PrivateKey: sk[:],
	})
	if err != nil {
		panic(err)
	}
	return key
}
