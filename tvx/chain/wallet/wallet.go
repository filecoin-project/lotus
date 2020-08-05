package wallet

import (
	"errors"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipsn/go-secp256k1"
	blake2b "github.com/minio/blake2b-simd"

	bls "github.com/filecoin-project/filecoin-ffi"
)

// KeyInfo is used for storing keys in KeyStore
type KeyInfo struct {
	Type       crypto.SigType
	PrivateKey []byte
}

type Key struct {
	KeyInfo

	PublicKey []byte
	Address   address.Address
}

func NewKey(keyinfo KeyInfo) (*Key, error) {
	k := &Key{
		KeyInfo: keyinfo,
	}

	var err error
	k.PublicKey, err = ToPublic(k.Type, k.PrivateKey)
	if err != nil {
		return nil, err
	}

	switch k.Type {
	case crypto.SigTypeSecp256k1:
		k.Address, err = address.NewSecp256k1Address(k.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("converting Secp256k1 to address: %w", err)
		}
	case crypto.SigTypeBLS:
		k.Address, err = address.NewBLSAddress(k.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("converting BLS to address: %w", err)
		}
	default:
		return nil, errors.New("unknown key type")
	}
	return k, nil

}

func Sign(data []byte, secretKey []byte, sigtype crypto.SigType) (crypto.Signature, error) {
	var signature []byte
	var err error
	if sigtype == crypto.SigTypeSecp256k1 {
		hash := blake2b.Sum256(data)
		signature, err = SignSecp(secretKey, hash[:])
	} else if sigtype == crypto.SigTypeBLS {
		signature, err = SignBLS(secretKey, data)
	} else {
		err = fmt.Errorf("unknown signature type %d", sigtype)
	}
	return crypto.Signature{
		Type: sigtype,
		Data: signature,
	}, err
}

func SignSecp(sk, msg []byte) ([]byte, error) {
	return secp256k1.Sign(msg, sk)
}

// SignBLS signs the given message with BLS.
func SignBLS(sk, msg []byte) ([]byte, error) {
	var privateKey bls.PrivateKey
	copy(privateKey[:], sk)
	sig := bls.PrivateKeySign(privateKey, msg)
	return sig[:], nil
}

// ported from lotus
// SigShim is used for introducing signature functions
type SigShim interface {
	GenPrivate() ([]byte, error)
	ToPublic(pk []byte) ([]byte, error)
	Sign(pk []byte, msg []byte) ([]byte, error)
	Verify(sig []byte, a address.Address, msg []byte) error
}

var sigs map[crypto.SigType]SigShim

// RegisterSig should be only used during init
func RegisterSignature(typ crypto.SigType, vs SigShim) {
	if sigs == nil {
		sigs = make(map[crypto.SigType]SigShim)
	}
	sigs[typ] = vs
}

// ToPublic converts private key to public key
func ToPublic(sigType crypto.SigType, pk []byte) ([]byte, error) {
	sv, ok := sigs[sigType]
	if !ok {
		return nil, fmt.Errorf("cannot generate public key of unsupported type: %v", sigType)
	}

	return sv.ToPublic(pk)
}
