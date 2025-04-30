package eth

import (
	"context"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var (
	_ EthBasicAPI = (*ethBasic)(nil)
	_ EthBasicAPI = (*EthBasicDisabled)(nil)
)

type ethBasic struct {
	chainStore   ChainStore
	syncApi      SyncAPI
	stateManager StateManager
}

func NewEthBasicAPI(chainStore ChainStore, syncApi SyncAPI, stateManager StateManager) EthBasicAPI {
	return &ethBasic{
		chainStore:   chainStore,
		syncApi:      syncApi,
		stateManager: stateManager,
	}
}

func (e *ethBasic) Web3ClientVersion(ctx context.Context) (string, error) {
	return string(build.NodeUserVersion()), nil
}

func (e *ethBasic) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return ethtypes.EthUint64(buildconstants.Eip155ChainId), nil
}

func (e *ethBasic) NetVersion(_ context.Context) (string, error) {
	return strconv.FormatInt(buildconstants.Eip155ChainId, 10), nil
}

func (e *ethBasic) NetListening(ctx context.Context) (bool, error) {
	return true, nil
}

func (e *ethBasic) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	height := e.chainStore.GetHeaviestTipSet().Height()
	return ethtypes.EthUint64(e.stateManager.GetNetworkVersion(ctx, height)), nil
}

func (e *ethBasic) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
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

func (e *ethBasic) EthAccounts(context.Context) ([]ethtypes.EthAddress, error) {
	// The lotus node is not expected to hold manage accounts, so we'll always return an empty array
	return []ethtypes.EthAddress{}, nil
}

type EthBasicDisabled struct{}

func (EthBasicDisabled) Web3ClientVersion(ctx context.Context) (string, error) {
	return string(build.NodeUserVersion()), nil
}
func (EthBasicDisabled) EthChainId(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}
func (EthBasicDisabled) NetVersion(ctx context.Context) (string, error) {
	return "", ErrModuleDisabled
}
func (EthBasicDisabled) NetListening(ctx context.Context) (bool, error) {
	return false, ErrModuleDisabled
}
func (EthBasicDisabled) EthProtocolVersion(ctx context.Context) (ethtypes.EthUint64, error) {
	return 0, ErrModuleDisabled
}
func (EthBasicDisabled) EthSyncing(ctx context.Context) (ethtypes.EthSyncingResult, error) {
	return ethtypes.EthSyncingResult{}, ErrModuleDisabled
}
func (EthBasicDisabled) EthAccounts(ctx context.Context) ([]ethtypes.EthAddress, error) {
	return nil, ErrModuleDisabled
}
