package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	gopath "path"
	"strings"
	txtempl "text/template"

	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfsnode"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
)

func (h *dxhnd) handleViewIPLD(w http.ResponseWriter, r *http.Request, node format.Node, dserv format.DAGService, tpldata map[string]interface{}) {
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

	tpldata["content"] = res

	tpldata["carurl"] = strings.Replace(r.URL.Path, "/view", "/car", 1)
	tpldata["url"] = r.URL.Path
	tpldata["ipfs"] = node.Cid().Type() == cid.DagProtobuf || node.Cid().Type() == cid.Raw

	tpldata["reinterpCbor"] = reinterpCbor
	tpldata["reinterpPB"] = reinterpPB
	tpldata["reinterpRaw"] = reinterpRaw

	tpldata["desc"] = ni.Desc
	tpldata["node"] = node.Cid()

	if err := tpl.Execute(w, tpldata); err != nil {
		fmt.Println(err)
		return
	}

}
