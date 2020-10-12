package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"text/tabwriter"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-address"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	init0 "github.com/filecoin-project/specs-actors/actors/builtin/init"
	msig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"
)

var multisigCmd = &cli.Command{
	Name:  "msig",
	Usage: "Interact with a multisig wallet",
	Subcommands: []*cli.Command{
		msigCreateCmd,
		msigInspectCmd,
		msigProposeCmd,
		msigApproveCmd,
		msigAddProposeCmd,
		msigAddApproveCmd,
		msigAddCancelCmd,
		msigSwapProposeCmd,
		msigSwapApproveCmd,
		msigSwapCancelCmd,
		msigLockProposeCmd,
		msigLockApproveCmd,
		msigLockCancelCmd,
		msigVestedCmd,
	},
}

var msigCreateCmd = &cli.Command{
	Name:      "create",
	Usage:     "Create a new multisig wallet",
	ArgsUsage: "[address1 address2 ...]",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "required",
			Usage: "number of required approvals (uses number of signers provided if omitted)",
		},
		&cli.StringFlag{
			Name:  "value",
			Usage: "initial funds to give to multisig",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "duration",
			Usage: "length of the period over which funds unlock",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the create message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 1 {
			return ShowHelp(cctx, fmt.Errorf("multisigs must have at least one signer"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		var addrs []address.Address
		for _, a := range cctx.Args().Slice() {
			addr, err := address.NewFromString(a)
			if err != nil {
				return err
			}
			addrs = append(addrs, addr)
		}

		// get the address we're going to use to create the multisig (can be one of the above, as long as they have funds)
		var sendAddr address.Address
		if send := cctx.String("from"); send == "" {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}

			sendAddr = defaddr
		} else {
			addr, err := address.NewFromString(send)
			if err != nil {
				return err
			}

			sendAddr = addr
		}

		val := cctx.String("value")
		filval, err := types.ParseFIL(val)
		if err != nil {
			return err
		}

		intVal := types.BigInt(filval)

		required := cctx.Uint64("required")
		if required == 0 {
			required = uint64(len(addrs))
		}

		d := abi.ChainEpoch(cctx.Uint64("duration"))

		gp := types.NewInt(1)

		msgCid, err := api.MsigCreate(ctx, required, addrs, d, intVal, sendAddr, gp)
		if err != nil {
			return err
		}

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Println("actor creation failed!")
			return err
		}

		// get address of newly created miner

		var execreturn init0.ExecReturn
		if err := execreturn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return err
		}
		fmt.Println("Created new multisig: ", execreturn.IDAddress, execreturn.RobustAddress)

		// TODO: maybe register this somewhere
		return nil
	},
}

var msigInspectCmd = &cli.Command{
	Name:      "inspect",
	Usage:     "Inspect a multisig wallet",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "vesting",
			Usage: "Include vesting details",
		},
		&cli.BoolFlag{
			Name:  "decode-params",
			Usage: "Decode parameters of transaction proposals",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return ShowHelp(cctx, fmt.Errorf("must specify address of multisig to inspect"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		store := adt.WrapStore(ctx, cbor.NewCborStore(apibstore.NewAPIBlockstore(api)))

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, maddr, head.Key())
		if err != nil {
			return err
		}

		ownId, err := api.StateLookupID(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		mstate, err := multisig.Load(store, act)
		if err != nil {
			return err
		}
		locked, err := mstate.LockedBalance(head.Height())
		if err != nil {
			return err
		}

		fmt.Printf("Balance: %s\n", types.FIL(act.Balance))
		fmt.Printf("Spendable: %s\n", types.FIL(types.BigSub(act.Balance, locked)))

		if cctx.Bool("vesting") {
			ib, err := mstate.InitialBalance()
			if err != nil {
				return err
			}
			fmt.Printf("InitialBalance: %s\n", types.FIL(ib))
			se, err := mstate.StartEpoch()
			if err != nil {
				return err
			}
			fmt.Printf("StartEpoch: %d\n", se)
			ud, err := mstate.UnlockDuration()
			if err != nil {
				return err
			}
			fmt.Printf("UnlockDuration: %d\n", ud)
		}

		signers, err := mstate.Signers()
		if err != nil {
			return err
		}
		threshold, err := mstate.Threshold()
		if err != nil {
			return err
		}
		fmt.Printf("Threshold: %d / %d\n", threshold, len(signers))
		fmt.Println("Signers:")
		for _, s := range signers {
			fmt.Printf("\t%s\n", s)
		}

		pending := make(map[int64]multisig.Transaction)
		if err := mstate.ForEachPendingTxn(func(id int64, txn multisig.Transaction) error {
			pending[id] = txn
			return nil
		}); err != nil {
			return xerrors.Errorf("reading pending transactions: %w", err)
		}

		decParams := cctx.Bool("decode-params")
		fmt.Println("Transactions: ", len(pending))
		if len(pending) > 0 {
			var txids []int64
			for txid := range pending {
				txids = append(txids, txid)
			}
			sort.Slice(txids, func(i, j int) bool {
				return txids[i] < txids[j]
			})

			w := tabwriter.NewWriter(os.Stdout, 8, 4, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tState\tApprovals\tTo\tValue\tMethod\tParams\n")
			for _, txid := range txids {
				tx := pending[txid]
				target := tx.To.String()
				if tx.To == ownId {
					target += " (self)"
				}
				targAct, err := api.StateGetActor(ctx, tx.To, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("failed to resolve 'To' address of multisig transaction %d: %w", txid, err)
				}
				method := stmgr.MethodsMap[targAct.Code][tx.Method]

				paramStr := fmt.Sprintf("%x", tx.Params)
				if decParams && tx.Method != 0 {
					ptyp := reflect.New(method.Params.Elem()).Interface().(cbg.CBORUnmarshaler)
					if err := ptyp.UnmarshalCBOR(bytes.NewReader(tx.Params)); err != nil {
						return xerrors.Errorf("failed to decode parameters of transaction %d: %w", txid, err)
					}

					b, err := json.Marshal(ptyp)
					if err != nil {
						return xerrors.Errorf("could not json marshal parameter type: %w", err)
					}

					paramStr = string(b)
				}

				fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n", txid, "pending", len(tx.Approved), target, types.FIL(tx.Value), method.Name, tx.Method, paramStr)
			}
			if err := w.Flush(); err != nil {
				return xerrors.Errorf("flushing output: %+v", err)
			}

		}

		return nil
	},
}

var msigProposeCmd = &cli.Command{
	Name:      "propose",
	Usage:     "Propose a multisig transaction",
	ArgsUsage: "[multisigAddress destinationAddress value <methodId methodParams> (optional)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 3 {
			return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address, destination, and value"))
		}

		if cctx.Args().Len() > 3 && cctx.Args().Len() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must either pass three or five arguments"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		dest, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		value, err := types.ParseFIL(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		var method uint64
		var params []byte
		if cctx.Args().Len() == 5 {
			m, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
			if err != nil {
				return err
			}
			method = m

			p, err := hex.DecodeString(cctx.Args().Get(4))
			if err != nil {
				return err
			}
			params = p
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		act, err := api.StateGetActor(ctx, msig, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("failed to look up multisig %s: %w", msig, err)
		}

		if !builtin.IsMultisigActor(act.Code) {
			return fmt.Errorf("actor %s is not a multisig actor", msig)
		}

		msgCid, err := api.MsigPropose(ctx, msig, dest, types.BigInt(value), from, method, params)
		if err != nil {
			return err
		}

		fmt.Println("send proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}

		var retval msig0.ProposeReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return fmt.Errorf("failed to unmarshal propose return value: %w", err)
		}

		fmt.Printf("Transaction ID: %d\n", retval.TxnID)
		if retval.Applied {
			fmt.Printf("Transaction was executed during propose\n")
			fmt.Printf("Exit Code: %d\n", retval.Code)
			fmt.Printf("Return Value: %x\n", retval.Ret)
		}

		return nil
	},
}

var msigApproveCmd = &cli.Command{
	Name:      "approve",
	Usage:     "Approve a multisig message",
	ArgsUsage: "<multisigAddress messageId> [proposerAddress destination value [methodId methodParams]]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 5 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, message ID, proposer address, destination, and value"))
		}

		if cctx.Args().Len() > 5 && cctx.Args().Len() != 7 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig approve <msig addr> <message ID> <proposer address> <desination> <value> [ <method> <params> ]"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		proposer, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		if proposer.Protocol() != address.ID {
			proposer, err = api.StateLookupID(ctx, proposer, types.EmptyTSK)
			if err != nil {
				return err
			}
		}

		dest, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		value, err := types.ParseFIL(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		var method uint64
		var params []byte
		if cctx.Args().Len() == 7 {
			m, err := strconv.ParseUint(cctx.Args().Get(5), 10, 64)
			if err != nil {
				return err
			}
			method = m

			p, err := hex.DecodeString(cctx.Args().Get(6))
			if err != nil {
				return err
			}
			params = p
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigApproveTxnHash(ctx, msig, txid, proposer, dest, types.BigInt(value), from, method, params)
		if err != nil {
			return err
		}

		fmt.Println("sent approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("approve returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigAddProposeCmd = &cli.Command{
	Name:      "add-propose",
	Usage:     "Propose to add a signer",
	ArgsUsage: "[multisigAddress signer]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "increase-threshold",
			Usage: "whether the number of required signers should be increased",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address and signer address"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		addr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigAddPropose(ctx, msig, from, addr, cctx.Bool("increase-threshold"))
		if err != nil {
			return err
		}

		fmt.Println("sent add proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigAddApproveCmd = &cli.Command{
	Name:      "add-approve",
	Usage:     "Approve a message to add a signer",
	ArgsUsage: "[multisigAddress proposerAddress txId newAddress increaseThreshold]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, proposer address, transaction id, new signer address, whether to increase threshold"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		prop, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		inc, err := strconv.ParseBool(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigAddApprove(ctx, msig, from, txid, prop, newAdd, inc)
		if err != nil {
			return err
		}

		fmt.Println("sent add approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add approval returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigAddCancelCmd = &cli.Command{
	Name:      "add-cancel",
	Usage:     "Cancel a message to add a signer",
	ArgsUsage: "[multisigAddress txId newAddress increaseThreshold]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 4 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, transaction id, new signer address, whether to increase threshold"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		inc, err := strconv.ParseBool(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigAddCancel(ctx, msig, from, txid, newAdd, inc)
		if err != nil {
			return err
		}

		fmt.Println("sent add cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("add cancellation returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigSwapProposeCmd = &cli.Command{
	Name:      "swap-propose",
	Usage:     "Propose to swap signers",
	ArgsUsage: "[multisigAddress oldAddress newAddress]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, old signer address, new signer address"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		oldAdd, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigSwapPropose(ctx, msig, from, oldAdd, newAdd)
		if err != nil {
			return err
		}

		fmt.Println("sent swap proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("swap proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigSwapApproveCmd = &cli.Command{
	Name:      "swap-approve",
	Usage:     "Approve a message to swap signers",
	ArgsUsage: "[multisigAddress proposerAddress txId oldAddress newAddress]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, proposer address, transaction id, old signer address, new signer address"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		prop, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		oldAdd, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigSwapApprove(ctx, msig, from, txid, prop, oldAdd, newAdd)
		if err != nil {
			return err
		}

		fmt.Println("sent swap approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("swap approval returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigSwapCancelCmd = &cli.Command{
	Name:      "swap-cancel",
	Usage:     "Cancel a message to swap signers",
	ArgsUsage: "[multisigAddress txId oldAddress newAddress]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 4 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, transaction id, old signer address, new signer address"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		oldAdd, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		newAdd, err := address.NewFromString(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		msgCid, err := api.MsigSwapCancel(ctx, msig, from, txid, oldAdd, newAdd)
		if err != nil {
			return err
		}

		fmt.Println("sent swap cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("swap cancellation returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigLockProposeCmd = &cli.Command{
	Name:      "lock-propose",
	Usage:     "Propose to lock up some balance",
	ArgsUsage: "[multisigAddress startEpoch unlockDuration amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 4 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, start epoch, unlock duration, and amount"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		start, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		duration, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		amount, err := types.ParseFIL(cctx.Args().Get(3))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		params, actErr := actors.SerializeParams(&msig0.LockBalanceParams{
			StartEpoch:     abi.ChainEpoch(start),
			UnlockDuration: abi.ChainEpoch(duration),
			Amount:         abi.NewTokenAmount(amount.Int64()),
		})

		if actErr != nil {
			return actErr
		}

		msgCid, err := api.MsigPropose(ctx, msig, msig, big.Zero(), from, uint64(builtin2.MethodsMultisig.LockBalance), params)
		if err != nil {
			return err
		}

		fmt.Println("sent lock proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("lock proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigLockApproveCmd = &cli.Command{
	Name:      "lock-approve",
	Usage:     "Approve a message to lock up some balance",
	ArgsUsage: "[multisigAddress proposerAddress txId startEpoch unlockDuration amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the approve message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 6 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, proposer address, tx id, start epoch, unlock duration, and amount"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		prop, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		start, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
		if err != nil {
			return err
		}

		duration, err := strconv.ParseUint(cctx.Args().Get(4), 10, 64)
		if err != nil {
			return err
		}

		amount, err := types.ParseFIL(cctx.Args().Get(5))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		params, actErr := actors.SerializeParams(&msig0.LockBalanceParams{
			StartEpoch:     abi.ChainEpoch(start),
			UnlockDuration: abi.ChainEpoch(duration),
			Amount:         abi.NewTokenAmount(amount.Int64()),
		})

		if actErr != nil {
			return actErr
		}

		msgCid, err := api.MsigApproveTxnHash(ctx, msig, txid, prop, msig, big.Zero(), from, uint64(builtin2.MethodsMultisig.LockBalance), params)
		if err != nil {
			return err
		}

		fmt.Println("sent lock approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("lock approval returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigLockCancelCmd = &cli.Command{
	Name:      "lock-cancel",
	Usage:     "Cancel a message to lock up some balance",
	ArgsUsage: "[multisigAddress txId startEpoch unlockDuration amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the cancel message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 6 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address, tx id, start epoch, unlock duration, and amount"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		start, err := strconv.ParseUint(cctx.Args().Get(2), 10, 64)
		if err != nil {
			return err
		}

		duration, err := strconv.ParseUint(cctx.Args().Get(3), 10, 64)
		if err != nil {
			return err
		}

		amount, err := types.ParseFIL(cctx.Args().Get(4))
		if err != nil {
			return err
		}

		var from address.Address
		if cctx.IsSet("from") {
			f, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return err
			}
			from = f
		} else {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}
			from = defaddr
		}

		params, actErr := actors.SerializeParams(&msig0.LockBalanceParams{
			StartEpoch:     abi.ChainEpoch(start),
			UnlockDuration: abi.ChainEpoch(duration),
			Amount:         abi.NewTokenAmount(amount.Int64()),
		})

		if actErr != nil {
			return actErr
		}

		msgCid, err := api.MsigCancel(ctx, msig, txid, msig, big.Zero(), from, uint64(builtin2.MethodsMultisig.LockBalance), params)
		if err != nil {
			return err
		}

		fmt.Println("sent lock cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, build.MessageConfidence)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode != 0 {
			return fmt.Errorf("lock cancellation returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigVestedCmd = &cli.Command{
	Name:      "vested",
	Usage:     "Gets the amount vested in an msig between two epochs",
	ArgsUsage: "[multisigAddress]",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "start-epoch",
			Usage: "start epoch to measure vesting from",
			Value: 0,
		},
		&cli.Int64Flag{
			Name:  "end-epoch",
			Usage: "end epoch to stop measure vesting at",
			Value: -1,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return ShowHelp(cctx, fmt.Errorf("must pass multisig address"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		start, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Int64("start-epoch")), types.EmptyTSK)
		if err != nil {
			return err
		}

		var end *types.TipSet
		if cctx.Int64("end-epoch") < 0 {
			end, err = LoadTipSet(ctx, cctx, api)
			if err != nil {
				return err
			}
		} else {
			end, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Int64("end-epoch")), types.EmptyTSK)
			if err != nil {
				return err
			}
		}

		ret, err := api.MsigGetVested(ctx, msig, start.Key(), end.Key())
		if err != nil {
			return err
		}

		fmt.Printf("Vested: %s between %d and %d\n", types.FIL(ret), start.Height(), end.Height())

		return nil
	},
}
