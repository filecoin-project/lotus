package sealing

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

type PreCommitBatcherApi interface {
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (big.Int, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
	StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifregtypes.Allocation, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)

	// Address selector
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

type preCommitEntry struct {
	deposit abi.TokenAmount
	pci     *miner.SectorPreCommitInfo
}

type PreCommitBatcher struct {
	api       PreCommitBatcherApi
	maddr     address.Address
	mctx      context.Context
	addrSel   AddressSelector
	feeCfg    config.MinerFeeConfig
	getConfig dtypes.GetSealingConfigFunc

	cutoffs map[abi.SectorNumber]time.Time
	todo    map[abi.SectorNumber]*preCommitEntry
	waiting map[abi.SectorNumber][]chan sealiface.PreCommitBatchRes

	notify, stop, stopped chan struct{}
	force                 chan chan []sealiface.PreCommitBatchRes
	lk                    sync.Mutex
}

func NewPreCommitBatcher(mctx context.Context, maddr address.Address, api PreCommitBatcherApi, addrSel AddressSelector, feeCfg config.MinerFeeConfig, getConfig dtypes.GetSealingConfigFunc) (*PreCommitBatcher, error) {
	b := &PreCommitBatcher{
		api:       api,
		maddr:     maddr,
		mctx:      mctx,
		addrSel:   addrSel,
		feeCfg:    feeCfg,
		getConfig: getConfig,

		cutoffs: map[abi.SectorNumber]time.Time{},
		todo:    map[abi.SectorNumber]*preCommitEntry{},
		waiting: map[abi.SectorNumber][]chan sealiface.PreCommitBatchRes{},

		notify:  make(chan struct{}, 1),
		force:   make(chan chan []sealiface.PreCommitBatchRes),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	cfg, err := b.getConfig()
	if err != nil {
		return nil, xerrors.Errorf("failed to get sealer config: %w", err)
	}

	go b.run(cfg)

	return b, nil
}

func (b *PreCommitBatcher) run(cfg sealiface.Config) {
	var forceRes chan []sealiface.PreCommitBatchRes
	var lastRes []sealiface.PreCommitBatchRes

	timer := time.NewTimer(b.batchWait(cfg.PreCommitBatchWait, cfg.PreCommitBatchSlack))
	for {
		if forceRes != nil {
			forceRes <- lastRes
			forceRes = nil
		}
		lastRes = nil

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
		lastRes, err = b.maybeStartBatch(sendAboveMax)
		if err != nil {
			log.Warnw("PreCommitBatcher processBatch error", "error", err)
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		timer.Reset(b.batchWait(cfg.PreCommitBatchWait, cfg.PreCommitBatchSlack))
	}
}

func (b *PreCommitBatcher) batchWait(maxWait, slack time.Duration) time.Duration {
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

func (b *PreCommitBatcher) maybeStartBatch(notif bool) ([]sealiface.PreCommitBatchRes, error) {
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

	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return nil, err
	}

	nv, err := b.api.StateNetworkVersion(b.mctx, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("couldn't get network version: %w", err)
	}

	// if this wasn't an user-forced batch, and we're not at/above the max batch size,
	// and we're not above the basefee threshold, don't batch yet
	if notif && total < cfg.MaxPreCommitBatch {
		return nil, nil
	}

	// For precommits the only method to precommit sectors after nv21(22?) is to use the new precommit_batch2 method
	// So we always batch
	res, err := b.processBatch(cfg, ts.Key(), ts.MinTicketBlock().ParentBaseFee, nv)
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

func (b *PreCommitBatcher) processPreCommitBatch(cfg sealiface.Config, bf abi.TokenAmount, entries []*preCommitEntry, nv network.Version) ([]sealiface.PreCommitBatchRes, error) {
	params := miner.PreCommitSectorBatchParams2{}
	deposit := big.Zero()
	var res sealiface.PreCommitBatchRes

	for _, p := range entries {
		res.Sectors = append(res.Sectors, p.pci.SectorNumber)
		params.Sectors = append(params.Sectors, *p.pci)
		deposit = big.Add(deposit, p.deposit)
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		res.Error = err.Error()
		return []sealiface.PreCommitBatchRes{res}, xerrors.Errorf("couldn't serialize PreCommitSectorBatchParams: %w", err)
	}

	mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, types.EmptyTSK)
	if err != nil {
		res.Error = err.Error()
		return []sealiface.PreCommitBatchRes{res}, xerrors.Errorf("couldn't get miner info: %w", err)
	}

	maxFee := b.feeCfg.MaxPreCommitBatchGasFee.FeeForSectors(len(params.Sectors))

	aggFeeRaw, err := policy.AggregatePreCommitNetworkFee(nv, len(params.Sectors), bf)
	if err != nil {
		log.Errorf("getting aggregate precommit network fee: %s", err)
		res.Error = err.Error()
		return []sealiface.PreCommitBatchRes{res}, xerrors.Errorf("getting aggregate precommit network fee: %s", err)
	}

	aggFee := big.Div(big.Mul(aggFeeRaw, aggFeeNum), aggFeeDen)

	needFunds := big.Add(deposit, aggFee)
	needFunds, err = collateralSendAmount(b.mctx, b.api, b.maddr, cfg, needFunds)
	if err != nil {
		return []sealiface.PreCommitBatchRes{res}, err
	}

	goodFunds := big.Add(maxFee, needFunds)

	from, _, err := b.addrSel.AddressFor(b.mctx, b.api, mi, api.PreCommitAddr, goodFunds, deposit)
	if err != nil {
		return []sealiface.PreCommitBatchRes{res}, xerrors.Errorf("no good address found: %w", err)
	}

	_, err = simulateMsgGas(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.PreCommitSectorBatch2, needFunds, maxFee, enc.Bytes())

	if err != nil && (!api.ErrorIsIn(err, []error{&api.ErrOutOfGas{}}) || len(entries) == 1) {
		res.Error = err.Error()
		return []sealiface.PreCommitBatchRes{res}, xerrors.Errorf("simulating PreCommitBatch %w", err)
	}

	// If we're out of gas, split the batch in half and evaluate again
	if api.ErrorIsIn(err, []error{&api.ErrOutOfGas{}}) {
		log.Warnf("PreCommitBatch out of gas, splitting batch in half and trying again")
		mid := len(entries) / 2
		ret0, _ := b.processPreCommitBatch(cfg, bf, entries[:mid], nv)
		ret1, _ := b.processPreCommitBatch(cfg, bf, entries[mid:], nv)

		return append(ret0, ret1...), nil
	}

	// If state call succeeds, we can send the message for real
	mcid, err := sendMsg(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.PreCommitSectorBatch2, needFunds, maxFee, enc.Bytes())
	if err != nil {
		res.Error = err.Error()
		return []sealiface.PreCommitBatchRes{res}, xerrors.Errorf("pushing message to mpool: %w", err)
	}
	res.Msg = &mcid
	return []sealiface.PreCommitBatchRes{res}, nil
}

func (b *PreCommitBatcher) processBatch(cfg sealiface.Config, tsk types.TipSetKey, bf abi.TokenAmount, nv network.Version) ([]sealiface.PreCommitBatchRes, error) {
	var pcEntries []*preCommitEntry
	for _, p := range b.todo {
		pcEntries = append(pcEntries, p)
	}

	return b.processPreCommitBatch(cfg, bf, pcEntries, nv)
}

// AddPreCommit registers PreCommit, waits for batch message, returns message CID
func (b *PreCommitBatcher) AddPreCommit(ctx context.Context, s SectorInfo, deposit abi.TokenAmount, in *miner.SectorPreCommitInfo) (res sealiface.PreCommitBatchRes, err error) {
	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		log.Errorf("getting chain head: %s", err)
		return sealiface.PreCommitBatchRes{}, err
	}

	dealStartCutoff := getDealStartCutoff(s)
	if dealStartCutoff <= ts.Height() {
		return sealiface.PreCommitBatchRes{}, xerrors.Errorf("cutoff has already passed (cutoff %d <= curEpoch %d)", dealStartCutoff, ts.Height())
	}

	// Allocation cutoff is a soft deadline, so don't fail if we've passed it.
	allocationCutoff := b.getAllocationCutoff(s)

	var cutoffEpoch abi.ChainEpoch
	if dealStartCutoff < allocationCutoff {
		cutoffEpoch = dealStartCutoff
	} else {
		cutoffEpoch = allocationCutoff
	}

	sn := s.SectorNumber

	b.lk.Lock()
	b.cutoffs[sn] = time.Now().Add(time.Duration(cutoffEpoch-ts.Height()) * time.Duration(buildconstants.BlockDelaySecs) * time.Second)
	b.todo[sn] = &preCommitEntry{
		deposit: deposit,
		pci:     in,
	}

	sent := make(chan sealiface.PreCommitBatchRes, 1)
	b.waiting[sn] = append(b.waiting[sn], sent)

	select {
	case b.notify <- struct{}{}:
	default: // already have a pending notification, don't need more
	}
	b.lk.Unlock()

	select {
	case c := <-sent:
		return c, nil
	case <-ctx.Done():
		return sealiface.PreCommitBatchRes{}, ctx.Err()
	}
}

func (b *PreCommitBatcher) Flush(ctx context.Context) ([]sealiface.PreCommitBatchRes, error) {
	resCh := make(chan []sealiface.PreCommitBatchRes, 1)
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

func (b *PreCommitBatcher) Pending(ctx context.Context) ([]abi.SectorID, error) {
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
			Number: s.pci.SectorNumber,
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

func (b *PreCommitBatcher) Stop(ctx context.Context) error {
	close(b.stop)

	select {
	case <-b.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func getDealStartCutoff(si SectorInfo) abi.ChainEpoch {
	cutoffEpoch := si.TicketEpoch + policy.MaxPreCommitRandomnessLookback
	for _, p := range si.Pieces {
		if !p.HasDealInfo() {
			continue
		}

		startEpoch, err := p.StartEpoch()
		if err != nil {
			// almost definitely can't happen, but if it does there's less harm in
			// just logging the error and moving on
			log.Errorw("failed to get deal start epoch", "error", err)
			continue
		}

		if startEpoch < cutoffEpoch {
			cutoffEpoch = startEpoch
		}
	}

	return cutoffEpoch
}

func (b *PreCommitBatcher) getAllocationCutoff(si SectorInfo) abi.ChainEpoch {
	cutoff := si.TicketEpoch + policy.MaxPreCommitRandomnessLookback
	for _, p := range si.Pieces {
		if !p.HasDealInfo() {
			continue
		}

		alloc, err := p.GetAllocation(b.mctx, b.api, types.EmptyTSK)
		if err != nil {
			log.Errorw("failed to get deal allocation", "error", err)
		}
		// alloc is nil if this is not a verified deal in nv17 or later
		if alloc == nil {
			continue
		}

		if alloc.Expiration < cutoff {
			cutoff = alloc.Expiration
		}
	}
	return cutoff
}
