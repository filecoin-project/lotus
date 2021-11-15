package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

const DefaultMaxRetrievePrice = "0.01"

func retrieve(ctx context.Context, cctx *cli.Context, fapi lapi.FullNode, sel *lapi.Selector, printf func(string, ...interface{})) (*lapi.ExportRef, error) {
	var payer address.Address
	var err error
	if cctx.String("from") != "" {
		payer, err = address.NewFromString(cctx.String("from"))
	} else {
		payer, err = fapi.WalletDefaultAddress(ctx)
	}
	if err != nil {
		return nil, err
	}

	file, err := cid.Parse(cctx.Args().Get(0))
	if err != nil {
		return nil, err
	}

	var pieceCid *cid.Cid
	if cctx.String("pieceCid") != "" {
		parsed, err := cid.Parse(cctx.String("pieceCid"))
		if err != nil {
			return nil, err
		}
		pieceCid = &parsed
	}

	var eref *lapi.ExportRef
	if cctx.Bool("allow-local") {
		imports, err := fapi.ClientListImports(ctx)
		if err != nil {
			return nil, err
		}

		for _, i := range imports {
			if i.Root != nil && i.Root.Equals(file) {
				eref = &lapi.ExportRef{
					Root:         file,
					FromLocalCAR: i.CARPath,
				}
				break
			}
		}
	}

	// no local found, so make a retrieval
	if eref == nil {
		var offer lapi.QueryOffer
		minerStrAddr := cctx.String("miner")
		if minerStrAddr == "" { // Local discovery
			offers, err := fapi.ClientFindData(ctx, file, pieceCid)

			var cleaned []lapi.QueryOffer
			// filter out offers that errored
			for _, o := range offers {
				if o.Err == "" {
					cleaned = append(cleaned, o)
				}
			}

			offers = cleaned

			// sort by price low to high
			sort.Slice(offers, func(i, j int) bool {
				return offers[i].MinPrice.LessThan(offers[j].MinPrice)
			})
			if err != nil {
				return nil, err
			}

			// TODO: parse offer strings from `client find`, make this smarter
			if len(offers) < 1 {
				fmt.Println("Failed to find file")
				return nil, nil
			}
			offer = offers[0]
		} else { // Directed retrieval
			minerAddr, err := address.NewFromString(minerStrAddr)
			if err != nil {
				return nil, err
			}
			offer, err = fapi.ClientMinerQueryOffer(ctx, minerAddr, file, pieceCid)
			if err != nil {
				return nil, err
			}
		}
		if offer.Err != "" {
			return nil, fmt.Errorf("offer error: %s", offer.Err)
		}

		maxPrice := types.MustParseFIL(DefaultMaxRetrievePrice)

		if cctx.String("maxPrice") != "" {
			maxPrice, err = types.ParseFIL(cctx.String("maxPrice"))
			if err != nil {
				return nil, xerrors.Errorf("parsing maxPrice: %w", err)
			}
		}

		if offer.MinPrice.GreaterThan(big.Int(maxPrice)) {
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

			printf("Recv %s, Paid %s, %s (%s), %s\n",
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

var clientRetrieveCmd = &cli.Command{
	Name: "retrieve",
	Subcommands: []*cli.Command{
		clientRetrieveCatCmd,
		clientRetrieveLsCmd,
	},
	Usage:     "Retrieve data from network",
	ArgsUsage: "[dataCid outputPath]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "address to send transactions from",
		},
		&cli.BoolFlag{
			Name:  "car",
			Usage: "export to a car file instead of a regular file",
		},
		&cli.StringFlag{
			Name:  "miner",
			Usage: "miner address for retrieval, if not present it'll use local discovery",
		},
		&cli.StringFlag{
			Name:  "datamodel-path-selector",
			Usage: "a rudimentary (DM-level-only) text-path selector, allowing for sub-selection within a deal",
		},
		&cli.StringFlag{
			Name:  "maxPrice",
			Usage: fmt.Sprintf("maximum price the client is willing to consider (default: %s FIL)", DefaultMaxRetrievePrice),
		},
		&cli.StringFlag{
			Name:  "pieceCid",
			Usage: "require data to be retrieved from a specific Piece CID",
		},
		&cli.BoolFlag{
			Name: "allow-local",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		fapi, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		afmt := NewAppFmt(cctx.App)

		var s *lapi.Selector
		if sel := lapi.Selector(cctx.String("datamodel-path-selector")); sel != "" {
			s = &sel
		}

		eref, err := retrieve(ctx, cctx, fapi, s, afmt.Printf)
		if err != nil {
			return err
		}

		if s != nil {
			eref.DAGs = append(eref.DAGs, lapi.DagSpec{DataSelector: s})
		}

		err = fapi.ClientExport(ctx, *eref, lapi.FileRef{
			Path:  cctx.Args().Get(1),
			IsCAR: cctx.Bool("car"),
		})
		if err != nil {
			return err
		}
		afmt.Println("Success")
		return nil
	},
}

func ClientExportStream(apiAddr string, apiAuth http.Header, eref lapi.ExportRef, car bool) (io.ReadCloser, error) {
	rj, err := json.Marshal(eref)
	if err != nil {
		return nil, xerrors.Errorf("marshaling export ref: %w", err)
	}

	ma, err := multiaddr.NewMultiaddr(apiAddr)
	if err == nil {
		_, addr, err := manet.DialArgs(ma)
		if err != nil {
			return nil, err
		}

		// todo: make cliutil helpers for this
		apiAddr = "http://" + addr
	}

	aa, err := url.Parse(apiAddr)
	if err != nil {
		return nil, xerrors.Errorf("parsing api address: %w", err)
	}
	switch aa.Scheme {
	case "ws":
		aa.Scheme = "http"
	case "wss":
		aa.Scheme = "https"
	}

	aa.Path = path.Join(aa.Path, "rest/v0/export")
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?car=%t&export=%s", aa, car, url.QueryEscape(string(rj))), nil)
	if err != nil {
		return nil, err
	}

	req.Header = apiAuth

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close() // nolint
		return nil, xerrors.Errorf("getting root car: http %d", resp.StatusCode)
	}

	return resp.Body, nil
}

var clientRetrieveCatCmd = &cli.Command{
	Name:      "cat",
	Usage:     "Show data from network",
	ArgsUsage: "[dataCid]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		ainfo, err := GetAPIInfo(cctx, repo.FullNode)
		if err != nil {
			return xerrors.Errorf("could not get API info: %w", err)
		}

		fapi, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		afmt := NewAppFmt(cctx.App)

		// todo selector
		eref, err := retrieve(ctx, cctx, fapi, nil, afmt.Printf)
		if err != nil {
			return err
		}

		if sel := lapi.Selector(cctx.String("datamodel-path-selector")); sel != "" {
			eref.DAGs = append(eref.DAGs, lapi.DagSpec{DataSelector: &sel})
		}

		rc, err := ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, false)
		if err != nil {
			return err
		}
		defer rc.Close() // nolint

		_, err = io.Copy(os.Stdout, rc)
		return err
	},
}

var clientRetrieveLsCmd = &cli.Command{
	Name:      "ls",
	Usage:     "Show object links",
	ArgsUsage: "[dataCid]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		ainfo, err := GetAPIInfo(cctx, repo.FullNode)
		if err != nil {
			return xerrors.Errorf("could not get API info: %w", err)
		}

		fapi, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		afmt := NewAppFmt(cctx.App)

		rootSelector := lapi.Selector(`{".": {}}`)
		dataSelector := lapi.Selector(`{"R":{"l":{"depth":1},":>":{"a":{">":{"@":{}}}}}}`)

		eref, err := retrieve(ctx, cctx, fapi, &dataSelector, afmt.Printf)
		if err != nil {
			return err
		}

		eref.DAGs = append(eref.DAGs, lapi.DagSpec{
			RootSelector: &rootSelector,
			DataSelector: &rootSelector,
		})

		rc, err := ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, true)
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
			return xerrors.Errorf("opening car blockstore: %w", err)
		}

		roots, err := cbs.Roots()
		if err != nil {
			return xerrors.Errorf("getting roots: %w", err)
		}

		if len(roots) != 1 {
			return xerrors.Errorf("expected 1 car root, got %d")
		}

		dserv := merkledag.NewDAGService(blockservice.New(cbs, offline.Exchange(cbs)))

		links, err := dserv.GetLinks(ctx, roots[0])
		if err != nil {
			return xerrors.Errorf("getting links: %w", err)
		}

		for _, link := range links {
			fmt.Printf("%s %s\t%d\n", link.Cid, link.Name, link.Size)
		}

		return err
	},
}

type bytesReaderAt struct {
	btr *bytes.Reader
}

func (b bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	return b.btr.ReadAt(p, off)
}

var _ io.ReaderAt = &bytesReaderAt{}
