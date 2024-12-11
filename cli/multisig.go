package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	msig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/types"
)

var MultisigCmd = &cli.Command{
	Name:  "msig",
	Usage: "Interact with a multisig wallet",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "confidence",
			Usage: "number of block confirmations to wait for",
			Value: int(buildconstants.MessageConfidence),
		},
	},
	Subcommands: []*cli.Command{
		msigCreateCmd,
		msigInspectCmd,
		msigProposeCmd,
		msigRemoveProposeCmd,
		msigApproveCmd,
		msigCancelCmd,
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
		msigProposeThresholdCmd,
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
		if cctx.NArg() < 1 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigCreate(ctx, required, addrs, d, intVal, sendAddr, gp)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent create in message: ", msgCid)
		fmt.Println("waiting for confirmation..")

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "actor creation failed!")
			return err
		}

		// get address of newly created miner

		var execreturn init2.ExecReturn
		if err := execreturn.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cctx.App.Writer, "Created new multisig: ", execreturn.IDAddress, execreturn.RobustAddress)

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

		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

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

		_, _ = fmt.Fprintf(cctx.App.Writer, "Balance: %s\n", types.FIL(act.Balance))
		_, _ = fmt.Fprintf(cctx.App.Writer, "Spendable: %s\n", types.FIL(types.BigSub(act.Balance, locked)))

		if cctx.Bool("vesting") {
			ib, err := mstate.InitialBalance()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "InitialBalance: %s\n", types.FIL(ib))
			se, err := mstate.StartEpoch()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "StartEpoch: %d\n", se)
			ud, err := mstate.UnlockDuration()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "UnlockDuration: %d\n", ud)
		}

		signers, err := mstate.Signers()
		if err != nil {
			return err
		}
		threshold, err := mstate.Threshold()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cctx.App.Writer, "Threshold: %d / %d\n", threshold, len(signers))
		_, _ = fmt.Fprintln(cctx.App.Writer, "Signers:")

		signerTable := tabwriter.NewWriter(cctx.App.Writer, 8, 4, 2, ' ', 0)
		_, _ = fmt.Fprintf(signerTable, "ID\tAddress\n")
		for _, s := range signers {
			signerActor, err := api.StateAccountKey(ctx, s, types.EmptyTSK)
			if err != nil {
				_, _ = fmt.Fprintf(signerTable, "%s\t%s\n", s, "N/A")
			} else {
				_, _ = fmt.Fprintf(signerTable, "%s\t%s\n", s, signerActor)
			}
		}
		if err := signerTable.Flush(); err != nil {
			return xerrors.Errorf("flushing output: %+v", err)
		}

		pending := make(map[int64]multisig.Transaction)
		if err := mstate.ForEachPendingTxn(func(id int64, txn multisig.Transaction) error {
			pending[id] = txn
			return nil
		}); err != nil {
			return xerrors.Errorf("reading pending transactions: %w", err)
		}

		decParams := cctx.Bool("decode-params")
		_, _ = fmt.Fprintln(cctx.App.Writer, "Transactions: ", len(pending))
		if len(pending) > 0 {
			var txids []int64
			for txid := range pending {
				txids = append(txids, txid)
			}
			sort.Slice(txids, func(i, j int) bool {
				return txids[i] < txids[j]
			})

			w := tabwriter.NewWriter(cctx.App.Writer, 8, 4, 2, ' ', 0)
			_, _ = fmt.Fprintf(w, "ID\tState\tApprovals\tTo\tValue\tMethod\tParams\n")
			for _, txid := range txids {
				tx := pending[txid]
				target := tx.To.String()
				if tx.To == ownId {
					target += " (self)"
				}
				targAct, err := api.StateGetActor(ctx, tx.To, types.EmptyTSK)
				paramStr := fmt.Sprintf("%x", tx.Params)

				if err != nil {
					if tx.Method == 0 {
						_, _ = fmt.Fprintf(
							w,
							"%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n",
							txid,
							"pending",
							len(tx.Approved),
							target,
							types.FIL(tx.Value),
							"Send",
							tx.Method,
							paramStr,
						)
					} else {
						_, _ = fmt.Fprintf(
							w,
							"%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n",
							txid,
							"pending",
							len(tx.Approved),
							target,
							types.FIL(tx.Value),
							"new account, unknown method",
							tx.Method,
							paramStr,
						)
					}
				} else {
					method := consensus.NewActorRegistry().Methods[targAct.Code][tx.Method] // TODO: use remote map

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

					_, _ = fmt.Fprintf(
						w,
						"%d\t%s\t%d\t%s\t%s\t%s(%d)\t%s\n",
						txid,
						"pending",
						len(tx.Approved),
						target,
						types.FIL(tx.Value),
						method.Name,
						tx.Method,
						paramStr,
					)
				}
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
		if cctx.NArg() < 3 {
			return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address, destination, and value"))
		}

		if cctx.NArg() > 3 && cctx.NArg() != 5 {
			return ShowHelp(cctx, fmt.Errorf("must either pass three or five arguments"))
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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
		if cctx.NArg() == 5 {
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

		proto, err := api.MsigPropose(ctx, msig, dest, types.BigInt(value), from, method, params)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("proposal returned exit %d", wait.Receipt.ExitCode)
		}

		var retval msig2.ProposeReturn
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
		if cctx.NArg() < 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address and message ID"))
		}

		if cctx.NArg() > 2 && cctx.NArg() < 5 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig approve <msig addr> <message ID> <proposer address> <destination> <value>"))
		}

		if cctx.NArg() > 5 && cctx.NArg() != 7 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig approve <msig addr> <message ID> <proposer address> <destination> <value> [ <method> <params> ]"))
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
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

		var msgCid cid.Cid
		if cctx.NArg() == 2 {
			proto, err := api.MsigApprove(ctx, msig, txid, from)
			if err != nil {
				return err
			}

			sm, err := InteractiveSend(ctx, cctx, srv, proto)
			if err != nil {
				return err
			}

			msgCid = sm.Cid()
		} else {
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
			if cctx.NArg() == 7 {
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

			proto, err := api.MsigApproveTxnHash(ctx, msig, txid, proposer, dest, types.BigInt(value), from, method, params)
			if err != nil {
				return err
			}

			sm, err := InteractiveSend(ctx, cctx, srv, proto)
			if err != nil {
				return err
			}

			msgCid = sm.Cid()
		}

		fmt.Println("sent approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("approve returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigCancelCmd = &cli.Command{
	Name:      "cancel",
	Usage:     "Cancel a multisig message",
	ArgsUsage: "<multisigAddress messageId> [destination value [methodId methodParams]]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the cancel message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 2 {
			return ShowHelp(cctx, fmt.Errorf("must pass at least multisig address and message ID"))
		}

		if cctx.NArg() > 2 && cctx.NArg() < 4 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig cancel <msig addr> <message ID> <destination> <value>"))
		}

		if cctx.NArg() > 4 && cctx.NArg() != 6 {
			return ShowHelp(cctx, fmt.Errorf("usage: msig cancel <msig addr> <message ID> <destination> <value> [ <method> <params> ]"))
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		txid, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
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

		var msgCid cid.Cid
		if cctx.NArg() == 2 {
			proto, err := api.MsigCancel(ctx, msig, txid, from)
			if err != nil {
				return err
			}

			sm, err := InteractiveSend(ctx, cctx, srv, proto)
			if err != nil {
				return err
			}

			msgCid = sm.Cid()
		} else {
			dest, err := address.NewFromString(cctx.Args().Get(2))
			if err != nil {
				return err
			}

			value, err := types.ParseFIL(cctx.Args().Get(3))
			if err != nil {
				return err
			}

			var method uint64
			var params []byte
			if cctx.NArg() == 6 {
				m, err := strconv.ParseUint(cctx.Args().Get(4), 10, 64)
				if err != nil {
					return err
				}
				method = m

				p, err := hex.DecodeString(cctx.Args().Get(5))
				if err != nil {
					return err
				}
				params = p
			}

			proto, err := api.MsigCancelTxnHash(ctx, msig, txid, dest, types.BigInt(value), from, method, params)
			if err != nil {
				return err
			}

			sm, err := InteractiveSend(ctx, cctx, srv, proto)
			if err != nil {
				return err
			}

			msgCid = sm.Cid()
		}

		fmt.Println("sent cancel in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("cancel returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}

var msigRemoveProposeCmd = &cli.Command{
	Name:      "propose-remove",
	Usage:     "Propose to remove a signer",
	ArgsUsage: "[multisigAddress signer]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "decrease-threshold",
			Usage: "whether the number of required signers should be decreased",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the propose message from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigRemoveSigner(ctx, msig, from, addr, cctx.Bool("decrease-threshold"))
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent remove proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("add proposal returned exit %d", wait.Receipt.ExitCode)
		}

		var ret multisig.ProposeReturn
		err = ret.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return))
		if err != nil {
			return xerrors.Errorf("decoding proposal return: %w", err)
		}
		fmt.Printf("TxnID: %d", ret.TxnID)

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
		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, msig, head.Key())
		if err != nil {
			return err
		}

		mstate, err := multisig.Load(store, act)
		if err != nil {
			return err
		}

		signers, err := mstate.Signers()
		if err != nil {
			return err
		}

		addrId, err := api.StateLookupID(ctx, addr, types.EmptyTSK)
		if err != nil {
			return err
		}

		for _, s := range signers {
			if s == addrId {
				return fmt.Errorf("%s is already a signer", addr.String())
			}
		}

		proto, err := api.MsigAddPropose(ctx, msig, from, addr, cctx.Bool("increase-threshold"))
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		_, _ = fmt.Fprintln(cctx.App.Writer, "sent add proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 5 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigAddApprove(ctx, msig, from, txid, prop, newAdd, inc)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent add approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 4 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigAddCancel(ctx, msig, from, txid, newAdd, inc)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent add cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 3 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigSwapPropose(ctx, msig, from, oldAdd, newAdd)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent swap proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 5 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigSwapApprove(ctx, msig, from, txid, prop, oldAdd, newAdd)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent swap approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 4 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		proto, err := api.MsigSwapCancel(ctx, msig, from, txid, oldAdd, newAdd)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent swap cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 4 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		params, actErr := actors.SerializeParams(&msig2.LockBalanceParams{
			StartEpoch:     abi.ChainEpoch(start),
			UnlockDuration: abi.ChainEpoch(duration),
			Amount:         big.Int(amount),
		})

		if actErr != nil {
			return actErr
		}

		proto, err := api.MsigPropose(ctx, msig, msig, big.Zero(), from, uint64(multisig.Methods.LockBalance), params)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent lock proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 6 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		params, actErr := actors.SerializeParams(&msig2.LockBalanceParams{
			StartEpoch:     abi.ChainEpoch(start),
			UnlockDuration: abi.ChainEpoch(duration),
			Amount:         big.Int(amount),
		})

		if actErr != nil {
			return actErr
		}

		proto, err := api.MsigApproveTxnHash(ctx, msig, txid, prop, msig, big.Zero(), from, uint64(multisig.Methods.LockBalance), params)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent lock approval in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 5 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
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

		params, actErr := actors.SerializeParams(&msig2.LockBalanceParams{
			StartEpoch:     abi.ChainEpoch(start),
			UnlockDuration: abi.ChainEpoch(duration),
			Amount:         big.Int(amount),
		})

		if actErr != nil {
			return actErr
		}

		proto, err := api.MsigCancelTxnHash(ctx, msig, txid, msig, big.Zero(), from, uint64(multisig.Methods.LockBalance), params)
		if err != nil {
			return err
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent lock cancellation in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
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
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
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

var msigProposeThresholdCmd = &cli.Command{
	Name:      "propose-threshold",
	Usage:     "Propose setting a different signing threshold on the account",
	ArgsUsage: "<multisigAddress newM>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "account to send the proposal from",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := ReqContext(cctx)

		msig, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		newM, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
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

		params, actErr := actors.SerializeParams(&msig2.ChangeNumApprovalsThresholdParams{
			NewThreshold: newM,
		})

		if actErr != nil {
			return actErr
		}

		proto, err := api.MsigPropose(ctx, msig, msig, types.NewInt(0), from, uint64(multisig.Methods.ChangeNumApprovalsThreshold), params)
		if err != nil {
			return fmt.Errorf("failed to propose change of threshold: %w", err)
		}

		sm, err := InteractiveSend(ctx, cctx, srv, proto)
		if err != nil {
			return err
		}

		msgCid := sm.Cid()

		fmt.Println("sent change threshold proposal in message: ", msgCid)

		wait, err := api.StateWaitMsg(ctx, msgCid, uint64(cctx.Int("confidence")), policy.ChainFinality, true)
		if err != nil {
			return err
		}

		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("change threshold proposal returned exit %d", wait.Receipt.ExitCode)
		}

		return nil
	},
}
