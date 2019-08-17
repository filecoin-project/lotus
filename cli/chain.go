package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/repo"

	bserv "github.com/ipfs/go-blockservice"
	car "github.com/ipfs/go-car"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	mdag "github.com/ipfs/go-merkledag"
)

var chainCmd = &cli.Command{
	Name:  "chain",
	Usage: "Interact with filecoin blockchain",
	Subcommands: []*cli.Command{
		chainHeadCmd,
		chainGetBlock,
		chainExportCmd,
	},
}

var chainHeadCmd = &cli.Command{
	Name:  "head",
	Usage: "Print chain head",
	Action: func(cctx *cli.Context) error {
		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
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
		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
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
			return err
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
			return err
		}

		recpts, err := api.ChainGetBlockReceipts(ctx, bcid)
		if err != nil {
			return err
		}

		cblock := struct {
			types.BlockHeader
			BlsMessages     []*types.Message
			SecpkMessages   []*types.SignedMessage
			MessageReceipts []*types.MessageReceipt
		}{}

		cblock.BlockHeader = *blk
		cblock.BlsMessages = msgs.BlsMessages
		cblock.SecpkMessages = msgs.SecpkMessages
		cblock.MessageReceipts = recpts

		out, err := json.MarshalIndent(cblock, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(out))
		return nil

	},
}

var chainExportCmd = &cli.Command{
	Name:  "export",
	Usage: "Export chain to a file",
	Action: func(cctx *cli.Context) error {
		outname := "chain.car"

		ctx := context.Background()
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		lr, err := r.Lock()
		if err != nil {
			return err
		}

		mds, err := lr.Datastore("/metadata")
		if err != nil {
			return err
		}

		kb, err := mds.Get(store.ChainHeadKey)
		if err != nil {
			return err
		}

		var ts []cid.Cid
		if err := json.Unmarshal(kb, &ts); err != nil {
			return err
		}

		bds, err := lr.Datastore("/blocks")
		if err != nil {
			return err
		}

		bs := blockstore.NewBlockstore(bds)
		bs = blockstore.NewIdStore(bs)
		dserv := mdag.NewDAGService(bserv.New(bs, nil))

		fi, err := os.Create(outname)
		if err != nil {
			return err
		}
		defer fi.Close()

		if err := car.WriteCar(ctx, dserv, ts, fi); err != nil {
			return xerrors.Errorf("failed to write car: %w", err)
		}

		return nil
	},
}
