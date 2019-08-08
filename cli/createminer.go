package cli

import (
	"fmt"
	"strconv"

	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	actors "github.com/filecoin-project/go-lotus/chain/actors"
	address "github.com/filecoin-project/go-lotus/chain/address"
	types "github.com/filecoin-project/go-lotus/chain/types"

	"github.com/libp2p/go-libp2p-core/peer"
)

var createMinerCmd = &cli.Command{
	Name:  "createminer",
	Usage: "Create a new storage market actor",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 4 {
			return fmt.Errorf("must pass four arguments: worker address, owner address, sector size, peer ID")
		}

		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

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

		params, err := actors.SerializeParams(createMinerArgs)
		if err != nil {
			return err
		}

		nonce, err := api.MpoolGetNonce(ctx, addr)
		if err != nil {
			return xerrors.Errorf("failed to get account nonce: %w", err)
		}

		msg := types.Message{
			To:       actors.StorageMarketAddress,
			From:     addr,
			Method:   actors.SMAMethods.CreateStorageMiner,
			Params:   params,
			Value:    types.NewInt(0),
			Nonce:    nonce,
			GasPrice: types.NewInt(1),
			GasLimit: types.NewInt(1),
		}

		msgbytes, err := msg.Serialize()
		if err != nil {
			return err
		}

		sig, err := api.WalletSign(ctx, addr, msgbytes)
		if err != nil {
			return xerrors.Errorf("failed to sign message: %w", err)
		}

		smsg := &types.SignedMessage{
			Message:   msg,
			Signature: *sig,
		}

		if err := api.MpoolPush(ctx, smsg); err != nil {
			return xerrors.Errorf("failed to push signed message: %w", err)
		}

		mwait, err := api.ChainWaitMsg(ctx, smsg.Cid())
		if err != nil {
			return xerrors.Errorf("failed waiting for message inclusion: %w", err)
		}

		maddr, err := address.NewFromBytes(mwait.Receipt.Return)
		if err != nil {
			return err
		}

		fmt.Printf("miner created in block %s\n", mwait.InBlock)
		fmt.Printf("new miner address: %s\n", maddr)

		return nil
	},
}
