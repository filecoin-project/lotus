package cli

import (
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	types "github.com/filecoin-project/go-lotus/chain/types"
	cid "github.com/ipfs/go-cid"
)

var chainCmd = &cli.Command{
	Name:  "chain",
	Usage: "Interact with filecoin blockchain",
	Subcommands: []*cli.Command{
		chainHeadCmd,
		chainGetBlock,
		chainReadObjCmd,
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
		cblock.ParentMessages = pmsgs

		out, err := json.MarshalIndent(cblock, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(out))
		return nil

	},
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
