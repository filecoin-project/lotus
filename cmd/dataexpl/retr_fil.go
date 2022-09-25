package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/big"

	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

func getCarFilRetrieval(ainfo cliutil.APIInfo, api lapi.FullNode, r *http.Request, ma address.Address, pcid, dcid cid.Cid) func(ss builder.SelectorSpec) (io.ReadCloser, error) {
	return func(ss builder.SelectorSpec) (io.ReadCloser, error) {
		vars := mux.Vars(r)

		sel, err := pathToSel(vars["path"], false, ss)
		if err != nil {
			return nil, err
		}

		eref, err := retrieveFil(r.Context(), api, nil, ma, pcid, dcid, &sel)
		if err != nil {
			return nil, xerrors.Errorf("retrieve: %w", err)
		}

		eref.DAGs = append(eref.DAGs, lapi.DagSpec{
			DataSelector:      &sel,
			ExportMerkleProof: true,
		})

		rc, err := lcli.ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, true)
		if err != nil {
			return nil, err
		}

		return rc, nil
	}
}

func getFilRetrieval(abs *apiBstoreServer, api lapi.FullNode, r *http.Request, ma address.Address, pcid, dcid cid.Cid) selGetter {
	return func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, error) {
		vars := mux.Vars(r)

		sel, err := pathToSel(vars["path"], false, ss)
		if err != nil {
			return cid.Undef, nil, err
		}

		tmpcar, err := os.CreateTemp("", "retrcartmp")
		if err != nil {
			return cid.Cid{}, nil, xerrors.Errorf("creating tmp car: %w", err)
		}
		defer tmpcar.Close()

		cbs, err := blockstore.OpenReadWrite(tmpcar.Name(), []cid.Cid{dcid}, blockstore.UseWholeCIDs(false))
		if err != nil {
			return cid.Cid{}, nil, xerrors.Errorf("opening temp rw store: %w", err)
		}

		apistore := abs.MakeRemoteBstore(bstore.Adapt(cbs))

		eref, err := retrieveFil(r.Context(), api, &apistore, ma, pcid, dcid, &sel)
		if err != nil {
			return cid.Undef, nil, xerrors.Errorf("retrieve: %w", err)
		}

		eref.DAGs = append(eref.DAGs, lapi.DagSpec{
			DataSelector:      &sel,
			ExportMerkleProof: true,
		})

		bs := bstore.NewTieredBstore(bstore.Adapt(cbs), bstore.NewMemory())
		ds := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

		rs, err := textselector.SelectorSpecFromPath(textselector.Expression(vars["path"]), false, ss)
		if err != nil {
			return cid.Cid{}, nil, xerrors.Errorf("failed to parse path-selector: %w", err)
		}

		root, err := findRoot(r.Context(), dcid, rs, ds)
		return root, ds, err
	}
}

func retrieveFil(ctx context.Context, fapi lapi.FullNode, apiStore *lapi.RemoteStore, minerAddr address.Address, pieceCid, file cid.Cid, sel *lapi.Selector) (*lapi.ExportRef, error) {
	payer, err := fapi.WalletDefaultAddress(ctx)
	if err != nil {
		return nil, err
	}

	var eref *lapi.ExportRef

	// no local found, so make a retrieval
	if eref == nil {
		var offer lapi.QueryOffer
		{ // Directed retrieval
			offer, err = fapi.ClientMinerQueryOffer(ctx, minerAddr, file, &pieceCid)
			if err != nil {
				return nil, xerrors.Errorf("offer: %w", err)
			}
		}
		if offer.Err != "" {
			return nil, fmt.Errorf("offer error: %s", offer.Err)
		}

		maxPrice := big.Zero()
		//maxPrice := big.NewInt(6818260582400)

		if offer.MinPrice.GreaterThan(maxPrice) {
			return nil, xerrors.Errorf("failed to find offer satisfying maxPrice: %s (min %s, %s)", maxPrice, offer.MinPrice, types.FIL(offer.MinPrice))
		}

		o := offer.Order(payer)
		o.DataSelector = sel
		o.RemoteStore = apiStore

		subscribeEvents, err := fapi.ClientGetRetrievalUpdates(ctx)
		if err != nil {
			return nil, xerrors.Errorf("error setting up retrieval updates: %w", err)
		}
		retrievalRes, err := fapi.ClientRetrieve(ctx, o)
		if err != nil {
			return nil, xerrors.Errorf("error setting up retrieval: %w", err)
		}

		start := time.Now()
	readEvents:
		for {
			var evt lapi.RetrievalInfo
			select {
			case <-ctx.Done():
				go func() {
					err := fapi.ClientCancelRetrievalDeal(context.Background(), retrievalRes.DealID)
					if err != nil {
						log.Errorw("cancelling deal failed", "error", err)
					}
				}()

				return nil, xerrors.New("Retrieval Timed Out")
			case evt = <-subscribeEvents:
				if evt.ID != retrievalRes.DealID {
					// we can't check the deal ID ahead of time because:
					// 1. We need to subscribe before retrieving.
					// 2. We won't know the deal ID until after retrieving.
					continue
				}
			}

			event := "New"
			if evt.Event != nil {
				event = retrievalmarket.ClientEvents[*evt.Event]
			}

			fmt.Printf("Recv %s, Paid %s, %s (%s), %s\n",
				types.SizeStr(types.NewInt(evt.BytesReceived)),
				types.FIL(evt.TotalPaid),
				strings.TrimPrefix(event, "ClientEvent"),
				strings.TrimPrefix(retrievalmarket.DealStatuses[evt.Status], "DealStatus"),
				time.Now().Sub(start).Truncate(time.Millisecond),
			)

			switch evt.Status {
			case retrievalmarket.DealStatusCompleted:
				break readEvents
			case retrievalmarket.DealStatusRejected:
				return nil, xerrors.Errorf("Retrieval Proposal Rejected: %s", evt.Message)
			case
				retrievalmarket.DealStatusDealNotFound,
				retrievalmarket.DealStatusErrored:
				return nil, xerrors.Errorf("Retrieval Error: %s", evt.Message)
			}
		}

		eref = &lapi.ExportRef{
			Root:   file,
			DealID: retrievalRes.DealID,
		}
	}

	return eref, nil
}
