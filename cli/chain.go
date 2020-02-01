package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	cid "github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	types "github.com/filecoin-project/lotus/chain/types"
)

var chainCmd = &cli.Command{
	Name:  "chain",
	Usage: "Interact with filecoin blockchain",
	Subcommands: []*cli.Command{
		chainHeadCmd,
		chainGetBlock,
		chainReadObjCmd,
		chainGetMsgCmd,
		chainSetHeadCmd,
		chainListCmd,
		chainGetCmd,
		chainExportCmd,
		slashConsensusFault,
	},
}

var chainHeadCmd = &cli.Command{
	Name:  "head",
	Usage: "Print chain head",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		for _, c := range head.Cids() {
			fmt.Println(c)
		}
		return nil
	},
}

var chainGetBlock = &cli.Command{
	Name:  "getblock",
	Usage: "Get a block and print its details",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "raw",
			Usage: "print just the raw block header",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass cid of block to print")
		}

		bcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		blk, err := api.ChainGetBlock(ctx, bcid)
		if err != nil {
			return xerrors.Errorf("get block failed: %w", err)
		}

		if cctx.Bool("raw") {
			out, err := json.MarshalIndent(blk, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(out))
			return nil
		}

		msgs, err := api.ChainGetBlockMessages(ctx, bcid)
		if err != nil {
			return xerrors.Errorf("failed to get messages: %w", err)
		}

		pmsgs, err := api.ChainGetParentMessages(ctx, bcid)
		if err != nil {
			return xerrors.Errorf("failed to get parent messages: %w", err)
		}

		recpts, err := api.ChainGetParentReceipts(ctx, bcid)
		if err != nil {
			log.Warn(err)
			//return xerrors.Errorf("failed to get receipts: %w", err)
		}

		cblock := struct {
			types.BlockHeader
			BlsMessages    []*types.Message
			SecpkMessages  []*types.SignedMessage
			ParentReceipts []*types.MessageReceipt
			ParentMessages []cid.Cid
		}{}

		cblock.BlockHeader = *blk
		cblock.BlsMessages = msgs.BlsMessages
		cblock.SecpkMessages = msgs.SecpkMessages
		cblock.ParentReceipts = recpts
		cblock.ParentMessages = apiMsgCids(pmsgs)

		out, err := json.MarshalIndent(cblock, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(out))
		return nil

	},
}

func apiMsgCids(in []api.Message) []cid.Cid {
	out := make([]cid.Cid, len(in))
	for k, v := range in {
		out[k] = v.Cid
	}
	return out
}

var chainReadObjCmd = &cli.Command{
	Name:  "read-obj",
	Usage: "Read the raw bytes of an object",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to parse cid input: %s", err)
		}

		obj, err := api.ChainReadObj(ctx, c)
		if err != nil {
			return err
		}

		fmt.Printf("%x\n", obj)
		return nil
	},
}

var chainGetMsgCmd = &cli.Command{
	Name:  "getmessage",
	Usage: "Get and print a message by its cid",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass a cid of a message to get")
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		c, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("failed to parse cid input: %w", err)
		}

		mb, err := api.ChainReadObj(ctx, c)
		if err != nil {
			return xerrors.Errorf("failed to read object: %w", err)
		}

		var i interface{}
		m, err := types.DecodeMessage(mb)
		if err != nil {
			sm, err := types.DecodeSignedMessage(mb)
			if err != nil {
				return xerrors.Errorf("failed to decode object as a message: %w", err)
			}
			i = sm
		} else {
			i = m
		}

		enc, err := json.MarshalIndent(i, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(enc))
		return nil
	},
}

var chainSetHeadCmd = &cli.Command{
	Name:  "sethead",
	Usage: "manually set the local nodes head tipset (Caution: normally only used for recovery)",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "genesis",
			Usage: "reset head to genesis",
		},
		&cli.Uint64Flag{
			Name:  "epoch",
			Usage: "reset head to given epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var ts *types.TipSet

		if cctx.Bool("genesis") {
			ts, err = api.ChainGetGenesis(ctx)
		}
		if ts == nil && cctx.IsSet("epoch") {
			ts, err = api.ChainGetTipSetByHeight(ctx, cctx.Uint64("epoch"), nil)
		}
		if ts == nil {
			ts, err = parseTipSet(api, ctx, cctx.Args().Slice())
		}
		if err != nil {
			return err
		}

		if ts == nil {
			return fmt.Errorf("must pass cids for tipset to set as head")
		}

		if err := api.ChainSetHead(ctx, ts); err != nil {
			return err
		}

		return nil
	},
}

func parseTipSet(api api.FullNode, ctx context.Context, vals []string) (*types.TipSet, error) {
	var headers []*types.BlockHeader
	for _, c := range vals {
		blkc, err := cid.Decode(c)
		if err != nil {
			return nil, err
		}

		bh, err := api.ChainGetBlock(ctx, blkc)
		if err != nil {
			return nil, err
		}

		headers = append(headers, bh)
	}

	return types.NewTipSet(headers)
}

var chainListCmd = &cli.Command{
	Name:  "list",
	Usage: "View a segment of the chain",
	Flags: []cli.Flag{
		&cli.Uint64Flag{Name: "height"},
		&cli.IntFlag{Name: "count", Value: 30},
		&cli.StringFlag{
			Name:  "format",
			Usage: "specify the format to print out tipsets",
			Value: "<height>: (<time>) <blocks>",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var head *types.TipSet

		if cctx.IsSet("height") {
			head, err = api.ChainGetTipSetByHeight(ctx, cctx.Uint64("height"), nil)
		} else {
			head, err = api.ChainHead(ctx)
		}
		if err != nil {
			return err
		}

		count := cctx.Int("count")
		if count < 1 {
			return nil
		}

		tss := make([]*types.TipSet, 0, count)
		tss = append(tss, head)

		for i := 1; i < count; i++ {
			if head.Height() == 0 {
				break
			}

			head, err = api.ChainGetTipSet(ctx, head.Parents())
			if err != nil {
				return err
			}

			tss = append(tss, head)
		}

		for i := len(tss) - 1; i >= 0; i-- {
			printTipSet(cctx.String("format"), tss[i])
		}
		return nil
	},
}

var chainGetCmd = &cli.Command{
	Name:  "get",
	Usage: "Get chain DAG node by path",
	Description: `Get ipld node under a specified path:

   lotus chain get /ipfs/[cid]/some/path

   Note:
   You can use special path elements to traverse through some data structures:
   - /ipfs/[cid]/@H:elem - get 'elem' from hamt
   - /ipfs/[cid]/@Ha:t01 - get element under Addr(t01).Bytes
   - /ipfs/[cid]/@A:10 - get 10th amt element
`,
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		nd, err := api.ChainGetNode(ctx, cctx.Args().First())
		if err != nil {
			return err
		}

		b, err := json.MarshalIndent(nd, "", "\t")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

func printTipSet(format string, ts *types.TipSet) {
	format = strings.ReplaceAll(format, "<height>", fmt.Sprint(ts.Height()))
	format = strings.ReplaceAll(format, "<time>", time.Unix(int64(ts.MinTimestamp()), 0).Format(time.Stamp))
	blks := "[ "
	for _, b := range ts.Blocks() {
		blks += fmt.Sprintf("%s: %s,", b.Cid(), b.Miner)
	}
	blks += " ]"

	sCids := make([]string, 0, len(blks))

	for _, c := range ts.Cids() {
		sCids = append(sCids, c.String())
	}

	format = strings.ReplaceAll(format, "<tipset>", strings.Join(sCids, ","))
	format = strings.ReplaceAll(format, "<blocks>", blks)
	format = strings.ReplaceAll(format, "<weight>", fmt.Sprint(ts.Blocks()[0].ParentWeight))

	fmt.Println(format)
}

var chainExportCmd = &cli.Command{
	Name:  "export",
	Usage: "export chain to a car file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "tipset",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify filename to export chain to")
		}

		fi, err := os.Create(cctx.Args().First())
		if err != nil {
			return err
		}
		defer fi.Close()

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		stream, err := api.ChainExport(ctx, ts)
		if err != nil {
			return err
		}

		for b := range stream {
			_, err := fi.Write(b)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var slashConsensusFault = &cli.Command{
	Name:  "slash-consensus",
	Usage: "Report consensus fault",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		c1, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parsing cid 1: %w", err)
		}

		b1, err := api.ChainGetBlock(ctx, c1)
		if err != nil {
			return xerrors.Errorf("getting block 1: %w", err)
		}

		c2, err := cid.Parse(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parsing cid 2: %w", err)
		}

		b2, err := api.ChainGetBlock(ctx, c2)
		if err != nil {
			return xerrors.Errorf("getting block 2: %w", err)
		}

		def, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&actors.ArbitrateConsensusFaultParams{
			Block1: b1,
			Block2: b2,
		})

		msg := &types.Message{
			To:       actors.StoragePowerAddress,
			From:     def,
			Value:    types.NewInt(0),
			GasPrice: types.NewInt(1),
			GasLimit: types.NewInt(10000000),
			Method:   actors.SPAMethods.ArbitrateConsensusFault,
			Params:   params,
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Println(smsg.Cid())

		return nil
	},
}
