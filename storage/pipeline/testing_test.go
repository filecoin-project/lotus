package sealing

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

// makeTestTipSet builds a single-block tipset at height, carrying only what NewTipSet validates.
func makeTestTipSet(t *testing.T, height abi.ChainEpoch) *types.TipSet {
	t.Helper()

	dummyCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	dummyAddr, err := address.NewIDAddress(0)
	require.NoError(t, err)

	ts, err := types.NewTipSet([]*types.BlockHeader{{
		Height:                height,
		Miner:                 dummyAddr,
		Parents:               []cid.Cid{},
		Ticket:                &types.Ticket{VRFProof: []byte{byte(height % 2)}},
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		ParentBaseFee:         big.Zero(),
	}})
	require.NoError(t, err)

	return ts
}
