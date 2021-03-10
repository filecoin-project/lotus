package sealing

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	miner3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/miner"
	proof3 "github.com/filecoin-project/specs-actors/v3/actors/runtime/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
)

var (
	// TODO: config!

	CommitBatchWait = 5 * time.Minute
)

type CommitBatcherApi interface {
	SendMsg(ctx context.Context, from, to address.Address, method abi.MethodNum, value, maxFee abi.TokenAmount, params []byte) (cid.Cid, error)
	StateMinerInfo(context.Context, address.Address, TipSetToken) (miner.MinerInfo, error)
}

type AggregateInput struct {
	info  proof3.AggregateSealVerifyInfo
	proof []byte
}

type CommitBatcher struct {
	api     CommitBatcherApi
	maddr   address.Address
	mctx    context.Context
	addrSel AddrSel
	feeCfg  FeeConfig
	getConfig GetSealingConfigFunc

	todo    map[abi.SectorNumber]AggregateInput
	waiting map[abi.SectorNumber][]chan cid.Cid

	notify, stop, stopped chan struct{}
	force                 chan chan *cid.Cid
	lk                    sync.Mutex
}

func NewCommitBatcher(mctx context.Context, maddr address.Address, api CommitBatcherApi, addrSel AddrSel, feeCfg FeeConfig, getConfig GetSealingConfigFunc) *CommitBatcher {
	b := &CommitBatcher{
		api:     api,
		maddr:   maddr,
		mctx:    mctx,
		addrSel: addrSel,
		feeCfg:  feeCfg,
		getConfig: getConfig,

		todo:    map[abi.SectorNumber]AggregateInput{},
		waiting: map[abi.SectorNumber][]chan cid.Cid{},

		notify:  make(chan struct{}, 1),
		force:   make(chan chan *cid.Cid),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	go b.run()

	return b
}

func (b *CommitBatcher) run() {
	var forceRes chan *cid.Cid
	var lastMsg *cid.Cid

	for {
		if forceRes != nil {
			forceRes <- lastMsg
			forceRes = nil
		}
		lastMsg = nil

		var sendAboveMax, sendAboveMin bool
		select {
		case <-b.stop:
			close(b.stopped)
			return
		case <-b.notify:
			sendAboveMax = true
		case <-time.After(TerminateBatchWait):
			sendAboveMin = true
		case fr := <-b.force: // user triggered
			forceRes = fr
		}

		var err error
		lastMsg, err = b.processBatch(sendAboveMax, sendAboveMin)
		if err != nil {
			log.Warnw("TerminateBatcher processBatch error", "error", err)
		}
	}
}

func (b *CommitBatcher) processBatch(notif, after bool) (*cid.Cid, error) {
	b.lk.Lock()
	defer b.lk.Unlock()
	params := miner3.ProveCommitAggregateParams{}

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

	if after && total < cfg.MinCommitBatch {
		return nil, nil
	}

	for id := range b.todo {
		params.SectorNumbers.Set(uint64(id))
	}

	// todo: Aggregate here
	params.AggregateProof = []byte("this is a valid aggregated proof for some sectors")

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return nil, xerrors.Errorf("couldn't serialize TerminateSectors params: %w", err)
	}

	mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, nil)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get miner info: %w", err)
	}

	from, _, err := b.addrSel(b.mctx, mi, api.CommitAddr, b.feeCfg.MaxCommitGasFee, b.feeCfg.MaxCommitGasFee)
	if err != nil {
		return nil, xerrors.Errorf("no good address found: %w", err)
	}

	mcid, err := b.api.SendMsg(b.mctx, from, b.maddr, miner.Methods.ProveCommitAggregate, big.Zero(), b.feeCfg.MaxCommitGasFee, enc.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("sending message failed: %w", err)
	}

	log.Infow("Sent ProveCommitAggregate message", "cid", mcid, "from", from, "sectors", total)

	err = params.SectorNumbers.ForEach(func(us uint64) error {
		sn := abi.SectorNumber(us)

		for _, ch := range b.waiting[sn] {
			ch <- mcid // buffered
		}
		delete(b.waiting, sn)
		delete(b.todo, sn)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("done sectors foreach: %w", err)
	}

	return &mcid, nil
}

// register commit, wait for batch message, return message CID
func (b *CommitBatcher) AddCommit(ctx context.Context, s abi.SectorNumber, in AggregateInput) (mcid cid.Cid, err error) {
	b.lk.Lock()
	b.todo[s] = in

	sent := make(chan cid.Cid, 1)
	b.waiting[s] = append(b.waiting[s], sent)

	select {
	case b.notify <- struct{}{}:
	default: // already have a pending notification, don't need more
	}
	b.lk.Unlock()

	select {
	case c := <-sent:
		return c, nil
	case <-ctx.Done():
		return cid.Undef, ctx.Err()
	}
}

func (b *CommitBatcher) Flush(ctx context.Context) (*cid.Cid, error) {
	resCh := make(chan *cid.Cid, 1)
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

	panic("todo")
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
