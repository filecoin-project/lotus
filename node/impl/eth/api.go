package eth

import (
	"context"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	ErrChainIndexerDisabled = xerrors.New("chain indexer is disabled; please enable the ChainIndexer to use the ETH RPC API")
	ErrModuleDisabled       = xerrors.New("module disabled, enable with Fevm.EnableEthRPC / LOTUS_FEVM_ENABLEETHRPC")
)

var log = logging.Logger("node/eth")

// SyncAPI is a minimal version of full.SyncAPI
type SyncAPI interface {
	SyncState(ctx context.Context) (*api.SyncState, error)
}

// ChainStore is a minimal version of store.ChainStore just for tipsets
type ChainStore interface {
	// TipSets
	GetHeaviestTipSet() *types.TipSet
	GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error)
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
	GetTipSetByCid(ctx context.Context, c cid.Cid) (*types.TipSet, error)
	LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)

	// Messages
	GetSignedMessage(ctx context.Context, c cid.Cid) (*types.SignedMessage, error)
	GetMessage(ctx context.Context, c cid.Cid) (*types.Message, error)
	BlockMsgsForTipset(ctx context.Context, ts *types.TipSet) ([]store.BlockMessages, error)
	MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error)
	ReadReceipts(ctx context.Context, root cid.Cid) ([]types.MessageReceipt, error)

	// Misc
	ActorStore(ctx context.Context) adt.Store
}

// StateAPI is a minimal version of full.StateAPI
type StateAPI interface {
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
}

// StateManager is a minimal version of stmgr.StateManager
type StateManager interface {
	GetNetworkVersion(ctx context.Context, height abi.ChainEpoch) network.Version

	TipSetState(ctx context.Context, ts *types.TipSet) (cid.Cid, cid.Cid, error)
	ParentState(ts *types.TipSet) (*state.StateTree, error)
	StateTree(st cid.Cid) (*state.StateTree, error)

	LookupIDAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)
	LoadActor(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, error)
	LoadActorRaw(ctx context.Context, addr address.Address, st cid.Cid) (*types.Actor, error)
	ResolveToDeterministicAddress(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error)

	ExecutionTrace(ctx context.Context, ts *types.TipSet) (cid.Cid, []*api.InvocResult, error)
	Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)
	CallWithGas(ctx context.Context, msg *types.Message, priorMsgs []types.ChainMsg, ts *types.TipSet, applyTsMessages bool) (*api.InvocResult, error)
	ApplyOnStateWithGas(ctx context.Context, stateCid cid.Cid, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error)

	HasExpensiveForkBetween(parent, height abi.ChainEpoch) bool
}

// MpoolAPI is a minimal version of full.MpoolAPI
type MpoolAPI interface {
	MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error)
	MpoolGetNonce(ctx context.Context, addr address.Address) (uint64, error)
	MpoolPushUntrusted(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error)
	MpoolPush(ctx context.Context, smsg *types.SignedMessage) (cid.Cid, error)
}

// MessagePool is a minimal version of messagepool.MessagePool
type MessagePool interface {
	PendingFor(ctx context.Context, a address.Address) ([]*types.SignedMessage, *types.TipSet)
	GetConfig() *types.MpoolConfig
}

// GasAPI is a minimal version of full.GasAPI
type GasAPI interface {
	GasEstimateGasPremium(ctx context.Context, nblocksincl uint64, sender address.Address, gaslimit int64, ts types.TipSetKey) (types.BigInt, error)
	GasEstimateMessageGas(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, ts types.TipSetKey) (*types.Message, error)
}
