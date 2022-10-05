package sealing

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var aggFeeNum = big.NewInt(110)
var aggFeeDen = big.NewInt(100)

//go:generate go run github.com/golang/mock/mockgen -destination=mocks/mock_commit_batcher.go -package=mocks . CommitBatcherApi

type CommitBatcherApi interface {
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)

	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, miner.SectorPreCommitInfo, types.TipSetKey) (big.Int, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (big.Int, error)

	// Address selector
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

type AggregateInput struct {
	Spt   abi.RegisteredSealProof
	Info  proof.AggregateSealVerifyInfo
	Proof []byte
}

type CommitBatcher struct {
	api       CommitBatcherApi
	maddr     address.Address
	mctx      context.Context
	addrSel   AddressSelector
	feeCfg    config.MinerFeeConfig
	getConfig dtypes.GetSealingConfigFunc
	prover    storiface.Prover

	cutoffs map[abi.SectorNumber]time.Time
	todo    map[abi.SectorNumber]AggregateInput
	waiting map[abi.SectorNumber][]chan sealiface.CommitBatchRes

	notify, stop, stopped chan struct{}
	force                 chan chan []sealiface.CommitBatchRes
	lk                    sync.Mutex
}

func NewCommitBatcher(mctx context.Context, maddr address.Address, api CommitBatcherApi, addrSel AddressSelector, feeCfg config.MinerFeeConfig, getConfig dtypes.GetSealingConfigFunc, prov storiface.Prover) *CommitBatcher {
	b := &CommitBatcher{
		api:       api,
		maddr:     maddr,
		mctx:      mctx,
		addrSel:   addrSel,
		feeCfg:    feeCfg,
		getConfig: getConfig,
		prover:    prov,

		cutoffs: map[abi.SectorNumber]time.Time{},
		todo:    map[abi.SectorNumber]AggregateInput{},
		waiting: map[abi.SectorNumber][]chan sealiface.CommitBatchRes{},

		notify:  make(chan struct{}, 1),
		force:   make(chan chan []sealiface.CommitBatchRes),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	go b.run()

	return b
}

func (b *CommitBatcher) run() {
	var forceRes chan []sealiface.CommitBatchRes
	var lastMsg []sealiface.CommitBatchRes

	cfg, err := b.getConfig()
	if err != nil {
		panic(err)
	}

	timer := time.NewTimer(b.batchWait(cfg.CommitBatchWait, cfg.CommitBatchSlack))
	for {
		if forceRes != nil {
			forceRes <- lastMsg
			forceRes = nil
		}
		lastMsg = nil

		// indicates whether we should only start a batch if we have reached or exceeded cfg.MaxCommitBatch
		var sendAboveMax bool
		select {
		case <-b.stop:
			close(b.stopped)
			return
		case <-b.notify:
			sendAboveMax = true
		case <-timer.C:
			// do nothing
		case fr := <-b.force: // user triggered
			forceRes = fr
		}

		var err error
		lastMsg, err = b.maybeStartBatch(sendAboveMax)
		if err != nil {
			log.Warnw("CommitBatcher processBatch error", "error", err)
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		timer.Reset(b.batchWait(cfg.CommitBatchWait, cfg.CommitBatchSlack))
	}
}

func (b *CommitBatcher) batchWait(maxWait, slack time.Duration) time.Duration {
	now := time.Now()

	b.lk.Lock()
	defer b.lk.Unlock()

	if len(b.todo) == 0 {
		return maxWait
	}

	var cutoff time.Time
	for sn := range b.todo {
		sectorCutoff := b.cutoffs[sn]
		if cutoff.IsZero() || (!sectorCutoff.IsZero() && sectorCutoff.Before(cutoff)) {
			cutoff = sectorCutoff
		}
	}
	for sn := range b.waiting {
		sectorCutoff := b.cutoffs[sn]
		if cutoff.IsZero() || (!sectorCutoff.IsZero() && sectorCutoff.Before(cutoff)) {
			cutoff = sectorCutoff
		}
	}

	if cutoff.IsZero() {
		return maxWait
	}

	cutoff = cutoff.Add(-slack)
	if cutoff.Before(now) {
		return time.Nanosecond // can't return 0
	}

	wait := cutoff.Sub(now)
	if wait > maxWait {
		wait = maxWait
	}

	return wait
}

func (b *CommitBatcher) maybeStartBatch(notif bool) ([]sealiface.CommitBatchRes, error) {
	b.lk.Lock()
	defer b.lk.Unlock()

	total := len(b.todo)
	if total == 0 {
		return nil, nil // nothing to do
	}

	cfg, err := b.getConfig()
	if err != nil {
		return nil, xerrors.Errorf("getting config: %w", err)
	}

	if notif && total < cfg.MaxCommitBatch {
		return nil, nil
	}

	var res []sealiface.CommitBatchRes

	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return nil, err
	}

	blackedOut := func() bool {
		const nv16BlackoutWindow = abi.ChainEpoch(20) // a magik number
		if ts.Height() <= build.UpgradeSkyrHeight && build.UpgradeSkyrHeight-ts.Height() < nv16BlackoutWindow {
			return true
		}
		return false
	}

	individual := (total < cfg.MinCommitBatch) || (total < miner.MinAggregatedSectors) || blackedOut()

	if !individual && !cfg.AggregateAboveBaseFee.Equals(big.Zero()) {
		if ts.MinTicketBlock().ParentBaseFee.LessThan(cfg.AggregateAboveBaseFee) {
			individual = true
		}
	}

	if individual {
		res, err = b.processIndividually(cfg)
	} else {
		res, err = b.processBatch(cfg)
	}

	if err != nil {
		log.Warnf("CommitBatcher maybeStartBatch individual:%v processBatch %v", individual, err)
	}

	if err != nil && len(res) == 0 {
		return nil, err
	}

	for _, r := range res {
		if err != nil {
			r.Error = err.Error()
		}

		for _, sn := range r.Sectors {
			for _, ch := range b.waiting[sn] {
				ch <- r // buffered
			}

			delete(b.waiting, sn)
			delete(b.todo, sn)
			delete(b.cutoffs, sn)
		}
	}

	return res, nil
}

func (b *CommitBatcher) processBatch(cfg sealiface.Config) ([]sealiface.CommitBatchRes, error) {
	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return nil, err
	}

	total := len(b.todo)

	res := sealiface.CommitBatchRes{
		FailedSectors: map[abi.SectorNumber]string{},
	}

	params := miner.ProveCommitAggregateParams{
		SectorNumbers: bitfield.New(),
	}

	proofs := make([][]byte, 0, total)
	infos := make([]proof.AggregateSealVerifyInfo, 0, total)
	collateral := big.Zero()

	for id, p := range b.todo {
		if len(infos) >= cfg.MaxCommitBatch {
			log.Infow("commit batch full")
			break
		}

		res.Sectors = append(res.Sectors, id)

		sc, err := b.getSectorCollateral(id, ts.Key())
		if err != nil {
			res.FailedSectors[id] = err.Error()
			continue
		}

		collateral = big.Add(collateral, sc)

		params.SectorNumbers.Set(uint64(id))
		infos = append(infos, p.Info)
	}

	if len(infos) == 0 {
		return nil, nil
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Number < infos[j].Number
	})

	for _, info := range infos {
		proofs = append(proofs, b.todo[info.Number].Proof)
	}

	mid, err := address.IDFromAddress(b.maddr)
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting miner id: %w", err)
	}

	nv, err := b.api.StateNetworkVersion(b.mctx, ts.Key())
	if err != nil {
		log.Errorf("getting network version: %s", err)
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting network version: %s", err)
	}

	arp, err := b.aggregateProofType(nv)
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting aggregate proof type: %w", err)
	}

	params.AggregateProof, err = b.prover.AggregateSealProofs(proof.AggregateSealVerifyProofAndInfos{
		Miner:          abi.ActorID(mid),
		SealProof:      b.todo[infos[0].Number].Spt,
		AggregateProof: arp,
		Infos:          infos,
	}, proofs)
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("aggregating proofs: %w", err)
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("couldn't serialize ProveCommitAggregateParams: %w", err)
	}

	mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, types.EmptyTSK)
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("couldn't get miner info: %w", err)
	}

	maxFee := b.feeCfg.MaxCommitBatchGasFee.FeeForSectors(len(infos))

	aggFeeRaw, err := policy.AggregateProveCommitNetworkFee(nv, len(infos), ts.MinTicketBlock().ParentBaseFee)
	if err != nil {
		log.Errorf("getting aggregate commit network fee: %s", err)
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting aggregate commit network fee: %s", err)
	}

	aggFee := big.Div(big.Mul(aggFeeRaw, aggFeeNum), aggFeeDen)

	needFunds := big.Add(collateral, aggFee)
	needFunds, err = collateralSendAmount(b.mctx, b.api, b.maddr, cfg, needFunds)
	if err != nil {
		return []sealiface.CommitBatchRes{res}, err
	}

	goodFunds := big.Add(maxFee, needFunds)

	from, _, err := b.addrSel.AddressFor(b.mctx, b.api, mi, api.CommitAddr, goodFunds, needFunds)
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("no good address found: %w", err)
	}

	mcid, err := sendMsg(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.ProveCommitAggregate, needFunds, maxFee, enc.Bytes())
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("sending message failed: %w", err)
	}

	res.Msg = &mcid

	log.Infow("Sent ProveCommitAggregate message", "cid", mcid, "from", from, "todo", total, "sectors", len(infos))

	return []sealiface.CommitBatchRes{res}, nil
}

func (b *CommitBatcher) processIndividually(cfg sealiface.Config) ([]sealiface.CommitBatchRes, error) {
	mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get miner info: %w", err)
	}

	avail := types.TotalFilecoinInt

	if cfg.CollateralFromMinerBalance && !cfg.DisableCollateralFallback {
		avail, err = b.api.StateMinerAvailableBalance(b.mctx, b.maddr, types.EmptyTSK)
		if err != nil {
			return nil, xerrors.Errorf("getting available miner balance: %w", err)
		}

		avail = big.Sub(avail, cfg.AvailableBalanceBuffer)
		if avail.LessThan(big.Zero()) {
			avail = big.Zero()
		}
	}

	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return nil, err
	}

	var res []sealiface.CommitBatchRes

	for sn, info := range b.todo {
		r := sealiface.CommitBatchRes{
			Sectors:       []abi.SectorNumber{sn},
			FailedSectors: map[abi.SectorNumber]string{},
		}

		mcid, err := b.processSingle(cfg, mi, &avail, sn, info, ts.Key())
		if err != nil {
			log.Errorf("process single error: %+v", err) // todo: return to user
			r.FailedSectors[sn] = err.Error()
		} else {
			r.Msg = &mcid
		}

		res = append(res, r)
	}

	return res, nil
}

func (b *CommitBatcher) processSingle(cfg sealiface.Config, mi api.MinerInfo, avail *abi.TokenAmount, sn abi.SectorNumber, info AggregateInput, tsk types.TipSetKey) (cid.Cid, error) {
	enc := new(bytes.Buffer)
	params := &miner.ProveCommitSectorParams{
		SectorNumber: sn,
		Proof:        info.Proof,
	}

	if err := params.MarshalCBOR(enc); err != nil {
		return cid.Undef, xerrors.Errorf("marshaling commit params: %w", err)
	}

	collateral, err := b.getSectorCollateral(sn, tsk)
	if err != nil {
		return cid.Undef, err
	}

	if cfg.CollateralFromMinerBalance {
		c := big.Sub(collateral, *avail)
		*avail = big.Sub(*avail, collateral)
		collateral = c

		if collateral.LessThan(big.Zero()) {
			collateral = big.Zero()
		}
		if (*avail).LessThan(big.Zero()) {
			*avail = big.Zero()
		}
	}

	goodFunds := big.Add(collateral, big.Int(b.feeCfg.MaxCommitGasFee))

	from, _, err := b.addrSel.AddressFor(b.mctx, b.api, mi, api.CommitAddr, goodFunds, collateral)
	if err != nil {
		return cid.Undef, xerrors.Errorf("no good address to send commit message from: %w", err)
	}

	mcid, err := sendMsg(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.ProveCommitSector, collateral, big.Int(b.feeCfg.MaxCommitGasFee), enc.Bytes())
	if err != nil {
		return cid.Undef, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	return mcid, nil
}

// register commit, wait for batch message, return message CID
func (b *CommitBatcher) AddCommit(ctx context.Context, s SectorInfo, in AggregateInput) (res sealiface.CommitBatchRes, err error) {
	sn := s.SectorNumber

	cu, err := b.getCommitCutoff(s)
	if err != nil {
		return sealiface.CommitBatchRes{}, err
	}

	b.lk.Lock()
	b.cutoffs[sn] = cu
	b.todo[sn] = in

	sent := make(chan sealiface.CommitBatchRes, 1)
	b.waiting[sn] = append(b.waiting[sn], sent)

	select {
	case b.notify <- struct{}{}:
	default: // already have a pending notification, don't need more
	}
	b.lk.Unlock()

	select {
	case r := <-sent:
		return r, nil
	case <-ctx.Done():
		return sealiface.CommitBatchRes{}, ctx.Err()
	}
}

func (b *CommitBatcher) Flush(ctx context.Context) ([]sealiface.CommitBatchRes, error) {
	resCh := make(chan []sealiface.CommitBatchRes, 1)
	select {
	case b.force <- resCh:
		select {
		case res := <-resCh:
			return res, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *CommitBatcher) Pending(ctx context.Context) ([]abi.SectorID, error) {
	b.lk.Lock()
	defer b.lk.Unlock()

	mid, err := address.IDFromAddress(b.maddr)
	if err != nil {
		return nil, err
	}

	res := make([]abi.SectorID, 0)
	for _, s := range b.todo {
		res = append(res, abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: s.Info.Number,
		})
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].Miner != res[j].Miner {
			return res[i].Miner < res[j].Miner
		}

		return res[i].Number < res[j].Number
	})

	return res, nil
}

func (b *CommitBatcher) Stop(ctx context.Context) error {
	close(b.stop)

	select {
	case <-b.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TODO: If this returned epochs, it would make testing much easier
func (b *CommitBatcher) getCommitCutoff(si SectorInfo) (time.Time, error) {
	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return time.Now(), xerrors.Errorf("getting chain head: %s", err)
	}

	nv, err := b.api.StateNetworkVersion(b.mctx, ts.Key())
	if err != nil {
		log.Errorf("getting network version: %s", err)
		return time.Now(), xerrors.Errorf("getting network version: %s", err)
	}

	pci, err := b.api.StateSectorPreCommitInfo(b.mctx, b.maddr, si.SectorNumber, ts.Key())
	if err != nil {
		log.Errorf("getting precommit info: %s", err)
		return time.Now(), err
	}
	if pci == nil {
		return time.Now(), xerrors.Errorf("precommit info not found")
	}
	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		log.Errorf("unsupported network vrsion: %s", err)
		return time.Now(), err
	}
	mpcd, err := policy.GetMaxProveCommitDuration(av, si.SectorType)
	if err != nil {
		log.Errorf("getting max prove commit duration: %s", err)
		return time.Now(), err
	}

	cutoffEpoch := pci.PreCommitEpoch + mpcd

	for _, p := range si.Pieces {
		if p.DealInfo == nil {
			continue
		}

		startEpoch := p.DealInfo.DealSchedule.StartEpoch
		if startEpoch < cutoffEpoch {
			cutoffEpoch = startEpoch
		}
	}

	if cutoffEpoch <= ts.Height() {
		return time.Now(), nil
	}

	return time.Now().Add(time.Duration(cutoffEpoch-ts.Height()) * time.Duration(build.BlockDelaySecs) * time.Second), nil
}

func (b *CommitBatcher) getSectorCollateral(sn abi.SectorNumber, tsk types.TipSetKey) (abi.TokenAmount, error) {
	pci, err := b.api.StateSectorPreCommitInfo(b.mctx, b.maddr, sn, tsk)
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return big.Zero(), xerrors.Errorf("precommit info not found on chain")
	}

	collateral, err := b.api.StateMinerInitialPledgeCollateral(b.mctx, b.maddr, pci.Info, tsk)
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting initial pledge collateral: %w", err)
	}

	collateral = big.Sub(collateral, pci.PreCommitDeposit)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	return collateral, nil
}
func (b *CommitBatcher) aggregateProofType(nv network.Version) (abi.RegisteredAggregationProof, error) {
	if nv < network.Version16 {
		return abi.RegisteredAggregationProof_SnarkPackV1, nil
	}
	return abi.RegisteredAggregationProof_SnarkPackV2, nil
}
