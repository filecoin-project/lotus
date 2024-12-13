package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "manipulate the miner actor",
	Subcommands: []*cli.Command{
		actorWithdrawCmd,
		actorSetOwnerCmd,
		actorControl,
		actorProposeChangeWorker,
		actorConfirmChangeWorker,
		actorGetMethodNum,
		actorProposeChangeBeneficiary,
		actorConfirmChangeBeneficiary,
		actorSetAddrsCmd,
		actorSetPeeridCmd,
	},
}

var actorSetAddrsCmd = &cli.Command{
	Name:      "set-p2p-addrs",
	Usage:     "set addresses that your miner can be publicly dialed on",
	ArgsUsage: "<multiaddrs>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "actor",
			Usage:    "specify the address of miner actor",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send the message from",
		},
		&cli.BoolFlag{
			Name:  "unset",
			Usage: "unset address",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		args := cctx.Args().Slice()
		unset := cctx.Bool("unset")
		if len(args) == 0 && !unset {
			return cli.ShowSubcommandHelp(cctx)
		}
		if len(args) > 0 && unset {
			return fmt.Errorf("unset can only be used with no arguments")
		}

		var maddr address.Address
		maddr, err := address.NewFromString(cctx.String("actor"))
		if err != nil {
			return fmt.Errorf("parsing address %s: %w", cctx.String("actor"), err)
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		var addrs []abi.Multiaddrs
		for _, a := range args {
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

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		fromAddr := minfo.Worker
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

		if !isController(minfo, fromId) {
			return xerrors.Errorf("sender isn't a controller of miner: %s", fromId)
		}

		params, err := actors.SerializeParams(&miner.ChangeMultiaddrsParams{NewMultiaddrs: addrs})
		if err != nil {
			return err
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:     maddr,
			From:   fromId,
			Value:  types.NewInt(0),
			Method: builtin.MethodsMiner.ChangeMultiaddrs,
			Params: params,
		}, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Requested multiaddrs change in message %s\n", smsg.Cid())
		return nil

	},
}

var actorSetPeeridCmd = &cli.Command{
	Name:      "set-peer-id",
	Usage:     "set the peer id of your miner",
	ArgsUsage: "<peer id>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "actor",
			Usage:    "specify the address of miner actor",
			Required: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.String("actor"))
		if err != nil {
			return fmt.Errorf("parsing address %s: %w", cctx.String("actor"), err)
		}

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		pid, err := peer.Decode(cctx.Args().Get(0))
		if err != nil {
			return fmt.Errorf("failed to parse input as a peerId: %w", err)
		}

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&miner.ChangePeerIDParams{NewID: abi.PeerID(pid)})
		if err != nil {
			return err
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:     maddr,
			From:   minfo.Worker,
			Value:  types.NewInt(0),
			Method: builtin.MethodsMiner.ChangePeerID,
			Params: params,
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
	Usage:     "withdraw available balance to beneficiary",
	ArgsUsage: "[amount (FIL)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
		&cli.IntFlag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: int(buildconstants.MessageConfidence),
		},
		&cli.BoolFlag{
			Name:  "beneficiary",
			Usage: "send withdraw message from the beneficiary address",
		},
	},
	Action: func(cctx *cli.Context) error {
		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		if maddr.Empty() {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		available, err := nodeAPI.StateMinerAvailableBalance(ctx, maddr, types.EmptyTSK)
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
				return xerrors.Errorf("can't withdraw more funds than available; requested: %s; available: %s", types.FIL(amount), types.FIL(available))
			}
		}

		params, err := actors.SerializeParams(&miner2.WithdrawBalanceParams{
			AmountRequested: amount, // Default to attempting to withdraw all the extra funds in the miner actor
		})
		if err != nil {
			return err
		}

		var sender address.Address
		if cctx.IsSet("beneficiary") {
			sender = mi.Beneficiary
		} else {
			sender = mi.Owner
		}

		smsg, err := nodeAPI.MpoolPushMessage(ctx, &types.Message{
			To:     maddr,
			From:   sender,
			Value:  types.NewInt(0),
			Method: builtin.MethodsMiner.WithdrawBalance,
			Params: params,
		}, &api.MessageSendSpec{MaxFee: abi.TokenAmount(types.MustParseFIL("0.1"))})
		if err != nil {
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := nodeAPI.StateWaitMsg(ctx, smsg.Cid(), uint64(cctx.Int("confidence")))
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			fmt.Println(cctx.App.Writer, "withdrawal failed!")
			return err
		}

		nv, err := nodeAPI.StateNetworkVersion(ctx, wait.TipSet)
		if err != nil {
			return err
		}

		if nv >= network.Version14 {
			var withdrawn abi.TokenAmount
			if err := withdrawn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
				return err
			}

			fmt.Printf("Successfully withdrew %s \n", types.FIL(withdrawn))
			if withdrawn.LessThan(amount) {
				fmt.Printf("Note that this is less than the requested amount of %s \n", types.FIL(amount))
			}
		}

		return nil
	},
}

var actorSetOwnerCmd = &cli.Command{
	Name:      "set-owner",
	Usage:     "Set owner address (this command should be invoked twice, first with the old owner as the senderAddress, and then with the new owner)",
	ArgsUsage: "[newOwnerAddress senderAddress]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
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

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddrId, err := nodeAPI.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		fa, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		fromAddrId, err := nodeAPI.StateLookupID(ctx, fa, types.EmptyTSK)
		if err != nil {
			return err
		}

		if maddr.Empty() {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if fromAddrId != mi.Owner && fromAddrId != newAddrId {
			return xerrors.New("from address must either be the old owner or the new owner")
		}

		sp, err := actors.SerializeParams(&newAddrId)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := nodeAPI.MpoolPushMessage(ctx, &types.Message{
			From:   fromAddrId,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeOwnerAddress,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := nodeAPI.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			fmt.Println("owner change failed!")
			return err
		}

		fmt.Println("message succeeded!")

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
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
		&cli.BoolFlag{
			Name: "verbose",
		},
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		if maddr.Empty() {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		tw := tablewriter.New(
			tablewriter.Col("name"),
			tablewriter.Col("ID"),
			tablewriter.Col("key"),
			tablewriter.Col("balance"),
		)

		printKey := func(name string, a address.Address) {
			b, err := nodeAPI.WalletBalance(ctx, a)
			if err != nil {
				fmt.Printf("%s\t%s: error getting balance: %s\n", name, a, err)
				return
			}

			k, err := nodeAPI.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				fmt.Printf("%s\t%s: error getting account key: %s\n", name, a, err)
				return
			}

			kstr := k.String()
			if !cctx.Bool("verbose") {
				if len(kstr) > 9 {
					kstr = kstr[:6] + "..."
				}
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

			tw.Write(map[string]interface{}{
				"name":    name,
				"ID":      a,
				"key":     kstr,
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
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
		&cli.BoolFlag{
			Name:  "dump-bytes",
			Usage: "Dumps the bytes of the message that would propose this change",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		if maddr.Empty() {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		del := map[address.Address]struct{}{}
		existing := map[address.Address]struct{}{}
		for _, controlAddress := range mi.ControlAddresses {
			ka, err := nodeAPI.StateAccountKey(ctx, controlAddress, types.EmptyTSK)
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

			ka, err := nodeAPI.StateAccountKey(ctx, a, types.EmptyTSK)
			if err != nil {
				return err
			}

			// make sure the address exists on chain
			_, err = nodeAPI.StateLookupID(ctx, ka, types.EmptyTSK)
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

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		msg := &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeWorkerAddress,
			Value:  big.Zero(),
			Params: sp,
		}

		if cctx.Bool("dump-bytes") {

			msg, err = nodeAPI.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
			if err != nil {
				return err
			}

			msgBytes, err := msg.Serialize()
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cctx.App.Writer, hex.EncodeToString(msgBytes))
			return nil
		}

		if !cctx.Bool("really-do-it") {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Pass --really-do-it to actually execute this action")
			return nil
		}

		smsg, err := nodeAPI.MpoolPushMessage(ctx, msg, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Message CID:", smsg.Cid())

		return nil
	},
}

var actorProposeChangeWorker = &cli.Command{
	Name:      "propose-change-worker",
	Usage:     "Propose a worker address change",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of new worker address")
		}

		if !cctx.Bool("really-do-it") {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Pass --really-do-it to actually execute this action")
			return nil
		}

		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := nodeAPI.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		if maddr.Empty() {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.NewWorker.Empty() {
			if mi.Worker == newAddr {
				return fmt.Errorf("worker address already set to %s", na)
			}
		} else {
			if mi.NewWorker == newAddr {
				return fmt.Errorf("change to worker address %s already pending", na)
			}
		}

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       newAddr,
			NewControlAddrs: mi.ControlAddresses,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := nodeAPI.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeWorkerAddress,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := nodeAPI.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose worker change failed!")
			return err
		}

		mi, err = nodeAPI.StateMinerInfo(ctx, maddr, wait.TipSet)
		if err != nil {
			return err
		}
		if mi.NewWorker != newAddr {
			return fmt.Errorf("proposed worker address change not reflected on chain: expected '%s', found '%s'", na, mi.NewWorker)
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "Worker key change to %s successfully proposed.\n", na)
		_, _ = fmt.Fprintf(cctx.App.Writer, "Call 'confirm-change-worker' at or after height %d to complete.\n", mi.WorkerChangeEpoch)

		return nil
	},
}

var actorConfirmChangeWorker = &cli.Command{
	Name:      "confirm-change-worker",
	Usage:     "Confirm a worker address change",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "actor",
			Usage: "specify the address of miner actor",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of new worker address")
		}

		if !cctx.Bool("really-do-it") {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Pass --really-do-it to actually execute this action")
			return nil
		}

		var maddr address.Address
		if act := cctx.String("actor"); act != "" {
			var err error
			maddr, err = address.NewFromString(act)
			if err != nil {
				return fmt.Errorf("parsing address %s: %w", act, err)
			}
		}

		nodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := nodeAPI.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		if maddr.Empty() {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			maddr, err = minerApi.ActorAddress(ctx)
			if err != nil {
				return err
			}
		}

		mi, err := nodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.NewWorker.Empty() {
			return xerrors.Errorf("no worker key change proposed")
		} else if mi.NewWorker != newAddr {
			return xerrors.Errorf("worker key %s does not match current worker key proposal %s", newAddr, mi.NewWorker)
		}

		if head, err := nodeAPI.ChainHead(ctx); err != nil {
			return xerrors.Errorf("failed to get the chain head: %w", err)
		} else if head.Height() < mi.WorkerChangeEpoch {
			return xerrors.Errorf("worker key change cannot be confirmed until %d, current height is %d", mi.WorkerChangeEpoch, head.Height())
		}

		smsg, err := nodeAPI.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: builtin.MethodsMiner.ConfirmChangeWorkerAddress,
			Value:  big.Zero(),
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Confirm Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := nodeAPI.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Worker change failed!")
			return err
		}

		mi, err = nodeAPI.StateMinerInfo(ctx, maddr, wait.TipSet)
		if err != nil {
			return err
		}
		if mi.Worker != newAddr {
			return fmt.Errorf("confirmed worker address change not reflected on chain: expected '%s', found '%s'", newAddr, mi.Worker)
		}

		return nil
	},
}

var actorGetMethodNum = &cli.Command{
	Name:      "generate-method-num",
	Usage:     "Generate method number from method name",
	ArgsUsage: "[methodName]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass methodNum")
		}

		methodName := cctx.Args().First()
		methodNum, err := builtin.GenerateFRCMethodNum(methodName)
		if err != nil {
			return err
		}

		fmt.Println("Method Num: ", methodNum)

		return nil
	},
}

var actorProposeChangeBeneficiary = &cli.Command{
	Name:      "propose-change-beneficiary",
	Usage:     "Propose a beneficiary address change",
	ArgsUsage: "[beneficiaryAddress quota expiration minerID]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "overwrite-pending-change",
			Usage: "Overwrite the current beneficiary change proposal",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 4 {
			return lcli.IncorrectNumArgs(cctx)
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting fullnode api: %w", err)
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		na, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("parsing beneficiary address: %w", err)
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("looking up new beneficiary address: %w", err)
		}

		quota, err := types.ParseFIL(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing quota: %w", err)
		}

		expiration, err := strconv.ParseInt(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing expiration: %w", err)
		}

		maddr, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return xerrors.Errorf("getting miner address: %w", err)
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		if mi.Beneficiary == mi.Owner && newAddr == mi.Owner {
			return fmt.Errorf("beneficiary %s already set to owner address", mi.Beneficiary)
		}

		if mi.PendingBeneficiaryTerm != nil {
			fmt.Println("WARNING: replacing Pending Beneficiary Term of:")
			fmt.Println("Beneficiary: ", mi.PendingBeneficiaryTerm.NewBeneficiary)
			fmt.Println("Quota:", mi.PendingBeneficiaryTerm.NewQuota)
			fmt.Println("Expiration Epoch:", mi.PendingBeneficiaryTerm.NewExpiration)

			if !cctx.Bool("overwrite-pending-change") {
				return fmt.Errorf("must pass --overwrite-pending-change to replace current pending beneficiary change. Please review CAREFULLY")
			}
		}

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action. Review what you're about to approve CAREFULLY please")
			return nil
		}

		params := &miner.ChangeBeneficiaryParams{
			NewBeneficiary: newAddr,
			NewQuota:       abi.TokenAmount(quota),
			NewExpiration:  abi.ChainEpoch(expiration),
		}

		sp, err := actors.SerializeParams(params)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Owner,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeBeneficiary,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Propose Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return xerrors.Errorf("waiting for message to be included in block: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("propose beneficiary change failed")
		}

		updatedMinerInfo, err := api.StateMinerInfo(ctx, maddr, wait.TipSet)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		if updatedMinerInfo.PendingBeneficiaryTerm == nil && updatedMinerInfo.Beneficiary == newAddr {
			fmt.Println("Beneficiary address successfully changed")
		} else {
			fmt.Println("Beneficiary address change awaiting additional confirmations")
		}

		return nil
	},
}

var actorConfirmChangeBeneficiary = &cli.Command{
	Name:      "confirm-change-beneficiary",
	Usage:     "Confirm a beneficiary address change",
	ArgsUsage: "[minerID]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "existing-beneficiary",
			Usage: "send confirmation from the existing beneficiary address",
		},
		&cli.BoolFlag{
			Name:  "new-beneficiary",
			Usage: "send confirmation from the new beneficiary address",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting fullnode api: %w", err)
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("parsing beneficiary address: %w", err)
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		if mi.PendingBeneficiaryTerm == nil {
			return fmt.Errorf("no pending beneficiary term found for miner %s", maddr)
		}

		if (cctx.IsSet("existing-beneficiary") && cctx.IsSet("new-beneficiary")) || (!cctx.IsSet("existing-beneficiary") && !cctx.IsSet("new-beneficiary")) {
			return lcli.ShowHelp(cctx, fmt.Errorf("must pass exactly one of --existing-beneficiary or --new-beneficiary"))
		}

		var fromAddr address.Address
		if cctx.IsSet("existing-beneficiary") {
			if mi.PendingBeneficiaryTerm.ApprovedByBeneficiary {
				return fmt.Errorf("beneficiary change already approved by current beneficiary")
			}
			fromAddr = mi.Beneficiary
		} else {
			if mi.PendingBeneficiaryTerm.ApprovedByNominee {
				return fmt.Errorf("beneficiary change already approved by new beneficiary")
			}
			fromAddr = mi.PendingBeneficiaryTerm.NewBeneficiary
		}

		fmt.Println("Confirming Pending Beneficiary Term of:")
		fmt.Println("Beneficiary: ", mi.PendingBeneficiaryTerm.NewBeneficiary)
		fmt.Println("Quota:", mi.PendingBeneficiaryTerm.NewQuota)
		fmt.Println("Expiration Epoch:", mi.PendingBeneficiaryTerm.NewExpiration)

		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action. Review what you're about to approve CAREFULLY please")
			return nil
		}

		params := &miner.ChangeBeneficiaryParams{
			NewBeneficiary: mi.PendingBeneficiaryTerm.NewBeneficiary,
			NewQuota:       mi.PendingBeneficiaryTerm.NewQuota,
			NewExpiration:  mi.PendingBeneficiaryTerm.NewExpiration,
		}

		sp, err := actors.SerializeParams(params)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   fromAddr,
			To:     maddr,
			Method: builtin.MethodsMiner.ChangeBeneficiary,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Println("Confirm Message CID:", smsg.Cid())

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), buildconstants.MessageConfidence)
		if err != nil {
			return xerrors.Errorf("waiting for message to be included in block: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("confirm beneficiary change failed with code %d", wait.Receipt.ExitCode)
		}

		updatedMinerInfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if updatedMinerInfo.PendingBeneficiaryTerm == nil && updatedMinerInfo.Beneficiary == mi.PendingBeneficiaryTerm.NewBeneficiary {
			fmt.Println("Beneficiary address successfully changed")
		} else {
			fmt.Println("Beneficiary address change awaiting additional confirmations")
		}

		return nil
	},
}

func isController(mi api.MinerInfo, addr address.Address) bool {
	if addr == mi.Owner || addr == mi.Worker {
		return true
	}

	for _, ca := range mi.ControlAddresses {
		if addr == ca {
			return true
		}
	}

	return false
}
