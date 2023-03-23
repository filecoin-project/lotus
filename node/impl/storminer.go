package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	"github.com/ipfs/go-graphsync/peerstate"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/dagstore/shard"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	gst "github.com/filecoin-project/go-data-transfer/v2/transport/graphsync"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	filmktsstore "github.com/filecoin-project/go-fil-markets/stores"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	mktsdagstore "github.com/filecoin-project/lotus/markets/dagstore"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

type StorageMinerAPI struct {
	fx.In

	api.Common
	api.Net

	EnabledSubsystems api.MinerSubsystems

	Full        api.FullNode
	LocalStore  *paths.Local
	RemoteStore *paths.Remote

	// Markets
	PieceStore        dtypes.ProviderPieceStore         `optional:"true"`
	StorageProvider   storagemarket.StorageProvider     `optional:"true"`
	RetrievalProvider retrievalmarket.RetrievalProvider `optional:"true"`
	SectorAccessor    retrievalmarket.SectorAccessor    `optional:"true"`
	DataTransfer      dtypes.ProviderDataTransfer       `optional:"true"`
	StagingGraphsync  dtypes.StagingGraphsync           `optional:"true"`
	Transport         dtypes.ProviderTransport          `optional:"true"`
	DealPublisher     *storageadapter.DealPublisher     `optional:"true"`
	SectorBlocks      *sectorblocks.SectorBlocks        `optional:"true"`
	Host              host.Host                         `optional:"true"`
	DAGStore          *dagstore.DAGStore                `optional:"true"`
	DAGStoreWrapper   *mktsdagstore.Wrapper             `optional:"true"`

	// Miner / storage
	Miner       *sealing.Sealing     `optional:"true"`
	BlockMiner  *miner.Miner         `optional:"true"`
	StorageMgr  *sealer.Manager      `optional:"true"`
	IStorageMgr sealer.SectorManager `optional:"true"`
	paths.SectorIndex
	storiface.WorkerReturn `optional:"true"`
	AddrSel                *ctladdr.AddressSelector

	WdPoSt *wdpost.WindowPoStScheduler `optional:"true"`

	Epp gen.WinningPoStProver `optional:"true"`
	DS  dtypes.MetadataDS

	// StorageService is populated when we're not the main storage node (e.g. we're a markets node)
	StorageService modules.MinerStorageService `optional:"true"`

	ConsiderOnlineStorageDealsConfigFunc        dtypes.ConsiderOnlineStorageDealsConfigFunc        `optional:"true"`
	SetConsiderOnlineStorageDealsConfigFunc     dtypes.SetConsiderOnlineStorageDealsConfigFunc     `optional:"true"`
	ConsiderOnlineRetrievalDealsConfigFunc      dtypes.ConsiderOnlineRetrievalDealsConfigFunc      `optional:"true"`
	SetConsiderOnlineRetrievalDealsConfigFunc   dtypes.SetConsiderOnlineRetrievalDealsConfigFunc   `optional:"true"`
	StorageDealPieceCidBlocklistConfigFunc      dtypes.StorageDealPieceCidBlocklistConfigFunc      `optional:"true"`
	SetStorageDealPieceCidBlocklistConfigFunc   dtypes.SetStorageDealPieceCidBlocklistConfigFunc   `optional:"true"`
	ConsiderOfflineStorageDealsConfigFunc       dtypes.ConsiderOfflineStorageDealsConfigFunc       `optional:"true"`
	SetConsiderOfflineStorageDealsConfigFunc    dtypes.SetConsiderOfflineStorageDealsConfigFunc    `optional:"true"`
	ConsiderOfflineRetrievalDealsConfigFunc     dtypes.ConsiderOfflineRetrievalDealsConfigFunc     `optional:"true"`
	SetConsiderOfflineRetrievalDealsConfigFunc  dtypes.SetConsiderOfflineRetrievalDealsConfigFunc  `optional:"true"`
	ConsiderVerifiedStorageDealsConfigFunc      dtypes.ConsiderVerifiedStorageDealsConfigFunc      `optional:"true"`
	SetConsiderVerifiedStorageDealsConfigFunc   dtypes.SetConsiderVerifiedStorageDealsConfigFunc   `optional:"true"`
	ConsiderUnverifiedStorageDealsConfigFunc    dtypes.ConsiderUnverifiedStorageDealsConfigFunc    `optional:"true"`
	SetConsiderUnverifiedStorageDealsConfigFunc dtypes.SetConsiderUnverifiedStorageDealsConfigFunc `optional:"true"`
	SetSealingConfigFunc                        dtypes.SetSealingConfigFunc                        `optional:"true"`
	GetSealingConfigFunc                        dtypes.GetSealingConfigFunc                        `optional:"true"`
	GetExpectedSealDurationFunc                 dtypes.GetExpectedSealDurationFunc                 `optional:"true"`
	SetExpectedSealDurationFunc                 dtypes.SetExpectedSealDurationFunc                 `optional:"true"`
}

var _ api.StorageMiner = &StorageMinerAPI{}

func (sm *StorageMinerAPI) StorageAuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	if sm.StorageService != nil {
		return sm.StorageService.AuthVerify(ctx, token)
	}

	return sm.AuthVerify(ctx, token)
}

func (sm *StorageMinerAPI) ServeRemote(perm bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if perm == true {
			if !auth.HasPerm(r.Context(), nil, api.PermAdmin) {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
				return
			}
		}

		sm.StorageMgr.ServeHTTP(w, r)
	}
}

func (sm *StorageMinerAPI) WorkerStats(ctx context.Context) (map[uuid.UUID]storiface.WorkerStats, error) {
	return sm.StorageMgr.WorkerStats(ctx), nil
}

func (sm *StorageMinerAPI) WorkerJobs(ctx context.Context) (map[uuid.UUID][]storiface.WorkerJob, error) {
	return sm.StorageMgr.WorkerJobs(), nil
}

func (sm *StorageMinerAPI) ActorAddress(context.Context) (address.Address, error) {
	return sm.Miner.Address(), nil
}

func (sm *StorageMinerAPI) MiningBase(ctx context.Context) (*types.TipSet, error) {
	mb, err := sm.BlockMiner.GetBestMiningCandidate(ctx)
	if err != nil {
		return nil, err
	}
	return mb.TipSet, nil
}

func (sm *StorageMinerAPI) ActorSectorSize(ctx context.Context, addr address.Address) (abi.SectorSize, error) {
	mi, err := sm.Full.StateMinerInfo(ctx, addr, types.EmptyTSK)
	if err != nil {
		return 0, err
	}
	return mi.SectorSize, nil
}

func (sm *StorageMinerAPI) PledgeSector(ctx context.Context) (abi.SectorID, error) {
	sr, err := sm.Miner.PledgeSector(ctx)
	if err != nil {
		return abi.SectorID{}, err
	}

	return sm.waitSectorStarted(ctx, sr.ID)
}

func (sm *StorageMinerAPI) waitSectorStarted(ctx context.Context, si abi.SectorID) (abi.SectorID, error) {
	// wait for the sector to enter the Packing state
	// TODO: instead of polling implement some pubsub-type thing in storagefsm
	for {
		info, err := sm.Miner.SectorsStatus(ctx, si.Number, false)
		if err != nil {
			return abi.SectorID{}, xerrors.Errorf("getting pledged sector info: %w", err)
		}

		if info.State != api.SectorState(sealing.UndefinedSectorState) {
			return si, nil
		}

		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			return abi.SectorID{}, ctx.Err()
		}
	}
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	sInfo, err := sm.Miner.SectorsStatus(ctx, sid, false)
	if err != nil {
		return api.SectorInfo{}, err
	}

	if !showOnChainInfo {
		return sInfo, nil
	}

	onChainInfo, err := sm.Full.StateSectorGetInfo(ctx, sm.Miner.Address(), sid, types.EmptyTSK)
	if err != nil {
		return sInfo, err
	}
	if onChainInfo == nil {
		return sInfo, nil
	}
	sInfo.SealProof = onChainInfo.SealProof
	sInfo.Activation = onChainInfo.Activation
	sInfo.Expiration = onChainInfo.Expiration
	sInfo.DealWeight = onChainInfo.DealWeight
	sInfo.VerifiedDealWeight = onChainInfo.VerifiedDealWeight
	sInfo.InitialPledge = onChainInfo.InitialPledge

	ex, err := sm.Full.StateSectorExpiration(ctx, sm.Miner.Address(), sid, types.EmptyTSK)
	if err != nil {
		return sInfo, nil
	}
	sInfo.OnTime = ex.OnTime
	sInfo.Early = ex.Early

	return sInfo, nil
}

func (sm *StorageMinerAPI) SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d api.PieceDealInfo) (api.SectorOffset, error) {
	so, err := sm.Miner.SectorAddPieceToAny(ctx, size, r, d)
	if err != nil {
		// jsonrpc doesn't support returning values with errors, make sure we never do that
		return api.SectorOffset{}, err
	}

	return so, nil
}

func (sm *StorageMinerAPI) SectorsUnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error {
	return sm.StorageMgr.SectorsUnsealPiece(ctx, sector, offset, size, randomness, commd)
}

// List all staged sectors
func (sm *StorageMinerAPI) SectorsList(context.Context) ([]abi.SectorNumber, error) {
	sectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, err
	}

	out := make([]abi.SectorNumber, 0, len(sectors))
	for _, sector := range sectors {
		if sector.State == sealing.UndefinedSectorState {
			continue // sector ID not set yet
		}

		out = append(out, sector.SectorNumber)
	}
	return out, nil
}

func (sm *StorageMinerAPI) SectorsListInStates(ctx context.Context, states []api.SectorState) ([]abi.SectorNumber, error) {
	filterStates := make(map[sealing.SectorState]struct{})
	for _, state := range states {
		st := sealing.SectorState(state)
		if _, ok := sealing.ExistSectorStateList[st]; !ok {
			continue
		}
		filterStates[st] = struct{}{}
	}

	var sns []abi.SectorNumber
	if len(filterStates) == 0 {
		return sns, nil
	}

	sectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, err
	}

	for i := range sectors {
		if _, ok := filterStates[sectors[i].State]; ok {
			sns = append(sns, sectors[i].SectorNumber)
		}
	}
	return sns, nil
}

func (sm *StorageMinerAPI) SectorsSummary(ctx context.Context) (map[api.SectorState]int, error) {
	sectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, err
	}

	out := make(map[api.SectorState]int)
	for i := range sectors {
		state := api.SectorState(sectors[i].State)
		out[state]++
	}

	return out, nil
}

func (sm *StorageMinerAPI) StorageLocal(ctx context.Context) (map[storiface.ID]string, error) {
	l, err := sm.LocalStore.Local(ctx)
	if err != nil {
		return nil, err
	}

	out := map[storiface.ID]string{}
	for _, st := range l {
		out[st.ID] = st.LocalPath
	}

	return out, nil
}

func (sm *StorageMinerAPI) SectorsRefs(ctx context.Context) (map[string][]api.SealedRef, error) {
	// json can't handle cids as map keys
	out := map[string][]api.SealedRef{}

	refs, err := sm.SectorBlocks.List(ctx)
	if err != nil {
		return nil, err
	}

	for k, v := range refs {
		out[strconv.FormatUint(k, 10)] = v
	}

	return out, nil
}

func (sm *StorageMinerAPI) StorageStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error) {
	return sm.RemoteStore.FsStat(ctx, id)
}

func (sm *StorageMinerAPI) SectorStartSealing(ctx context.Context, number abi.SectorNumber) error {
	return sm.Miner.StartPackingSector(number)
}

func (sm *StorageMinerAPI) SectorSetSealDelay(ctx context.Context, delay time.Duration) error {
	cfg, err := sm.GetSealingConfigFunc()
	if err != nil {
		return xerrors.Errorf("get config: %w", err)
	}

	cfg.WaitDealsDelay = delay

	return sm.SetSealingConfigFunc(cfg)
}

func (sm *StorageMinerAPI) SectorGetSealDelay(ctx context.Context) (time.Duration, error) {
	cfg, err := sm.GetSealingConfigFunc()
	if err != nil {
		return 0, err
	}
	return cfg.WaitDealsDelay, nil
}

func (sm *StorageMinerAPI) SectorSetExpectedSealDuration(ctx context.Context, delay time.Duration) error {
	return sm.SetExpectedSealDurationFunc(delay)
}

func (sm *StorageMinerAPI) SectorGetExpectedSealDuration(ctx context.Context) (time.Duration, error) {
	return sm.GetExpectedSealDurationFunc()
}

func (sm *StorageMinerAPI) SectorsUpdate(ctx context.Context, id abi.SectorNumber, state api.SectorState) error {
	return sm.Miner.ForceSectorState(ctx, id, sealing.SectorState(state))
}

func (sm *StorageMinerAPI) SectorRemove(ctx context.Context, id abi.SectorNumber) error {
	return sm.Miner.RemoveSector(ctx, id)
}

func (sm *StorageMinerAPI) SectorTerminate(ctx context.Context, id abi.SectorNumber) error {
	return sm.Miner.TerminateSector(ctx, id)
}

func (sm *StorageMinerAPI) SectorTerminateFlush(ctx context.Context) (*cid.Cid, error) {
	return sm.Miner.TerminateFlush(ctx)
}

func (sm *StorageMinerAPI) SectorTerminatePending(ctx context.Context) ([]abi.SectorID, error) {
	return sm.Miner.TerminatePending(ctx)
}

func (sm *StorageMinerAPI) SectorPreCommitFlush(ctx context.Context) ([]sealiface.PreCommitBatchRes, error) {
	return sm.Miner.SectorPreCommitFlush(ctx)
}

func (sm *StorageMinerAPI) SectorPreCommitPending(ctx context.Context) ([]abi.SectorID, error) {
	return sm.Miner.SectorPreCommitPending(ctx)
}

func (sm *StorageMinerAPI) SectorMarkForUpgrade(ctx context.Context, id abi.SectorNumber, snap bool) error {
	if !snap {
		return fmt.Errorf("non-snap upgrades are not supported")
	}

	return sm.Miner.MarkForUpgrade(ctx, id)
}

func (sm *StorageMinerAPI) SectorAbortUpgrade(ctx context.Context, number abi.SectorNumber) error {
	return sm.Miner.SectorAbortUpgrade(number)
}

func (sm *StorageMinerAPI) SectorCommitFlush(ctx context.Context) ([]sealiface.CommitBatchRes, error) {
	return sm.Miner.CommitFlush(ctx)
}

func (sm *StorageMinerAPI) SectorCommitPending(ctx context.Context) ([]abi.SectorID, error) {
	return sm.Miner.CommitPending(ctx)
}

func (sm *StorageMinerAPI) SectorMatchPendingPiecesToOpenSectors(ctx context.Context) error {
	return sm.Miner.SectorMatchPendingPiecesToOpenSectors(ctx)
}

func (sm *StorageMinerAPI) SectorNumAssignerMeta(ctx context.Context) (api.NumAssignerMeta, error) {
	return sm.Miner.NumAssignerMeta(ctx)
}

func (sm *StorageMinerAPI) SectorNumReservations(ctx context.Context) (map[string]bitfield.BitField, error) {
	return sm.Miner.NumReservations(ctx)
}

func (sm *StorageMinerAPI) SectorNumReserve(ctx context.Context, name string, field bitfield.BitField, force bool) error {
	return sm.Miner.NumReserve(ctx, name, field, force)
}

func (sm *StorageMinerAPI) SectorNumReserveCount(ctx context.Context, name string, count uint64) (bitfield.BitField, error) {
	return sm.Miner.NumReserveCount(ctx, name, count)
}

func (sm *StorageMinerAPI) SectorNumFree(ctx context.Context, name string) error {
	return sm.Miner.NumFree(ctx, name)
}

func (sm *StorageMinerAPI) SectorReceive(ctx context.Context, meta api.RemoteSectorMeta) error {
	if err := sm.Miner.Receive(ctx, meta); err != nil {
		return err
	}

	_, err := sm.waitSectorStarted(ctx, meta.Sector)
	return err
}

func (sm *StorageMinerAPI) ComputeWindowPoSt(ctx context.Context, dlIdx uint64, tsk types.TipSetKey) ([]minertypes.SubmitWindowedPoStParams, error) {
	var ts *types.TipSet
	var err error
	if tsk == types.EmptyTSK {
		ts, err = sm.Full.ChainHead(ctx)
	} else {
		ts, err = sm.Full.ChainGetTipSet(ctx, tsk)
	}
	if err != nil {
		return nil, err
	}

	return sm.WdPoSt.ComputePoSt(ctx, dlIdx, ts)
}

func (sm *StorageMinerAPI) ComputeDataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error) {
	return sm.StorageMgr.DataCid(ctx, pieceSize, pieceData)
}

func (sm *StorageMinerAPI) WorkerConnect(ctx context.Context, url string) error {
	w, err := connectRemoteWorker(ctx, sm, url)
	if err != nil {
		return xerrors.Errorf("connecting remote storage failed: %w", err)
	}

	log.Infof("Connected to a remote worker at %s", url)

	return sm.StorageMgr.AddWorker(ctx, w)
}

func (sm *StorageMinerAPI) SealingSchedDiag(ctx context.Context, doSched bool) (interface{}, error) {
	return sm.StorageMgr.SchedDiag(ctx, doSched)
}

func (sm *StorageMinerAPI) SealingAbort(ctx context.Context, call storiface.CallID) error {
	return sm.StorageMgr.Abort(ctx, call)
}

func (sm *StorageMinerAPI) SealingRemoveRequest(ctx context.Context, schedId uuid.UUID) error {
	return sm.StorageMgr.RemoveSchedRequest(ctx, schedId)
}

func (sm *StorageMinerAPI) MarketImportDealData(ctx context.Context, propCid cid.Cid, path string) error {
	fi, err := os.Open(path)
	if err != nil {
		return xerrors.Errorf("failed to open file: %w", err)
	}
	defer fi.Close() //nolint:errcheck

	return sm.StorageProvider.ImportDataForDeal(ctx, propCid, fi)
}

func (sm *StorageMinerAPI) listDeals(ctx context.Context) ([]*api.MarketDeal, error) {
	ts, err := sm.Full.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	tsk := ts.Key()
	allDeals, err := sm.Full.StateMarketDeals(ctx, tsk)
	if err != nil {
		return nil, err
	}

	var out []*api.MarketDeal

	for _, deal := range allDeals {
		if deal.Proposal.Provider == sm.Miner.Address() {
			out = append(out, deal)
		}
	}

	return out, nil
}

func (sm *StorageMinerAPI) MarketListDeals(ctx context.Context) ([]*api.MarketDeal, error) {
	return sm.listDeals(ctx)
}

func (sm *StorageMinerAPI) MarketListRetrievalDeals(ctx context.Context) ([]struct{}, error) {
	return []struct{}{}, nil
}

func (sm *StorageMinerAPI) MarketGetDealUpdates(ctx context.Context) (<-chan storagemarket.MinerDeal, error) {
	results := make(chan storagemarket.MinerDeal)
	unsub := sm.StorageProvider.SubscribeToEvents(func(evt storagemarket.ProviderEvent, deal storagemarket.MinerDeal) {
		select {
		case results <- deal:
		case <-ctx.Done():
		}
	})
	go func() {
		<-ctx.Done()
		unsub()
		close(results)
	}()
	return results, nil
}

func (sm *StorageMinerAPI) MarketListIncompleteDeals(ctx context.Context) ([]storagemarket.MinerDeal, error) {
	return sm.StorageProvider.ListLocalDeals()
}

func (sm *StorageMinerAPI) MarketSetAsk(ctx context.Context, price types.BigInt, verifiedPrice types.BigInt, duration abi.ChainEpoch, minPieceSize abi.PaddedPieceSize, maxPieceSize abi.PaddedPieceSize) error {
	options := []storagemarket.StorageAskOption{
		storagemarket.MinPieceSize(minPieceSize),
		storagemarket.MaxPieceSize(maxPieceSize),
	}

	return sm.StorageProvider.SetAsk(price, verifiedPrice, duration, options...)
}

func (sm *StorageMinerAPI) MarketGetAsk(ctx context.Context) (*storagemarket.SignedStorageAsk, error) {
	return sm.StorageProvider.GetAsk(), nil
}

func (sm *StorageMinerAPI) MarketSetRetrievalAsk(ctx context.Context, rask *retrievalmarket.Ask) error {
	sm.RetrievalProvider.SetAsk(rask)
	return nil
}

func (sm *StorageMinerAPI) MarketGetRetrievalAsk(ctx context.Context) (*retrievalmarket.Ask, error) {
	return sm.RetrievalProvider.GetAsk(), nil
}

func (sm *StorageMinerAPI) MarketListDataTransfers(ctx context.Context) ([]api.DataTransferChannel, error) {
	inProgressChannels, err := sm.DataTransfer.InProgressChannels(ctx)
	if err != nil {
		return nil, err
	}

	apiChannels := make([]api.DataTransferChannel, 0, len(inProgressChannels))
	for _, channelState := range inProgressChannels {
		apiChannels = append(apiChannels, api.NewDataTransferChannel(sm.Host.ID(), channelState))
	}

	return apiChannels, nil
}

func (sm *StorageMinerAPI) MarketRestartDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error {
	selfPeer := sm.Host.ID()
	if isInitiator {
		return sm.DataTransfer.RestartDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: selfPeer, Responder: otherPeer, ID: transferID})
	}
	return sm.DataTransfer.RestartDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: otherPeer, Responder: selfPeer, ID: transferID})
}

func (sm *StorageMinerAPI) MarketCancelDataTransfer(ctx context.Context, transferID datatransfer.TransferID, otherPeer peer.ID, isInitiator bool) error {
	selfPeer := sm.Host.ID()
	if isInitiator {
		return sm.DataTransfer.CloseDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: selfPeer, Responder: otherPeer, ID: transferID})
	}
	return sm.DataTransfer.CloseDataTransferChannel(ctx, datatransfer.ChannelID{Initiator: otherPeer, Responder: selfPeer, ID: transferID})
}

func (sm *StorageMinerAPI) MarketDataTransferUpdates(ctx context.Context) (<-chan api.DataTransferChannel, error) {
	channels := make(chan api.DataTransferChannel)

	unsub := sm.DataTransfer.SubscribeToEvents(func(evt datatransfer.Event, channelState datatransfer.ChannelState) {
		channel := api.NewDataTransferChannel(sm.Host.ID(), channelState)
		select {
		case <-ctx.Done():
		case channels <- channel:
		}
	})

	go func() {
		defer unsub()
		<-ctx.Done()
	}()

	return channels, nil
}

func (sm *StorageMinerAPI) MarketDataTransferDiagnostics(ctx context.Context, mpid peer.ID) (*api.TransferDiagnostics, error) {
	gsTransport, ok := sm.Transport.(*gst.Transport)
	if !ok {
		return nil, errors.New("api only works for graphsync as transport")
	}
	graphsyncConcrete, ok := sm.StagingGraphsync.(*gsimpl.GraphSync)
	if !ok {
		return nil, errors.New("api only works for non-mock graphsync implementation")
	}

	inProgressChannels, err := sm.DataTransfer.InProgressChannels(ctx)
	if err != nil {
		return nil, err
	}

	allReceivingChannels := make(map[datatransfer.ChannelID]datatransfer.ChannelState)
	allSendingChannels := make(map[datatransfer.ChannelID]datatransfer.ChannelState)
	for channelID, channel := range inProgressChannels {
		if channel.OtherPeer() != mpid {
			continue
		}
		if channel.Status() == datatransfer.Completed {
			continue
		}
		if channel.Status() == datatransfer.Failed || channel.Status() == datatransfer.Cancelled {
			continue
		}
		if channel.SelfPeer() == channel.Sender() {
			allSendingChannels[channelID] = channel
		} else {
			allReceivingChannels[channelID] = channel
		}
	}

	// gather information about active transport channels
	transportChannels := gsTransport.ChannelsForPeer(mpid)
	// gather information about graphsync state for peer
	gsPeerState := graphsyncConcrete.PeerState(mpid)

	sendingTransfers := sm.generateTransfers(ctx, transportChannels.SendingChannels, gsPeerState.IncomingState, allSendingChannels)
	receivingTransfers := sm.generateTransfers(ctx, transportChannels.ReceivingChannels, gsPeerState.OutgoingState, allReceivingChannels)

	return &api.TransferDiagnostics{
		SendingTransfers:   sendingTransfers,
		ReceivingTransfers: receivingTransfers,
	}, nil
}

// generate transfers matches graphsync state and data transfer state for a given peer
// to produce detailed output on what's happening with a transfer
func (sm *StorageMinerAPI) generateTransfers(ctx context.Context,
	transportChannels map[datatransfer.ChannelID]gst.ChannelGraphsyncRequests,
	gsPeerState peerstate.PeerState,
	allChannels map[datatransfer.ChannelID]datatransfer.ChannelState) []*api.GraphSyncDataTransfer {
	tc := &transferConverter{
		matchedChannelIds: make(map[datatransfer.ChannelID]struct{}),
		matchedRequests:   make(map[graphsync.RequestID]*api.GraphSyncDataTransfer),
		gsDiagnostics:     gsPeerState.Diagnostics(),
		requestStates:     gsPeerState.RequestStates,
		allChannels:       allChannels,
	}

	// iterate through all operating data transfer transport channels
	for channelID, channelRequests := range transportChannels {
		originalState, err := sm.DataTransfer.ChannelState(ctx, channelID)
		var baseDiagnostics []string
		var channelState *api.DataTransferChannel
		if err != nil {
			baseDiagnostics = append(baseDiagnostics, fmt.Sprintf("Unable to lookup channel state: %s", err))
		} else {
			cs := api.NewDataTransferChannel(sm.Host.ID(), originalState)
			channelState = &cs
		}
		// add the current request for this channel
		tc.convertTransfer(channelID, true, channelState, baseDiagnostics, channelRequests.Current, true)
		for _, requestID := range channelRequests.Previous {
			// add any previous requests that were cancelled for a restart
			tc.convertTransfer(channelID, true, channelState, baseDiagnostics, requestID, false)
		}
	}

	// collect any graphsync data for channels we don't have any data transfer data for
	tc.collectRemainingTransfers()

	return tc.transfers
}

type transferConverter struct {
	matchedChannelIds map[datatransfer.ChannelID]struct{}
	matchedRequests   map[graphsync.RequestID]*api.GraphSyncDataTransfer
	transfers         []*api.GraphSyncDataTransfer
	gsDiagnostics     map[graphsync.RequestID][]string
	requestStates     graphsync.RequestStates
	allChannels       map[datatransfer.ChannelID]datatransfer.ChannelState
}

// convert transfer assembles transfer and diagnostic data for a given graphsync/data-transfer request
func (tc *transferConverter) convertTransfer(channelID datatransfer.ChannelID, hasChannelID bool, channelState *api.DataTransferChannel, baseDiagnostics []string,
	requestID graphsync.RequestID, isCurrentChannelRequest bool) {
	diagnostics := baseDiagnostics
	state, hasState := tc.requestStates[requestID]
	stateString := state.String()
	if !hasState {
		stateString = "no graphsync state found"
	}
	var channelIDPtr *datatransfer.ChannelID
	if !hasChannelID {
		diagnostics = append(diagnostics, fmt.Sprintf("No data transfer channel id for GraphSync request ID %s", requestID))
	} else {
		channelIDPtr = &channelID
		if isCurrentChannelRequest && !hasState {
			diagnostics = append(diagnostics, fmt.Sprintf("No current request state for data transfer channel id %s", channelID))
		} else if !isCurrentChannelRequest && hasState {
			diagnostics = append(diagnostics, fmt.Sprintf("Graphsync request %s is a previous request on data transfer channel id %s that was restarted, but it is still running", requestID, channelID))
		}
	}
	diagnostics = append(diagnostics, tc.gsDiagnostics[requestID]...)
	transfer := &api.GraphSyncDataTransfer{
		RequestID:               &requestID,
		RequestState:            stateString,
		IsCurrentChannelRequest: isCurrentChannelRequest,
		ChannelID:               channelIDPtr,
		ChannelState:            channelState,
		Diagnostics:             diagnostics,
	}
	tc.transfers = append(tc.transfers, transfer)
	tc.matchedRequests[requestID] = transfer
	if hasChannelID {
		tc.matchedChannelIds[channelID] = struct{}{}
	}
}

func (tc *transferConverter) collectRemainingTransfers() {
	for requestID := range tc.requestStates {
		if _, ok := tc.matchedRequests[requestID]; !ok {
			tc.convertTransfer(datatransfer.ChannelID{}, false, nil, nil, requestID, false)
		}
	}
	for requestID := range tc.gsDiagnostics {
		if _, ok := tc.matchedRequests[requestID]; !ok {
			tc.convertTransfer(datatransfer.ChannelID{}, false, nil, nil, requestID, false)
		}
	}
	for channelID, channelState := range tc.allChannels {
		if _, ok := tc.matchedChannelIds[channelID]; !ok {
			channelID := channelID
			cs := api.NewDataTransferChannel(channelState.SelfPeer(), channelState)
			transfer := &api.GraphSyncDataTransfer{
				RequestID:               nil,
				RequestState:            "graphsync state unknown",
				IsCurrentChannelRequest: false,
				ChannelID:               &channelID,
				ChannelState:            &cs,
				Diagnostics:             []string{"data transfer with no open transport channel, cannot determine linked graphsync request"},
			}
			tc.transfers = append(tc.transfers, transfer)
		}
	}
}

func (sm *StorageMinerAPI) MarketPendingDeals(ctx context.Context) (api.PendingDealInfo, error) {
	return sm.DealPublisher.PendingDeals(), nil
}

func (sm *StorageMinerAPI) MarketRetryPublishDeal(ctx context.Context, propcid cid.Cid) error {
	return sm.StorageProvider.RetryDealPublishing(propcid)
}

func (sm *StorageMinerAPI) MarketPublishPendingDeals(ctx context.Context) error {
	sm.DealPublisher.ForcePublishPendingDeals()
	return nil
}

func (sm *StorageMinerAPI) DagstoreListShards(ctx context.Context) ([]api.DagstoreShardInfo, error) {
	if sm.DAGStore == nil {
		return nil, fmt.Errorf("dagstore not available on this node")
	}

	info := sm.DAGStore.AllShardsInfo()
	ret := make([]api.DagstoreShardInfo, 0, len(info))
	for k, i := range info {
		ret = append(ret, api.DagstoreShardInfo{
			Key:   k.String(),
			State: i.ShardState.String(),
			Error: func() string {
				if i.Error == nil {
					return ""
				}
				return i.Error.Error()
			}(),
		})
	}

	// order by key.
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Key < ret[j].Key
	})

	return ret, nil
}

func (sm *StorageMinerAPI) DagstoreRegisterShard(ctx context.Context, key string) error {
	if sm.DAGStore == nil {
		return fmt.Errorf("dagstore not available on this node")
	}

	// First check if the shard has already been registered
	k := shard.KeyFromString(key)
	_, err := sm.DAGStore.GetShardInfo(k)
	if err == nil {
		// Shard already registered, nothing further to do
		return nil
	}
	// If the shard is not registered we would expect ErrShardUnknown
	if !errors.Is(err, dagstore.ErrShardUnknown) {
		return fmt.Errorf("getting shard info from DAG store: %w", err)
	}

	pieceCid, err := cid.Parse(key)
	if err != nil {
		return fmt.Errorf("parsing shard key as piece cid: %w", err)
	}

	if err = filmktsstore.RegisterShardSync(ctx, sm.DAGStoreWrapper, pieceCid, "", true); err != nil {
		return fmt.Errorf("failed to register shard: %w", err)
	}

	return nil
}

func (sm *StorageMinerAPI) DagstoreInitializeShard(ctx context.Context, key string) error {
	if sm.DAGStore == nil {
		return fmt.Errorf("dagstore not available on this node")
	}

	k := shard.KeyFromString(key)

	info, err := sm.DAGStore.GetShardInfo(k)
	if err != nil {
		return fmt.Errorf("failed to get shard info: %w", err)
	}
	if st := info.ShardState; st != dagstore.ShardStateNew {
		return fmt.Errorf("cannot initialize shard; expected state ShardStateNew, was: %s", st.String())
	}

	ch := make(chan dagstore.ShardResult, 1)
	if err = sm.DAGStore.AcquireShard(ctx, k, ch, dagstore.AcquireOpts{}); err != nil {
		return fmt.Errorf("failed to acquire shard: %w", err)
	}

	var res dagstore.ShardResult
	select {
	case res = <-ch:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := res.Error; err != nil {
		return fmt.Errorf("failed to acquire shard: %w", err)
	}

	if res.Accessor != nil {
		err = res.Accessor.Close()
		if err != nil {
			log.Warnw("failed to close shard accessor; continuing", "shard_key", k, "error", err)
		}
	}

	return nil
}

func (sm *StorageMinerAPI) DagstoreInitializeAll(ctx context.Context, params api.DagstoreInitializeAllParams) (<-chan api.DagstoreInitializeAllEvent, error) {
	if sm.DAGStore == nil {
		return nil, fmt.Errorf("dagstore not available on this node")
	}

	if sm.SectorAccessor == nil {
		return nil, fmt.Errorf("sector accessor not available on this node")
	}

	// prepare the thottler tokens.
	var throttle chan struct{}
	if c := params.MaxConcurrency; c > 0 {
		throttle = make(chan struct{}, c)
		for i := 0; i < c; i++ {
			throttle <- struct{}{}
		}
	}

	// are we initializing only unsealed pieces?
	onlyUnsealed := !params.IncludeSealed

	info := sm.DAGStore.AllShardsInfo()
	var toInitialize []string
	for k, i := range info {
		if i.ShardState != dagstore.ShardStateNew {
			continue
		}

		// if we're initializing only unsealed pieces, check if there's an
		// unsealed deal for this piece available.
		if onlyUnsealed {
			pieceCid, err := cid.Decode(k.String())
			if err != nil {
				log.Warnw("DagstoreInitializeAll: failed to decode shard key as piece CID; skipping", "shard_key", k.String(), "error", err)
				continue
			}

			pi, err := sm.PieceStore.GetPieceInfo(pieceCid)
			if err != nil {
				log.Warnw("DagstoreInitializeAll: failed to get piece info; skipping", "piece_cid", pieceCid, "error", err)
				continue
			}

			var isUnsealed bool
			for _, d := range pi.Deals {
				isUnsealed, err = sm.SectorAccessor.IsUnsealed(ctx, d.SectorID, d.Offset.Unpadded(), d.Length.Unpadded())
				if err != nil {
					log.Warnw("DagstoreInitializeAll: failed to get unsealed status; skipping deal", "deal_id", d.DealID, "error", err)
					continue
				}
				if isUnsealed {
					break
				}
			}

			if !isUnsealed {
				log.Infow("DagstoreInitializeAll: skipping piece because it's sealed", "piece_cid", pieceCid, "error", err)
				continue
			}
		}

		// yes, we're initializing this shard.
		toInitialize = append(toInitialize, k.String())
	}

	total := len(toInitialize)
	if total == 0 {
		out := make(chan api.DagstoreInitializeAllEvent)
		close(out)
		return out, nil
	}

	// response channel must be closed when we're done, or the context is cancelled.
	// this buffering is necessary to prevent inflight children goroutines from
	// publishing to a closed channel (res) when the context is cancelled.
	out := make(chan api.DagstoreInitializeAllEvent, 32) // internal buffer.
	res := make(chan api.DagstoreInitializeAllEvent, 32) // returned to caller.

	// pump events back to caller.
	// two events per shard.
	go func() {
		defer close(res)

		for i := 0; i < total*2; i++ {
			select {
			case res <- <-out:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for i, k := range toInitialize {
			if throttle != nil {
				select {
				case <-throttle:
					// acquired a throttle token, proceed.
				case <-ctx.Done():
					return
				}
			}

			go func(k string, i int) {
				r := api.DagstoreInitializeAllEvent{
					Key:     k,
					Event:   "start",
					Total:   total,
					Current: i + 1, // start with 1
				}
				select {
				case out <- r:
				case <-ctx.Done():
					return
				}

				err := sm.DagstoreInitializeShard(ctx, k)

				if throttle != nil {
					throttle <- struct{}{}
				}

				r.Event = "end"
				if err == nil {
					r.Success = true
				} else {
					r.Success = false
					r.Error = err.Error()
				}

				select {
				case out <- r:
				case <-ctx.Done():
				}
			}(k, i)
		}
	}()

	return res, nil

}

func (sm *StorageMinerAPI) DagstoreRecoverShard(ctx context.Context, key string) error {
	if sm.DAGStore == nil {
		return fmt.Errorf("dagstore not available on this node")
	}

	k := shard.KeyFromString(key)

	info, err := sm.DAGStore.GetShardInfo(k)
	if err != nil {
		return fmt.Errorf("failed to get shard info: %w", err)
	}
	if st := info.ShardState; st != dagstore.ShardStateErrored {
		return fmt.Errorf("cannot recover shard; expected state ShardStateErrored, was: %s", st.String())
	}

	ch := make(chan dagstore.ShardResult, 1)
	if err = sm.DAGStore.RecoverShard(ctx, k, ch, dagstore.RecoverOpts{}); err != nil {
		return fmt.Errorf("failed to recover shard: %w", err)
	}

	var res dagstore.ShardResult
	select {
	case res = <-ch:
	case <-ctx.Done():
		return ctx.Err()
	}

	return res.Error
}

func (sm *StorageMinerAPI) DagstoreGC(ctx context.Context) ([]api.DagstoreShardResult, error) {
	if sm.DAGStore == nil {
		return nil, fmt.Errorf("dagstore not available on this node")
	}

	res, err := sm.DAGStore.GC(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to gc: %w", err)
	}

	ret := make([]api.DagstoreShardResult, 0, len(res.Shards))
	for k, err := range res.Shards {
		r := api.DagstoreShardResult{Key: k.String()}
		if err == nil {
			r.Success = true
		} else {
			r.Success = false
			r.Error = err.Error()
		}
		ret = append(ret, r)
	}

	return ret, nil
}

func (sm *StorageMinerAPI) IndexerAnnounceDeal(ctx context.Context, proposalCid cid.Cid) error {
	return sm.StorageProvider.AnnounceDealToIndexer(ctx, proposalCid)
}

func (sm *StorageMinerAPI) IndexerAnnounceAllDeals(ctx context.Context) error {
	return sm.StorageProvider.AnnounceAllDealsToIndexer(ctx)
}

func (sm *StorageMinerAPI) DagstoreLookupPieces(ctx context.Context, cid cid.Cid) ([]api.DagstoreShardInfo, error) {
	if sm.DAGStore == nil {
		return nil, fmt.Errorf("dagstore not available on this node")
	}

	keys, err := sm.DAGStore.TopLevelIndex.GetShardsForMultihash(ctx, cid.Hash())
	if err != nil {
		return nil, err
	}

	var ret []api.DagstoreShardInfo

	for _, k := range keys {
		shard, err := sm.DAGStore.GetShardInfo(k)
		if err != nil {
			return nil, err
		}

		ret = append(ret, api.DagstoreShardInfo{
			Key:   k.String(),
			State: shard.ShardState.String(),
			Error: func() string {
				if shard.Error == nil {
					return ""
				}
				return shard.Error.Error()
			}(),
		})
	}

	// order by key.
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Key < ret[j].Key
	})

	return ret, nil
}

func (sm *StorageMinerAPI) DealsList(ctx context.Context) ([]*api.MarketDeal, error) {
	return sm.listDeals(ctx)
}

func (sm *StorageMinerAPI) RetrievalDealsList(ctx context.Context) (map[retrievalmarket.ProviderDealIdentifier]retrievalmarket.ProviderDealState, error) {
	return sm.RetrievalProvider.ListDeals(), nil
}

func (sm *StorageMinerAPI) DealsConsiderOnlineStorageDeals(ctx context.Context) (bool, error) {
	return sm.ConsiderOnlineStorageDealsConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetConsiderOnlineStorageDeals(ctx context.Context, b bool) error {
	return sm.SetConsiderOnlineStorageDealsConfigFunc(b)
}

func (sm *StorageMinerAPI) DealsConsiderOnlineRetrievalDeals(ctx context.Context) (bool, error) {
	return sm.ConsiderOnlineRetrievalDealsConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetConsiderOnlineRetrievalDeals(ctx context.Context, b bool) error {
	return sm.SetConsiderOnlineRetrievalDealsConfigFunc(b)
}

func (sm *StorageMinerAPI) DealsConsiderOfflineStorageDeals(ctx context.Context) (bool, error) {
	return sm.ConsiderOfflineStorageDealsConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetConsiderOfflineStorageDeals(ctx context.Context, b bool) error {
	return sm.SetConsiderOfflineStorageDealsConfigFunc(b)
}

func (sm *StorageMinerAPI) DealsConsiderOfflineRetrievalDeals(ctx context.Context) (bool, error) {
	return sm.ConsiderOfflineRetrievalDealsConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetConsiderOfflineRetrievalDeals(ctx context.Context, b bool) error {
	return sm.SetConsiderOfflineRetrievalDealsConfigFunc(b)
}

func (sm *StorageMinerAPI) DealsConsiderVerifiedStorageDeals(ctx context.Context) (bool, error) {
	return sm.ConsiderVerifiedStorageDealsConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetConsiderVerifiedStorageDeals(ctx context.Context, b bool) error {
	return sm.SetConsiderVerifiedStorageDealsConfigFunc(b)
}

func (sm *StorageMinerAPI) DealsConsiderUnverifiedStorageDeals(ctx context.Context) (bool, error) {
	return sm.ConsiderUnverifiedStorageDealsConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetConsiderUnverifiedStorageDeals(ctx context.Context, b bool) error {
	return sm.SetConsiderUnverifiedStorageDealsConfigFunc(b)
}

func (sm *StorageMinerAPI) DealsGetExpectedSealDurationFunc(ctx context.Context) (time.Duration, error) {
	return sm.GetExpectedSealDurationFunc()
}

func (sm *StorageMinerAPI) DealsSetExpectedSealDurationFunc(ctx context.Context, d time.Duration) error {
	return sm.SetExpectedSealDurationFunc(d)
}

func (sm *StorageMinerAPI) DealsImportData(ctx context.Context, deal cid.Cid, fname string) error {
	fi, err := os.Open(fname)
	if err != nil {
		return xerrors.Errorf("failed to open given file: %w", err)
	}
	defer fi.Close() //nolint:errcheck

	return sm.StorageProvider.ImportDataForDeal(ctx, deal, fi)
}

func (sm *StorageMinerAPI) DealsPieceCidBlocklist(ctx context.Context) ([]cid.Cid, error) {
	return sm.StorageDealPieceCidBlocklistConfigFunc()
}

func (sm *StorageMinerAPI) DealsSetPieceCidBlocklist(ctx context.Context, cids []cid.Cid) error {
	return sm.SetStorageDealPieceCidBlocklistConfigFunc(cids)
}

func (sm *StorageMinerAPI) StorageAddLocal(ctx context.Context, path string) error {
	if sm.StorageMgr == nil {
		return xerrors.Errorf("no storage manager")
	}

	return sm.StorageMgr.AddLocalStorage(ctx, path)
}

func (sm *StorageMinerAPI) StorageDetachLocal(ctx context.Context, path string) error {
	if sm.StorageMgr == nil {
		return xerrors.Errorf("no storage manager")
	}

	return sm.StorageMgr.DetachLocalStorage(ctx, path)
}

func (sm *StorageMinerAPI) StorageRedeclareLocal(ctx context.Context, id *storiface.ID, dropMissing bool) error {
	if sm.StorageMgr == nil {
		return xerrors.Errorf("no storage manager")
	}

	return sm.StorageMgr.RedeclareLocalStorage(ctx, id, dropMissing)
}

func (sm *StorageMinerAPI) PiecesListPieces(ctx context.Context) ([]cid.Cid, error) {
	return sm.PieceStore.ListPieceInfoKeys()
}

func (sm *StorageMinerAPI) PiecesListCidInfos(ctx context.Context) ([]cid.Cid, error) {
	return sm.PieceStore.ListCidInfoKeys()
}

func (sm *StorageMinerAPI) PiecesGetPieceInfo(ctx context.Context, pieceCid cid.Cid) (*piecestore.PieceInfo, error) {
	pi, err := sm.PieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return nil, err
	}
	return &pi, nil
}

func (sm *StorageMinerAPI) PiecesGetCIDInfo(ctx context.Context, payloadCid cid.Cid) (*piecestore.CIDInfo, error) {
	ci, err := sm.PieceStore.GetCIDInfo(payloadCid)
	if err != nil {
		return nil, err
	}

	return &ci, nil
}

func (sm *StorageMinerAPI) CreateBackup(ctx context.Context, fpath string) error {
	return backup(ctx, sm.DS, fpath)
}

func (sm *StorageMinerAPI) CheckProvable(ctx context.Context, pp abi.RegisteredPoStProof, sectors []storiface.SectorRef) (map[abi.SectorNumber]string, error) {
	rg := func(ctx context.Context, id abi.SectorID) (cid.Cid, bool, error) {
		si, err := sm.Miner.SectorsStatus(ctx, id.Number, false)
		if err != nil {
			return cid.Undef, false, err
		}
		if si.CommR == nil {
			return cid.Undef, false, xerrors.Errorf("commr is nil")
		}

		return *si.CommR, si.ReplicaUpdateMessage != nil, nil
	}

	bad, err := sm.StorageMgr.CheckProvable(ctx, pp, sectors, rg)
	if err != nil {
		return nil, err
	}

	var out = make(map[abi.SectorNumber]string)
	for sid, err := range bad {
		out[sid.Number] = err
	}

	return out, nil
}

func (sm *StorageMinerAPI) ActorAddressConfig(ctx context.Context) (api.AddressConfig, error) {
	return sm.AddrSel.AddressConfig, nil
}

func (sm *StorageMinerAPI) Discover(ctx context.Context) (apitypes.OpenRPCDocument, error) {
	return build.OpenRPCDiscoverJSON_Miner(), nil
}

func (sm *StorageMinerAPI) ComputeProof(ctx context.Context, ssi []builtin.ExtendedSectorInfo, rand abi.PoStRandomness, poStEpoch abi.ChainEpoch, nv network.Version) ([]builtin.PoStProof, error) {
	return sm.Epp.ComputeProof(ctx, ssi, rand, poStEpoch, nv)
}

func (sm *StorageMinerAPI) RecoverFault(ctx context.Context, sectors []abi.SectorNumber) ([]cid.Cid, error) {
	allsectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, xerrors.Errorf("could not get a list of all sectors from the miner: %w", err)
	}
	var found bool
	for _, v := range sectors {
		found = false
		for _, s := range allsectors {
			if v == s.SectorNumber {
				found = true
				break
			}
		}
		if !found {
			return nil, xerrors.Errorf("sectors %d not found in the sector list for miner", v)
		}
	}
	return sm.WdPoSt.ManualFaultRecovery(ctx, sm.Miner.Address(), sectors)
}

func (sm *StorageMinerAPI) RuntimeSubsystems(context.Context) (res api.MinerSubsystems, err error) {
	return sm.EnabledSubsystems, nil
}

func (sm *StorageMinerAPI) ActorWithdrawBalance(ctx context.Context, amount abi.TokenAmount) (cid.Cid, error) {
	return sm.withdrawBalance(ctx, amount, true)
}

func (sm *StorageMinerAPI) BeneficiaryWithdrawBalance(ctx context.Context, amount abi.TokenAmount) (cid.Cid, error) {
	return sm.withdrawBalance(ctx, amount, false)
}

func (sm *StorageMinerAPI) withdrawBalance(ctx context.Context, amount abi.TokenAmount, fromOwner bool) (cid.Cid, error) {
	available, err := sm.Full.StateMinerAvailableBalance(ctx, sm.Miner.Address(), types.EmptyTSK)
	if err != nil {
		return cid.Undef, xerrors.Errorf("Error getting miner balance: %w", err)
	}

	if amount.GreaterThan(available) {
		return cid.Undef, xerrors.Errorf("can't withdraw more funds than available; requested: %s; available: %s", types.FIL(amount), types.FIL(available))
	}

	if amount.Equals(big.Zero()) {
		amount = available
	}

	params, err := actors.SerializeParams(&minertypes.WithdrawBalanceParams{
		AmountRequested: amount,
	})
	if err != nil {
		return cid.Undef, err
	}

	mi, err := sm.Full.StateMinerInfo(ctx, sm.Miner.Address(), types.EmptyTSK)
	if err != nil {
		return cid.Undef, xerrors.Errorf("Error getting miner's owner address: %w", err)
	}

	var sender address.Address
	if fromOwner {
		sender = mi.Owner
	} else {
		sender = mi.Beneficiary
	}

	smsg, err := sm.Full.MpoolPushMessage(ctx, &types.Message{
		To:     sm.Miner.Address(),
		From:   sender,
		Value:  types.NewInt(0),
		Method: builtintypes.MethodsMiner.WithdrawBalance,
		Params: params,
	}, nil)
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}
