package mir

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	filcrypto "github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/sigs"
	mirTypes "github.com/filecoin-project/mir/pkg/types"
)

type cryptoNode struct {
	api *wallet.LocalWallet
	key address.Address
}

func newCryptoNode() (*cryptoNode, error) {
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		return nil, err
	}

	addr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		return nil, err
	}
	return &cryptoNode{w, addr}, nil
}

func (n *cryptoNode) WalletSign(ctx context.Context, k address.Address, msg []byte) (signature *filcrypto.Signature, err error) {
	if k.Protocol() != address.SECP256K1 {
		return nil, xerrors.New("must be SECP address")
	}
	if k != n.key {
		return nil, xerrors.New("wrong address")
	}
	signature, err = n.api.WalletSign(ctx, k, msg, MsgMeta)
	return
}

func (n *cryptoNode) WalletVerify(ctx context.Context, k address.Address, msg []byte, sig *filcrypto.Signature) (bool, error) {
	err := sigs.Verify(sig, k, msg)
	return err == nil, err
}

func TestCryptoManager(t *testing.T) {
	node, err := newCryptoNode()
	require.NoError(t, err)

	addr := node.key
	c, err := NewCryptoManager(addr, node)
	require.NoError(t, err)

	data := [][]byte{{1, 2, 3}, {1, 2, 3}, {1, 2, 3}, {1, 2, 3}}
	sigBytes, err := c.Sign(data)
	require.NoError(t, err)

	nodeID := mirTypes.NodeID(newMirID("/root", addr.String()))
	err = c.Verify([][]byte{{1, 2, 3}}, sigBytes, nodeID)
	require.Error(t, err)

	err = c.Verify([][]byte{{1, 2, 3}}, sigBytes, nodeID)
	require.Error(t, err)

	err = c.Verify(data, []byte{1, 2, 3}, nodeID)
	require.Error(t, err)

	nodeID = mirTypes.NodeID(addr.String())
	err = c.Verify(data, sigBytes, nodeID)
	require.Error(t, err)

	nodeID = mirTypes.NodeID(newMirID("/root:", addr.String()))
	err = c.Verify(data, sigBytes, nodeID)
	require.Error(t, err)
}
