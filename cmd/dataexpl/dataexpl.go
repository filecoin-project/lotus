package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	gopath "path"
	"sort"
	"sync"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	io2 "github.com/ipfs/go-unixfs/io"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	finderhttpclient "github.com/filecoin-project/storetheindex/api/v0/finder/client/http"

	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
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

			go func(a address.Address, mb lapi.MarketBalance) {
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
			}(a, mb)
		}
		wg.Wait()
		sort.Slice(mminers, func(i, j int) bool {
			return big.Cmp(abi.TokenAmount(mminers[i].Locked), abi.TokenAmount(mminers[j].Locked)) > 0
		})

		aurl, err := lcli.ApiAddrToUrl(ainfo.Addr)
		if err != nil {
			return err
		}

		apiBss := &apiBstoreServer{
			remoteAddr: aurl,
			stores:     map[uuid.UUID]bstore.Blockstore{},
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
