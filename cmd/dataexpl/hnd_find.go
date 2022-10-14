package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	txtempl "text/template"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/index-provider/metadata"

	"github.com/filecoin-project/lotus/chain/types"
)

func (h *dxhnd) handleFind(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	dcid, err := cid.Parse(vars["cid"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	resp, err := h.idx.Find(ctx, dcid.Hash())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type providerProto struct {
		Provider peer.AddrInfo
		BasePI   string
		Protocol string

		MinerAddr string

		Piece         cid.Cid
		VerifiedDeal  bool
		FastRetrieval bool
	}

	var providers []providerProto

	for _, result := range resp.MultihashResults {
		for _, providerResult := range result.ProviderResults {
			var meta metadata.Metadata
			if err := meta.UnmarshalBinary(providerResult.Metadata); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			for _, proto := range meta.Protocols() {
				res := providerProto{
					Provider: providerResult.Provider,
					BasePI:   base64.URLEncoding.EncodeToString(noerr(json.Marshal(providerResult.Provider))),
				}

				if maddr, found := h.minerPids[providerResult.Provider.ID]; found {
					res.MinerAddr = maddr.String()
				}

				switch p := meta.Get(proto).(type) {
				case *metadata.GraphsyncFilecoinV1:
					res.Protocol = "Graphsync"

					res.Piece = p.PieceCID
					res.VerifiedDeal = p.VerifiedDeal
					res.FastRetrieval = p.FastRetrieval
				case *metadata.Bitswap:
					res.Protocol = "Bitswap"
				}

				providers = append(providers, res)
			}
		}
	}

	tpl, err := txtempl.New("find.gohtml").ParseFS(dres, "dexpl/find.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"root":      dcid,
		"providers": providers,
	}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}
}

func (h *dxhnd) handleMatchPiece(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ma, err := address.NewFromString(vars["mid"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pcid, err := cid.Parse(vars["piece"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	ms, err := h.api.StateMinerSectors(ctx, ma, nil, types.EmptyTSK)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var deals []abi.DealID
	for _, info := range ms {
		for _, d := range info.DealIDs {
			deals = append(deals, d)
		}
	}

	var matchedDeals []abi.DealID
	var wg sync.WaitGroup
	wg.Add(len(deals))
	var lk sync.Mutex

	for _, deal := range deals {
		go func(deal abi.DealID) {
			defer wg.Done()

			md, err := h.api.StateMarketStorageDeal(ctx, deal, types.EmptyTSK)
			if err != nil {
				return
			}

			if md.Proposal.PieceCID.Equals(pcid) {
				lk.Lock()
				matchedDeals = append(matchedDeals, deal)
				lk.Unlock()
			}
		}(deal)
	}
	wg.Wait()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(noerr(json.Marshal(matchedDeals)))
}
