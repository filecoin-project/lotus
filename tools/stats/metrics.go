package stats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	cbg "github.com/whyrusleeping/cbor-gen"

	_ "github.com/influxdata/influxdb1-client"
	models "github.com/influxdata/influxdb1-client/models"
	client "github.com/influxdata/influxdb1-client/v2"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("stats")

type PointList struct {
	points []models.Point
}

func NewPointList() *PointList {
	return &PointList{}
}

func (pl *PointList) AddPoint(p models.Point) {
	pl.points = append(pl.points, p)
}

func (pl *PointList) Points() []models.Point {
	return pl.points
}

type InfluxWriteQueue struct {
	ch chan client.BatchPoints
}

func NewInfluxWriteQueue(ctx context.Context, influx client.Client) *InfluxWriteQueue {
	ch := make(chan client.BatchPoints, 128)

	maxRetries := 10

	go func() {
	main:
		for {
			select {
			case <-ctx.Done():
				return
			case batch := <-ch:
				for i := 0; i < maxRetries; i++ {
					if err := influx.Write(batch); err != nil {
						log.Warnw("Failed to write batch", "error", err)
						time.Sleep(time.Second * 15)
						continue
					}

					continue main
				}

				log.Error("Dropping batch due to failure to write")
			}
		}
	}()

	return &InfluxWriteQueue{
		ch: ch,
	}
}

func (i *InfluxWriteQueue) AddBatch(bp client.BatchPoints) {
	i.ch <- bp
}

func (i *InfluxWriteQueue) Close() {
	close(i.ch)
}

func InfluxClient(addr, user, pass string) (client.Client, error) {
	return client.NewHTTPClient(client.HTTPConfig{
		Addr:     addr,
		Username: user,
		Password: pass,
	})
}

func InfluxNewBatch() (client.BatchPoints, error) {
	return client.NewBatchPoints(client.BatchPointsConfig{})
}

func NewPoint(name string, value interface{}) models.Point {
	pt, _ := models.NewPoint(name, models.Tags{}, map[string]interface{}{"value": value}, time.Now())
	return pt
}

func NewPointFrom(p models.Point) *client.Point {
	return client.NewPointFrom(p)
}

func RecordTipsetPoints(ctx context.Context, api api.FullNode, pl *PointList, tipset *types.TipSet) error {
	cids := []string{}
	for _, cid := range tipset.Cids() {
		cids = append(cids, cid.String())
	}

	p := NewPoint("chain.height", int64(tipset.Height()))
	p.AddTag("tipset", strings.Join(cids, " "))
	pl.AddPoint(p)

	p = NewPoint("chain.block_count", len(cids))
	pl.AddPoint(p)

	tsTime := time.Unix(int64(tipset.MinTimestamp()), int64(0))
	p = NewPoint("chain.blocktime", tsTime.Unix())
	pl.AddPoint(p)

	for _, blockheader := range tipset.Blocks() {
		bs, err := blockheader.Serialize()
		if err != nil {
			return err
		}
		p := NewPoint("chain.election", 1)
		p.AddTag("miner", blockheader.Miner.String())
		pl.AddPoint(p)

		p = NewPoint("chain.blockheader_size", len(bs))
		pl.AddPoint(p)
	}

	return nil
}

type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("Object does not implement CBORUnmarshaler")
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return cid.Undef, fmt.Errorf("Put is not implemented on apiIpldStore")
}

func RecordTipsetStatePoints(ctx context.Context, api api.FullNode, pl *PointList, tipset *types.TipSet) error {
	attoFil := types.NewInt(build.FilecoinPrecision).Int

	//TODO: StatePledgeCollateral API is not implemented and is commented out - re-enable this block once the API is implemented again.
	//pc, err := api.StatePledgeCollateral(ctx, tipset.Key())
	//if err != nil {
	//return err
	//}

	//pcFil := new(big.Rat).SetFrac(pc.Int, attoFil)
	//pcFilFloat, _ := pcFil.Float64()
	//p := NewPoint("chain.pledge_collateral", pcFilFloat)
	//pl.AddPoint(p)

	netBal, err := api.WalletBalance(ctx, builtin.RewardActorAddr)
	if err != nil {
		return err
	}

	netBalFil := new(big.Rat).SetFrac(netBal.Int, attoFil)
	netBalFilFloat, _ := netBalFil.Float64()
	p := NewPoint("network.balance", netBalFilFloat)
	pl.AddPoint(p)

	totalPower, err := api.StateMinerPower(ctx, address.Address{}, tipset.Key())
	if err != nil {
		return err
	}

	p = NewPoint("chain.power", totalPower.TotalPower.QualityAdjPower.Int64())
	pl.AddPoint(p)

	powerActor, err := api.StateGetActor(ctx, builtin.StoragePowerActorAddr, tipset.Key())
	if err != nil {
		return err
	}

	powerRaw, err := api.ChainReadObj(ctx, powerActor.Head)
	if err != nil {
		return err
	}

	var powerActorState power.State

	if err := powerActorState.UnmarshalCBOR(bytes.NewReader(powerRaw)); err != nil {
		return fmt.Errorf("failed to unmarshal power actor state: %w", err)
	}

	s := &apiIpldStore{ctx, api}
	mp, err := adt.AsMap(s, powerActorState.Claims)
	if err != nil {
		return err
	}

	err = mp.ForEach(nil, func(key string) error {
		addr, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}

		var claim power.Claim
		keyerAddr := adt.AddrKey(addr)
		mp.Get(keyerAddr, &claim)

		if claim.QualityAdjPower.Int64() == 0 {
			return nil
		}

		p = NewPoint("chain.miner_power", claim.QualityAdjPower.Int64())
		p.AddTag("miner", addr.String())
		pl.AddPoint(p)

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

type msgTag struct {
	actor    string
	method   uint64
	exitcode uint8
}

func RecordTipsetMessagesPoints(ctx context.Context, api api.FullNode, pl *PointList, tipset *types.TipSet) error {
	cids := tipset.Cids()
	if len(cids) == 0 {
		return fmt.Errorf("no cids in tipset")
	}

	msgs, err := api.ChainGetParentMessages(ctx, cids[0])
	if err != nil {
		return err
	}

	recp, err := api.ChainGetParentReceipts(ctx, cids[0])
	if err != nil {
		return err
	}

	msgn := make(map[msgTag][]cid.Cid)

	for i, msg := range msgs {
		p := NewPoint("chain.message_gasprice", msg.Message.GasPrice.Int64())
		pl.AddPoint(p)

		bs, err := msg.Message.Serialize()
		if err != nil {
			return err
		}

		p = NewPoint("chain.message_size", len(bs))
		pl.AddPoint(p)

		actor, err := api.StateGetActor(ctx, msg.Message.To, tipset.Key())
		if err != nil {
			return err
		}

		dm, err := multihash.Decode(actor.Code.Hash())
		if err != nil {
			continue
		}
		tag := msgTag{
			actor:    string(dm.Digest),
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
		p := NewPoint("chain.message_count", len(m))
		p.AddTag("actor", t.actor)
		p.AddTag("method", fmt.Sprintf("%d", t.method))
		p.AddTag("exitcode", fmt.Sprintf("%d", t.exitcode))
		pl.AddPoint(p)

	}

	return nil
}

func ResetDatabase(influx client.Client, database string) error {
	log.Info("Resetting database")
	q := client.NewQuery(fmt.Sprintf(`DROP DATABASE "%s"; CREATE DATABASE "%s";`, database, database), "", "")
	_, err := influx.Query(q)
	return err
}

func GetLastRecordedHeight(influx client.Client, database string) (int64, error) {
	log.Info("Retrieving last record height")
	q := client.NewQuery(`SELECT "value" FROM "chain.height" ORDER BY time DESC LIMIT 1`, database, "")
	res, err := influx.Query(q)
	if err != nil {
		return 0, err
	}

	if len(res.Results) == 0 {
		return 0, fmt.Errorf("No results found for last recorded height")
	}

	if len(res.Results[0].Series) == 0 {
		return 0, fmt.Errorf("No results found for last recorded height")
	}

	height, err := (res.Results[0].Series[0].Values[0][1].(json.Number)).Int64()
	if err != nil {
		return 0, err
	}

	log.Infow("Last record height", "height", height)

	return height, nil
}
