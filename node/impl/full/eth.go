package full

import (
	"context"
	"strconv"
	"strings"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type EthModuleAPI interface {
	EthBlockNumber(context.Context) (api.EthInt, error)
	EthAccounts(context.Context) ([]types.EthAddress, error)
	EthGetBlockTransactionCountByNumber(context.Context, string) (api.EthInt, error)
	EthGetBlockTransactionCountByHash(context.Context, string) (api.EthInt, error)
}

var _ EthModuleAPI = *new(api.FullNode)

// EthModule provides a default implementation of EthModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type EthModule struct {
	fx.In

	Chain *store.ChainStore
}

var _ EthModuleAPI = (*EthModule)(nil)

type EthAPI struct {
	fx.In

	Chain *store.ChainStore

	EthModuleAPI
}

func (a *EthModule) EthBlockNumber(context.Context) (api.EthInt, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return api.EthInt(height), nil
}

func (a *EthModule) EthAccounts(context.Context) ([]types.EthAddress, error) {
	// The lotus node is not expected to hold manage accounts, so we'll always return an empty array
	return []types.EthAddress{}, nil
}

func (a *EthModule) EthGetBlockTransactionCountByNumber(ctx context.Context, blkNumHex string) (api.EthInt, error) {
	blkNum, err := strconv.ParseInt(strings.Replace(blkNumHex, "0x", "", -1), 16, 64)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("invalid block number %s: %w", blkNumHex, err)
	}

	ts, err := a.Chain.GetTipsetByHeight(ctx, abi.ChainEpoch(blkNum), nil, false)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	blkMsgs, err := a.Chain.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	count := 0
	for _, blkMsg := range blkMsgs {
		count += len(blkMsg.BlsMessages) + len(blkMsg.SecpkMessages)
	}
	return api.EthInt(count), nil
}

func (a *EthModule) EthGetBlockTransactionCountByHash(ctx context.Context, blkHash string) (api.EthInt, error) {
	hash, err := types.EthHashFromHex(blkHash)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("invalid hash %s: %w", blkHash, err)
	}

	ts, err := a.Chain.GetTipSetByCid(ctx, hash.ToCid())
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading tipset %s: %w", ts, err)
	}

	blkMsgs, err := a.Chain.BlockMsgsForTipset(ctx, ts)
	if err != nil {
		return api.EthInt(0), xerrors.Errorf("error loading messages for tipset: %v: %w", ts, err)
	}

	count := 0
	for _, blkMsg := range blkMsgs {
		count += len(blkMsg.BlsMessages) + len(blkMsg.SecpkMessages)
	}
	return api.EthInt(count), nil
}
