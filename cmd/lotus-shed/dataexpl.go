package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
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

var maxDirTypeChecks, typeCheckDepth int64 = 16, 15

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

		m.HandleFunc("/deals", func(w http.ResponseWriter, r *http.Request) {
			deals, err := api.ClientListDeals(r.Context())
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
				"deals": deals,
				"StorageDealActive": storagemarket.StorageDealActive,
			}
			if err := tpl.Execute(w, data); err != nil {
				fmt.Println(err)
				return
			}

		}).Methods("GET")

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
			}
			if err := tpl.Execute(w, data); err != nil {
				fmt.Println(err)
				return
			}

		}).Methods("GET")

		getHeadReader := func(get func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, error), pss func(spec builder.SelectorSpec, ssb builder.SelectorSpecBuilder) builder.SelectorSpec) (io.ReadSeeker, error) {

			// (.) / Links / 0 / Hash @
			ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
			root, dserv, err := get(pss(ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth),
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

		get := func(r *http.Request, ma address.Address, pcid, dcid cid.Cid) func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, error) {
			return func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, error) {
				vars := mux.Vars(r)

				sel, err := pathToSel(vars["path"], false, ss)
				if err != nil {
					return cid.Undef, nil, err
				}

				eref, err := retrieve(r.Context(), api, ma, pcid, dcid, &sel)
				if err != nil {
					return cid.Undef, nil, err
				}

				eref.DAGs = append(eref.DAGs, lapi.DagSpec{
					DataSelector: &sel,
				})

				rc, err := lcli.ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, true)
				if err != nil {
					return cid.Undef, nil, err
				}
				defer rc.Close() // nolint

				var memcar bytes.Buffer
				_, err = io.Copy(&memcar, rc)
				if err != nil {
					return cid.Undef, nil, err
				}

				cbs, err := blockstore.NewReadOnly(&bytesReaderAt{bytes.NewReader(memcar.Bytes())}, nil,
					carv2.ZeroLengthSectionAsEOF(true),
					blockstore.UseWholeCIDs(true))
				if err != nil {
					return cid.Undef, nil, err
				}

				roots, err := cbs.Roots()
				if err != nil {
					return cid.Undef, nil, err
				}

				if len(roots) != 1 {
					return cid.Undef, nil, xerrors.Errorf("wanted one root")
				}

				return roots[0], merkledag.NewDAGService(blockservice.New(cbs, offline.Exchange(cbs))), nil
			}
		}

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
				ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
				g := get(r, d.Proposal.Provider, d.Proposal.PieceCID, dcid)
				root, dserv, err := g(ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth),
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
						rd, err := io2.NewDagReader(ctx, node, dserv)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							return
						}

						mimeType, err := mimetype.DetectReader(rd)
						if err != nil {
							http.Error(w, fmt.Sprintf("cannot detect content-type: %s", err.Error()), http.StatusInternalServerError)
							return
						}

						typ = fmt.Sprintf("FILE(%s)", mimeType)

					default:
						http.Error(w, "unknown ufs type "+fmt.Sprint(fsNode.Type()), http.StatusInternalServerError)
						return
					}
				case cid.Raw:
					typ = "FILE"
					mimeType, err := mimetype.DetectReader(bytes.NewReader(node.RawData()))
					if err != nil {
						http.Error(w, fmt.Sprintf("cannot detect content-type: %s", err.Error()), http.StatusInternalServerError)
						return
					}

					typ = fmt.Sprintf("FILE(%s)", mimeType)
					s = uint64(len(node.RawData()))

				default:
					http.Error(w, "unknown codec "+fmt.Sprint(root.Type()), http.StatusInternalServerError)
					return
				}

				sz = types.SizeStr(types.NewInt(s))
			}

			now, err := api.ChainHead(r.Context())
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

			// retr root
			g := get(r, ma, pcid, dcid)

			ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
			sel := ssb.Matcher()

			if r.Method != "HEAD" {
				ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
				sel = ssb.ExploreUnion(
					ssb.Matcher(),
					ssb.ExploreFields(func(eb builder.ExploreFieldsSpecBuilder) {
						eb.Insert("Links", ssb.ExploreRange(0, maxDirTypeChecks,
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

			root, dserv, err := g(sel)
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

					w.Header().Set("X-Desc", fmt.Sprintf("DIR (%d entries)", len(ls)))

					if r.Method == "HEAD" {
						w.Header().Set("Content-Type", "text/html")
						w.WriteHeader(http.StatusOK)
						return
					}

					type le struct {
						Name string
						Size string
						Cid  cid.Cid
						Desc string
					}

					links := make([]le, len(ls))
					for i, l := range ls {
						links[i] = le{
							Name: l.Name,
							Size: types.SizeStr(types.NewInt(l.Size)),
							Cid:  l.Cid,
						}

						if int64(i) < maxDirTypeChecks {
							var rrd interface {
								io.ReadSeeker
							}

							switch l.Cid.Type() {
							case cid.DagProtobuf:
								fnode, err := dserv.Get(ctx, l.Cid)
								if err != nil {
									http.Error(w, err.Error(), http.StatusInternalServerError)
									return
								}
								protoBufNode, ok := fnode.(*merkledag.ProtoNode)
								if !ok {
									links[i].Desc = "?? (e1)"
									continue
								}
								fsNode, err := unixfs.FSNodeFromBytes(protoBufNode.Data())
								if err != nil {
									http.Error(w, err.Error(), http.StatusInternalServerError)
									return
								}

								switch fsNode.Type() {
								case unixfs.TDirectory:
									links[i].Desc = fmt.Sprintf("DIR (%d entries)", len(fnode.Links()))
									continue
								case unixfs.TSymlink:
									links[i].Desc = fmt.Sprintf("LINK")
									continue
								case unixfs.TFile:
								default:
									http.Error(w, "unknown ufs type "+fmt.Sprint(fsNode.Type()), http.StatusInternalServerError)
									return
								}

								rrd, err = io2.NewDagReader(ctx, fnode, dserv)
								if err != nil {
									http.Error(w, err.Error(), http.StatusInternalServerError)
									return
								}

								fallthrough
							case cid.Raw:
								if rrd == nil {
									rrd = bytes.NewReader(node.RawData())
								}

								ctype := mime.TypeByExtension(gopath.Ext(l.Name))
								if ctype == "" {
									mimeType, err := mimetype.DetectReader(rrd)
									if err != nil {
										http.Error(w, fmt.Sprintf("cannot detect content-type: %s", err.Error()), http.StatusInternalServerError)
										return
									}

									ctype = mimeType.String()
								}

								links[i].Desc = fmt.Sprintf("FILE (%s)", ctype)
							}
						}
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
					}
					if err := tpl.Execute(w, data); err != nil {
						fmt.Println(err)
						return
					}

					return
				case unixfs.TFile:
					ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
					if r.Method == "HEAD" {
						rd, err = getHeadReader(g, func(spec builder.SelectorSpec, ssb builder.SelectorSpecBuilder) builder.SelectorSpec {
							return spec
						})
						break
					} else {
						_, dserv, err = g(ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth), ssb.ExploreAll(ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreRecursiveEdge()))))
					}
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
		}).Methods("GET", "HEAD")

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
