package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	gopath "path"
	"sort"
	"strconv"
	"strings"
	"sync"
	txtempl "text/template"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	io2 "github.com/ipfs/go-unixfs/io"
	"github.com/ipfs/go-unixfsnode"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/market"
	"github.com/filecoin-project/index-provider/metadata"
	finderhttpclient "github.com/filecoin-project/storetheindex/api/v0/finder/client/http"

	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/repo"
)

//go:embed dexpl
var dres embed.FS

var maxDirTypeChecks, typeCheckDepth int64 = 16, 15

type marketMiner struct {
	Addr   address.Address
	Locked types.FIL
}

type dxhnd struct {
	api    lapi.FullNode
	ainfo  cliutil.APIInfo
	apiBss *apiBstoreServer

	idx *finderhttpclient.Client

	mminers   []marketMiner
	minerPids map[peer.ID]address.Address
}

func (h *dxhnd) handleIndex(w http.ResponseWriter, r *http.Request) {
	tpl, err := template.ParseFS(dres, "dexpl/index.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}
}

func (h *dxhnd) handleMiners(w http.ResponseWriter, r *http.Request) {
	tpl, err := template.ParseFS(dres, "dexpl/miners.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"miners": h.mminers,
	}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}
}

func (h *dxhnd) handlePingMiner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	a, err := address.NewFromString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	mi, err := h.api.StateMinerInfo(ctx, a, types.EmptyTSK)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if mi.PeerId == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	multiaddrs := make([]multiaddr.Multiaddr, 0, len(mi.Multiaddrs))
	for i, a := range mi.Multiaddrs {
		maddr, err := multiaddr.NewMultiaddrBytes(a)
		if err != nil {
			log.Warnf("parsing multiaddr %d (%x): %s", i, a, err)
			continue
		}
		multiaddrs = append(multiaddrs, maddr)
	}

	pi := peer.AddrInfo{
		ID:    *mi.PeerId,
		Addrs: multiaddrs,
	}

	{
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		if err := h.api.NetConnect(ctx, pi); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	d, err := h.api.NetPing(ctx, pi.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%s %s", pi.ID, d.Round(time.Millisecond))))
}

func (h *dxhnd) handlePingLotus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)

	aijson, err := base64.URLEncoding.DecodeString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pi := peer.AddrInfo{}
	if err := json.Unmarshal(aijson, &pi); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	{
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		if err := h.api.NetConnect(ctx, pi); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	d, err := h.api.NetPing(ctx, pi.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%s", d.Round(time.Millisecond))))
}

func (h *dxhnd) handlePingIPFS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)

	aijson, err := base64.URLEncoding.DecodeString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pi := peer.AddrInfo{}
	if err := json.Unmarshal(aijson, &pi); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	iapi, err := httpapi.NewLocalApi()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	start := time.Now()

	// todo: not quite pinging

	if err := iapi.Swarm().Connect(ctx, pi); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	d := time.Now().Sub(start)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%s", d.Round(time.Millisecond))))
}

func (h *dxhnd) handleDeals(w http.ResponseWriter, r *http.Request) {
	deals, err := h.api.ClientListDeals(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tpl, err := template.ParseFS(dres, "dexpl/client_deals.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"deals":             deals,
		"StorageDealActive": storagemarket.StorageDealActive,
	}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}
}

func (h *dxhnd) handleMinerSectors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	ma, err := address.NewFromString(vars["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	commps := map[abi.DealID]cid.Cid{}
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

			lk.Lock()
			commps[deal] = md.Proposal.PieceCID
			lk.Unlock()
		}(deal)
	}
	wg.Wait()

	tpl, err := template.ParseFS(dres, "dexpl/sectors.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"maddr":   ma,
		"sectors": ms,
		"deals":   commps,
	}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}
}

func (h *dxhnd) handleDeal(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	did, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	d, err := h.api.StateMarketStorageDeal(ctx, abi.DealID(did), types.EmptyTSK)
	if err != nil {
		http.Error(w, xerrors.Errorf("StateMarketStorageDeal: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	lstr, err := d.Proposal.Label.ToString()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dcid, err := cid.Parse(lstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lstr = dcid.String()
	d.Proposal.Label, _ = market.NewLabelFromString(lstr) // if it's b64, will break urls

	var cdesc string

	{
		// get left side of the dag up to typeCheckDepth
		g := getFilRetrieval(h.apiBss, h.api, r, d.Proposal.Provider, d.Proposal.PieceCID, dcid)

		ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
		root, dserv, done, err := g(ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth),
			ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
				eb.Insert("Links", ssb.ExploreIndex(0, ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
					eb.Insert("Hash", ssb.ExploreRecursiveEdge())
				})))
			})),
		))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer done()

		// this gets type / size / linkcount for root

		desc, _, err := linkDesc(ctx, root, "", dserv)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cdesc = desc.Desc

		if desc.Size != "" {
			cdesc = fmt.Sprintf("%s %s", cdesc, desc.Size)
		}
	}

	now, err := h.api.ChainHead(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tpl, err := template.New("deal.gohtml").Funcs(map[string]interface{}{
		"EpochTime": func(e abi.ChainEpoch) string {
			return lcli.EpochTime(now.Height(), e)
		},
		"SizeStr": func(s abi.PaddedPieceSize) string {
			return types.SizeStr(types.NewInt(uint64(s)))
		},
	}).ParseFS(dres, "dexpl/deal.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"deal":  d,
		"label": lstr,
		"id":    did,

		"contentDesc": cdesc,
	}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}
}

func (h *dxhnd) handleViewFil(w http.ResponseWriter, r *http.Request) {
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

	dcid, err := cid.Parse(vars["cid"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// retr root
	g := getFilRetrieval(h.apiBss, h.api, r, ma, pcid, dcid)
	h.handleViewInner(w, r, g)
}

func (h *dxhnd) handleViewIPFS(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	dcid, err := cid.Parse(vars["cid"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sg := func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, func(), error) {
		lbs, err := bstore.NewLocalIPFSBlockstore(r.Context(), true)
		if err != nil {
			return cid.Cid{}, nil, nil, err
		}

		bs := bstore.NewTieredBstore(bstore.Adapt(lbs), bstore.NewMemory())
		ds := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

		rs, err := textselector.SelectorSpecFromPath(textselector.Expression(vars["path"]), false, ss)
		if err != nil {
			return cid.Cid{}, nil, nil, xerrors.Errorf("failed to parse path-selector: %w", err)
		}

		root, err := findRoot(r.Context(), dcid, rs, ds)
		return root, ds, func() {}, err
	}

	h.handleViewInner(w, r, sg)
}

func findRoot(ctx context.Context, root cid.Cid, selspec builder.SelectorSpec, ds format.DAGService) (cid.Cid, error) {
	rsn := selspec.Node()

	var newRoot cid.Cid
	var errHalt = errors.New("halt walk")
	if err := utils.TraverseDag(
		ctx,
		ds,
		root,
		rsn,
		nil,
		func(p traversal.Progress, n ipld.Node, r traversal.VisitReason) error {
			if r == traversal.VisitReason_SelectionMatch {
				if p.LastBlock.Path.String() != p.Path.String() {
					return xerrors.Errorf("unsupported selection path '%s' does not correspond to a block boundary (a.k.a. CID link)", p.Path.String())
				}

				if p.LastBlock.Link == nil {
					// this is likely the root node that we've matched here
					newRoot = root
					return errHalt
				}

				cidLnk, castOK := p.LastBlock.Link.(cidlink.Link)
				if !castOK {
					return xerrors.Errorf("cidlink cast unexpectedly failed on '%s'", p.LastBlock.Link)
				}

				newRoot = cidLnk.Cid

				return errHalt
			}
			return nil
		},
	); err != nil && err != errHalt {
		return cid.Undef, xerrors.Errorf("error while locating partial retrieval sub-root: %w", err)
	}

	if newRoot == cid.Undef {
		return cid.Undef, xerrors.Errorf("path selection does not match a node within %s", root)
	}
	return newRoot, nil
}

func (h *dxhnd) handleViewInner(w http.ResponseWriter, r *http.Request, g selGetter) {
	ctx := r.Context()

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	sel := ssb.Matcher()

	dirchecks := maxDirTypeChecks
	if r.FormValue("dirchecks") != "" {
		var err error
		dirchecks, err = strconv.ParseInt(r.FormValue("dirchecks"), 0, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if r.Method != "HEAD" && dirchecks > 0 {
		ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)

		sel = ssb.ExploreUnion(
			ssb.Matcher(),
			//ssb.ExploreInterpretAs("unixfs", ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreAll(ssb.Matcher()))),

			ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
				eb.Insert("Links", ssb.ExploreRange(0, dirchecks,
					ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
						eb.Insert("Hash", ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth),
							ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
								eb.Insert("Links", ssb.ExploreIndex(0, ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
									eb.Insert("Hash", ssb.ExploreRecursiveEdge())
								})))
							})),
						))
					}),
				))
			}),
		)
	}

	root, dserv, done, err := g(sel)
	if err != nil {
		http.Error(w, xerrors.Errorf("inner get 0: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		done()
	}()

	swapCodec := func(c cid.Cid, cd uint64) cid.Cid {
		if c.Prefix().Version == 0 && cd == cid.DagProtobuf {
			return cid.NewCidV0(c.Hash())
		}
		return cid.NewCidV1(cd, c.Hash())
	}

	switch r.FormValue("reinterp") {
	case "dag-cbor":
		root = swapCodec(root, cid.DagCBOR)
	case "dag-pb":
		root = swapCodec(root, cid.DagProtobuf)
	case "raw":
		root = swapCodec(root, cid.Raw)
	}

	node, err := dserv.Get(ctx, root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	viewIPLD := root.Type() == cid.DagCBOR || r.FormValue("view") == "ipld"

	if viewIPLD {
		h.handleViewIPLD(w, r, node, dserv)
		return
	}

	var rd interface {
		io.ReadSeeker
	}

	switch root.Type() {
	case cid.DagProtobuf:
		protoBufNode, ok := node.(*merkledag.ProtoNode)
		if !ok {
			http.Error(w, "not a dir", http.StatusInternalServerError)
			return
		}

		fsNode, err := unixfs.FSNodeFromBytes(protoBufNode.Data())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		carpath := strings.Replace(r.URL.Path, "/view", "/car", 1)

		switch fsNode.Type() {
		case unixfs.THAMTShard:
			w.Header().Set("X-HumanSize", types.SizeStr(types.NewInt(must(node.Size))))

			// TODO This in principle should be in the case below, and just use NewDirectoryFromNode; but that doesn't work
			// non-working example: http://127.0.0.1:5658/view/f01872811/baga6ea4seaqej7xagp2cmxdclpzqspd7zmu6dpnjt3qr2tywdzovqosmlvhxemi/bafybeif7kzcu2ezkkr34zu562kvutrsypr5o3rjeqrwgukellocbidfcsu/Links/3/Hash/Links/0/Hash/?filename=baf...qhgi
			ls := node.Links()

			links, err := parseLinks(ctx, ls, dserv, dirchecks)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			tpl, err := template.New("dir.gohtml").ParseFS(dres, "dexpl/dir.gohtml")
			if err != nil {
				fmt.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			data := map[string]interface{}{
				"entries": links,
				"url":     r.URL.Path,
				"carurl":  carpath,
			}
			if err := tpl.Execute(w, data); err != nil {
				fmt.Println(err)
				return
			}

			return
		case unixfs.TDirectory:
			w.Header().Set("X-HumanSize", types.SizeStr(types.NewInt(must(node.Size))))

			d, err := io2.NewDirectoryFromNode(dserv, node)
			if err != nil {
				http.Error(w, xerrors.Errorf("newdir: %w", err).Error(), http.StatusInternalServerError)
				return
			}

			ls, err := d.Links(ctx)
			if err != nil {
				http.Error(w, xerrors.Errorf("links: %w", err).Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("X-Desc", fmt.Sprintf("DIR (%d entries)", len(ls)))

			if r.Method == "HEAD" {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				return
			}

			links, err := parseLinks(ctx, ls, dserv, dirchecks)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			tpl, err := template.New("dir.gohtml").Funcs(map[string]any{
				"sizeClass": func(s string) string {
					switch {
					case strings.Contains(s, "GiB"):
						return "g"
					case strings.Contains(s, "MiB"):
						return "m"
					case strings.Contains(s, "KiB"):
						return "k"
					default:
						return "b"
					}
				},
			}).ParseFS(dres, "dexpl/dir.gohtml")
			if err != nil {
				fmt.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			data := map[string]interface{}{
				"entries": links,
				"url":     r.URL.Path,
				"carurl":  carpath,

				"desc": fmt.Sprintf("DIR (%d entries)", len(links)),
				"node": node.Cid(),
			}
			if err := tpl.Execute(w, data); err != nil {
				fmt.Println(err)
				return
			}

			return
		case unixfs.TFile:
			w.Header().Set("X-HumanSize", types.SizeStr(types.NewInt(fsNode.FileSize())))

			ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
			if r.Method == "HEAD" {
				done()
				done = func() {}

				rd, err = getHeadReader(ctx, g, func(spec builder.SelectorSpec, ssb builder.SelectorSpecBuilder) builder.SelectorSpec {
					return spec
				})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				break
			} else {
				done()
				_, dserv, done, err = g(
					ssb.ExploreInterpretAs("unixfs", ssb.Matcher()),
				)
			}
			if err != nil {
				http.Error(w, fmt.Sprintf("%+v", err), http.StatusInternalServerError)
				return
			}

			rd, err = io2.NewDagReader(ctx, node, dserv)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

		default:
			http.Error(w, "(2) unknown ufs type "+fmt.Sprint(fsNode.Type()), http.StatusInternalServerError)
			return
		}
	case cid.Raw:
		w.Header().Set("X-HumanSize", types.SizeStr(types.NewInt(must(node.Size))))
		rd = bytes.NewReader(node.RawData())

	default:
		// note - cbor in handled above

		http.Error(w, "unknown codec "+fmt.Sprint(root.Type()), http.StatusInternalServerError)
		return
	}

	{
		ctype := mime.TypeByExtension(gopath.Ext(r.FormValue("filename")))
		if ctype == "" {
			// uses https://github.com/gabriel-vasile/mimetype library to determine the content type.
			// Fixes https://github.com/ipfs/go-ipfs/issues/7252
			mimeType, err := mimetype.DetectReader(rd)
			if err != nil {
				http.Error(w, fmt.Sprintf("cannot detect content-type: %s", err.Error()), http.StatusInternalServerError)
				return
			}

			ctype = mimeType.String()
			_, err = rd.Seek(0, io.SeekStart)
			if err != nil {
				http.Error(w, "seeker can't seek", http.StatusInternalServerError)
				return
			}
		}
		if strings.HasPrefix(ctype, "text/html;") {
			ctype = "text/html"
		}

		w.Header().Set("Content-Type", ctype)
		if r.FormValue("filename") != "" {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, r.FormValue("filename")))
		}
	}

	http.ServeContent(w, r, r.FormValue("filename"), time.Time{}, rd)
}

func (h *dxhnd) handleViewIPLD(w http.ResponseWriter, r *http.Request, node format.Node, dserv format.DAGService) {
	ctx := r.Context()

	w.Header().Set("X-HumanSize", types.SizeStr(types.NewInt(must(node.Size))))

	// not sure what this is for TBH: we also provide ctx in  &traversal.Config{}
	linkContext := ipld.LinkContext{Ctx: ctx}

	// this is what allows us to understand dagpb
	nodePrototypeChooser := dagpb.AddSupportToChooser(
		func(ipld.Link, ipld.LinkContext) (ipld.NodePrototype, error) {
			return basicnode.Prototype.Any, nil
		},
	)

	// this is how we implement GETs
	linkSystem := cidlink.DefaultLinkSystem()
	linkSystem.StorageReadOpener = func(lctx ipld.LinkContext, lnk ipld.Link) (io.Reader, error) {
		cl, isCid := lnk.(cidlink.Link)
		if !isCid {
			return nil, fmt.Errorf("unexpected link type %#v", lnk)
		}

		if node.Cid() != cl.Cid {
			return nil, fmt.Errorf("not found")
		}

		return bytes.NewBuffer(node.RawData()), nil
	}
	unixfsnode.AddUnixFSReificationToLinkSystem(&linkSystem)

	// this is how we pull the start node out of the DS
	startLink := cidlink.Link{Cid: node.Cid()}
	startNodePrototype, err := nodePrototypeChooser(startLink, linkContext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	startNode, err := linkSystem.Load(
		linkContext,
		startLink,
		startNodePrototype,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var dumpNode func(n ipld.Node, p string) (string, error)
	dumpNode = func(n ipld.Node, recPath string) (string, error) {
		switch n.Kind() {
		case datamodel.Kind_Invalid:
			return `<span class="node">INVALID</span>`, nil
		case datamodel.Kind_Map:
			var inner string

			it := n.MapIterator()
			for !it.Done() {
				k, v, err := it.Next()
				if err != nil {
					return "", err
				}

				ks, err := dumpNode(k, recPath)
				if err != nil {
					return "", err
				}
				vs, err := dumpNode(v, recPath+must(k.AsString)+"/")
				if err != nil {
					return "", err
				}

				inner += fmt.Sprintf("<div class'multinode'><div class='node nodekey'>%s:</div><div class='subnode'>%s</div></div>", ks, vs)
			}

			return fmt.Sprintf(`<div class="node"><div>%s</div></div>`, inner), nil
		case datamodel.Kind_List:
			var inner string

			it := n.ListIterator()
			for !it.Done() {
				k, v, err := it.Next()
				if err != nil {
					return "", err
				}

				vs, err := dumpNode(v, recPath+fmt.Sprint(k)+"/")
				if err != nil {
					return "", err
				}

				inner += fmt.Sprintf("<div class'multinode'><div class='node nodekey'>%d:</div><div class='subnode'>%s</div></div>", k, vs)
			}

			return fmt.Sprintf(`<div class="node"><div>%s</div></div>`, inner), nil
		case datamodel.Kind_Null:
			return `<span class="node nullnode">NULL</span>`, nil
		case datamodel.Kind_Bool:
			return fmt.Sprintf(`<span class="node boolnode">%t</span>`, must(n.AsBool)), nil
		case datamodel.Kind_Int:
			return fmt.Sprintf(`<span class="node numnode">%d</span>`, must(n.AsInt)), nil
		case datamodel.Kind_Float:
			return fmt.Sprintf(`<span class="node numnode">%f</span>`, must(n.AsFloat)), nil
		case datamodel.Kind_String:
			return fmt.Sprintf(`<span class="node strnode">"%s"</span>`, template.HTMLEscapeString(must(n.AsString))), nil
		case datamodel.Kind_Bytes:
			b := must(n.AsBytes)

			var row string
			var printable string

			var rows []string
			var offsets []string

			var printableRows []string

			for i, byt := range b {
				row = fmt.Sprintf("%s%02x&nbsp;", row, byt)
				if i%16 == 7 {
					row += "&nbsp;"
				}

				switch {
				case byt == 0:
					printable = fmt.Sprintf("%s<span class='hex-noprint-null'>.</span>", printable)
				case byt > 0 && byt < ' ':
					printable = fmt.Sprintf("%s<span class='hex-noprint-low'>.</span>", printable)
				case byt == ' ':
					printable += "&nbsp;"
				case byt == '<':
					printable = printable + "&lt;"
				case byt == '>':
					printable = printable + "&gt;"
				case byt == '&':
					printable = printable + "&amp;"
				case byt > ' ' && byt <= '~':
					printable = fmt.Sprintf("%s%c", printable, byt)
				default:
					printable = fmt.Sprintf("%s<span class='hex-noprint-high'>.</span>", printable)
				}

				if i%16 == 15 {
					rows = append(rows, "<div class='hex-row'>"+row+"</div>")
					printableRows = append(printableRows, fmt.Sprintf("<div class='hex-printrow'>|%s|</div>", printable))
					offsets = append(offsets, fmt.Sprintf("<div class='hex-row'>%08x</div>", i-15))

					row = ""
					printable = ""
				}
			}

			if row != "" {
				rows = append(rows, "<div class='hex-row'>"+row+"</div>")
				printableRows = append(printableRows, fmt.Sprintf("<div class='hex-printrow'>|%s|</div>", printable))
				offsets = append(offsets, fmt.Sprintf("<div class='hex-row'>%08x</div>", (len(b)/16)*16))
			}

			return fmt.Sprintf(`<div class="node bytenode"><div class='hex-offsets'>%s</div><div class='hex-bytes'>%s</div><div class='hex-printables'>%s</div></div>`, strings.Join(offsets, "\n"), strings.Join(rows, "\n"), strings.Join(printableRows, "\n")), nil
		case datamodel.Kind_Link:
			lnk := must(n.AsLink)

			cl, isCid := lnk.(cidlink.Link)
			if !isCid {
				return "", fmt.Errorf("unexpected link type %#v", lnk)
			}

			ni, full, err := linkDesc(ctx, cl.Cid, gopath.Base(recPath), dserv)
			if err != nil {
				return "", xerrors.Errorf("link desc: %w", err)
			}
			ldescr := ni.Desc
			if ni.Size != "" {
				ldescr = fmt.Sprintf("%s %s", ldescr, ni.Size)
			}

			if !full {
				ldescr = fmt.Sprintf(`%s <a href="javascript:void(0)" onclick="checkDesc(this, '%s?filename=%s')">[?]</a>`, ldescr, recPath, gopath.Base(recPath))
			}

			carpath := strings.Replace(recPath, "/view", "/car", 1)

			return fmt.Sprintf(`<span class="node"><a href="%s">%s</a> <a href="%s">[car]</a><a href="%s">[ipld]</a><a href="/find/%s">[find]</a> <span>%s</span></span>`, recPath, lnk.String(), carpath, recPath+"?view=ipld", lnk.String(), ldescr), nil
		default:
			return `<span>UNKNOWN</span>`, nil
		}
	}

	// check if the node can be reinterpreted
	var reinterpRaw, reinterpCbor, reinterpPB bool

	switch node.Cid().Type() {
	case cid.Raw:
		if cbg.ScanForLinks(bytes.NewReader(node.RawData()), func(c cid.Cid) {}) == nil {
			reinterpCbor = true
		}
		if _, err := merkledag.DecodeProtobuf(node.RawData()); err == nil {
			reinterpPB = true
		}
	default:
		reinterpRaw = true
	}

	// get node info
	res, err := dumpNode(startNode, r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ni, _, err := linkDesc(ctx, node.Cid(), "", dserv)
	if err != nil {
		http.Error(w, xerrors.Errorf("linkdesc: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	if ni.Size != "" {
		ni.Desc = fmt.Sprintf("%s %s", ni.Desc, ni.Size)
	}

	tpl, err := txtempl.New("ipld.gohtml").ParseFS(dres, "dexpl/ipld.gohtml")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"content": res,

		"carurl": strings.Replace(r.URL.Path, "/view", "/car", 1),
		"url":    r.URL.Path,
		"ipfs":   node.Cid().Type() == cid.DagProtobuf || node.Cid().Type() == cid.Raw,

		"reinterpCbor": reinterpCbor,
		"reinterpPB":   reinterpPB,
		"reinterpRaw":  reinterpRaw,

		"desc": ni.Desc,
		"node": node.Cid(),
	}
	if err := tpl.Execute(w, data); err != nil {
		fmt.Println(err)
		return
	}

}

func (h *dxhnd) handleCar(w http.ResponseWriter, r *http.Request) {
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

	dcid, err := cid.Parse(vars["cid"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// retr root
	g := getCarFilRetrieval(h.ainfo, h.api, r, ma, pcid, dcid)

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	sel := ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreUnion(
		ssb.Matcher(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge()),
	))

	reader, err := g(sel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.ipld.car")

	name := r.FormValue("filename")
	if name == "" {
		name = dcid.String()
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.car"`, name))

	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
	_ = reader.Close()
}

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

var dataexplCmd = &cli.Command{
	Name:  "run",
	Usage: "Explore data stored on filecoin",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		idx, err := finderhttpclient.New("https://cid.contact")
		if err != nil {
			return err
		}

		ainfo, err := lcli.GetAPIInfo(cctx, repo.FullNode)
		if err != nil {
			return xerrors.Errorf("could not get API info: %w", err)
		}

		mpcs, err := api.StateMarketParticipants(ctx, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting market participants: %w", err)
		}

		var lk sync.Mutex
		var wg sync.WaitGroup
		var mminers []marketMiner
		pidMiners := map[peer.ID]address.Address{}

		fmt.Println("loading miner states")

		wg.Add(len(mpcs))
		for sa, mb := range mpcs {
			if mb.Locked.IsZero() {
				wg.Done()
				continue
			}

			a, err := address.NewFromString(sa)
			if err != nil {
				wg.Done()
				return err
			}

			go func() {
				defer wg.Done()

				mi, err := api.StateMinerInfo(ctx, a, types.EmptyTSK)
				if err == nil {
					lk.Lock()
					defer lk.Unlock()

					mminers = append(mminers, marketMiner{
						Addr:   a,
						Locked: types.FIL(mb.Locked),
					})

					if mi.PeerId != nil {
						pidMiners[*mi.PeerId] = a
					}
				}
			}()
		}
		wg.Wait()
		sort.Slice(mminers, func(i, j int) bool {
			return big.Cmp(abi.TokenAmount(mminers[i].Locked), abi.TokenAmount(mminers[j].Locked)) > 0
		})

		apiBss := &apiBstoreServer{
			urlPrefix: "http://127.0.0.1:5658/stores/",
			stores:    map[uuid.UUID]bstore.Blockstore{},
		}

		dh := &dxhnd{
			api:   api,
			ainfo: ainfo,

			idx: idx,

			mminers:   mminers,
			minerPids: pidMiners,

			apiBss: apiBss,
		}

		m := mux.NewRouter()

		var staticFS = fs.FS(dres)
		static, err := fs.Sub(staticFS, "dexpl")
		if err != nil {
			log.Fatal(err)
		}

		m.PathPrefix("/static/").Handler(http.FileServer(http.FS(static))).Methods("GET", "HEAD")

		m.HandleFunc("/", dh.handleIndex).Methods("GET")
		m.HandleFunc("/miners", dh.handleMiners).Methods("GET")
		m.HandleFunc("/ping/miner/{id}", dh.handlePingMiner).Methods("GET")
		m.HandleFunc("/ping/peer/ipfs/{id}", dh.handlePingIPFS).Methods("GET")
		m.HandleFunc("/ping/peer/lotus/{id}", dh.handlePingLotus).Methods("GET")
		m.HandleFunc("/deals", dh.handleDeals).Methods("GET")
		m.HandleFunc("/minersectors/{id}", dh.handleMinerSectors).Methods("GET")

		m.HandleFunc("/deal/{id}", dh.handleDeal).Methods("GET")
		m.HandleFunc("/view/ipfs/{cid}/{path:.*}", dh.handleViewIPFS).Methods("GET", "HEAD")
		m.HandleFunc("/view/{mid}/{piece}/{cid}/{path:.*}", dh.handleViewFil).Methods("GET", "HEAD")
		m.HandleFunc("/car/{mid}/{piece}/{cid}/{path:.*}", dh.handleCar).Methods("GET")

		m.HandleFunc("/matchdeal/{mid}/{piece}", dh.handleMatchPiece).Methods("GET")

		m.HandleFunc("/find/{cid}", dh.handleFind).Methods("GET")

		m.HandleFunc("/stores/put", apiBss.ServePut)
		m.HandleFunc("/stores/get", apiBss.ServeGet)

		server := &http.Server{Addr: ":5658", Handler: m, BaseContext: func(_ net.Listener) context.Context {
			return cctx.Context
		}}
		go func() {
			_ = server.ListenAndServe()
		}()

		fmt.Println("ready")

		<-ctx.Done()

		return nil
	},
}

type nodeInfo struct {
	Name string
	Size string
	Cid  cid.Cid
	Desc string
}

func parseLinks(ctx context.Context, ls []*format.Link, dserv format.DAGService, maxChecks int64) ([]nodeInfo, error) {
	links := make([]nodeInfo, len(ls))

	for i, l := range ls {
		links[i] = nodeInfo{
			Name: l.Name,
			Size: types.SizeStr(types.NewInt(l.Size)),
			Cid:  l.Cid,
		}

		if int64(i) < maxChecks {
			ni, _, err := linkDesc(ctx, l.Cid, l.Name, dserv)
			if err != nil {
				return nil, err
			}

			links[i].Desc = ni.Desc
		}
	}

	return links, nil
}

func linkDesc(ctx context.Context, c cid.Cid, name string, dserv format.DAGService) (*nodeInfo, bool, error) {
	var rrd interface {
		io.ReadSeeker
	}

	out := &nodeInfo{
		Cid: c,
	}

	switch c.Type() {
	case cid.DagProtobuf:
		out.Desc = "DAG-PB"

		fnode, err := dserv.Get(ctx, c)
		if err != nil {
			return out, false, nil
		}
		protoBufNode, ok := fnode.(*merkledag.ProtoNode)
		if !ok {
			out.Desc = "NOT!?-DAG-PB"
			return out, false, nil
		}
		fsNode, err := unixfs.FSNodeFromBytes(protoBufNode.Data())
		if err != nil {
			return out, true, err
		}

		switch fsNode.Type() {
		case unixfs.TDirectory:
			out.Desc = fmt.Sprintf("DIR (%d entries)", len(fnode.Links()))
			return out, true, nil
		case unixfs.THAMTShard:
			out.Desc = fmt.Sprintf("HAMT (%d links)", len(fnode.Links()))
			return out, true, nil
		case unixfs.TSymlink:
			out.Desc = fmt.Sprintf("SYMLINK")
			return out, true, nil
		case unixfs.TFile:
			out.Size = types.SizeStr(types.NewInt(fsNode.FileSize()))
		default:
			return nil, false, xerrors.Errorf("unknown ufs type " + fmt.Sprint(fsNode.Type()))
		}

		rrd, err = io2.NewDagReader(ctx, fnode, dserv)
		if err != nil {
			out.Desc = fmt.Sprintf("FILE (pb,e1:%s)", err)
			return out, true, nil
		}

		ctype := mime.TypeByExtension(gopath.Ext(name))
		if ctype == "" {
			mimeType, err := mimetype.DetectReader(rrd)
			if err != nil {
				out.Desc = fmt.Sprintf("FILE (pb,e2:%s)", err)
				return out, true, nil
			}

			ctype = mimeType.String()
		}

		out.Desc = fmt.Sprintf("FILE (pb,%s)", ctype)

		return out, true, nil
	case cid.Raw:
		fnode, err := dserv.Get(ctx, c)
		if err != nil {
			out.Desc = "RAW"
			return out, false, nil
		}

		out.Size = types.SizeStr(types.NewInt(uint64(len(fnode.RawData()))))

		rrd = bytes.NewReader(fnode.RawData())

		ctype := mime.TypeByExtension(gopath.Ext(name))
		if ctype == "" {
			mimeType, err := mimetype.DetectReader(rrd)
			if err != nil {
				return nil, false, err
			}

			ctype = mimeType.String()
		}

		out.Desc = fmt.Sprintf("FILE (raw,%s)", ctype)
		return out, true, nil
	case cid.DagCBOR:
		out.Desc = "DAG-CBOR"
		return out, true, nil
	default:
		out.Desc = fmt.Sprintf("UNK:0x%x", c.Type())
		return out, true, nil
	}
}

func getHeadReader(ctx context.Context, get selGetter, pss func(spec builder.SelectorSpec, ssb builder.SelectorSpecBuilder) builder.SelectorSpec) (io.ReadSeeker, error) {

	// (.) / Links / 0 / Hash @
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	root, dserv, _, err := get(pss(ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth),
		ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
			eb.Insert("Links", ssb.ExploreIndex(0, ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
				eb.Insert("Hash", ssb.ExploreRecursiveEdge())
			})))
		})),
	), ssb))
	if err != nil {
		return nil, err
	}

	node, err := dserv.Get(ctx, root)
	if err != nil {
		return nil, err
	}

	return io2.NewDagReader(ctx, node, dserv)
}

type selGetter func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, func(), error)

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

type bytesReaderAt struct {
	btr *bytes.Reader
}

func (b bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	return b.btr.ReadAt(p, off)
}

func must[T any](c func() (T, error)) T {
	res, err := c()
	if err != nil {
		panic(err)
	}
	return res
}

func noerr[T any](res T, err error) T {
	if err != nil {
		panic(err)
	}
	return res
}

var _ io.ReaderAt = &bytesReaderAt{}
