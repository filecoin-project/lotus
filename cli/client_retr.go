package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-state-types/big"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/markets/utils"
	"github.com/filecoin-project/lotus/node/repo"
)

const DefaultMaxRetrievePrice = "0"

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
		minerStrAddr := cctx.String("provider")
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

var retrFlagsCommon = []cli.Flag{
	&cli.StringFlag{
		Name:  "from",
		Usage: "address to send transactions from",
	},
	&cli.StringFlag{
		Name:    "provider",
		Usage:   "provider to use for retrieval, if not present it'll use local discovery",
		Aliases: []string{"miner"},
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
		// todo: default to true?
	},
}

var clientRetrieveCmd = &cli.Command{
	Name:      "retrieve",
	Usage:     "Retrieve data from network",
	ArgsUsage: "[dataCid outputPath]",
	Description: `Retrieve data from the Filecoin network.

The retrieve command will attempt to find a provider make a retrieval deal with
them. In case a provider can't be found, it can be specified with the --provider
flag.

By default the data will be interpreted as DAG-PB UnixFSv1 File. Alternatively
a CAR file containing the raw IPLD graph can be exported by setting the --car
flag.

Partial Retrieval:

The --data-selector flag can be used to specify a sub-graph to fetch. The
selector can be specified as either IPLD datamodel text-path selector, or IPLD
json selector.

In case of unixfs retrieval, the selector must point at a single root node, and
match the entire graph under that node.

In case of CAR retrieval, the selector must have one common "sub-root" node.

Examples:

- Retrieve a file by CID
	$ lotus client retrieve Qm... my-file.txt

- Retrieve a file by CID from f0123
	$ lotus client retrieve --provider f0123 Qm... my-file.txt

- Retrieve a first file from a specified directory
	$ lotus client retrieve --data-selector /Links/0/Hash Qm... my-file.txt
`,
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:  "car",
			Usage: "Export to a car file instead of a regular file",
		},
		&cli.StringFlag{
			Name:    "data-selector",
			Aliases: []string{"datamodel-path-selector"},
			Usage:   "IPLD datamodel text-path selector, or IPLD json selector",
		},
		&cli.BoolFlag{
			Name:  "car-export-merkle-proof",
			Usage: "(requires --data-selector and --car) Export data-selector merkle proof",
		},
	}, retrFlagsCommon...),
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return ShowHelp(cctx, fmt.Errorf("incorrect number of arguments"))
		}

		if cctx.Bool("car-export-merkle-proof") {
			if !cctx.Bool("car") || !cctx.IsSet("data-selector") {
				return ShowHelp(cctx, fmt.Errorf("--car-export-merkle-proof requires --car and --data-selector"))
			}
		}

		fapi, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)
		afmt := NewAppFmt(cctx.App)

		var s *lapi.Selector
		if sel := lapi.Selector(cctx.String("data-selector")); sel != "" {
			s = &sel
		}

		eref, err := retrieve(ctx, cctx, fapi, s, afmt.Printf)
		if err != nil {
			return err
		}

		if s != nil {
			eref.DAGs = append(eref.DAGs, lapi.DagSpec{DataSelector: s, ExportMerkleProof: cctx.Bool("car-export-merkle-proof")})
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
		em, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, xerrors.Errorf("reading error body: %w", err)
		}

		resp.Body.Close() // nolint
		return nil, xerrors.Errorf("getting root car: http %d: %s", resp.StatusCode, string(em))
	}

	return resp.Body, nil
}

var clientRetrieveCatCmd = &cli.Command{
	Name:      "cat",
	Usage:     "Show data from network",
	ArgsUsage: "[dataCid]",
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:  "ipld",
			Usage: "list IPLD datamodel links",
		},
		&cli.StringFlag{
			Name:  "data-selector",
			Usage: "IPLD datamodel text-path selector, or IPLD json selector",
		},
	}, retrFlagsCommon...),
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

		sel := lapi.Selector(cctx.String("data-selector"))
		selp := &sel
		if sel == "" {
			selp = nil
		}

		eref, err := retrieve(ctx, cctx, fapi, selp, afmt.Printf)
		if err != nil {
			return err
		}

		fmt.Println() // separate retrieval events from results

		if sel != "" {
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

var clientRetrieveLsCmd = &cli.Command{
	Name:      "ls",
	Usage:     "List object links",
	ArgsUsage: "[dataCid]",
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:  "ipld",
			Usage: "list IPLD datamodel links",
		},
		&cli.IntFlag{
			Name:  "depth",
			Usage: "list links recursively up to the specified depth",
			Value: 1,
		},
		&cli.StringFlag{
			Name:  "data-selector",
			Usage: "IPLD datamodel text-path selector, or IPLD json selector",
		},
	}, retrFlagsCommon...),
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

		dataSelector := lapi.Selector(fmt.Sprintf(`{"R":{"l":{"depth":%d},":>":{"a":{">":{"|":[{"@":{}},{".":{}}]}}}}}`, cctx.Int("depth")))

		if cctx.IsSet("data-selector") {
			ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
			dataSelector, err = pathToSel(cctx.String("data-selector"), cctx.Bool("ipld"),
				ssb.ExploreUnion(
					ssb.Matcher(),
					ssb.ExploreAll(
						ssb.ExploreRecursive(selector.RecursionLimitDepth(int64(cctx.Int("depth"))), ssb.ExploreAll(ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreRecursiveEdge()))),
					)))
			if err != nil {
				return xerrors.Errorf("parsing datamodel path: %w", err)
			}
		}

		eref, err := retrieve(ctx, cctx, fapi, &dataSelector, afmt.Printf)
		if err != nil {
			return xerrors.Errorf("retrieve: %w", err)
		}

		fmt.Println() // separate retrieval events from results

		eref.DAGs = append(eref.DAGs, lapi.DagSpec{
			DataSelector: &dataSelector,
		})

		rc, err := ClientExportStream(ainfo.Addr, ainfo.AuthHeader(), *eref, true)
		if err != nil {
			return xerrors.Errorf("export: %w", err)
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
			return xerrors.Errorf("expected 1 car root, got %d", len(roots))
		}
		dserv := merkledag.NewDAGService(blockservice.New(cbs, offline.Exchange(cbs)))

		if !cctx.Bool("ipld") {
			links, err := dserv.GetLinks(ctx, roots[0])
			if err != nil {
				return xerrors.Errorf("getting links: %w", err)
			}

			for _, link := range links {
				fmt.Printf("%s %s\t%d\n", link.Cid, link.Name, link.Size)
			}
		} else {
			jsel := lapi.Selector(fmt.Sprintf(`{"R":{"l":{"depth":%d},":>":{"a":{">":{"|":[{"@":{}},{".":{}}]}}}}}`, cctx.Int("depth")))

			if cctx.IsSet("data-selector") {
				ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
				jsel, err = pathToSel(cctx.String("data-selector"), false,
					ssb.ExploreRecursive(selector.RecursionLimitDepth(int64(cctx.Int("depth"))), ssb.ExploreAll(ssb.ExploreUnion(ssb.Matcher(), ssb.ExploreRecursiveEdge()))),
				)
			}

			sel, _ := selectorparse.ParseJSONSelector(string(jsel))

			if err := utils.TraverseDag(
				ctx,
				dserv,
				roots[0],
				sel,
				func(p traversal.Progress, n ipld.Node, r traversal.VisitReason) error {
					if r == traversal.VisitReason_SelectionMatch {
						fmt.Println(p.Path)
					}
					return nil
				},
			); err != nil {
				return err
			}
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
