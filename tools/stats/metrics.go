package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	_ "github.com/influxdata/influxdb1-client"
	models "github.com/influxdata/influxdb1-client/models"
	client "github.com/influxdata/influxdb1-client/v2"
)

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

		p := NewPoint("chain.blockheader_size", len(bs))
		pl.AddPoint(p)
	}

	return nil
}

func RecordTipsetStatePoints(ctx context.Context, api api.FullNode, pl *PointList, tipset *types.TipSet) error {
	pc, err := api.StatePledgeCollateral(ctx, tipset)
	if err != nil {
		return err
	}

	pcfil := types.BigDiv(pc, types.NewInt(uint64(build.FilecoinPrecision)))
	p := NewPoint("chain.pledge_collateral", pcfil.Int64())
	pl.AddPoint(p)

	power, err := api.StateMinerPower(ctx, address.Address{}, tipset)
	if err != nil {
		return err
	}

	p = NewPoint("chain.power", power.TotalPower.Int64())
	pl.AddPoint(p)

	miners, err := api.StateListMiners(ctx, tipset)
	for _, miner := range miners {
		power, err := api.StateMinerPower(ctx, miner, tipset)
		if err != nil {
			return err
		}

		p = NewPoint("chain.miner_power", power.MinerPower.Int64())
		p.AddTag("miner", miner.String())
		pl.AddPoint(p)
	}

	return nil
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

	for i, msg := range msgs {
		p := NewPoint("chain.message_gasprice", msg.Message.GasPrice.Int64())
		pl.AddPoint(p)

		bs, err := msg.Message.Serialize()
		if err != nil {
			return err
		}

		p = NewPoint("chain.message_size", len(bs))
		pl.AddPoint(p)

		actor, err := api.StateGetActor(ctx, msg.Message.To, tipset)
		if err != nil {
			return err
		}

		p = NewPoint("chain.message_count", 1)
		p.AddTag("actor", actor.Code.String())
		p.AddTag("method", fmt.Sprintf("%d", msg.Message.Method))
		p.AddTag("exitcode", fmt.Sprintf("%d", recp[i].ExitCode))
		pl.AddPoint(p)

	}

	return nil
}

func ResetDatabase(influx client.Client, database string) error {
	log.Print("Resetting database")
	q := client.NewQuery(fmt.Sprintf(`DROP DATABASE "%s"; CREATE DATABASE "%s";`, database, database), "", "")
	_, err := influx.Query(q)
	return err
}

func GetLastRecordedHeight(influx client.Client, database string) (int64, error) {
	log.Print("Retrieving last record height")
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

	log.Printf("Last record height %d", height)

	return height, nil
}
