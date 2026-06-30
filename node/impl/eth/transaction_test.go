package eth

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// stubIndexer reports that it has no txHash -> message CID mapping for any hash,
// simulating the case where the chain indexer has not (yet) indexed a locally
// pending Ethereum transaction.
type stubIndexer struct {
	index.Indexer
}

func (stubIndexer) GetCidFromHash(context.Context, ethtypes.EthHash) (cid.Cid, error) {
	return cid.Undef, index.ErrNotFound
}

// stubStateAPI never finds a transaction on-chain.
type stubStateAPI struct{}

func (stubStateAPI) StateSearchMsg(context.Context, types.TipSetKey, cid.Cid, abi.ChainEpoch, bool) (*api.MsgLookup, error) {
	return nil, nil
}

// stubMpoolAPI returns a fixed set of pending messages.
type stubMpoolAPI struct {
	pending []*types.SignedMessage
}

func (s stubMpoolAPI) MpoolPending(context.Context, types.TipSetKey) ([]*types.SignedMessage, error) {
	return s.pending, nil
}

func (stubMpoolAPI) MpoolGetNonce(context.Context, address.Address) (uint64, error) {
	return 0, nil
}

func (stubMpoolAPI) MpoolPush(context.Context, *types.SignedMessage) (cid.Cid, error) {
	return cid.Undef, nil
}

func (stubMpoolAPI) MpoolPushUntrusted(context.Context, *types.SignedMessage) (cid.Cid, error) {
	return cid.Undef, nil
}

// TestEthGetTransactionByHashLimitedPendingNoIndex is a regression test for
// https://github.com/filecoin-project/lotus/issues/13665: a locally pending
// Ethereum transaction must still be returned by eth_getTransactionByHash even
// when the chain indexer has no txHash -> message CID mapping for it. Before the
// fix, getCidForTransaction fabricated a Filecoin CID from the Ethereum tx hash
// (EthHash.ToCid, which is only valid for blocks and Filecoin messages), the
// mpool scan compared that fabricated CID against the real message CID, never
// matched, and the transaction was reported as not found.
func TestEthGetTransactionByHashLimitedPendingNoIndex(t *testing.T) {
	ctx := context.Background()

	// A signed EIP-1559 transaction (chain id 314).
	rawTx, err := ethtypes.DecodeHexString("0x02f86282013a8080808094ff000000000000000000000000000000000003ec8080c080a0f411a73e33523b40c1a916e79e67746bd01a4a4fb4ecfa87b441375a215ddfb4a0551692c1553574fab4c227ca70cb1c121dc3a2ef82179a9c984bd7acc0880a38")
	require.NoError(t, err)

	ethTx, err := ethtypes.ParseEthTransaction(rawTx)
	require.NoError(t, err)

	smsg, err := ethtypes.ToSignedFilecoinMessage(ethTx)
	require.NoError(t, err)

	// The Ethereum transaction hash a client would query with.
	wantHash, err := ethTxHashFromSignedMessage(smsg)
	require.NoError(t, err)

	// The fabricated-CID fallback does not equal the real signed-message CID, so a
	// CID-only mpool match (the pre-fix behavior) would miss this pending tx.
	require.NotEqual(t, smsg.Cid(), wantHash.ToCid())

	e := &ethTransaction{
		chainIndexer: stubIndexer{},
		stateApi:     stubStateAPI{},
		mpoolApi:     stubMpoolAPI{pending: []*types.SignedMessage{smsg}},
	}

	// The pending tx is found by its real Ethereum hash despite the missing index.
	tx, err := e.EthGetTransactionByHashLimited(ctx, &wantHash, api.LookbackNoLimit)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, wantHash, tx.Hash)

	// An unknown hash that is not in the mpool returns an empty response, no error.
	otherHash := wantHash
	otherHash[0] ^= 0xff
	missing, err := e.EthGetTransactionByHashLimited(ctx, &otherHash, api.LookbackNoLimit)
	require.NoError(t, err)
	require.Nil(t, missing)

	// An empty mpool returns an empty response, no error.
	eEmpty := &ethTransaction{
		chainIndexer: stubIndexer{},
		stateApi:     stubStateAPI{},
		mpoolApi:     stubMpoolAPI{},
	}
	empty, err := eEmpty.EthGetTransactionByHashLimited(ctx, &wantHash, api.LookbackNoLimit)
	require.NoError(t, err)
	require.Nil(t, empty)
}
