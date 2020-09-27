package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

type dealStatsServer struct {
	api api.FullNode
}

// Requested by @jbenet
// How many epochs back to look at for dealstats
var epochLookback = abi.ChainEpoch(10)

// these lists grow continuously with the network
// TODO: need to switch this to an LRU of sorts, to ensure refreshes
var knownFiltered = new(sync.Map)
var resolvedWallets = new(sync.Map)

func init() {
	for _, a := range []string{
		"t0100", // client for genesis miner
		"t0101", // client for genesis miner
		"t0102", // client for genesis miner
		"t0112", // client for genesis miner
		"t0113", // client for genesis miner
		"t0114", // client for genesis miner
		"t1nslxql4pck5pq7hddlzym3orxlx35wkepzjkm3i", // SR1 dealbot wallet
		"t1stghxhdp2w53dym2nz2jtbpk6ccd4l2lxgmezlq", // SR1 dealbot wallet
		"t1mcr5xkgv4jdl3rnz77outn6xbmygb55vdejgbfi", // SR1 dealbot wallet
		"t1qiqdbbmrdalbntnuapriirduvxu5ltsc5mhy7si", // SR1 dealbot wallet
	} {
		a, err := address.NewFromString(a)
		if err != nil {
			panic(err)
		}
		knownFiltered.Store(a, true)
	}
}

type dealCountResp struct {
	Epoch    int64  `json:"epoch"`
	Endpoint string `json:"endpoint"`
	Payload  int64  `json:"payload"`
}

func (dss *dealStatsServer) handleStorageDealCount(w http.ResponseWriter, r *http.Request) {

	epoch, deals := dss.filteredDealList()
	if epoch == 0 {
		w.WriteHeader(500)
		return
	}

	if err := json.NewEncoder(w).Encode(&dealCountResp{
		Endpoint: "COUNT_DEALS",
		Payload:  int64(len(deals)),
		Epoch:    epoch,
	}); err != nil {
		log.Warnf("failed to write back deal count response: %s", err)
		return
	}
}

type dealAverageResp struct {
	Epoch    int64  `json:"epoch"`
	Endpoint string `json:"endpoint"`
	Payload  int64  `json:"payload"`
}

func (dss *dealStatsServer) handleStorageDealAverageSize(w http.ResponseWriter, r *http.Request) {

	epoch, deals := dss.filteredDealList()
	if epoch == 0 {
		w.WriteHeader(500)
		return
	}

	var totalBytes int64
	for _, d := range deals {
		totalBytes += int64(d.deal.Proposal.PieceSize.Unpadded())
	}

	if err := json.NewEncoder(w).Encode(&dealAverageResp{
		Endpoint: "AVERAGE_DEAL_SIZE",
		Payload:  totalBytes / int64(len(deals)),
		Epoch:    epoch,
	}); err != nil {
		log.Warnf("failed to write back deal average response: %s", err)
		return
	}
}

type dealTotalResp struct {
	Epoch    int64  `json:"epoch"`
	Endpoint string `json:"endpoint"`
	Payload  int64  `json:"payload"`
}

func (dss *dealStatsServer) handleStorageDealTotalReal(w http.ResponseWriter, r *http.Request) {
	epoch, deals := dss.filteredDealList()
	if epoch == 0 {
		w.WriteHeader(500)
		return
	}

	var totalBytes int64
	for _, d := range deals {
		totalBytes += int64(d.deal.Proposal.PieceSize.Unpadded())
	}

	if err := json.NewEncoder(w).Encode(&dealTotalResp{
		Endpoint: "DEAL_BYTES",
		Payload:  totalBytes,
		Epoch:    epoch,
	}); err != nil {
		log.Warnf("failed to write back deal average response: %s", err)
		return
	}

}

type clientStatsOutput struct {
	Epoch    int64          `json:"epoch"`
	Endpoint string         `json:"endpoint"`
	Payload  []*clientStats `json:"payload"`
}

type clientStats struct {
	Client    address.Address `json:"client"`
	DataSize  int64           `json:"data_size"`
	NumCids   int             `json:"num_cids"`
	NumDeals  int             `json:"num_deals"`
	NumMiners int             `json:"num_miners"`

	cids      map[cid.Cid]bool
	providers map[address.Address]bool
}

func (dss *dealStatsServer) handleStorageClientStats(w http.ResponseWriter, r *http.Request) {
	epoch, deals := dss.filteredDealList()
	if epoch == 0 {
		w.WriteHeader(500)
		return
	}

	stats := make(map[address.Address]*clientStats)

	for _, d := range deals {

		st, ok := stats[d.deal.Proposal.Client]
		if !ok {
			st = &clientStats{
				Client:    d.resolvedWallet,
				cids:      make(map[cid.Cid]bool),
				providers: make(map[address.Address]bool),
			}
			stats[d.deal.Proposal.Client] = st
		}

		st.DataSize += int64(d.deal.Proposal.PieceSize.Unpadded())
		st.cids[d.deal.Proposal.PieceCID] = true
		st.providers[d.deal.Proposal.Provider] = true
		st.NumDeals++
	}

	out := clientStatsOutput{
		Epoch:    epoch,
		Endpoint: "CLIENT_DEAL_STATS",
		Payload:  make([]*clientStats, 0, len(stats)),
	}
	for _, cs := range stats {
		cs.NumCids = len(cs.cids)
		cs.NumMiners = len(cs.providers)
		out.Payload = append(out.Payload, cs)
	}

	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Warnf("failed to write back client stats response: %s", err)
		return
	}
}

type dealInfo struct {
	deal           api.MarketDeal
	resolvedWallet address.Address
}

// filteredDealList returns the current epoch and a list of filtered deals
// on error returns an epoch of 0
func (dss *dealStatsServer) filteredDealList() (int64, map[string]dealInfo) {
	ctx := context.Background()

	head, err := dss.api.ChainHead(ctx)
	if err != nil {
		log.Warnf("failed to get chain head: %s", err)
		return 0, nil
	}

	head, err = dss.api.ChainGetTipSetByHeight(ctx, head.Height()-epochLookback, head.Key())
	if err != nil {
		log.Warnf("failed to walk back %s epochs: %s", epochLookback, err)
		return 0, nil
	}

	// Disabled as per @pooja's request
	//
	// // Exclude any address associated with a miner
	// miners, err := dss.api.StateListMiners(ctx, head.Key())
	// if err != nil {
	// 	log.Warnf("failed to get miner list: %s", err)
	// 	return 0, nil
	// }
	// for _, m := range miners {
	// 	info, err := dss.api.StateMinerInfo(ctx, m, head.Key())
	// 	if err != nil {
	// 		log.Warnf("failed to get info for known miner '%s': %s", m, err)
	// 		continue
	// 	}

	// 	knownFiltered.Store(info.Owner, true)
	// 	knownFiltered.Store(info.Worker, true)
	// 	for _, a := range info.ControlAddresses {
	// 		knownFiltered.Store(a, true)
	// 	}
	// }

	deals, err := dss.api.StateMarketDeals(ctx, head.Key())
	if err != nil {
		log.Warnf("failed to get market deals: %s", err)
		return 0, nil
	}

	ret := make(map[string]dealInfo, len(deals))
	for dealKey, d := range deals {

		// Counting no-longer-active deals as per Pooja's request
		// // https://github.com/filecoin-project/specs-actors/blob/v0.9.9/actors/builtin/market/deal.go#L81-L85
		// if d.State.SectorStartEpoch < 0 {
		// 	continue
		// }

		if _, isFiltered := knownFiltered.Load(d.Proposal.Client); isFiltered {
			continue
		}

		if _, wasSeen := resolvedWallets.Load(d.Proposal.Client); !wasSeen {
			w, err := dss.api.StateAccountKey(ctx, d.Proposal.Client, head.Key())
			if err != nil {
				log.Warnf("failed to resolve id '%s' to wallet address: %s", d.Proposal.Client, err)
				continue
			} else {
				resolvedWallets.Store(d.Proposal.Client, w)
			}
		}

		w, _ := resolvedWallets.Load(d.Proposal.Client)
		if _, isFiltered := knownFiltered.Load(w); isFiltered {
			continue
		}

		ret[dealKey] = dealInfo{
			deal:           d,
			resolvedWallet: w.(address.Address),
		}
	}

	return int64(head.Height()), ret
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
			if err := s.Shutdown(context.TODO()); err != nil {
				log.Error(err)
			}
		}()

		list, err := net.Listen("tcp", ":7272") // nolint
		if err != nil {
			panic(err)
		}

		log.Warnf("deal-stat server listening on %s\n== NOTE: QUERIES ARE EXPENSIVE - YOU MUST FRONT-CACHE THIS SERVICE\n", list.Addr().String())

		return s.Serve(list)
	},
}
