package eth

import (
	"context"
	"errors"
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

// stubIndexer maps every tx hash to msgCid, or reports ErrNotFound if unset.
type stubIndexer struct {
	index.Indexer
	msgCid cid.Cid
}

func (s stubIndexer) GetCidFromHash(context.Context, ethtypes.EthHash) (cid.Cid, error) {
	if !s.msgCid.Defined() {
		return cid.Undef, index.ErrNotFound
	}
	return s.msgCid, nil
}

type stubStateAPI struct{}

func (stubStateAPI) StateSearchMsg(context.Context, types.TipSetKey, cid.Cid, abi.ChainEpoch, bool) (*api.MsgLookup, error) {
	return nil, nil
}

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

// stubChainStore holds no messages.
type stubChainStore struct {
	ChainStore
}

func (stubChainStore) GetSignedMessage(context.Context, cid.Cid) (*types.SignedMessage, error) {
	return nil, errors.New("not found")
}

func (stubChainStore) GetMessage(context.Context, cid.Cid) (*types.Message, error) {
	return nil, errors.New("not found")
}

// pendingEthTx returns a signed EIP-1559 transaction (chain id 314) as a
// delegated Filecoin message, with its Eth tx hash.
func pendingEthTx(t *testing.T) (*types.SignedMessage, ethtypes.EthHash) {
	rawTx, err := ethtypes.DecodeHexString("0x02f86282013a8080808094ff000000000000000000000000000000000003ec8080c080a0f411a73e33523b40c1a916e79e67746bd01a4a4fb4ecfa87b441375a215ddfb4a0551692c1553574fab4c227ca70cb1c121dc3a2ef82179a9c984bd7acc0880a38")
	require.NoError(t, err)
	ethTx, err := ethtypes.ParseEthTransaction(rawTx)
	require.NoError(t, err)
	smsg, err := ethtypes.ToSignedFilecoinMessage(ethTx)
	require.NoError(t, err)
	txHash, err := ethTxHashFromSignedMessage(smsg)
	require.NoError(t, err)

	// The index-miss fallback CID cannot match, so that case relies on the hash.
	require.NotEqual(t, smsg.Cid(), txHash.ToCid())
	return smsg, txHash
}

func TestEthGetTransactionByHashLimitedPending(t *testing.T) {
	ctx := context.Background()
	smsg, txHash := pendingEthTx(t)

	for _, tc := range []struct {
		name   string
		msgCid cid.Cid
	}{
		{"index miss", cid.Undef},
		{"index hit", smsg.Cid()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &ethTransaction{
				chainIndexer: stubIndexer{msgCid: tc.msgCid},
				stateApi:     stubStateAPI{},
				mpoolApi:     stubMpoolAPI{pending: []*types.SignedMessage{smsg}},
			}

			tx, err := e.EthGetTransactionByHashLimited(ctx, &txHash, api.LookbackNoLimit)
			require.NoError(t, err)
			require.NotNil(t, tx)
			require.Equal(t, txHash, tx.Hash)
		})
	}

	t.Run("unknown hash", func(t *testing.T) {
		unknownHash := txHash
		unknownHash[0] ^= 0xff
		e := &ethTransaction{
			chainIndexer: stubIndexer{},
			stateApi:     stubStateAPI{},
			mpoolApi:     stubMpoolAPI{pending: []*types.SignedMessage{smsg}},
		}

		tx, err := e.EthGetTransactionByHashLimited(ctx, &unknownHash, api.LookbackNoLimit)
		require.NoError(t, err)
		require.Nil(t, tx)
	})
}

func TestEthGetMessageCidByTransactionHashPending(t *testing.T) {
	ctx := context.Background()
	smsg, txHash := pendingEthTx(t)

	e := &ethTransaction{
		chainStore:   stubChainStore{},
		chainIndexer: stubIndexer{},
		mpoolApi:     stubMpoolAPI{pending: []*types.SignedMessage{smsg}},
	}

	got, err := e.EthGetMessageCidByTransactionHash(ctx, &txHash)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, smsg.Cid(), *got)

	unknownHash := txHash
	unknownHash[0] ^= 0xff
	got, err = e.EthGetMessageCidByTransactionHash(ctx, &unknownHash)
	require.NoError(t, err)
	require.Nil(t, got)
}
