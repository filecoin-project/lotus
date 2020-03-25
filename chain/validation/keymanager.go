package validation

import (
	"fmt"
	"math/rand"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-crypto"
	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

type KeyManager struct {
	// Private keys by address
	keys map[address.Address]*wallet.Key

	// Seed for deterministic secp key generation.
	secpSeed int64
	// Seed for deterministic bls key generation.
	blsSeed int64 // nolint: structcheck
}

func newKeyManager() *KeyManager {
	return &KeyManager{
		keys:     make(map[address.Address]*wallet.Key),
		secpSeed: 0,
	}
}

func (k *KeyManager) NewSECP256k1AccountAddress() address.Address {
	secpKey := k.newSecp256k1Key()
	k.keys[secpKey.Address] = secpKey
	return secpKey.Address
}

func (k *KeyManager) NewBLSAccountAddress() address.Address {
	blsKey := k.newBLSKey()
	k.keys[blsKey.Address] = blsKey
	return blsKey.Address
}

func (k *KeyManager) Sign(addr address.Address, data []byte) (acrypto.Signature, error) {
	ki, ok := k.keys[addr]
	if !ok {
		return acrypto.Signature{}, fmt.Errorf("unknown address %v", addr)
	}
	sig, err := crypto.Sign(ki.PrivateKey, data)
	if err != nil {
		return acrypto.Signature{}, err
	}
	var sigType acrypto.SigType
	if ki.Type == wallet.KTBLS {
		sigType = acrypto.SigTypeBLS
	} else if ki.Type == wallet.KTSecp256k1 {
		sigType = acrypto.SigTypeSecp256k1
	} else {
		panic("unknown signature type")
	}
	return acrypto.Signature{
		Type: sigType,
		Data: sig,
	}, nil

}

func (k *KeyManager) newSecp256k1Key() *wallet.Key {
	randSrc := rand.New(rand.NewSource(k.secpSeed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		panic(err)
	}
	k.secpSeed++
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       wallet.KTSecp256k1,
		PrivateKey: prv,
	})
	if err != nil {
		panic(err)
	}
	return key
}

func (k *KeyManager) newBLSKey() *wallet.Key {
	// FIXME: bls needs deterministic key generation
	//sk := ffi.PrivateKeyGenerate(s.blsSeed)
	// s.blsSeed++
	sk := [32]byte{}
	sk[0] = uint8(k.blsSeed) // hack to keep gas values determinist
	k.blsSeed++
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       wallet.KTBLS,
		PrivateKey: sk[:],
	})
	if err != nil {
		panic(err)
	}
	return key
}
