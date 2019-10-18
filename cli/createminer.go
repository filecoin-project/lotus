package cli

import (
	"fmt"
	"strconv"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	actors "github.com/filecoin-project/lotus/chain/actors"
	address "github.com/filecoin-project/lotus/chain/address"
	types "github.com/filecoin-project/lotus/chain/types"

	"github.com/libp2p/go-libp2p-core/peer"
)

var createMinerCmd = &cli.Command{
	Name:  "createminer",
	Usage: "Create a new storage market actor",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 4 {
			return fmt.Errorf("must pass four arguments: worker address, owner address, sector size, peer ID")
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		args := cctx.Args().Slice()

		worker, err := address.NewFromString(args[0])
		if err != nil {
			return err
		}

		owner, err := address.NewFromString(args[1])
		if err != nil {
			return err
		}

		ssize, err := strconv.ParseUint(args[2], 10, 64)
		if err != nil {
			return err
		}

		pid, err := peer.IDB58Decode(args[3])
		if err != nil {
			return err
		}

		createMinerArgs := actors.CreateStorageMinerParams{
			Worker:     worker,
			Owner:      owner,
			SectorSize: types.NewInt(ssize),
			PeerID:     pid,
		}

		ctx := ReqContext(cctx)
		addr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get default address: %w", err)
		}

		params, err := actors.SerializeParams(&createMinerArgs)
		if err != nil {
			return err
		}

		msg := &types.Message{
			To:       actors.StorageMarketAddress,
			From:     addr,
			Method:   actors.SPAMethods.CreateStorageMiner,
			Params:   params,
			Value:    types.NewInt(0),
			GasPrice: types.NewInt(0),
			GasLimit: types.NewInt(10000),
		}

		smsg, err := api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return xerrors.Errorf("failed to push signed message: %w", err)
		}

		mwait, err := api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return xerrors.Errorf("failed waiting for message inclusion: %w", err)
		}

		maddr, err := address.NewFromBytes(mwait.Receipt.Return)
		if err != nil {
			return err
		}

		fmt.Printf("new miner address: %s\n", maddr)

		return nil
	},
}
