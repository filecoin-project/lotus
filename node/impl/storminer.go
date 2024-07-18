package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
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
	SectorBlocks *sectorblocks.SectorBlocks `optional:"true"`

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

	SetSealingConfigFunc        dtypes.SetSealingConfigFunc        `optional:"true"`
	GetSealingConfigFunc        dtypes.GetSealingConfigFunc        `optional:"true"`
	GetExpectedSealDurationFunc dtypes.GetExpectedSealDurationFunc `optional:"true"`
	SetExpectedSealDurationFunc dtypes.SetExpectedSealDurationFunc `optional:"true"`

	HarmonyDB *harmonydb.DB `optional:"true"`
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

func (sm *StorageMinerAPI) SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d piece.PieceDealInfo) (api.SectorOffset, error) {
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

func (sm *StorageMinerAPI) SectorUnseal(ctx context.Context, sectorNum abi.SectorNumber) error {

	status, err := sm.Miner.SectorsStatus(ctx, sectorNum, false)
	if err != nil {
		return err
	}

	minerAddr, err := sm.ActorAddress(ctx)
	if err != nil {
		return err
	}
	minerID, err := address.IDFromAddress(minerAddr)
	if err != nil {
		return err
	}

	sector := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(minerID),
			Number: sectorNum,
		},
		ProofType: status.SealProof,
	}

	bgCtx := context.Background()

	go func() {
		err := sm.StorageMgr.SectorsUnsealPiece(bgCtx, sector, storiface.UnpaddedByteIndex(0), abi.UnpaddedPieceSize(0), status.Ticket.Value, status.CommD)
		if err != nil {
			log.Errorf("unseal for sector %d failed: %+v", sectorNum, err)
		}
	}()

	return nil
}

// SectorsList lists all staged sectors
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
	// Use SectorsSummary from stats (prometheus) for faster result
	return sm.Miner.SectorsSummary(ctx), nil
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

func (sm *StorageMinerAPI) ComputeWindowPoSt(ctx context.Context, dlIdx uint64, tsk types.TipSetKey) ([]lminer.SubmitWindowedPoStParams, error) {
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
	return sm.IStorageMgr.DataCid(ctx, pieceSize, pieceData)
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

func (sm *StorageMinerAPI) DealsGetExpectedSealDurationFunc(ctx context.Context) (time.Duration, error) {
	return sm.GetExpectedSealDurationFunc()
}

func (sm *StorageMinerAPI) DealsSetExpectedSealDurationFunc(ctx context.Context, d time.Duration) error {
	return sm.SetExpectedSealDurationFunc(d)
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
	return WithdrawBalance(ctx, sm.Full, sm.Miner.Address(), amount, true)
}

func (sm *StorageMinerAPI) BeneficiaryWithdrawBalance(ctx context.Context, amount abi.TokenAmount) (cid.Cid, error) {
	return WithdrawBalance(ctx, sm.Full, sm.Miner.Address(), amount, false)
}

func WithdrawBalance(ctx context.Context, full api.FullNode, maddr address.Address, amount abi.TokenAmount, fromOwner bool) (cid.Cid, error) {
	available, err := full.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return cid.Undef, xerrors.Errorf("Error getting miner balance: %w", err)
	}

	if amount.GreaterThan(available) {
		return cid.Undef, xerrors.Errorf("can't withdraw more funds than available; requested: %s; available: %s", types.FIL(amount), types.FIL(available))
	}

	if amount.Equals(big.Zero()) {
		amount = available
	}

	params, err := actors.SerializeParams(&lminer.WithdrawBalanceParams{
		AmountRequested: amount,
	})
	if err != nil {
		return cid.Undef, err
	}

	mi, err := full.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return cid.Undef, xerrors.Errorf("Error getting miner's owner address: %w", err)
	}

	var sender address.Address
	if fromOwner {
		sender = mi.Owner
	} else {
		sender = mi.Beneficiary
	}

	smsg, err := full.MpoolPushMessage(ctx, &types.Message{
		To:     maddr,
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
