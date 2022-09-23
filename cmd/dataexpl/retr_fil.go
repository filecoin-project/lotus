package main

import (
	"bytes"
	"github.com/filecoin-project/go-address"
	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"golang.org/x/xerrors"
	"io"
	"net/http"
)

func getCarFilRetrieval(ainfo cliutil.APIInfo, api lapi.FullNode, r *http.Request, ma address.Address, pcid, dcid cid.Cid) func(ss builder.SelectorSpec) (io.ReadCloser, error) {
	return func(ss builder.SelectorSpec) (io.ReadCloser, error) {
		vars := mux.Vars(r)

		sel, err := pathToSel(vars["path"], false, ss)
		if err != nil {
			return nil, err
		}

		eref, err := retrieveFil(r.Context(), api, ma, pcid, dcid, &sel)
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

func getFilRetrieval(ainfo cliutil.APIInfo, api lapi.FullNode, r *http.Request, ma address.Address, pcid, dcid cid.Cid) selGetter {
	gc := getCarFilRetrieval(ainfo, api, r, ma, pcid, dcid)

	return func(ss builder.SelectorSpec) (cid.Cid, format.DAGService, error) {
		rc, err := gc(ss)
		if err != nil {
			return cid.Cid{}, nil, err
		}
		defer rc.Close() // nolint

		var memcar bytes.Buffer
		_, err = io.Copy(&memcar, rc)
		if err != nil {
			return cid.Undef, nil, err
		}

		cbs, err := blockstore.NewReadOnly(&bytesReaderAt{bytes.NewReader(memcar.Bytes())}, nil,
			carv2.ZeroLengthSectionAsEOF(true),
			blockstore.UseWholeCIDs(false))
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

		bs := bstore.NewTieredBstore(bstore.Adapt(cbs), bstore.NewMemory())

		return roots[0], merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs))), nil
	}
}
