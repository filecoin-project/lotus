package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
)

type dealStatsServer struct {
	api api.FullNode
}

type dealCountResp struct {
	Total int64 `json:"total"`
	Epoch int64 `json:"epoch"`
}

func (dss *dealStatsServer) handleStorageDealCount(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	head, err := dss.api.ChainHead(ctx)
	if err != nil {
		log.Warnf("failed to get chain head: %s", err)
		w.WriteHeader(500)
		return
	}

	deals, err := dss.api.StateMarketDeals(ctx, head.Key())
	if err != nil {
		log.Warnf("failed to get market deals: %s", err)
		w.WriteHeader(500)
		return
	}

	if err := json.NewEncoder(w).Encode(&dealCountResp{
		Total: int64(len(deals)),
		Epoch: int64(head.Height()),
	}); err != nil {
		log.Warnf("failed to write back deal count response: %s", err)
		return
	}
}

func (dss *dealStatsServer) handleStorageDealAverageSize(w http.ResponseWriter, r *http.Request) {

}

func (dss *dealStatsServer) handleStorageDealTotalReal(w http.ResponseWriter, r *http.Request) {

}

var serveDealStatsCmd = &cli.Command{
	Name:  "serve-deal-stats",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		_ = ctx

		dss := &dealStatsServer{api}

		http.HandleFunc("/api/storagedeal/count", dss.handleStorageDealCount)
		http.HandleFunc("/api/storagedeal/averagesize", dss.handleStorageDealAverageSize)
		http.HandleFunc("/api/storagedeal/totalreal", dss.handleStorageDealTotalReal)

		panic(http.ListenAndServe(":7272", nil))
	},
}
