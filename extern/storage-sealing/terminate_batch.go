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
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/dline"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
)

var (
	// TODO: config

	TerminateBatchMax  uint64 = 100 // adjust based on real-world gas numbers, actors limit at 10k
	TerminateBatchMin  uint64 = 1
	TerminateBatchWait        = 5 * time.Minute
)

type TerminateBatcherApi interface {
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok TipSetToken) (*SectorLocation, error)
	SendMsg(ctx context.Context, from, to address.Address, method abi.MethodNum, value, maxFee abi.TokenAmount, params []byte) (cid.Cid, error)
	StateMinerInfo(context.Context, address.Address, TipSetToken) (miner.MinerInfo, error)
	StateMinerProvingDeadline(context.Context, address.Address, TipSetToken) (*dline.Info, error)
}

type TerminateBatcher struct {
	api     TerminateBatcherApi
	maddr   address.Address
	mctx    context.Context
	addrSel AddrSel
	feeCfg  FeeConfig

	todo map[SectorLocation]*bitfield.BitField // MinerSectorLocation -> BitField

	waiting map[SectorLocation][]chan cid.Cid

	notify, stop, stopped chan struct{}
	force                 chan chan *cid.Cid
	lk                    sync.Mutex
}

func NewTerminationBatcher(mctx context.Context, maddr address.Address, api TerminateBatcherApi, addrSel AddrSel, feeCfg FeeConfig) *TerminateBatcher {
	b := &TerminateBatcher{
		api:     api,
		maddr:   maddr,
		mctx:    mctx,
		addrSel: addrSel,
		feeCfg:  feeCfg,

		todo:    map[SectorLocation]*bitfield.BitField{},
		waiting: map[SectorLocation][]chan cid.Cid{},

		notify:  make(chan struct{}, 1),
		force:   make(chan chan *cid.Cid),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	go b.run()

	return b
}

func (b *TerminateBatcher) run() {
	var forceRes chan *cid.Cid
	var lastMsg *cid.Cid

	for {
		if forceRes != nil {
			forceRes <- lastMsg
			forceRes = nil
		}
		lastMsg = nil

		var notif, after bool
		select {
		case <-b.stop:
			close(b.stopped)
			return
		case <-b.notify:
			notif = true // send above max
		case <-time.After(TerminateBatchWait):
			after = true // send above min
		case fr := <-b.force: // user triggered
			forceRes = fr
		}

		dl, err := b.api.StateMinerProvingDeadline(b.mctx, b.maddr, nil)
		if err != nil {
			log.Errorw("TerminateBatcher: getting proving deadline info failed", "error", err)
			continue
		}

		b.lk.Lock()
		params := miner2.TerminateSectorsParams{}

		var total uint64
		for loc, sectors := range b.todo {
			n, err := sectors.Count()
			if err != nil {
				log.Errorw("TerminateBatcher: failed to count sectors to terminate", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			}

			// don't send terminations for currently challenged sectors
			if loc.Deadline == dl.Index || (loc.Deadline+1)%miner.WPoStPeriodDeadlines == dl.Index {
				continue
			}

			if n < 1 {
				log.Warnw("TerminateBatcher: zero sectors in bucket", "deadline", loc.Deadline, "partition", loc.Partition)
				continue
			}

			total += n

			params.Terminations = append(params.Terminations, miner2.TerminationDeclaration{
				Deadline:  loc.Deadline,
				Partition: loc.Partition,
				Sectors:   *sectors,
			})
		}

		if len(params.Terminations) == 0 {
			b.lk.Unlock()
			continue // nothing to do
		}

		if notif && total < TerminateBatchMax {
			b.lk.Unlock()
			continue
		}

		if after && total < TerminateBatchMin {
			b.lk.Unlock()
			continue
		}

		enc := new(bytes.Buffer)
		if err := params.MarshalCBOR(enc); err != nil {
			log.Warnw("TerminateBatcher: couldn't serialize TerminateSectors params", "error", err)
			b.lk.Unlock()
			continue
		}

		mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, nil)
		if err != nil {
			log.Warnw("TerminateBatcher: couldn't get miner info", "error", err)
			b.lk.Unlock()
			continue
		}

		from, _, err := b.addrSel(b.mctx, mi, api.TerminateSectorsAddr, b.feeCfg.MaxTerminateGasFee, b.feeCfg.MaxTerminateGasFee)
		if err != nil {
			log.Warnw("TerminateBatcher: no good address found", "error", err)
			b.lk.Unlock()
			continue
		}

		mcid, err := b.api.SendMsg(b.mctx, from, b.maddr, miner.Methods.TerminateSectors, big.Zero(), b.feeCfg.MaxTerminateGasFee, enc.Bytes())
		if err != nil {
			log.Errorw("TerminateBatcher: sending message failed", "error", err)
			b.lk.Unlock()
			continue
		}
		lastMsg = &mcid
		log.Infow("Sent TerminateSectors message", "cid", mcid, "from", from, "terminations", len(params.Terminations))

		for _, t := range params.Terminations {
			delete(b.todo, SectorLocation{
				Deadline:  t.Deadline,
				Partition: t.Partition,
			})
		}

		for _, w := range b.waiting {
			for _, ch := range w {
				ch <- mcid // buffered
			}
		}

		b.waiting = map[SectorLocation][]chan cid.Cid{}

		b.lk.Unlock()
	}
}

// register termination, wait for batch message, return message CID
func (b *TerminateBatcher) AddTermination(ctx context.Context, s abi.SectorID) (cid.Cid, error) {
	maddr, err := address.NewIDAddress(uint64(s.Miner))
	if err != nil {
		return cid.Undef, err
	}

	loc, err := b.api.StateSectorPartition(ctx, maddr, s.Number, nil)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting sector location: %w", err)
	}
	if loc == nil {
		return cid.Undef, xerrors.New("sector location not found")
	}

	b.lk.Lock()
	bf, ok := b.todo[*loc]
	if !ok {
		n := bitfield.New()
		bf = &n
		b.todo[*loc] = bf
	}
	bf.Set(uint64(s.Number))

	sent := make(chan cid.Cid, 1)
	b.waiting[*loc] = append(b.waiting[*loc], sent)

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

func (b *TerminateBatcher) Flush(ctx context.Context) (*cid.Cid, error) {
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

func (b *TerminateBatcher) Pending(ctx context.Context) ([]abi.SectorID, error) {
	b.lk.Lock()
	defer b.lk.Unlock()

	mid, err := address.IDFromAddress(b.maddr)
	if err != nil {
		return nil, err
	}

	res := make([]abi.SectorID, 0)
	for _, bf := range b.todo {
		err := bf.ForEach(func(id uint64) error {
			res = append(res, abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(id),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].Miner != res[j].Miner {
			return res[i].Miner < res[j].Miner
		}

		return res[i].Number < res[j].Number
	})

	return res, nil
}

func (b *TerminateBatcher) Stop(ctx context.Context) error {
	close(b.stop)

	select {
	case <-b.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
