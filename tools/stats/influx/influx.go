package influx

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/influxdata/influxdb1-client"
	"github.com/influxdata/influxdb1-client/models"
	client "github.com/influxdata/influxdb1-client/v2"

	"github.com/filecoin-project/lotus/build"
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

type WriteQueue struct {
	ch chan client.BatchPoints
}

func NewWriteQueue(ctx context.Context, influx client.Client) *WriteQueue {
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
						build.Clock.Sleep(3 * time.Second)
						continue
					}

					continue main
				}

				log.Error("dropping batch due to failure to write")
			}
		}
	}()

	return &WriteQueue{
		ch: ch,
	}
}

func (i *WriteQueue) AddBatch(bp client.BatchPoints) {
	i.ch <- bp
}

func (i *WriteQueue) Close() {
	close(i.ch)
}

func NewClient(addr, user, pass string) (client.Client, error) {
	return client.NewHTTPClient(client.HTTPConfig{
		Addr:     addr,
		Username: user,
		Password: pass,
	})
}

func NewBatch() (client.BatchPoints, error) {
	return client.NewBatchPoints(client.BatchPointsConfig{})
}

func NewPoint(name string, value interface{}) models.Point {
	pt, _ := models.NewPoint(name, models.Tags{},
		map[string]interface{}{"value": value}, build.Clock.Now().UTC())
	return pt
}

func NewPointFrom(p models.Point) *client.Point {
	return client.NewPointFrom(p)
}

func ResetDatabase(influx client.Client, database string) error {
	log.Debug("resetting database")
	q := client.NewQuery(fmt.Sprintf(`DROP DATABASE "%s"; CREATE DATABASE "%s";`, database, database), "", "")
	_, err := influx.Query(q)
	if err != nil {
		return err
	}
	log.Infow("database reset", "database", database)
	return nil
}

func GetLastRecordedHeight(influx client.Client, database string) (int64, error) {
	log.Debug("retrieving last record height")
	q := client.NewQuery(`SELECT "value" FROM "chain.height" ORDER BY time DESC LIMIT 1`, database, "")
	res, err := influx.Query(q)
	if err != nil {
		return 0, err
	}

	if len(res.Results) == 0 {
		return 0, fmt.Errorf("no results found for last recorded height")
	}

	if len(res.Results[0].Series) == 0 {
		return 0, fmt.Errorf("no results found for last recorded height")
	}

	height, err := (res.Results[0].Series[0].Values[0][1].(json.Number)).Int64()
	if err != nil {
		return 0, err
	}

	log.Infow("last record height", "height", height)

	return height, nil
}
