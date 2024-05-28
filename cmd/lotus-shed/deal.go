package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"

	"github.com/fatih/color"
	"github.com/mitchellh/go-homedir"
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
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/must"
)

var curioUtilCmd = &cli.Command{
	Name:  "curio-util",
	Usage: "curio utility commands",
	Subcommands: []*cli.Command{
		curioStartDealCmd,
	},
}

var curioStartDealCmd = &cli.Command{
	Name:      "start-deal",
	Usage:     "start a deal with a specific curio instance",
	ArgsUsage: "[dataFile] [miner]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "curio-rpc",
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
		var rpc api.CurioStruct
		closer2, err := jsonrpc.NewMergeClient(ctx, cctx.String("curio-rpc"), "Filecoin", []interface{}{&rpc.Internal}, nil)
		if err != nil {
			return xerrors.Errorf("open rpc: %w", err)
		}
		defer closer2()

		v, err := rpc.Version(ctx)
		if err != nil {
			return xerrors.Errorf("rpc version: %w", err)
		}

		fmt.Printf("* curio version: %s\n", v.String())

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
