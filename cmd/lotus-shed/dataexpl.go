package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	io2 "github.com/ipfs/go-unixfs/io"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	gopath "path"
	"strconv"
	"strings"
	"time"
)

//go:embed dexpl
var dres embed.FS

var dataexplCmd = &cli.Command{
	Name:  "dataexpl",
	Usage: "Explore data stored on filecoin",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		ainfo, err := lcli.GetAPIInfo(cctx, repo.FullNode)
		if err != nil {
			return xerrors.Errorf("could not get API info: %w", err)
		}

		m := mux.NewRouter()

		m.HandleFunc("/minersectors/{id}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			ma, err := address.NewFromString(vars["id"])
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			ms, err := api.StateMinerSectors(ctx, ma, nil, types.EmptyTSK)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			tpl, err := template.ParseFS(dres, "dexpl/deals.gohtml")
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
			}
			if err := tpl.Execute(w, data); err != nil {
				fmt.Println(err)
				return
			}

		}).Methods("GET")

		m.HandleFunc("/deal/{id}", func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			did, err := strconv.ParseInt(vars["id"], 10, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			d, err := api.StateMarketStorageDeal(ctx, abi.DealID(did), types.EmptyTSK)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			dcid, err := cid.Parse(d.Proposal.Label)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			d.Proposal.Label = dcid.String() // if it's b64, will break urls

			var typ string
			var sz string
			var linkc int

			{
				var dserv format.DAGService
				var root cid.Cid
				get := func(ss builder.SelectorSpec) error {
					sel := lapi.Selector(`{".": {}}`)

					eref, err := retrieve(r.Context(), api, d.Proposal.Provider, d.Proposal.PieceCID, dcid, &sel)
					if err != nil {
						return err
					}

					eref.DAGs = append(eref.DAGs, lapi.DagSpec{
						DataSelector: &sel,
					})

					rc, err := lcli.ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, true)
					if err != nil {
						return err
					}
					defer rc.Close() // nolint

					var memcar bytes.Buffer
					_, err = io.Copy(&memcar, rc)
					if err != nil {
						return err
					}

					cbs, err := blockstore.NewReadOnly(&bytesReaderAt{bytes.NewReader(memcar.Bytes())}, nil,
						carv2.ZeroLengthSectionAsEOF(true),
						blockstore.UseWholeCIDs(true))
					if err != nil {
						return err
					}

					roots, err := cbs.Roots()
					if err != nil {
						return err
					}

					if len(roots) != 1 {
						return xerrors.Errorf("wanted one root")
					}
					root = roots[0]
					dserv = merkledag.NewDAGService(blockservice.New(cbs, offline.Exchange(cbs)))

					return nil
				}

				ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
				err = get(ssb.Matcher())
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				node, err := dserv.Get(ctx, root)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				var s uint64

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

					s = fsNode.FileSize()

					switch fsNode.Type() {
					case unixfs.TDirectory:
						d, err := io2.NewDirectoryFromNode(dserv, node)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							return
						}

						ls, err := d.Links(ctx)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							return
						}
						linkc = len(ls)
						typ = "DIR"
					case unixfs.TFile:
						typ = "FILE"
					default:
						http.Error(w, "unknown ufs type "+fmt.Sprint(fsNode.Type()), http.StatusInternalServerError)
						return
					}
				case cid.Raw:
					typ = "FILE"
					s = uint64(len(node.RawData()))

				default:
					http.Error(w, "unknown codec "+fmt.Sprint(root.Type()), http.StatusInternalServerError)
					return
				}

				sz = types.SizeStr(types.NewInt(s))
			}

			tpl, err := template.ParseFS(dres, "dexpl/deal.gohtml")
			if err != nil {
				fmt.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			data := map[string]interface{}{
				"deal": d,
				"id":   did,

				"type":  typ,
				"size":  sz,
				"links": linkc,
			}
			if err := tpl.Execute(w, data); err != nil {
				fmt.Println(err)
				return
			}

		}).Methods("GET")

		m.HandleFunc("/view/{mid}/{piece}/{cid}/{path:.*}", func(w http.ResponseWriter, r *http.Request) {
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

			fmt.Println(vars["path"])

			// retr root

			var dserv format.DAGService
			var root cid.Cid
			get := func(ss builder.SelectorSpec) error {
				sel, err := pathToSel(vars["path"], false, ss)
				if err != nil {
					return err
				}

				eref, err := retrieve(r.Context(), api, ma, pcid, dcid, &sel)
				if err != nil {
					return err
				}

				eref.DAGs = append(eref.DAGs, lapi.DagSpec{
					DataSelector: &sel,
				})

				rc, err := lcli.ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, true)
				if err != nil {
					return err
				}
				defer rc.Close() // nolint

				var memcar bytes.Buffer
				_, err = io.Copy(&memcar, rc)
				if err != nil {
					return err
				}

				cbs, err := blockstore.NewReadOnly(&bytesReaderAt{bytes.NewReader(memcar.Bytes())}, nil,
					carv2.ZeroLengthSectionAsEOF(true),
					blockstore.UseWholeCIDs(true))
				if err != nil {
					return err
				}

				roots, err := cbs.Roots()
				if err != nil {
					return err
				}

				if len(roots) != 1 {
					return xerrors.Errorf("wanted one root")
				}
				root = roots[0]
				dserv = merkledag.NewDAGService(blockservice.New(cbs, offline.Exchange(cbs)))

				return nil
			}

			ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
			err = get(ssb.Matcher())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			node, err := dserv.Get(ctx, root)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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

				switch fsNode.Type() {
				case unixfs.TDirectory:
					d, err := io2.NewDirectoryFromNode(dserv, node)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

					ls, err := d.Links(ctx)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

					type le struct {
						Name string
						Size string
						Cid  cid.Cid
					}

					links := make([]le, len(ls))
					for i, l := range ls {
						links[i] = le{
							Name: l.Name,
							Size: types.SizeStr(types.NewInt(l.Size)),
							Cid:  l.Cid,
						}
					}

					tpl, err := template.ParseFS(dres, "dexpl/dir.gohtml")
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
					}
					if err := tpl.Execute(w, data); err != nil {
						fmt.Println(err)
						return
					}

					return
				case unixfs.TFile:
					ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
					err = get(ssb.ExploreRecursive(selector.RecursionLimitNone(), ssb.ExploreAll(ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreRecursiveEdge()))))
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

					rd, err = io2.NewDagReader(ctx, node, dserv)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

				default:
					http.Error(w, "unknown ufs type "+fmt.Sprint(fsNode.Type()), http.StatusInternalServerError)
					return
				}
			case cid.Raw:
				rd = bytes.NewReader(node.RawData())
			default:
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
		}).Methods("GET")

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

func retrieve(ctx context.Context, fapi lapi.FullNode, minerAddr address.Address, pieceCid, file cid.Cid, sel *lapi.Selector) (*lapi.ExportRef, error) {
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
				return nil, err
			}
		}
		if offer.Err != "" {
			return nil, fmt.Errorf("offer error: %s", offer.Err)
		}

		maxPrice := big.Zero()

		if offer.MinPrice.GreaterThan(maxPrice) {
			return nil, xerrors.Errorf("failed to find offer satisfying maxPrice: %s", maxPrice)
		}

		o := offer.Order(payer)
		o.DataSelector = sel

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

func pathToSel(psel string, matchTraversal bool, sub builder.SelectorSpec) (lapi.Selector, error) {
	rs, err := textselector.SelectorSpecFromPath(textselector.Expression(psel), matchTraversal, sub)
	if err != nil {
		return "", xerrors.Errorf("failed to parse path-selector: %w", err)
	}

	var b bytes.Buffer
	if err := dagjson.Encode(rs.Node(), &b); err != nil {
		return "", err
	}

	return lapi.Selector(b.String()), nil
}

type bytesReaderAt struct {
	btr *bytes.Reader
}

func (b bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	return b.btr.ReadAt(p, off)
}

var _ io.ReaderAt = &bytesReaderAt{}
