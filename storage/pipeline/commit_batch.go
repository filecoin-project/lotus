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
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var aggFeeNum = big.NewInt(110)
var aggFeeDen = big.NewInt(100)

type CommitBatcherApi interface {
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	GasEstimateMessageGas(context.Context, *types.Message, *api.MessageSendSpec, types.TipSetKey) (*types.Message, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	ChainHead(ctx context.Context) (*types.TipSet, error)

	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	StateMinerInitialPledgeForSector(ctx context.Context, sectorDuration abi.ChainEpoch, sectorSize abi.SectorSize, verifiedSize uint64, tsk types.TipSetKey) (types.BigInt, error)
	StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error)
	StateMinerAvailableBalance(context.Context, address.Address, types.TipSetKey) (big.Int, error)
	StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifregtypes.AllocationId, tsk types.TipSetKey) (*verifregtypes.Allocation, error)

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

	ActivationManifest miner.SectorActivationManifest
	DealIDPrecommit    bool
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

func NewCommitBatcher(mctx context.Context, maddr address.Address, api CommitBatcherApi, addrSel AddressSelector, feeCfg config.MinerFeeConfig, getConfig dtypes.GetSealingConfigFunc, prov storiface.Prover) (*CommitBatcher, error) {
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

	cfg, err := b.getConfig()
	if err != nil {
		return nil, err
	}

	go b.run(cfg)

	return b, nil
}

func (b *CommitBatcher) run(cfg sealiface.Config) {
	var forceRes chan []sealiface.CommitBatchRes
	var lastMsg []sealiface.CommitBatchRes

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

	if notif && total < cfg.MaxCommitBatch && cfg.AggregateCommits {
		return nil, nil
	}

	var res []sealiface.CommitBatchRes

	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return nil, err
	}

	nv, err := b.api.StateNetworkVersion(b.mctx, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("getting network version: %s", err)
	}

	blackedOut := func() bool {
		const nv16BlackoutWindow = abi.ChainEpoch(20) // a magik number
		if ts.Height() <= buildconstants.UpgradeSkyrHeight && buildconstants.UpgradeSkyrHeight-ts.Height() < nv16BlackoutWindow {
			return true
		}
		return false
	}

	individual := (total < cfg.MinCommitBatch) || (total < miner.MinAggregatedSectors) || blackedOut() || !cfg.AggregateCommits

	// After nv21, we have a new ProveCommitSectors2 method, which supports
	// batching without aggregation, but it doesn't support onboarding
	// sectors which were precommitted with DealIDs in the precommit message.
	// We prefer it for all other sectors, so first we use the new processBatchV2

	var sectors []abi.SectorNumber
	for sn := range b.todo {
		sectors = append(sectors, sn)
	}
	res, err = b.processBatchV2(cfg, sectors, nv, !individual)
	if err != nil {
		err = xerrors.Errorf("processBatchV2: %w", err)
	}

	// Mark sectors as done
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

	if err != nil {
		log.Warnf("CommitBatcher maybeStartBatch processBatch-ddo %v", err)
	}

	if err != nil && len(res) == 0 {
		return nil, err
	}

	return res, nil
}

// processBatchV2 processes a batch of sectors after nv22. It will always send
// ProveCommitSectors3Params which may contain either individual proofs or an
// aggregate proof depending on SP condition and network conditions.
func (b *CommitBatcher) processBatchV2(cfg sealiface.Config, sectors []abi.SectorNumber, nv network.Version, aggregate bool) ([]sealiface.CommitBatchRes, error) {
	ts, err := b.api.ChainHead(b.mctx)
	if err != nil {
		return nil, err
	}

	// sort sectors by number
	sort.Slice(sectors, func(i, j int) bool { return sectors[i] < sectors[j] })

	total := len(sectors)

	res := sealiface.CommitBatchRes{
		FailedSectors: map[abi.SectorNumber]string{},
	}

	params := miner.ProveCommitSectors3Params{
		RequireActivationSuccess:   cfg.RequireActivationSuccess,
		RequireNotificationSuccess: cfg.RequireNotificationSuccess,
	}

	infos := make([]proof.AggregateSealVerifyInfo, 0, total)
	collateral := big.Zero()

	mid, err := address.IDFromAddress(b.maddr)
	if err != nil {
		return nil, err
	}

	for _, sector := range sectors {
		if b.todo[sector].DealIDPrecommit {
			continue
		}

		res.Sectors = append(res.Sectors, sector)
		manifest := b.todo[sector].ActivationManifest

		sc, err := b.getSectorCollateral(sector, manifest.Pieces, ts)
		if err != nil {
			res.FailedSectors[sector] = err.Error()
			continue
		}

		collateral = big.Add(collateral, sc)

		if len(manifest.Pieces) > 0 {
			precomitInfo, err := b.api.StateSectorPreCommitInfo(b.mctx, b.maddr, sector, ts.Key())
			if err != nil {
				res.FailedSectors[sector] = err.Error()
				continue
			}
			err = b.allocationCheck(manifest.Pieces, precomitInfo, abi.ActorID(mid), ts)
			if err != nil {
				res.FailedSectors[sector] = err.Error()
				continue
			}
		}

		params.SectorActivations = append(params.SectorActivations, b.todo[sector].ActivationManifest)
		params.SectorProofs = append(params.SectorProofs, b.todo[sector].Proof)

		infos = append(infos, b.todo[sector].Info)
	}

	if len(infos) == 0 {
		return nil, nil
	}

	proofs := make([][]byte, 0, total)
	for _, info := range infos {
		proofs = append(proofs, b.todo[info.Number].Proof)
	}

	needFunds := collateral

	if aggregate {
		params.SectorProofs = nil // can't be set when aggregating
		arp, err := b.aggregateProofType(nv)
		if err != nil {
			res.Error = err.Error()
			return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting aggregate proof type: %w", err)
		}
		params.AggregateProofType = &arp

		mid, err := address.IDFromAddress(b.maddr)
		if err != nil {
			res.Error = err.Error()
			return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting miner id: %w", err)
		}

		params.AggregateProof, err = b.prover.AggregateSealProofs(proof.AggregateSealVerifyProofAndInfos{
			Miner:          abi.ActorID(mid),
			SealProof:      b.todo[infos[0].Number].Spt,
			AggregateProof: arp,
			Infos:          infos,
		}, proofs)
		if err != nil {
			res.Error = err.Error()
			return []sealiface.CommitBatchRes{res}, xerrors.Errorf("aggregating proofs: %w", err)
		}

		aggFeeRaw, err := policy.AggregateProveCommitNetworkFee(nv, len(infos), ts.MinTicketBlock().ParentBaseFee)
		if err != nil {
			res.Error = err.Error()
			log.Errorf("getting aggregate commit network fee: %s", err)
			return []sealiface.CommitBatchRes{res}, xerrors.Errorf("getting aggregate commit network fee: %s", err)
		}

		aggFee := big.Div(big.Mul(aggFeeRaw, aggFeeNum), aggFeeDen)

		needFunds = big.Add(collateral, aggFee)
	}

	needFunds, err = collateralSendAmount(b.mctx, b.api, b.maddr, cfg, needFunds)
	if err != nil {
		res.Error = err.Error()
		return []sealiface.CommitBatchRes{res}, err
	}

	maxFee := b.feeCfg.MaxCommitBatchGasFee.FeeForSectors(len(infos))
	goodFunds := big.Add(maxFee, needFunds)

	mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, types.EmptyTSK)
	if err != nil {
		res.Error = err.Error()
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("couldn't get miner info: %w", err)
	}

	from, _, err := b.addrSel.AddressFor(b.mctx, b.api, mi, api.CommitAddr, goodFunds, needFunds)
	if err != nil {
		res.Error = err.Error()
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("no good address found: %w", err)
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		res.Error = err.Error()
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("couldn't serialize ProveCommitSectors3Params: %w", err)
	}

	_, err = simulateMsgGas(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.ProveCommitSectors3, needFunds, maxFee, enc.Bytes())

	if err != nil && (!api.ErrorIsIn(err, []error{&api.ErrOutOfGas{}}) || len(sectors) < miner.MinAggregatedSectors*2) {
		log.Errorf("simulating CommitBatch %s", err)
		res.Error = err.Error()
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("simulating CommitBatch %w", err)
	}

	msgTooLarge := len(enc.Bytes()) > (messagepool.MaxMessageSize - 128)

	// If we're out of gas, split the batch in half and evaluate again
	if api.ErrorIsIn(err, []error{&api.ErrOutOfGas{}}) || msgTooLarge {
		log.Warnf("CommitAggregate message ran out of gas or is too large, splitting batch in half and trying again (sectors: %d, params: %d)", len(sectors), len(enc.Bytes()))
		mid := len(sectors) / 2
		ret0, _ := b.processBatchV2(cfg, sectors[:mid], nv, aggregate)
		ret1, _ := b.processBatchV2(cfg, sectors[mid:], nv, aggregate)

		return append(ret0, ret1...), nil
	}

	mcid, err := sendMsg(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.ProveCommitSectors3, needFunds, maxFee, enc.Bytes())
	if err != nil {
		return []sealiface.CommitBatchRes{res}, xerrors.Errorf("sending message failed (params size: %d, sectors: %d, agg: %t): %w", len(enc.Bytes()), len(sectors), aggregate, err)
	}

	res.Msg = &mcid

	log.Infow("Sent ProveCommitSectors3 message", "cid", mcid, "from", from, "todo", total, "sectors", len(infos))

	return []sealiface.CommitBatchRes{res}, nil
}

// AddCommit registers commit, wait for batch message, return message CID
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
		log.Errorf("unsupported network version: %s", err)
		return time.Now(), err
	}
	mpcd, err := policy.GetMaxProveCommitDuration(av, si.SectorType)
	if err != nil {
		log.Errorf("getting max prove commit duration: %s", err)
		return time.Now(), err
	}

	cutoffEpoch := pci.PreCommitEpoch + mpcd

	for _, p := range si.Pieces {
		if !p.HasDealInfo() {
			continue
		}

		startEpoch, err := p.StartEpoch()
		if err != nil {
			log.Errorf("getting deal start epoch: %s", err)
			return time.Now(), err
		}
		if startEpoch < cutoffEpoch {
			cutoffEpoch = startEpoch
		}
	}

	if cutoffEpoch <= ts.Height() {
		return time.Now(), nil
	}

	return time.Now().Add(time.Duration(cutoffEpoch-ts.Height()) * time.Duration(buildconstants.BlockDelaySecs) * time.Second), nil
}

func (b *CommitBatcher) getSectorCollateral(sn abi.SectorNumber, pieces []miner.PieceActivationManifest, ts *types.TipSet) (abi.TokenAmount, error) {
	pci, err := b.api.StateSectorPreCommitInfo(b.mctx, b.maddr, sn, ts.Key())
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return big.Zero(), xerrors.Errorf("precommit info not found on chain")
	}

	duration := pci.Info.Expiration - ts.Height()

	ssize, err := pci.Info.SealProof.SectorSize()
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to resolve sector size for seal proof: %w", err)
	}

	var verifiedSize uint64
	for _, piece := range pieces {
		if piece.VerifiedAllocationKey != nil {
			verifiedSize += uint64(piece.Size)
		}
	}

	collateral, err := b.api.StateMinerInitialPledgeForSector(b.mctx, duration, ssize, verifiedSize, ts.Key())
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting initial pledge collateral: %w", err)
	}

	collateral = big.Sub(collateral, pci.PreCommitDeposit)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	log.Infow("getSectorCollateral", "collateral", types.FIL(collateral), "sn", sn, "precommit", types.FIL(pci.PreCommitDeposit), "pledge", types.FIL(collateral), "verifiedSize", verifiedSize)

	return collateral, nil
}
func (b *CommitBatcher) aggregateProofType(nv network.Version) (abi.RegisteredAggregationProof, error) {
	if nv < network.Version16 {
		return abi.RegisteredAggregationProof_SnarkPackV1, nil
	}
	return abi.RegisteredAggregationProof_SnarkPackV2, nil
}

func (b *CommitBatcher) allocationCheck(Pieces []miner.PieceActivationManifest, precomitInfo *miner.SectorPreCommitOnChainInfo, miner abi.ActorID, ts *types.TipSet) error {
	for _, p := range Pieces {
		p := p
		// skip pieces not claiming an allocation
		if p.VerifiedAllocationKey == nil {
			continue
		}
		addr, err := address.NewIDAddress(uint64(p.VerifiedAllocationKey.Client))
		if err != nil {
			return err
		}

		alloc, err := b.api.StateGetAllocation(b.mctx, addr, verifregtypes.AllocationId(p.VerifiedAllocationKey.ID), ts.Key())
		if err != nil {
			return err
		}
		if alloc == nil {
			return xerrors.Errorf("no allocation found for piece %s with allocation ID %d", p.CID.String(), p.VerifiedAllocationKey.ID)
		}
		if alloc.Provider != miner {
			return xerrors.Errorf("provider id mismatch for piece %s: expected %d and found %d", p.CID.String(), miner, alloc.Provider)
		}
		if alloc.Size != p.Size {
			return xerrors.Errorf("size mismatch for piece %s: expected %d and found %d", p.CID.String(), p.Size, alloc.Size)
		}
		if precomitInfo.Info.Expiration < ts.Height()+alloc.TermMin {
			return xerrors.Errorf("sector expiration %d is before than allocation TermMin %d for piece %s", precomitInfo.Info.Expiration, ts.Height()+alloc.TermMin, p.CID.String())
		}
		if precomitInfo.Info.Expiration > ts.Height()+alloc.TermMax {
			return xerrors.Errorf("sector expiration %d is later than allocation TermMax %d for piece %s", precomitInfo.Info.Expiration, ts.Height()+alloc.TermMax, p.CID.String())
		}
	}
	return nil
}
