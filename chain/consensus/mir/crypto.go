package mir

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/filecoin-project/go-address"
	filcrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/sigs"
	mircrypto "github.com/filecoin-project/mir/pkg/crypto"
	t "github.com/filecoin-project/mir/pkg/types"
)

var MsgMeta = api.MsgMeta{Type: "mir-message"}

type WalletCrypto interface {
	WalletSign(ctx context.Context, k address.Address, msg []byte) (*filcrypto.Signature, error)
	WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *filcrypto.Signature) (bool, error)
}

var _ mircrypto.Crypto = &CryptoManager{}

type CryptoManager struct {
	key address.Address // The address corresponding to the private key.
	api WalletCrypto    // API used to sign data in HSM-model.
}

func NewCryptoManager(key address.Address, wallet WalletCrypto) (*CryptoManager, error) {
	if key.Protocol() != address.SECP256K1 {
		return nil, fmt.Errorf("must be SECP address")
	}
	return &CryptoManager{key, wallet}, nil
}

func (c *CryptoManager) ImplementsModule() {}

// Sign signs the provided data and returns the resulting signature.
// The data to be signed is the concatenation of all the passed byte slices.
// A signature produced by Sign is verifiable using Verify,
// if, respectively, RegisterNodeKey or RegisterClientKey has been invoked with the corresponding public key.
// Note that the private key used to produce the signature cannot be set ("registered") through this interface.
// Storing and using the private key is completely implementation-dependent.
func (c *CryptoManager) Sign(data [][]byte) ([]byte, error) {
	signature, err := c.api.WalletSign(context.Background(), c.key, hash(data))
	if err != nil {
		return nil, err
	}
	return signature.MarshalBinary()
}

// Verify verifies a signature produced by the node with ID nodeID over data.
// Returns nil on success (i.e., if the given signature is valid) and a non-nil error otherwise.
// Note that RegisterNodeKey must be used to register the node's public key before calling Verify,
// otherwise Verify will fail.
func (c *CryptoManager) Verify(data [][]byte, sigBytes []byte, nodeID t.NodeID) error {
	nodeAddr, err := getAddr(nodeID.Pb())
	if err != nil {
		return err
	}
	return c.verifySig(data, sigBytes, nodeAddr)
}

func (c *CryptoManager) verifySig(data [][]byte, sigBytes []byte, addr address.Address) error {
	var sig filcrypto.Signature
	if err := sig.UnmarshalBinary(sigBytes); err != nil {
		return err
	}

	return sigs.Verify(&sig, addr, hash(data))
}

func hash(data [][]byte) []byte {
	h := sha256.New()
	for _, d := range data {
		h.Write(d)
	}
	return h.Sum(nil)
}

func getAddr(nodeID string) (address.Address, error) {
	addrParts := strings.Split(nodeID, ":")
	if len(addrParts) != 2 {
		return address.Undef, fmt.Errorf("invalid node ID: %s", nodeID)
	}
	nodeAddr, err := address.NewFromString(addrParts[1])
	if err != nil {
		return address.Undef, err
	}
	return nodeAddr, nil
}
