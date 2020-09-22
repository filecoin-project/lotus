package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

type dealStatsServer struct {
	api api.FullNode
}

var filteredClients map[address.Address]bool

func init() {
	filteredClients = make(map[address.Address]bool)
	for _, a := range []string{"t0112", "t0113", "t0114", "t010089"} {
		addr, err := address.NewFromString(a)
		if err != nil {
			panic(err)
		}
		filteredClients[addr] = true
	}
}

type dealCountResp struct {
	Total int64 `json:"total"`
	Epoch int64 `json:"epoch"`
}

func filterDeals(deals map[string]api.MarketDeal) []*api.MarketDeal {
	out := make([]*api.MarketDeal, 0, len(deals))
	for _, d := range deals {
		if !filteredClients[d.Proposal.Client] {
			out = append(out, &d)
		}
	}
	return out
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

	var count int64
	for _, d := range deals {
		if !filteredClients[d.Proposal.Client] {
			count++
		}
	}

	if err := json.NewEncoder(w).Encode(&dealCountResp{
		Total: count,
		Epoch: int64(head.Height()),
	}); err != nil {
		log.Warnf("failed to write back deal count response: %s", err)
		return
	}
}

type dealAverageResp struct {
	AverageSize int64 `json:"average_size"`
	Epoch       int64 `json:"epoch"`
}

func (dss *dealStatsServer) handleStorageDealAverageSize(w http.ResponseWriter, r *http.Request) {
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

	var count int64
	var totalBytes int64
	for _, d := range deals {
		if !filteredClients[d.Proposal.Client] {
			count++
			totalBytes += int64(d.Proposal.PieceSize.Unpadded())
		}
	}

	if err := json.NewEncoder(w).Encode(&dealAverageResp{
		AverageSize: totalBytes / count,
		Epoch:       int64(head.Height()),
	}); err != nil {
		log.Warnf("failed to write back deal average response: %s", err)
		return
	}
}

type dealTotalResp struct {
	TotalBytes int64 `json:"total_size"`
	Epoch      int64 `json:"epoch"`
}

func (dss *dealStatsServer) handleStorageDealTotalReal(w http.ResponseWriter, r *http.Request) {
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

	var totalBytes int64
	for _, d := range deals {
		if !filteredClients[d.Proposal.Client] {
			totalBytes += int64(d.Proposal.PieceSize.Unpadded())
		}
	}

	if err := json.NewEncoder(w).Encode(&dealTotalResp{
		TotalBytes: totalBytes,
		Epoch:      int64(head.Height()),
	}); err != nil {
		log.Warnf("failed to write back deal average response: %s", err)
		return
	}

}

type clientStatsOutput struct {
	Client    address.Address `json:"client"`
	DataSize  int64           `json:"data_size"`
	NumCids   int             `json:"num_cids"`
	NumDeals  int             `json:"num_deals"`
	NumMiners int             `json:"num_miners"`

	cids      map[cid.Cid]bool
	providers map[address.Address]bool
}

func (dss *dealStatsServer) handleStorageClientStats(w http.ResponseWriter, r *http.Request) {
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

	stats := make(map[address.Address]*clientStatsOutput)

	for _, d := range deals {
		if filteredClients[d.Proposal.Client] {
			continue
		}

		st, ok := stats[d.Proposal.Client]
		if !ok {
			st = &clientStatsOutput{
				Client:    d.Proposal.Client,
				cids:      make(map[cid.Cid]bool),
				providers: make(map[address.Address]bool),
			}
			stats[d.Proposal.Client] = st
		}

		st.DataSize += int64(d.Proposal.PieceSize.Unpadded())
		st.cids[d.Proposal.PieceCID] = true
		st.providers[d.Proposal.Provider] = true
		st.NumDeals++
	}

	out := make([]*clientStatsOutput, 0, len(stats))
	for _, cso := range stats {
		cso.NumCids = len(cso.cids)
		cso.NumMiners = len(cso.providers)

		out = append(out, cso)
	}

	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Warnf("failed to write back client stats response: %s", err)
		return
	}
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

		mux := &http.ServeMux{}
		mux.HandleFunc("/api/storagedeal/count", dss.handleStorageDealCount)
		mux.HandleFunc("/api/storagedeal/averagesize", dss.handleStorageDealAverageSize)
		mux.HandleFunc("/api/storagedeal/totalreal", dss.handleStorageDealTotalReal)
		mux.HandleFunc("/api/storagedeal/clientstats", dss.handleStorageClientStats)

		s := &http.Server{
			Addr:    ":7272",
			Handler: mux,
		}

		go func() {
			<-ctx.Done()
			s.Shutdown(context.TODO())
		}()

		list, err := net.Listen("tcp", ":7272")
		if err != nil {
			panic(err)
		}

		s.Serve(list)
		return nil
	},
}
