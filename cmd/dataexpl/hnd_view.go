package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	gopath "path"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	io2 "github.com/ipfs/go-unixfs/io"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
)

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
	h.handleViewInner(w, r, g, map[string]interface{}{
		"filRetrieval": true,
		"provider":     ma,
		"pieceCid":     pcid,

		"dataCid": dcid,
	})
}

func (h *dxhnd) handleViewIPFS(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	dcid, err := cid.Parse(vars["cid"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sg := func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, map[string]struct{}, func(), error) {
		lbs, err := bstore.NewLocalIPFSBlockstore(r.Context(), true)
		if err != nil {
			return cid.Cid{}, nil, nil, nil, err
		}

		bs := bstore.NewTieredBstore(bstore.Adapt(lbs), bstore.NewMemory())
		ds := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

		rs, err := textselector.SelectorSpecFromPath(textselector.Expression(vars["path"]), false, ss)
		if err != nil {
			return cid.Cid{}, nil, nil, nil, xerrors.Errorf("failed to parse path-selector: %w", err)
		}

		root, links, err := findRoot(r.Context(), dcid, rs, ds)
		return root, ds, links, func() {}, err
	}

	h.handleViewInner(w, r, sg, map[string]interface{}{
		"filRetrieval": false,

		"dataCid": dcid,
	})
}

func (h *dxhnd) handleViewInner(w http.ResponseWriter, r *http.Request, g selGetter, tpldata map[string]interface{}) {
	vars := mux.Vars(r)
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

	root, dserv, links, done, err := g(sel)
	if err != nil {
		http.Error(w, xerrors.Errorf("inner get 0: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		done()
	}()

	tpldata["path"] = markLinkPaths(tplPathSegments(vars["path"]), links)

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
		h.handleViewIPLD(w, r, node, dserv, tpldata)
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
			tpldata["entries"] = links
			tpldata["url"] = r.URL.Path
			tpldata["carurl"] = carpath
			tpldata["desc"] = fmt.Sprintf("DIR (%d entries)", len(links))
			tpldata["node"] = node.Cid()
			if err := tpl.Execute(w, tpldata); err != nil {
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

			tpldata["entries"] = links
			tpldata["url"] = r.URL.Path
			tpldata["carurl"] = carpath
			tpldata["desc"] = fmt.Sprintf("DIR (%d entries)", len(links))
			tpldata["node"] = node.Cid()

			if err := tpl.Execute(w, tpldata); err != nil {
				fmt.Println(err)
				return
			}

			return
		case unixfs.TFile:
			w.Header().Set("X-HumanSize", types.SizeStr(types.NewInt(fsNode.FileSize())))

			var startoff, endoff uint64 = 0, fsNode.FileSize()

			rhead := r.Header.Get("Range")
			if rhead != "" {
				if !strings.HasPrefix(rhead, "bytes=") {
					http.Error(w, "no bytes= prefix in Range", http.StatusBadRequest)
					return
				}
				rhead = strings.TrimPrefix(rhead, "bytes=")
				seg := strings.Split(rhead, "-")
				if len(seg) != 2 {
					http.Error(w, "no support for multi-range", http.StatusBadRequest)
					return
				}

				if seg[0] != "" {
					startoff, err = strconv.ParseUint(seg[0], 10, 64)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				if seg[1] != "" {
					endoff, err = strconv.ParseUint(seg[1], 10, 64)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
			}

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

				_, _ = startoff, endoff // todo use those when range selectors work

				_, dserv, _, done, err = g(ssb.ExploreUnion(
					ssb.Matcher(),
					ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth), ssb.ExploreAll(ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreRecursiveEdge())))),
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

func getHeadReader(ctx context.Context, get selGetter, pss func(spec builder.SelectorSpec, ssb builder.SelectorSpecBuilder) builder.SelectorSpec) (io.ReadSeeker, error) {
	// (.) / Links / 0 / Hash @
	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	root, dserv, _, _, err := get(pss(ssb.ExploreRecursive(selector.RecursionLimitDepth(typeCheckDepth),
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

func tplPathSegments(p string) []string {
	return strings.Split(strings.TrimRight(p, "/"), "/")
}

type PathElem struct {
	Name string
	Link bool
}

func markLinkPaths(pseg []string, links map[string]struct{}) []PathElem {
	fmt.Printf("pss %#v\n", pseg)

	out := make([]PathElem, len(pseg))
	for i, s := range pseg {
		_, lnk := links[strings.Join(pseg[:i+1], "/")]

		out[i] = PathElem{
			Name: s,
			Link: lnk,
		}
	}
	return out
}
