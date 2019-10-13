package cli

import (
	"context"
	"encoding/json"
	"fmt"

	cid "github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
	types "github.com/filecoin-project/go-lotus/chain/types"
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
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		gen := cctx.Bool("genesis")

		if !cctx.Args().Present() && !gen {
			return fmt.Errorf("must pass cids for tipset to set as head")
		}

		var ts *types.TipSet
		if gen {
			gents, err := api.ChainGetGenesis(ctx)
			if err != nil {
				return err
			}
			ts = gents
		} else {
			parsedts, err := parseTipSet(api, ctx, cctx.Args().Slice())
			if err != nil {
				return err
			}
			ts = parsedts
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
		&cli.UintFlag{Name: "count", Value: 30},
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

		count := cctx.Uint("count")
		if count < 1 {
			return nil
		}

		tss := make([]*types.TipSet, count)
		tss[0] = head

		for i := 1; i < len(tss); i++ {
			if head.Height() == 0 {
				break
			}

			head, err = api.ChainGetTipSet(ctx, head.Parents())
			if err != nil {
				return err
			}

			tss[i] = head
		}

		for i := len(tss) - 1; i >= 0; i-- {
			fmt.Printf("%d [ ", tss[i].Height())
			for _, b := range tss[i].Blocks() {
				fmt.Printf("%s: %s,", b.Cid(), b.Miner)
			}
			fmt.Println("]")
		}
		return nil
	},
}
