package impl

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/host"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	retrievalmarket "github.com/filecoin-project/go-fil-markets/retrievalmarket"
	storagemarket "github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
)

type StorageMinerAPI struct {
	common.CommonAPI

	ProofsConfig *ffiwrapper.Config
	SectorBlocks *sectorblocks.SectorBlocks

	PieceStore        dtypes.ProviderPieceStore
	StorageProvider   storagemarket.StorageProvider
	RetrievalProvider retrievalmarket.RetrievalProvider
	Miner             *storage.Miner
	BlockMiner        *miner.Miner
	Full              api.FullNode
	StorageMgr        *sectorstorage.Manager `optional:"true"`
	IStorageMgr       sectorstorage.SectorManager
	*stores.Index
	DataTransfer dtypes.ProviderDataTransfer
	Host         host.Host

	DS dtypes.MetadataDS

	ConsiderOnlineStorageDealsConfigFunc       dtypes.ConsiderOnlineStorageDealsConfigFunc
	SetConsiderOnlineStorageDealsConfigFunc    dtypes.SetConsiderOnlineStorageDealsConfigFunc
	ConsiderOnlineRetrievalDealsConfigFunc     dtypes.ConsiderOnlineRetrievalDealsConfigFunc
	SetConsiderOnlineRetrievalDealsConfigFunc  dtypes.SetConsiderOnlineRetrievalDealsConfigFunc
	StorageDealPieceCidBlocklistConfigFunc     dtypes.StorageDealPieceCidBlocklistConfigFunc
	SetStorageDealPieceCidBlocklistConfigFunc  dtypes.SetStorageDealPieceCidBlocklistConfigFunc
	ConsiderOfflineStorageDealsConfigFunc      dtypes.ConsiderOfflineStorageDealsConfigFunc
	SetConsiderOfflineStorageDealsConfigFunc   dtypes.SetConsiderOfflineStorageDealsConfigFunc
	ConsiderOfflineRetrievalDealsConfigFunc    dtypes.ConsiderOfflineRetrievalDealsConfigFunc
	SetConsiderOfflineRetrievalDealsConfigFunc dtypes.SetConsiderOfflineRetrievalDealsConfigFunc
	SetSealingConfigFunc                       dtypes.SetSealingConfigFunc
	GetSealingConfigFunc                       dtypes.GetSealingConfigFunc
	GetExpectedSealDurationFunc                dtypes.GetExpectedSealDurationFunc
	SetExpectedSealDurationFunc                dtypes.SetExpectedSealDurationFunc
}

func (sm *StorageMinerAPI) ServeRemote(w http.ResponseWriter, r *http.Request) {
	if !auth.HasPerm(r.Context(), nil, apistruct.PermAdmin) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
		return
	}

	sm.StorageMgr.ServeHTTP(w, r)
}

func (sm *StorageMinerAPI) WorkerStats(context.Context) (map[uint64]storiface.WorkerStats, error) {
	return sm.StorageMgr.WorkerStats(), nil
}

func (sm *StorageMinerAPI) WorkerJobs(ctx context.Context) (map[uint64][]storiface.WorkerJob, error) {
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

func (sm *StorageMinerAPI) PledgeSector(ctx context.Context) error {
	return sm.Miner.PledgeSector()
}

func (sm *StorageMinerAPI) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	info, err := sm.Miner.GetSectorInfo(sid)
	if err != nil {
		return api.SectorInfo{}, err
	}

	deals := make([]abi.DealID, len(info.Pieces))
	for i, piece := range info.Pieces {
		if piece.DealInfo == nil {
			continue
		}
		deals[i] = piece.DealInfo.DealID
	}

	log := make([]api.SectorLog, len(info.Log))
	for i, l := range info.Log {
		log[i] = api.SectorLog{
			Kind:      l.Kind,
			Timestamp: l.Timestamp,
			Trace:     l.Trace,
			Message:   l.Message,
		}
	}

	sInfo := api.SectorInfo{
		SectorID: sid,
		State:    api.SectorState(info.State),
		CommD:    info.CommD,
		CommR:    info.CommR,
		Proof:    info.Proof,
		Deals:    deals,
		Ticket: api.SealTicket{
			Value: info.TicketValue,
			Epoch: info.TicketEpoch,
		},
		Seed: api.SealSeed{
			Value: info.SeedValue,
			Epoch: info.SeedEpoch,
		},
		PreCommitMsg: info.PreCommitMessage,
		CommitMsg:    info.CommitMessage,
		Retries:      info.InvalidProofs,
		ToUpgrade:    sm.Miner.IsMarkedForUpgrade(sid),

		LastErr: info.LastErr,
		Log:     log,
		// on chain info
		SealProof:          0,
		Activation:         0,
		Expiration:         0,
		DealWeight:         big.Zero(),
		VerifiedDealWeight: big.Zero(),
		InitialPledge:      big.Zero(),
		OnTime:             0,
		Early:              0,
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

// List all staged sectors
func (sm *StorageMinerAPI) SectorsList(context.Context) ([]abi.SectorNumber, error) {
	sectors, err := sm.Miner.ListSectors()
	if err != nil {
		return nil, err
	}

	out := make([]abi.SectorNumber, len(sectors))
	for i, sector := range sectors {
		out[i] = sector.SectorNumber
	}
	return out, nil
}

func (sm *StorageMinerAPI) StorageLocal(ctx context.Context) (map[stores.ID]string, error) {
	return sm.StorageMgr.StorageLocal(ctx)
}

func (sm *StorageMinerAPI) SectorsRefs(context.Context) (map[string][]api.SealedRef, error) {
	// json can't handle cids as map keys
	out := map[string][]api.SealedRef{}

	refs, err := sm.SectorBlocks.List()
	if err != nil {
		return nil, err
	}

	for k, v := range refs {
		out[strconv.FormatUint(k, 10)] = v
	}

	return out, nil
}

func (sm *StorageMinerAPI) StorageStat(ctx context.Context, id stores.ID) (fsutil.FsStat, error) {
	return sm.StorageMgr.FsStat(ctx, id)
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

func (sm *StorageMinerAPI) SectorMarkForUpgrade(ctx context.Context, id abi.SectorNumber) error {
	return sm.Miner.MarkForUpgrade(id)
}

func (sm *StorageMinerAPI) WorkerConnect(ctx context.Context, url string) error {
	w, err := connectRemoteWorker(ctx, sm, url)
	if err != nil {
		return xerrors.Errorf("connecting remote storage failed: %w", err)
	}

	log.Infof("Connected to a remote worker at %s", url)

	return sm.StorageMgr.AddWorker(ctx, w)
}

func (sm *StorageMinerAPI) SealingSchedDiag(ctx context.Context) (interface{}, error) {
	return sm.StorageMgr.SchedDiag(ctx)
}

func (sm *StorageMinerAPI) MarketImportDealData(ctx context.Context, propCid cid.Cid, path string) error {
	fi, err := os.Open(path)
	if err != nil {
		return xerrors.Errorf("failed to open file: %w", err)
	}
	defer fi.Close() //nolint:errcheck

	return sm.StorageProvider.ImportDataForDeal(ctx, propCid, fi)
}

func (sm *StorageMinerAPI) listDeals(ctx context.Context) ([]api.MarketDeal, error) {
	ts, err := sm.Full.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	tsk := ts.Key()
	allDeals, err := sm.Full.StateMarketDeals(ctx, tsk)
	if err != nil {
		return nil, err
	}

	var out []api.MarketDeal

	for _, deal := range allDeals {
		if deal.Proposal.Provider == sm.Miner.Address() {
			out = append(out, deal)
		}
	}

	return out, nil
}

func (sm *StorageMinerAPI) MarketListDeals(ctx context.Context) ([]api.MarketDeal, error) {
	return sm.listDeals(ctx)
}

func (sm *StorageMinerAPI) MarketListRetrievalDeals(ctx context.Context) ([]retrievalmarket.ProviderDealState, error) {
	var out []retrievalmarket.ProviderDealState
	deals := sm.RetrievalProvider.ListDeals()

	for _, deal := range deals {
		out = append(out, deal)
	}

	return out, nil
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

func (sm *StorageMinerAPI) DealsList(ctx context.Context) ([]api.MarketDeal, error) {
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
	return backup(sm.DS, fpath)
}

var _ api.StorageMiner = &StorageMinerAPI{}
