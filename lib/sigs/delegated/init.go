package delegated

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/builtin"
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	gocrypto "github.com/filecoin-project/go-crypto"
	crypto1 "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/lib/sigs"
)

type delegatedSigner struct{}

func (delegatedSigner) GenPrivate() ([]byte, error) {
	priv, err := gocrypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	return priv, nil
}

func (delegatedSigner) ToPublic(pk []byte) ([]byte, error) {
	return gocrypto.PublicKey(pk), nil
}

func (delegatedSigner) Sign(pk []byte, msg []byte) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (delegatedSigner) Verify(sig []byte, a address.Address, msg []byte) error {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(msg)
	hash := hasher.Sum(nil)

	pubk, err := gocrypto.EcRecover(hash, sig)
	if err != nil {
		return err
	}

	hasher.Reset()
	hasher.Write(pubk)
	addrHash := hasher.Sum(nil)

	maybeaddr, err := address.NewDelegatedAddress(builtin.EthereumAddressManagerActorID, addrHash[len(addrHash)-20:])
	if err != nil {
		return err
	}

	if maybeaddr != a {
		return fmt.Errorf("signature did not match")
	}

	return nil
}

func init() {
	sigs.RegisterSignature(crypto1.SigTypeDelegated, delegatedSigner{})
}
