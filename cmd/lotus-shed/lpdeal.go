package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/lib/nullreader"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/provider/lpmarket"
	"github.com/filecoin-project/lotus/provider/lpmarket/fakelm"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var lpUtilCmd = &cli.Command{
	Name:  "provider-util",
	Usage: "lotus provider utility commands",
	Subcommands: []*cli.Command{
		lpUtilStartDealCmd,
		lpBoostProxyCmd,
	},
}

var lpUtilStartDealCmd = &cli.Command{
	Name:      "start-deal",
	Usage:     "start a deal with a specific lotus-provider instance",
	ArgsUsage: "[dataFile] [miner]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "provider-rpc",
			Value: "http://127.0.0.1:12300",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.Errorf("expected 2 arguments")
		}

		maddr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parse miner address: %w", err)
		}

		full, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		defAddr, err := full.WalletDefaultAddress(ctx)
		if err != nil {
			return xerrors.Errorf("get default address: %w", err)
		}

		// open rpc
		var rpc api.LotusProviderStruct
		closer2, err := jsonrpc.NewMergeClient(ctx, cctx.String("provider-rpc"), "Filecoin", []interface{}{&rpc.Internal}, nil)
		if err != nil {
			return xerrors.Errorf("open rpc: %w", err)
		}
		defer closer2()

		v, err := rpc.Version(ctx)
		if err != nil {
			return xerrors.Errorf("rpc version: %w", err)
		}

		fmt.Printf("* provider version: %s\n", v.String())

		// open data file
		data, err := homedir.Expand(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("get data file: %w", err)
		}

		df, err := os.Open(data)
		if err != nil {
			return xerrors.Errorf("open data file: %w", err)
		}

		dstat, err := df.Stat()
		if err != nil {
			return xerrors.Errorf("stat data file: %w", err)
		}

		// compute commd
		color.Green("> computing piece CID\n")

		writer := new(commp.Calc)
		_, err = io.Copy(writer, df)
		if err != nil {
			return xerrors.Errorf("compute commd copy: %w", err)
		}

		commp, pps, err := writer.Digest()
		if err != nil {
			return xerrors.Errorf("compute commd: %w", err)
		}

		pieceCid, err := commcid.PieceCommitmentV1ToCID(commp)
		if err != nil {
			return xerrors.Errorf("make pieceCid: %w", err)
		}

		fmt.Printf("* piece CID: %s\n", pieceCid)
		fmt.Printf("* piece size: %d\n", pps)

		// start serving the file
		color.Green("> starting temp http server\n")

		deleteCalled := make(chan struct{})

		mux := http.NewServeMux()
		mux.HandleFunc("/"+pieceCid.String(), func(w http.ResponseWriter, r *http.Request) {
			// log request and method
			color.Blue("< %s %s\n", r.Method, r.URL)

			if r.Method == http.MethodDelete {
				close(deleteCalled)
				return
			}

			http.ServeFile(w, r, data)
		})

		ts := httptest.NewServer(mux)

		dataUrl, err := url.Parse(ts.URL)
		if err != nil {
			return xerrors.Errorf("parse data url: %w", err)
		}
		dataUrl.Path = "/" + pieceCid.String()

		fmt.Printf("* data url: %s\n", dataUrl)

		// publish the deal
		color.Green("> publishing deal\n")

		head, err := full.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("get chain head: %w", err)
		}

		verif := false

		bds, err := full.StateDealProviderCollateralBounds(ctx, abi.PaddedPieceSize(pps), verif, head.Key())
		if err != nil {
			return xerrors.Errorf("get provider collateral bounds: %w", err)
		}

		pcoll := big.Mul(bds.Min, big.NewInt(2))

		dealProposal := market.DealProposal{
			PieceCID:             pieceCid,
			PieceSize:            abi.PaddedPieceSize(pps),
			VerifiedDeal:         verif,
			Client:               defAddr,
			Provider:             maddr,
			Label:                must.One(market.NewLabelFromString("lotus-shed-made-this")),
			StartEpoch:           head.Height() + 2000,
			EndEpoch:             head.Height() + 2880*300,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   pcoll,
			ClientCollateral:     big.Zero(),
		}

		pbuf, err := cborutil.Dump(&dealProposal)
		if err != nil {
			return xerrors.Errorf("dump deal proposal: %w", err)
		}

		sig, err := full.WalletSign(ctx, defAddr, pbuf)
		if err != nil {
			return xerrors.Errorf("sign deal proposal: %w", err)
		}

		params := market.PublishStorageDealsParams{
			Deals: []market.ClientDealProposal{
				{
					Proposal:        dealProposal,
					ClientSignature: *sig,
				},
			},
		}

		var buf bytes.Buffer
		err = params.MarshalCBOR(&buf)
		if err != nil {
			return xerrors.Errorf("marshal params: %w", err)
		}

		msg := &types.Message{
			To:     builtin.StorageMarketActorAddr,
			From:   defAddr,
			Method: builtin.MethodsMarket.PublishStorageDeals,
			Params: buf.Bytes(),
		}

		smsg, err := full.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return xerrors.Errorf("push message: %w", err)
		}

		fmt.Printf("* PSD message cid: %s\n", smsg.Cid())

		// wait for deal to be published
		color.Green("> waiting for PublishStorageDeals to land on chain\n")

		rcpt, err := full.StateWaitMsg(ctx, smsg.Cid(), 3)
		if err != nil {
			return xerrors.Errorf("wait message: %w", err)
		}

		if rcpt.Receipt.ExitCode != 0 {
			return xerrors.Errorf("publish deal failed: exit code %d", rcpt.Receipt.ExitCode)
		}

		// parse results
		var ret market.PublishStorageDealsReturn
		err = ret.UnmarshalCBOR(bytes.NewReader(rcpt.Receipt.Return))
		if err != nil {
			return xerrors.Errorf("unmarshal return: %w", err)
		}

		if len(ret.IDs) != 1 {
			return xerrors.Errorf("expected 1 deal id, got %d", len(ret.IDs))
		}

		dealId := ret.IDs[0]

		fmt.Printf("* deal id: %d\n", dealId)

		// start deal
		color.Green("> starting deal\n")

		pcid := smsg.Cid()

		pdi := api.PieceDealInfo{
			PublishCid:   &pcid,
			DealID:       dealId,
			DealProposal: &dealProposal,
			DealSchedule: api.DealSchedule{
				StartEpoch: dealProposal.StartEpoch,
				EndEpoch:   dealProposal.EndEpoch,
			},
			KeepUnsealed: true,
		}

		soff, err := rpc.AllocatePieceToSector(ctx, maddr, pdi, dstat.Size(), *dataUrl, nil)
		if err != nil {
			return xerrors.Errorf("allocate piece to sector: %w", err)
		}

		fmt.Printf("* sector offset: %d\n", soff)

		// wait for delete call on the file
		color.Green("> waiting for file to be deleted (on sector finalize)\n")

		<-deleteCalled

		fmt.Println("* done")

		return nil
	},
}

var lpBoostProxyCmd = &cli.Command{
	Name:  "boost-proxy",
	Usage: "Start a legacy lotus-miner rpc proxy",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "actor-address",
			Usage:    "Address of the miner actor",
			Required: true,
		},

		&cli.StringFlag{
			Name:    "db-host",
			EnvVars: []string{"LOTUS_DB_HOST"},
			Usage:   "Command separated list of hostnames for yugabyte cluster",
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-name",
			EnvVars: []string{"LOTUS_DB_NAME", "LOTUS_HARMONYDB_HOSTS"},
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-user",
			EnvVars: []string{"LOTUS_DB_USER", "LOTUS_HARMONYDB_USERNAME"},
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-password",
			EnvVars: []string{"LOTUS_DB_PASSWORD", "LOTUS_HARMONYDB_PASSWORD"},
			Value:   "yugabyte",
		},
		&cli.StringFlag{
			Name:    "db-port",
			EnvVars: []string{"LOTUS_DB_PORT", "LOTUS_HARMONYDB_PORT"},
			Hidden:  true,
			Value:   "5433",
		},
		&cli.StringFlag{
			Name:    "layers",
			EnvVars: []string{"LOTUS_LAYERS", "LOTUS_CONFIG_LAYERS"},
			Value:   "base",
		},

		&cli.StringFlag{
			Name:  "listen",
			Usage: "Address to listen on",
			Value: ":32100",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		maddr, err := address.NewFromString(cctx.String("actor-address"))
		if err != nil {
			return xerrors.Errorf("parsing miner address: %w", err)
		}

		full, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}

		defer closer()

		pin := lpmarket.NewPieceIngester(db, full)

		si := paths.NewDBIndex(nil, db)

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return xerrors.Errorf("getting miner id: %w", err)
		}

		mi, err := full.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		lp := fakelm.NewLMRPCProvider(si, full, maddr, abi.ActorID(mid), mi.SectorSize, pin, db, cctx.String("layers"))

		laddr, err := net.ResolveTCPAddr("tcp", cctx.String("listen"))
		if err != nil {
			return xerrors.Errorf("net resolve: %w", err)
		}

		if len(laddr.IP) == 0 {
			// set localhost
			laddr.IP = net.IPv4(127, 0, 0, 1)
		}
		rootUrl := url.URL{
			Scheme: "http",
			Host:   laddr.String(),
		}

		ast := api.StorageMinerStruct{}

		ast.CommonStruct.Internal.Version = func(ctx context.Context) (api.APIVersion, error) {
			return api.APIVersion{
				Version:    "lp-proxy-v0",
				APIVersion: api.MinerAPIVersion0,
				BlockDelay: build.BlockDelaySecs,
			}, nil
		}

		ast.CommonStruct.Internal.AuthNew = lp.AuthNew

		ast.Internal.ActorAddress = lp.ActorAddress
		ast.Internal.WorkerJobs = lp.WorkerJobs
		ast.Internal.SectorsStatus = lp.SectorsStatus
		ast.Internal.SectorsList = lp.SectorsList
		ast.Internal.SectorsSummary = lp.SectorsSummary
		ast.Internal.SectorsListInStates = lp.SectorsListInStates
		ast.Internal.StorageRedeclareLocal = lp.StorageRedeclareLocal
		ast.Internal.ComputeDataCid = lp.ComputeDataCid

		type pieceInfo struct {
			data storiface.Data
			size abi.UnpaddedPieceSize

			done chan struct{}
		}

		pieceInfoLk := new(sync.Mutex)
		pieceInfos := map[cid.Cid][]pieceInfo{}

		ast.Internal.SectorAddPieceToAny = func(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data, deal api.PieceDealInfo) (api.SectorOffset, error) {
			origPieceData := pieceData
			defer func() {
				closer, ok := origPieceData.(io.Closer)
				if !ok {
					log.Warnf("DataCid: cannot close pieceData reader %T because it is not an io.Closer", origPieceData)
					return
				}
				if err := closer.Close(); err != nil {
					log.Warnw("closing pieceData in DataCid", "error", err)
				}
			}()

			pi := pieceInfo{
				data: pieceData,
				size: pieceSize,

				done: make(chan struct{}),
			}

			pieceInfoLk.Lock()
			pieceInfos[deal.DealProposal.PieceCID] = append(pieceInfos[deal.DealProposal.PieceCID], pi)
			pieceInfoLk.Unlock()

			// /piece?piece_cid=xxxx
			dataUrl := rootUrl
			dataUrl.Path = "/piece"
			dataUrl.RawQuery = "piece_cid=" + deal.DealProposal.PieceCID.String()

			// make a sector
			so, err := pin.AllocatePieceToSector(ctx, maddr, deal, int64(pieceSize), dataUrl, nil)
			if err != nil {
				return api.SectorOffset{}, err
			}

			color.Blue("%s piece assigned to sector f0%d:%d @ %d", deal.DealProposal.PieceCID, mid, so.Sector, so.Offset)

			<-pi.done

			return so, nil
		}

		ast.Internal.StorageList = si.StorageList
		ast.Internal.StorageDetach = si.StorageDetach
		ast.Internal.StorageReportHealth = si.StorageReportHealth
		ast.Internal.StorageDeclareSector = si.StorageDeclareSector
		ast.Internal.StorageDropSector = si.StorageDropSector
		ast.Internal.StorageFindSector = si.StorageFindSector
		ast.Internal.StorageInfo = si.StorageInfo
		ast.Internal.StorageBestAlloc = si.StorageBestAlloc
		ast.Internal.StorageLock = si.StorageLock
		ast.Internal.StorageTryLock = si.StorageTryLock
		ast.Internal.StorageGetLocks = si.StorageGetLocks

		var pieceHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			// /piece?piece_cid=xxxx
			pieceCid, err := cid.Decode(r.URL.Query().Get("piece_cid"))
			if err != nil {
				http.Error(w, "bad piece_cid", http.StatusBadRequest)
				return
			}

			if r.Method != http.MethodGet {
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
				return
			}

			fmt.Printf("%s request for piece from %s\n", pieceCid, r.RemoteAddr)

			pieceInfoLk.Lock()
			pis, ok := pieceInfos[pieceCid]
			if !ok {
				http.Error(w, "piece not found", http.StatusNotFound)
				color.Red("%s not found", pieceCid)
				pieceInfoLk.Unlock()
				return
			}

			// pop
			pi := pis[0]
			pis = pis[1:]

			pieceInfos[pieceCid] = pis

			pieceInfoLk.Unlock()

			start := time.Now()

			pieceData := io.LimitReader(io.MultiReader(
				pi.data,
				nullreader.Reader{},
			), int64(pi.size))

			n, err := io.Copy(w, pieceData)
			close(pi.done)

			took := time.Since(start)
			mbps := float64(n) / (1024 * 1024) / took.Seconds()

			if err != nil {
				log.Errorf("copying piece data: %s", err)
				return
			}

			color.Green("%s served %.3f MiB in %s (%.2f MiB/s)", pieceCid, float64(n)/(1024*1024), took, mbps)
		}

		mh, err := node.MinerHandler(&ast, false) // todo permissioned
		if err != nil {
			return err
		}

		mux := http.NewServeMux()
		mux.Handle("/piece", pieceHandler)
		mux.Handle("/", mh)

		{
			tok, err := lp.AuthNew(ctx, api.AllPermissions)
			if err != nil {
				return err
			}

			// parse listen into multiaddr
			ma, err := manet.FromNetAddr(laddr)
			if err != nil {
				return xerrors.Errorf("net from addr (%v): %w", laddr, err)
			}

			fmt.Printf("Token: %s:%s\n", tok, ma)
		}

		return http.ListenAndServe(cctx.String("listen"), mux)
	},
}
