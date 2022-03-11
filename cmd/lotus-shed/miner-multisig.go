package main

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-state-types/abi"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

    miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	msig5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/multisig"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var minerMultisigsCmd = &cli.Command{
	Name:        "miner-multisig",
	Description: "a collection of utilities for using multisigs as owner addresses of miners",
	Subcommands: []*cli.Command{
		mmProposeWithdrawBalance,
		mmApproveWithdrawBalance,
		mmProposeChangeOwner,
		mmApproveChangeOwner,
		mmProposeChangeWorker,
		mmConfirmChangeWorker,
		mmProposeControlSet,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "from",
			Usage:    "specify address to send message from",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "multisig",
			Usage:    "specify multisig that will receive the message",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "miner",
			Usage:    "specify miner being acted upon",
			Required: true,
		},
	},
}

var mmProposeWithdrawBalance = &cli.Command{
	Name:      "propose-withdraw",
	Usage:     "Propose to withdraw FIL from the miner",
	ArgsUsage: "[amount]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass amount to withdraw")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		val, err := types.ParseFIL(cctx.Args().First())
		if err != nil {
			return err
		}

		sp, err := actors.SerializeParams(&miner5.WithdrawBalanceParams{
			AmountRequested: abi.TokenAmount(val),
		})
		if err != nil {
			return err
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(miner.Methods.WithdrawBalance), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Propose owner change tx failed!")
			return err
		}

		var retval msig5.ProposeReturn
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

var mmApproveWithdrawBalance = &cli.Command{
	Name:      "approve-withdraw",
	Usage:     "Approve to withdraw FIL from the miner",
	ArgsUsage: "[amount txnId proposer]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return fmt.Errorf("must pass amount, txn Id, and proposer address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		val, err := types.ParseFIL(cctx.Args().First())
		if err != nil {
			return err
		}

		sp, err := actors.SerializeParams(&miner5.WithdrawBalanceParams{
			AmountRequested: abi.TokenAmount(val),
		})
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

		acid, err := api.MsigApproveTxnHash(ctx, multisigAddr, txid, proposer, minerAddr, big.Zero(), sender, uint64(miner.Methods.WithdrawBalance), sp)
		if err != nil {
			return xerrors.Errorf("approving message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Approve Message CID:", acid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, acid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Approve owner change tx failed!")
			return err
		}

		var retval msig5.ApproveReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return fmt.Errorf("failed to unmarshal approve return value: %w", err)
		}

		if retval.Applied {
			fmt.Printf("Transaction was executed with the approve\n")
			fmt.Printf("Exit Code: %d\n", retval.Code)
			fmt.Printf("Return Value: %x\n", retval.Ret)
		} else {
			fmt.Println("Transaction was approved, but not executed")
		}
		return nil
	},
}

var mmProposeChangeOwner = &cli.Command{
	Name:      "propose-change-owner",
	Usage:     "Propose an owner address change",
	ArgsUsage: "[newOwner]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass new owner address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.Owner == newAddr {
			return fmt.Errorf("owner address already set to %s", na)
		}

		sp, err := actors.SerializeParams(&newAddr)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(miner.Methods.ChangeOwnerAddress), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Propose owner change tx failed!")
			return err
		}

		var retval msig5.ProposeReturn
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

var mmApproveChangeOwner = &cli.Command{
	Name:      "approve-change-owner",
	Usage:     "Approve an owner address change",
	ArgsUsage: "[newOwner txnId proposer]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return fmt.Errorf("must pass new owner address, txn Id, and proposer address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
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

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.Owner == newAddr {
			return fmt.Errorf("owner address already set to %s", na)
		}

		sp, err := actors.SerializeParams(&newAddr)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		acid, err := api.MsigApproveTxnHash(ctx, multisigAddr, txid, proposer, minerAddr, big.Zero(), sender, uint64(miner.Methods.ChangeOwnerAddress), sp)
		if err != nil {
			return xerrors.Errorf("approving message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Approve Message CID:", acid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, acid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Approve owner change tx failed!")
			return err
		}

		var retval msig5.ApproveReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(wait.Receipt.Return)); err != nil {
			return fmt.Errorf("failed to unmarshal approve return value: %w", err)
		}

		if retval.Applied {
			fmt.Printf("Transaction was executed with the approve\n")
			fmt.Printf("Exit Code: %d\n", retval.Code)
			fmt.Printf("Return Value: %x\n", retval.Ret)
		} else {
			fmt.Println("Transaction was approved, but not executed")
		}
		return nil
	},
}

var mmProposeChangeWorker = &cli.Command{
	Name:      "propose-change-worker",
	Usage:     "Propose an worker address change",
	ArgsUsage: "[newWorker]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass new owner address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.NewWorker.Empty() {
			if mi.Worker == newAddr {
				return fmt.Errorf("worker address already set to %s", na)
			}
		} else {
			if mi.NewWorker == newAddr {
				fmt.Fprintf(cctx.App.Writer, "Worker key change to %s successfully proposed.\n", na)
				fmt.Fprintf(cctx.App.Writer, "Call 'confirm-change-worker' at or after height %d to complete.\n", mi.WorkerChangeEpoch)
				return fmt.Errorf("change to worker address %s already pending", na)
			}
		}

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       newAddr,
			NewControlAddrs: mi.ControlAddresses,
		}

		fmt.Fprintf(cctx.App.Writer, "newAddr: %s\n", newAddr)
		fmt.Fprintf(cctx.App.Writer, "NewControlAddrs: %s\n", mi.ControlAddresses)

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(miner.Methods.ChangeWorkerAddress), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Propose worker change tx failed!")
			return err
		}

		var retval msig5.ProposeReturn
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

var mmConfirmChangeWorker = &cli.Command{
	Name:      "Confirm-change-worker",
	Usage:     "Confirm an worker address change",
	ArgsUsage: "[newWorker]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass new owner address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		na, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		newAddr, err := api.StateLookupID(ctx, na, types.EmptyTSK)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.NewWorker.Empty() {
			return xerrors.Errorf("no worker key change proposed")
		} else if mi.NewWorker != newAddr {
			return xerrors.Errorf("worker key %s does not match current worker key proposal %s", newAddr, mi.NewWorker)
		}

		if head, err := api.ChainHead(ctx); err != nil {
			return xerrors.Errorf("failed to get the chain head: %w", err)
		} else if head.Height() < mi.WorkerChangeEpoch {
			return xerrors.Errorf("worker key change cannot be confirmed until %d, current height is %d", mi.WorkerChangeEpoch, head.Height())
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(miner.Methods.ConfirmUpdateWorkerKey), nil)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Propose worker change tx failed!")
			return err
		}

		var retval msig5.ProposeReturn
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

var mmProposeControlSet = &cli.Command{
	Name:      "ProposeControlSet",
	Usage:     "Set control address(-es)",
	ArgsUsage: "[...address]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass new owner address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
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

		cwp := &miner2.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(miner.Methods.ChangeWorkerAddress), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, build.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode != 0 {
			fmt.Fprintln(cctx.App.Writer, "Propose worker change tx failed!")
			return err
		}

		var retval msig5.ProposeReturn
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

func getInputs(cctx *cli.Context) (address.Address, address.Address, address.Address, error) {
	multisigAddr, err := address.NewFromString(cctx.String("multisig"))
	if err != nil {
		return address.Undef, address.Undef, address.Undef, err
	}

	sender, err := address.NewFromString(cctx.String("from"))
	if err != nil {
		return address.Undef, address.Undef, address.Undef, err
	}

	minerAddr, err := address.NewFromString(cctx.String("miner"))
	if err != nil {
		return address.Undef, address.Undef, address.Undef, err
	}

	return multisigAddr, sender, minerAddr, nil
}
