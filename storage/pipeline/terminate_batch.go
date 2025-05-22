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
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/api"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type TerminateBatcherApi interface {
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*lminer.SectorLocation, error)
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error)

	// Address selector
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

type TerminateBatcher struct {
	api       TerminateBatcherApi
	maddr     address.Address
	mctx      context.Context
	addrSel   AddressSelector
	feeCfg    config.MinerFeeConfig
	getConfig dtypes.GetSealingConfigFunc

	todo map[lminer.SectorLocation]*bitfield.BitField // MinerSectorLocation -> BitField

	waiting map[abi.SectorNumber][]chan cid.Cid

	notify, stop, stopped chan struct{}
	force                 chan chan *cid.Cid
	lk                    sync.Mutex
}

func NewTerminationBatcher(mctx context.Context, maddr address.Address, api TerminateBatcherApi, addrSel AddressSelector, feeCfg config.MinerFeeConfig, getConfig dtypes.GetSealingConfigFunc) *TerminateBatcher {
	b := &TerminateBatcher{
		api:       api,
		maddr:     maddr,
		mctx:      mctx,
		addrSel:   addrSel,
		feeCfg:    feeCfg,
		getConfig: getConfig,

		todo:    map[lminer.SectorLocation]*bitfield.BitField{},
		waiting: map[abi.SectorNumber][]chan cid.Cid{},

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

		cfg, err := b.getConfig()
		if err != nil {
			log.Warnw("TerminateBatcher getconfig error", "error", err)
		}

		var sendAboveMax, sendAboveMin bool
		select {
		case <-b.stop:
			close(b.stopped)
			return
		case <-b.notify:
			sendAboveMax = true
		case <-time.After(cfg.TerminateBatchWait):
			sendAboveMin = true
		case fr := <-b.force: // user triggered
			forceRes = fr
		}

		lastMsg, err = b.processBatch(sendAboveMax, sendAboveMin)
		if err != nil {
			log.Warnw("TerminateBatcher processBatch error", "error", err)
		}
	}
}

func (b *TerminateBatcher) processBatch(notif, after bool) (*cid.Cid, error) {
	dl, err := b.api.StateMinerProvingDeadline(b.mctx, b.maddr, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting proving deadline info failed: %w", err)
	}

	cfg, err := b.getConfig()
	if err != nil {
		return nil, xerrors.Errorf("getting sealing config: %W", err)
	}

	b.lk.Lock()
	defer b.lk.Unlock()
	params := miner.TerminateSectorsParams{}

	var total uint64
	for loc, sectors := range b.todo {
		n, err := sectors.Count()
		if err != nil {
			log.Errorw("TerminateBatcher: failed to count sectors to terminate", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			continue
		}

		// don't send terminations for currently challenged sectors
		if loc.Deadline == (dl.Index+1)%miner.WPoStPeriodDeadlines || // not in next (in case the terminate message takes a while to get on chain)
			loc.Deadline == dl.Index || // not in current
			(loc.Deadline+1)%miner.WPoStPeriodDeadlines == dl.Index { // not in previous
			continue
		}

		if n < 1 {
			log.Warnw("TerminateBatcher: zero sectors in bucket", "deadline", loc.Deadline, "partition", loc.Partition)
			continue
		}

		toTerminate, err := sectors.Copy()
		if err != nil {
			log.Warnw("TerminateBatcher: copy sectors bitfield", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			continue
		}

		ps, err := b.api.StateMinerPartitions(b.mctx, b.maddr, loc.Deadline, types.EmptyTSK)
		if err != nil {
			log.Warnw("TerminateBatcher: getting miner partitions", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			continue
		}

		toTerminate, err = bitfield.IntersectBitField(ps[loc.Partition].LiveSectors, toTerminate)
		if err != nil {
			log.Warnw("TerminateBatcher: intersecting liveSectors and toTerminate bitfields", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
			continue
		}

		if total+n > uint64(miner.AddressedSectorsMax) {
			n = uint64(miner.AddressedSectorsMax) - total

			toTerminate, err = toTerminate.Slice(0, n)
			if err != nil {
				log.Warnw("TerminateBatcher: slice toTerminate bitfield", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
				continue
			}

			s, err := bitfield.SubtractBitField(*sectors, toTerminate)
			if err != nil {
				log.Warnw("TerminateBatcher: sectors-toTerminate", "deadline", loc.Deadline, "partition", loc.Partition, "error", err)
				continue
			}
			*sectors = s
		}

		total += n

		params.Terminations = append(params.Terminations, miner.TerminationDeclaration{
			Deadline:  loc.Deadline,
			Partition: loc.Partition,
			Sectors:   toTerminate,
		})

		if total >= uint64(miner.AddressedSectorsMax) || total >= cfg.TerminateBatchMax {
			break
		}

		if len(params.Terminations) >= miner.DeclarationsMax {
			break
		}
	}

	if len(params.Terminations) == 0 {
		return nil, nil // nothing to do
	}

	if notif && total < cfg.TerminateBatchMax {
		return nil, nil
	}

	if after && total < cfg.TerminateBatchMin {
		return nil, nil
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return nil, xerrors.Errorf("couldn't serialize TerminateSectors params: %w", err)
	}

	mi, err := b.api.StateMinerInfo(b.mctx, b.maddr, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get miner info: %w", err)
	}

	from, _, err := b.addrSel.AddressFor(b.mctx, b.api, mi, api.TerminateSectorsAddr, big.Int(b.feeCfg.MaxTerminateGasFee), big.Int(b.feeCfg.MaxTerminateGasFee))
	if err != nil {
		return nil, xerrors.Errorf("no good address found: %w", err)
	}

	mcid, err := sendMsg(b.mctx, b.api, from, b.maddr, builtin.MethodsMiner.TerminateSectors, big.Zero(), big.Int(b.feeCfg.MaxTerminateGasFee), enc.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("sending message failed: %w", err)
	}
	log.Infow("Sent TerminateSectors message", "cid", mcid, "from", from, "terminations", len(params.Terminations))

	for _, t := range params.Terminations {
		delete(b.todo, lminer.SectorLocation{
			Deadline:  t.Deadline,
			Partition: t.Partition,
		})

		err := t.Sectors.ForEach(func(sn uint64) error {
			for _, ch := range b.waiting[abi.SectorNumber(sn)] {
				ch <- mcid // buffered
			}
			delete(b.waiting, abi.SectorNumber(sn))

			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("sectors foreach: %w", err)
		}
	}

	return &mcid, nil
}

// AddTermination registers termination, waits for batch message, returns message CID
// can return cid.Undef,true if the sector is already terminated on-chain
func (b *TerminateBatcher) AddTermination(ctx context.Context, s abi.SectorID) (mcid cid.Cid, terminated bool, err error) {
	maddr, err := address.NewIDAddress(uint64(s.Miner))
	if err != nil {
		return cid.Undef, false, err
	}

	loc, err := b.api.StateSectorPartition(ctx, maddr, s.Number, types.EmptyTSK)
	if err != nil {
		return cid.Undef, false, xerrors.Errorf("getting sector location: %w", err)
	}
	if loc == nil {
		return cid.Undef, false, xerrors.New("sector location not found")
	}

	{
		// check if maybe already terminated
		parts, err := b.api.StateMinerPartitions(ctx, maddr, loc.Deadline, types.EmptyTSK)
		if err != nil {
			return cid.Cid{}, false, xerrors.Errorf("getting partitions: %w", err)
		}
		live, err := parts[loc.Partition].LiveSectors.IsSet(uint64(s.Number))
		if err != nil {
			return cid.Cid{}, false, xerrors.Errorf("checking if sector is in live set: %w", err)
		}
		if !live {
			// already terminated
			return cid.Undef, true, nil
		}
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
	b.waiting[s.Number] = append(b.waiting[s.Number], sent)

	select {
	case b.notify <- struct{}{}:
	default: // already have a pending notification, don't need more
	}
	b.lk.Unlock()

	select {
	case c := <-sent:
		return c, false, nil
	case <-ctx.Done():
		return cid.Undef, false, ctx.Err()
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
