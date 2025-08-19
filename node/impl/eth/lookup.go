package eth

import (
	"bytes"
	"context"
	"errors"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/evm"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	builtinactors "github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var (
	_ EthLookupAPI = (*ethLookup)(nil)
	_ EthLookupAPI = (*EthLookupDisabled)(nil)
)

type ethLookup struct {
	chainStore      ChainStore
	stateManager    StateManager
	syncApi         SyncAPI
	stateBlockstore dtypes.StateBlockstore

	tipsetResolver TipSetResolver
}

func NewEthLookupAPI(
	chainStore ChainStore,
	stateManager StateManager,
	syncApi SyncAPI,
	stateBlockstore dtypes.StateBlockstore,
	tipsetResolver TipSetResolver,
) EthLookupAPI {
	return &ethLookup{
		chainStore:      chainStore,
		stateManager:    stateManager,
		syncApi:         syncApi,
		stateBlockstore: stateBlockstore,
		tipsetResolver:  tipsetResolver,
	}
}

// EthGetCode returns string value of the compiled bytecode
func (e *ethLookup) EthGetCode(ctx context.Context, ethAddr ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	ts, err := e.tipsetResolver.GetTipsetByBlockNumberOrHash(ctx, blkParam)
	if err != nil {
		return nil, err // don't wrap, to preserve ErrNullRound
	}

	// StateManager.Call will panic if there is no parent
	if ts.Height() == 0 {
		return nil, xerrors.New("block param must not specify genesis block")
	}

	stateCid, _, err := e.stateManager.TipSetState(ctx, ts)
	if err != nil {
		return nil, err
	}

	actor, err := e.stateManager.LoadActorRaw(ctx, to, stateCid)
	if err != nil {
		if errors.Is(err, types.ErrActorNotFound) {
			return nil, nil
		}
		return nil, xerrors.Errorf("failed to lookup contract %s: %w", ethAddr, err)
	}

	// Not a contract. We could try to distinguish between accounts and "native" contracts here,
	// but it's not worth it.
	if !builtinactors.IsEvmActor(actor.Code) {
		return nil, nil
	}

	msg := &types.Message{
		From:       builtinactors.SystemActorAddr,
		To:         to,
		Value:      big.Zero(),
		Method:     builtintypes.MethodsEVM.GetBytecode,
		Params:     nil,
		GasLimit:   buildconstants.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
	for {
		res, err = e.stateManager.CallOnState(ctx, stateCid, msg, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = e.chainStore.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}

	if err != nil {
		return nil, xerrors.Errorf("failed to call GetBytecode: %w", err)
	}

	if res.MsgRct == nil {
		return nil, xerrors.New("no message receipt")
	}

	if res.MsgRct.ExitCode.IsError() {
		return nil, xerrors.Errorf("GetBytecode failed: %s", res.Error)
	}

	var getBytecodeReturn evm.GetBytecodeReturn
	if err := getBytecodeReturn.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)); err != nil {
		return nil, xerrors.Errorf("failed to decode EVM bytecode CID: %w", err)
	}

	// The contract has selfdestructed, so the code is "empty".
	if getBytecodeReturn.Cid == nil {
		return nil, nil
	}

	blk, err := e.stateBlockstore.Get(ctx, *getBytecodeReturn.Cid)
	if err != nil {
		return nil, xerrors.Errorf("failed to get EVM bytecode: %w", err)
	}

	return blk.RawData(), nil
}

func (e *ethLookup) EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	ts, err := e.tipsetResolver.GetTipsetByBlockNumberOrHash(ctx, blkParam)
	if err != nil {
		return nil, err // don't wrap, to preserve ErrNullRound
	}

	pl := len(position)
	if pl > 32 {
		return nil, xerrors.New("supplied storage key is too long")
	}

	// pad with zero bytes if smaller than 32 bytes
	position = append(make([]byte, 32-pl, 32), position...)

	to, err := ethAddr.ToFilecoinAddress()
	if err != nil {
		return nil, xerrors.Errorf("cannot get Filecoin address: %w", err)
	}

	stateCid, _, err := e.stateManager.TipSetState(ctx, ts)
	if err != nil {
		return nil, err
	}

	actor, err := e.stateManager.LoadActorRaw(ctx, to, stateCid)
	if err != nil {
		if errors.Is(err, types.ErrActorNotFound) {
			return ethtypes.EthBytes(make([]byte, 32)), nil
		}
		return nil, xerrors.Errorf("failed to lookup contract %s: %w", ethAddr, err)
	}

	if !builtinactors.IsEvmActor(actor.Code) {
		return ethtypes.EthBytes(make([]byte, 32)), nil
	}

	params, err := actors.SerializeParams(&evm.GetStorageAtParams{
		StorageKey: *(*[32]byte)(position),
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize parameters: %w", err)
	}

	msg := &types.Message{
		From:       builtinactors.SystemActorAddr,
		To:         to,
		Value:      big.Zero(),
		Method:     builtintypes.MethodsEVM.GetStorageAt,
		Params:     params,
		GasLimit:   buildconstants.BlockGasLimit,
		GasFeeCap:  big.Zero(),
		GasPremium: big.Zero(),
	}

	// Try calling until we find a height with no migration.
	var res *api.InvocResult
	for {
		res, err = e.stateManager.CallOnState(ctx, stateCid, msg, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = e.chainStore.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}

	if err != nil {
		return nil, xerrors.Errorf("Call failed: %w", err)
	}

	if res.MsgRct == nil {
		return nil, xerrors.New("no message receipt")
	}

	if res.MsgRct.ExitCode.IsError() {
		return nil, xerrors.Errorf("failed to lookup storage slot: %s", res.Error)
	}

	var ret abi.CborBytes
	if err := ret.UnmarshalCBOR(bytes.NewReader(res.MsgRct.Return)); err != nil {
		return nil, xerrors.Errorf("failed to unmarshal storage slot: %w", err)
	}

	// pad with zero bytes if smaller than 32 bytes
	ret = append(make([]byte, 32-len(ret), 32), ret...)

	return ethtypes.EthBytes(ret), nil
}

func (e *ethLookup) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) {
	filAddr, err := address.ToFilecoinAddress()
	if err != nil {
		return ethtypes.EthBigInt{}, err
	}

	ts, err := e.tipsetResolver.GetTipsetByBlockNumberOrHash(ctx, blkParam)
	if err != nil {
		return ethtypes.EthBigInt{}, err // don't wrap, to preserve ErrNullRound
	}

	st, _, err := e.stateManager.TipSetState(ctx, ts)
	if err != nil {
		return ethtypes.EthBigInt{}, xerrors.Errorf("failed to compute tipset state: %w", err)
	}

	actor, err := e.stateManager.LoadActorRaw(ctx, filAddr, st)
	if errors.Is(err, types.ErrActorNotFound) {
		return ethtypes.EthBigIntZero, nil
	} else if err != nil {
		return ethtypes.EthBigInt{}, err
	}

	return ethtypes.EthBigInt{Int: actor.Balance.Int}, nil
}

func (e *ethLookup) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return ethtypes.EthUint64(buildconstants.Eip155ChainId), nil
}

func (e *ethLookup) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
	state, err := e.syncApi.SyncState(ctx)
	if err != nil {
		return ethtypes.EthSyncingResult{}, xerrors.Errorf("failed calling SyncState: %w", err)
	}

	if len(state.ActiveSyncs) == 0 {
		return ethtypes.EthSyncingResult{}, xerrors.New("no active syncs, try again")
	}

	working := -1
	for i, ss := range state.ActiveSyncs {
		if ss.Stage == api.StageIdle {
			continue
		}
		working = i
	}
	if working == -1 {
		working = len(state.ActiveSyncs) - 1
	}

	ss := state.ActiveSyncs[working]
	if ss.Base == nil || ss.Target == nil {
		return ethtypes.EthSyncingResult{}, xerrors.New("missing syncing information, try again")
	}

	res := ethtypes.EthSyncingResult{
		DoneSync:      ss.Stage == api.StageSyncComplete,
		CurrentBlock:  ethtypes.EthUint64(ss.Height),
		StartingBlock: ethtypes.EthUint64(ss.Base.Height()),
		HighestBlock:  ethtypes.EthUint64(ss.Target.Height()),
	}

	return res, nil
}

type EthLookupDisabled struct{}

func (EthLookupDisabled) EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	return nil, ErrModuleDisabled
}
func (EthLookupDisabled) EthGetStorageAt(ctx context.Context, ethAddr ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error) {
	return nil, ErrModuleDisabled
}
func (EthLookupDisabled) EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error) {
	return ethtypes.EthBigInt{}, ErrModuleDisabled
}
func (EthLookupDisabled) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return ethtypes.EthUint64(0), ErrModuleDisabled
}
