package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
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

type selGetter func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, map[string]struct{}, func(), error)

func getCarFilRetrieval(ainfo cliutil.APIInfo, api lapi.FullNode, r *http.Request, ma address.Address, pcid, dcid cid.Cid) func(ss builder.SelectorSpec) (io.ReadCloser, error) {
	return func(ss builder.SelectorSpec) (io.ReadCloser, error) {
		vars := mux.Vars(r)

		sel, err := pathToSel(vars["path"], false, ss)
		if err != nil {
			return nil, err
		}

		eref, done, err := retrieveFil(r.Context(), api, nil, ma, pcid, dcid, &sel, nil)
		if err != nil {
			return nil, xerrors.Errorf("retrieve: %w", err)
		}
		defer done()

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
	cbs := bstore.Blockstore(bstore.NewMemorySync())

	return func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, map[string]struct{}, func(), error) {
		vars := mux.Vars(r)

		sel, err := pathToSel(vars["path"], false, ss)
		if err != nil {
			return cid.Undef, nil, nil, nil, err
		}

		bbs := &blockReadBs{
			finalized: make(chan struct{}),
			waiting:   map[string]chan struct{}{},
			sub:       bstore.Adapt(cbs),
		}
		cbs = bbs

		storeid, err := abs.MakeRemoteBstore(context.TODO(), cbs)
		if err != nil {
			return cid.Cid{}, nil, nil, nil, err
		}

		eref, done, err := retrieveFil(r.Context(), api, &storeid, ma, pcid, dcid, &sel, bbs.Finalize)
		if err != nil {
			return cid.Undef, nil, nil, nil, xerrors.Errorf("retrieve: %w", err)
		}

		eref.DAGs = append(eref.DAGs, lapi.DagSpec{
			DataSelector:      &sel,
			ExportMerkleProof: true,
		})

		bs := bstore.NewTieredBstore(cbs, bstore.NewMemory())
		ds := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

		rs, err := textselector.SelectorSpecFromPath(textselector.Expression(vars["path"]), false, ss)
		if err != nil {
			return cid.Cid{}, nil, nil, nil, xerrors.Errorf("failed to parse path-selector: %w", err)
		}

		root, links, err := findRoot(r.Context(), dcid, rs, ds)
		return root, ds, links, done, err
	}
}

func retrieveFil(ctx context.Context, fapi lapi.FullNode, apiStore *lapi.RemoteStoreID, minerAddr address.Address, pieceCid, file cid.Cid, sel *lapi.Selector, retrDone func()) (*lapi.ExportRef, func(), error) {
	payer, err := fapi.WalletDefaultAddress(ctx)
	if err != nil {
		return nil, nil, err
	}

	var eref *lapi.ExportRef

	// no local found, so make a retrieval
	if eref == nil {
		var offer lapi.QueryOffer
		{ // Directed retrieval
			offer, err = fapi.ClientMinerQueryOffer(ctx, minerAddr, file, &pieceCid)
			if err != nil {
				return nil, nil, xerrors.Errorf("offer: %w", err)
			}
		}
		if offer.Err != "" {
			return nil, nil, fmt.Errorf("offer error: %s", offer.Err)
		}

		maxPrice := big.Zero()
		//maxPrice := big.NewInt(6818260582400)

		if offer.MinPrice.GreaterThan(maxPrice) {
			return nil, nil, xerrors.Errorf("failed to find offer satisfying maxPrice: %s (min %s, %s)", maxPrice, offer.MinPrice, types.FIL(offer.MinPrice))
		}

		o := offer.Order(payer)
		o.DataSelector = sel
		o.RemoteStore = apiStore

		subscribeEvents, err := fapi.ClientGetRetrievalUpdates(ctx)
		if err != nil {
			return nil, nil, xerrors.Errorf("error setting up retrieval updates: %w", err)
		}
		retrievalRes, err := fapi.ClientRetrieve(ctx, o)
		if err != nil {
			return nil, nil, xerrors.Errorf("error setting up retrieval: %w", err)
		}

		start := time.Now()
		resCh := make(chan error, 1)
		var resSent bool

		go func() {
			defer func() {
				if retrDone != nil {
					retrDone()
				}
			}()

			for {
				var evt lapi.RetrievalInfo
				select {
				case <-ctx.Done():
					if !resSent {
						go func() {
							err := fapi.ClientCancelRetrievalDeal(context.Background(), retrievalRes.DealID)
							if err != nil {
								log.Errorw("cancelling deal failed", "error", err)
							}
						}()
					}

					resCh <- xerrors.New("Retrieval Timed Out")
					return
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
				case retrievalmarket.DealStatusOngoing:
					if apiStore != nil && !resSent {
						resCh <- nil
						resSent = true
					}
				case retrievalmarket.DealStatusCompleted:
					if !resSent {
						resCh <- nil
					}
					return
				case retrievalmarket.DealStatusRejected:
					if !resSent {
						resCh <- xerrors.Errorf("Retrieval Proposal Rejected: %s", evt.Message)
					}
					return
				case
					retrievalmarket.DealStatusDealNotFound,
					retrievalmarket.DealStatusErrored:
					if !resSent {
						resCh <- xerrors.Errorf("Retrieval Error: %s", evt.Message)
					}
					return
				}
			}

		}()

		err = <-resCh
		if err != nil {
			return nil, nil, err
		}

		eref = &lapi.ExportRef{
			Root:   file,
			DealID: retrievalRes.DealID,
		}
		return eref, func() {
			err := fapi.ClientCancelRetrievalDeal(context.Background(), retrievalRes.DealID)
			if err != nil {
				log.Errorw("cancelling deal failed", "error", err)
			}
		}, nil
	}

	return eref, func() {}, nil
}

func pathToSel(psel string, matchTraversal bool, sub builder.SelectorSpec) (lapi.Selector, error) {
	rs, err := textselector.SelectorSpecFromPath(textselector.Expression(psel), matchTraversal, sub)
	if err != nil {
		return "", xerrors.Errorf("failed to parse path-selector: %w", err)
	}

	var b bytes.Buffer
	if err := dagjson.Encode(rs.Node(), &b); err != nil {
		return "", err
	}

	fmt.Println(b.String())

	return lapi.Selector(b.String()), nil
}
