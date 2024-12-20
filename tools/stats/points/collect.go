package points

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/tools/stats/influx"
	"github.com/filecoin-project/lotus/tools/stats/metrics"
)

type LotusApi interface {
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*api.MinerPower, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	ChainGetParentMessages(ctx context.Context, blockCid cid.Cid) ([]api.Message, error)
	ChainGetParentReceipts(ctx context.Context, blockCid cid.Cid) ([]*types.MessageReceipt, error)
	ChainGetBlockMessages(ctx context.Context, blockCid cid.Cid) (*api.BlockMessages, error)
}

type ChainPointCollector struct {
	ctx              context.Context
	api              LotusApi
	store            adt.Store
	actorDigestCache *lru.TwoQueueCache[address.Address, string]
}

func NewChainPointCollector(ctx context.Context, store adt.Store, api LotusApi) (*ChainPointCollector, error) {
	actorDigestCache, err := lru.New2Q[address.Address, string](2 << 15)
	if err != nil {
		return nil, err
	}

	collector := &ChainPointCollector{
		ctx:              ctx,
		store:            store,
		actorDigestCache: actorDigestCache,
		api:              api,
	}

	return collector, nil
}

func (c *ChainPointCollector) actorDigest(ctx context.Context, addr address.Address, tipset *types.TipSet) (string, error) {
	if code, ok := c.actorDigestCache.Get(addr); ok {
		return code, nil
	}

	actor, err := c.api.StateGetActor(ctx, addr, tipset.Key())
	if err != nil {
		return "", err
	}

	digest := builtin.ActorNameByCode(actor.Code)

	c.actorDigestCache.Add(addr, digest)

	return digest, nil
}

func (c *ChainPointCollector) Collect(ctx context.Context, tipset *types.TipSet) (client.BatchPoints, error) {
	start := time.Now()
	done := metrics.Timer(ctx, metrics.TipsetCollectionDuration)
	defer func() {
		log.Infow("record tipset", "elapsed", time.Since(start).Seconds())
		done()
	}()

	pl := influx.NewPointList()
	height := tipset.Height()

	log.Debugw("collecting tipset points", "height", tipset.Height())
	stats.Record(ctx, metrics.TipsetCollectionHeight.M(int64(height)))

	if err := c.collectBlockheaderPoints(ctx, pl, tipset); err != nil {
		log.Errorw("failed to record tipset", "height", height, "error", err, "tipset", tipset.Key())
	}

	if err := c.collectMessagePoints(ctx, pl, tipset); err != nil {
		log.Errorw("failed to record messages", "height", height, "error", err, "tipset", tipset.Key())
	}

	if err := c.collectStaterootPoints(ctx, pl, tipset); err != nil {
		log.Errorw("failed to record state", "height", height, "error", err, "tipset", tipset.Key())
	}

	tsTimestamp := time.Unix(int64(tipset.MinTimestamp()), int64(0))

	nb, err := influx.NewBatch()
	if err != nil {
		return nil, err
	}

	for _, pt := range pl.Points() {
		pt.SetTime(tsTimestamp)
		nb.AddPoint(influx.NewPointFrom(pt))
	}

	log.Infow("collected tipset points", "count", len(nb.Points()), "height", tipset.Height())

	stats.Record(ctx, metrics.TipsetCollectionPoints.M(int64(len(nb.Points()))))

	return nb, nil
}

func (c *ChainPointCollector) collectBlockheaderPoints(ctx context.Context, pl *influx.PointList, tipset *types.TipSet) error {
	start := time.Now()
	done := metrics.Timer(ctx, metrics.TipsetCollectionBlockHeaderDuration)
	defer func() {
		log.Infow("collect blockheader points", "elapsed", time.Since(start).Seconds())
		done()
	}()

	cids := []string{}
	for _, cid := range tipset.Cids() {
		cids = append(cids, cid.String())
	}

	p := influx.NewPoint("chain.height", int64(tipset.Height()))
	p.AddTag("tipset", strings.Join(cids, " "))
	pl.AddPoint(p)

	p = influx.NewPoint("chain.block_count", len(cids))
	pl.AddPoint(p)

	tsTime := time.Unix(int64(tipset.MinTimestamp()), int64(0))
	p = influx.NewPoint("chain.blocktime", tsTime.Unix())
	pl.AddPoint(p)

	totalGasLimit := int64(0)
	totalUniqGasLimit := int64(0)
	seen := make(map[cid.Cid]struct{})
	for _, blockheader := range tipset.Blocks() {
		bs, err := blockheader.Serialize()
		if err != nil {
			return err
		}
		p := influx.NewPoint("chain.election", blockheader.ElectionProof.WinCount)
		p.AddTag("miner", blockheader.Miner.String())
		pl.AddPoint(p)

		p = influx.NewPoint("chain.blockheader_size", len(bs))
		pl.AddPoint(p)

		msgs, err := c.api.ChainGetBlockMessages(ctx, blockheader.Cid())
		if err != nil {
			return xerrors.Errorf("ChainGetBlockMessages failed: %w", msgs)
		}
		for _, m := range msgs.BlsMessages {
			c := m.Cid()
			totalGasLimit += m.GasLimit
			if _, ok := seen[c]; !ok {
				totalUniqGasLimit += m.GasLimit
				seen[c] = struct{}{}
			}
		}
		for _, m := range msgs.SecpkMessages {
			c := m.Cid()
			totalGasLimit += m.Message.GasLimit
			if _, ok := seen[c]; !ok {
				totalUniqGasLimit += m.Message.GasLimit
				seen[c] = struct{}{}
			}
		}
	}
	p = influx.NewPoint("chain.gas_limit_total", totalGasLimit)
	pl.AddPoint(p)
	p = influx.NewPoint("chain.gas_limit_uniq_total", totalUniqGasLimit)
	pl.AddPoint(p)

	{
		baseFeeIn := tipset.Blocks()[0].ParentBaseFee
		newBaseFee := store.ComputeNextBaseFee(baseFeeIn, totalUniqGasLimit, len(tipset.Blocks()), tipset.Height())

		baseFeeRat := new(big.Rat).SetFrac(newBaseFee.Int, new(big.Int).SetUint64(buildconstants.FilecoinPrecision))
		baseFeeFloat, _ := baseFeeRat.Float64()
		p = influx.NewPoint("chain.basefee", baseFeeFloat)
		pl.AddPoint(p)

		baseFeeChange := new(big.Rat).SetFrac(newBaseFee.Int, baseFeeIn.Int)
		baseFeeChangeF, _ := baseFeeChange.Float64()
		p = influx.NewPoint("chain.basefee_change_log", math.Log(baseFeeChangeF)/math.Log(1.125))
		pl.AddPoint(p)
	}
	{
		blks := int64(len(cids))
		p = influx.NewPoint("chain.gas_fill_ratio", float64(totalGasLimit)/float64(blks*buildconstants.BlockGasTarget))
		pl.AddPoint(p)
		p = influx.NewPoint("chain.gas_capacity_ratio", float64(totalUniqGasLimit)/float64(blks*buildconstants.BlockGasTarget))
		pl.AddPoint(p)
		p = influx.NewPoint("chain.gas_waste_ratio", float64(totalGasLimit-totalUniqGasLimit)/float64(blks*buildconstants.BlockGasTarget))
		pl.AddPoint(p)
	}

	return nil
}

func (c *ChainPointCollector) collectStaterootPoints(ctx context.Context, pl *influx.PointList, tipset *types.TipSet) error {
	start := time.Now()
	done := metrics.Timer(ctx, metrics.TipsetCollectionStaterootDuration)
	defer func() {
		log.Infow("collect stateroot points", "elapsed", time.Since(start).Seconds())
		done()
	}()

	attoFil := types.NewInt(buildconstants.FilecoinPrecision).Int

	netBal, err := c.api.WalletBalance(ctx, reward.Address)
	if err != nil {
		return err
	}

	netBalFil := new(big.Rat).SetFrac(netBal.Int, attoFil)
	netBalFilFloat, _ := netBalFil.Float64()
	p := influx.NewPoint("network.balance", netBalFilFloat)
	pl.AddPoint(p)

	totalPower, err := c.api.StateMinerPower(ctx, address.Address{}, tipset.Key())
	if err != nil {
		return err
	}

	// We divide the power into gibibytes because 2^63 bytes is 8 exbibytes which is smaller than the Filecoin Mainnet.
	// Dividing by a gibibyte gives us more room to work with. This will allow the dashboard to report network and miner
	// sizes up to 8192 yobibytes.
	gibi := types.NewInt(1024 * 1024 * 1024)
	p = influx.NewPoint("chain.power", types.BigDiv(totalPower.TotalPower.QualityAdjPower, gibi).Int64())
	pl.AddPoint(p)

	powerActor, err := c.api.StateGetActor(ctx, power.Address, tipset.Key())
	if err != nil {
		return err
	}

	powerActorState, err := power.Load(c.store, powerActor)
	if err != nil {
		return err
	}

	return powerActorState.ForEachClaim(func(addr address.Address, claim power.Claim) error {
		// BigCmp returns 0 if values are equal
		if types.BigCmp(claim.QualityAdjPower, types.NewInt(0)) == 0 {
			return nil
		}

		p = influx.NewPoint("chain.miner_power", types.BigDiv(claim.QualityAdjPower, gibi).Int64())
		p.AddTag("miner", addr.String())
		pl.AddPoint(p)

		return nil
	}, false)
}

type msgTag struct {
	actor    string
	method   uint64
	exitcode uint8
}

func (c *ChainPointCollector) collectMessagePoints(ctx context.Context, pl *influx.PointList, tipset *types.TipSet) error {
	start := time.Now()
	done := metrics.Timer(ctx, metrics.TipsetCollectionMessageDuration)
	defer func() {
		log.Infow("collect message points", "elapsed", time.Since(start).Seconds())
		done()
	}()

	cids := tipset.Cids()
	if len(cids) == 0 {
		return fmt.Errorf("no cids in tipset")
	}

	msgs, err := c.api.ChainGetParentMessages(ctx, cids[0])
	if err != nil {
		return err
	}

	recp, err := c.api.ChainGetParentReceipts(ctx, cids[0])
	if err != nil {
		return err
	}

	msgn := make(map[msgTag][]cid.Cid)

	totalGasUsed := int64(0)
	for _, r := range recp {
		totalGasUsed += r.GasUsed
	}
	p := influx.NewPoint("chain.gas_used_total", totalGasUsed)
	pl.AddPoint(p)

	for i, msg := range msgs {
		digest, err := c.actorDigest(ctx, msg.Message.To, tipset)
		if err != nil {
			continue
		}

		// FIXME: use float so this doesn't overflow
		// FIXME: this doesn't work as time points get overridden
		p := influx.NewPoint("chain.message_gaspremium", msg.Message.GasPremium.Int64())
		pl.AddPoint(p)
		p = influx.NewPoint("chain.message_gasfeecap", msg.Message.GasFeeCap.Int64())
		pl.AddPoint(p)

		bs, err := msg.Message.Serialize()
		if err != nil {
			return err
		}

		p = influx.NewPoint("chain.message_size", len(bs))
		pl.AddPoint(p)

		tag := msgTag{
			actor:    digest,
			method:   uint64(msg.Message.Method),
			exitcode: uint8(recp[i].ExitCode),
		}

		found := false
		for _, c := range msgn[tag] {
			if c.Equals(msg.Cid) {
				found = true
				break
			}
		}
		if !found {
			msgn[tag] = append(msgn[tag], msg.Cid)
		}
	}

	for t, m := range msgn {
		p := influx.NewPoint("chain.message_count", len(m))
		p.AddTag("actor", t.actor)
		p.AddTag("method", fmt.Sprintf("%d", t.method))
		p.AddTag("exitcode", fmt.Sprintf("%d", t.exitcode))
		pl.AddPoint(p)

	}

	return nil
}
