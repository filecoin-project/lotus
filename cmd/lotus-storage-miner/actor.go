package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/build"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/filecoin-project/lotus/storage"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "manipulate the miner actor",
	Subcommands: []*cli.Command{
		actorSetAddrsCmd,
		actorWithdrawCmd,
		actorRepayDebtCmd,
		actorSetPeeridCmd,
		actorSetOwnerCmd,
		actorControl,
	},
}

var actorSetAddrsCmd = &cli.Command{
	Name:  "set-addrs",
	Usage: "set addresses that your miner can be publicly dialed on",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "set gas limit",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		var addrs []abi.Multiaddrs
		for _, a := range cctx.Args().Slice() {
			maddr, err := ma.NewMultiaddr(a)
			if err != nil {
				return fmt.Errorf("failed to parse %q as a multiaddr: %w", a, err)
			}

			maddrNop2p, strip := ma.SplitFunc(maddr, func(c ma.Component) bool {
				return c.Protocol().Code == ma.P_P2P
			})

			if strip != nil {
				fmt.Println("Stripping peerid ", strip, " from ", maddr)
			}
			addrs = append(addrs, maddrNop2p.Bytes())
		}

		maddr, err := nodeAPI.ActorAddress(ctx)
		if err != nil {
			return err
		}

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&miner0.ChangeMultiaddrsParams{NewMultiaddrs: addrs})
		if err != nil {
			return err
		}

		gasLimit := cctx.Int64("gas-limit")

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:       maddr,
			From:     minfo.Worker,
			Value:    types.NewInt(0),
			GasLimit: gasLimit,
			Method:   builtin.MethodsMiner.ChangeMultiaddrs,
			Params:   params,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Requested multiaddrs change in message %s\n", smsg.Cid())
		return nil

	},
}

var actorSetPeeridCmd = &cli.Command{
	Name:  "set-peer-id",
	Usage: "set the peer id of your miner",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "set gas limit",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		pid, err := peer.Decode(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse input as a peerId: %w", err)
		}

		maddr, err := nodeAPI.ActorAddress(ctx)
		if err != nil {
			return err
		}

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&miner0.ChangePeerIDParams{NewID: abi.PeerID(pid)})
		if err != nil {
			return err
		}

		gasLimit := cctx.Int64("gas-limit")

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:       maddr,
			From:     minfo.Worker,
			Value:    types.NewInt(0),
			GasLimit: gasLimit,
			Method:   builtin.MethodsMiner.ChangePeerID,
			Params:   params,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Requested peerid change in message %s\n", smsg.Cid())
		return nil

	},
}

var actorWithdrawCmd = &cli.Command{
	Name:      "withdraw",
	Usage:     "withdraw available balance",
	ArgsUsage: "[amount (FIL)]",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		available, err := api.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		amount := available
		if cctx.Args().Present() {
			f, err := types.ParseFIL(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing 'amount' argument: %w", err)
			}

			amount = abi.TokenAmount(f)

			if amount.GreaterThan(available) {
				return xerrors.Errorf("can't withdraw more funds than available; requested: %s; available: %s", amount, available)
			}
		}

		params, err := actors.SerializeParams(&miner0.WithdrawBalanceParams{
			AmountRequested: amount, // Default to attempting to withdraw all the extra funds in the miner actor
		})
		if err != nil {
			return err
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:     maddr,
			From:   mi.Owner,
			Value:  types.NewInt(0),
			Method: builtin.MethodsMiner.WithdrawBalance,
			Params: params,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", smsg.Cid())

		return nil
	},
}

var actorRepayDebtCmd = &cli.Command{
	Name:      "repay-debt",
	Usage:     "pay down a miner's debt",
	ArgsUsage: "[amount (FIL)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send funds from",
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		var amount abi.TokenAmount
		if cctx.Args().Present() {
			f, err := types.ParseFIL(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("parsing 'amount' argument: %w", err)
			}

			amount = abi.TokenAmount(f)
		} else {
			mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			store := adt.WrapStore(ctx, cbor.NewCborStore(apibstore.NewAPIBlockstore(api)))

			mst, err := miner.Load(store, mact)
			if err != nil {
				return err
			}

			amount, err = mst.FeeDebt()
			if err != nil {
				return err
			}

		}

		fromAddr := mi.Worker
		if from := cctx.String("from"); from != "" {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		fromId, err := api.StateLookupID(ctx, fromAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if !mi.IsController(fromId) {
			return xerrors.Errorf("sender isn't a controller of miner: %s", fromId)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:     maddr,
			From:   fromId,
			Value:  amount,
			Method: builtin2.MethodsMiner.RepayDebt,
			Params: nil,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Sent repay debt message %s\n", smsg.Cid())

		return nil
	},
}

var actorControl = &cli.Command{
	Name:  "control",
	Usage: "Manage control addresses",
	Subcommands: []*cli.Command{
		actorControlList,
		actorControlSet,
	},
}

var actorControlList = &cli.Command{
	Name:  "list",
	Usage: "Get currently set control addresses",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "verbose",
		},
		&cli.BoolFlag{
			Name:  "color",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		tw := tablewriter.New(
			tablewriter.Col("name"),
			tablewriter.Col("ID"),
			tablewriter.Col("key"),
			tablewriter.Col("use"),
			tablewriter.Col("balance"),
		)

		postAddr, err := storage.AddressFor(ctx, api, mi, storage.PoStAddr, types.FromFil(1))
		if err != nil {
			return xerrors.Errorf("getting address for post: %w", err)
		}

		printKey := func(name string, a address.Address) {
			b, err := api.WalletBalance(ctx, a)
			if err != nil {
				fmt.Printf("%s\t%s: error getting balance: %s\n", name, a, err)
				return
			}

			k, err := api.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				fmt.Printf("%s\t%s: error getting account key: %s\n", name, a, err)
				return
			}

			kstr := k.String()
			if !cctx.Bool("verbose") {
				kstr = kstr[:9] + "..."
			}

			bstr := types.FIL(b).String()
			switch {
			case b.LessThan(types.FromFil(10)):
				bstr = color.RedString(bstr)
			case b.LessThan(types.FromFil(50)):
				bstr = color.YellowString(bstr)
			default:
				bstr = color.GreenString(bstr)
			}

			var uses []string
			if a == mi.Worker {
				uses = append(uses, color.YellowString("other"))
			}
			if a == postAddr {
				uses = append(uses, color.GreenString("post"))
			}

			tw.Write(map[string]interface{}{
				"name":    name,
				"ID":      a,
				"key":     kstr,
				"use":     strings.Join(uses, " "),
				"balance": bstr,
			})
		}

		printKey("owner", mi.Owner)
		printKey("worker", mi.Worker)
		for i, ca := range mi.ControlAddresses {
			printKey(fmt.Sprintf("control-%d", i), ca)
		}

		return tw.Flush(os.Stdout)
	},
}

var actorControlSet = &cli.Command{
	Name:      "set",
	Usage:     "Set control address(-es)",
	ArgsUsage: "[...address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		del := map[address.Address]struct{}{}
		existing := map[address.Address]struct{}{}
		for _, controlAddress := range mi.ControlAddresses {
			ka, err := api.StateAccountKey(ctx, controlAddress, types.EmptyTSK)
			if err != nil {
				return err
			}

			del[ka] = struct{}{}
			existing[ka] = struct{}{}
		}

		var toSet []address.Address

		for i, as := range cctx.Args().Slice() {
			a, err := address.NewFromString(as)
			if err != nil {
				return xerrors.Errorf("parsing address %d: %w", i, err)
			}

			ka, err := api.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				return err
			}

			// make sure the address exists on chain
			_, err = api.StateLookupID(ctx, ka, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("looking up %s: %w", ka, err)
			}

			delete(del, ka)
			toSet = append(toSet, ka)
		}

		for a := range del {
			fmt.Println("Remove", a)
		}
		for _, a := range toSet {
			if _, exists := existing[a]; !exists {
				fmt.Println("Add", a)
			}
		}

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		cwp := &miner0.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeWorkerAddress,

			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Message CID:", smsg.Cid())

		return nil
	},
}

var actorSetOwnerCmd = &cli.Command{
	Name:      "set-owner",
	Usage:     "Set owner address",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of new owner address")
		}

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		sp, err := actors.SerializeParams(&newAddr)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: builtin2.MethodsMiner.ChangeOwnerAddress,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Propose Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("Propose owner change failed!")
			return err
		}

		smsg, err = api.MpoolPushMessage(ctx, &types.Message{
			From:   newAddr,
			To:     maddr,
			Method: builtin2.MethodsMiner.ChangeOwnerAddress,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Approve Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err = api.StateWaitMsg(ctx, smsg.Cid(), build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("Approve owner change failed!")
			return err
		}

		return nil
	},
}
