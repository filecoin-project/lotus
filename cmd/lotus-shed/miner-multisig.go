package main

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/builtin/v9/multisig"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
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
		mmProposeChangeBeneficiary,
		mmConfirmChangeBeneficiary,
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

		sp, err := actors.SerializeParams(&miner.WithdrawBalanceParams{
			AmountRequested: abi.TokenAmount(val),
		})
		if err != nil {
			return err
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.WithdrawBalance), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose owner change tx failed!")
			return err
		}

		var retval multisig.ProposeReturn
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
			return lcli.IncorrectNumArgs(cctx)
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

		sp, err := actors.SerializeParams(&miner.WithdrawBalanceParams{
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

		acid, err := api.MsigApproveTxnHash(ctx, multisigAddr, txid, proposer, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.WithdrawBalance), sp)
		if err != nil {
			return xerrors.Errorf("approving message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Approve Message CID:", acid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, acid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Approve owner change tx failed!")
			return err
		}

		var retval multisig.ApproveReturn
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

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ChangeOwnerAddress), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose owner change tx failed!")
			return err
		}

		var retval multisig.ProposeReturn
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
			return lcli.IncorrectNumArgs(cctx)
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

		acid, err := api.MsigApproveTxnHash(ctx, multisigAddr, txid, proposer, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ChangeOwnerAddress), sp)
		if err != nil {
			return xerrors.Errorf("approving message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Approve Message CID:", acid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, acid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Approve owner change tx failed!")
			return err
		}

		var retval multisig.ApproveReturn
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
			return fmt.Errorf("must pass new worker address")
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
				_, _ = fmt.Fprintf(cctx.App.Writer, "Worker key change to %s successfully proposed.\n", na)
				_, _ = fmt.Fprintf(cctx.App.Writer, "Call 'confirm-change-worker' at or after height %d to complete.\n", mi.WorkerChangeEpoch)
				return fmt.Errorf("change to worker address %s already pending", na)
			}
		}

		cwp := &miner.ChangeWorkerAddressParams{
			NewWorker:       newAddr,
			NewControlAddrs: mi.ControlAddresses,
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "newAddr: %s\n", newAddr)
		_, _ = fmt.Fprintf(cctx.App.Writer, "NewControlAddrs: %s\n", mi.ControlAddresses)

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ChangeWorkerAddress), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose worker change tx failed!")
			return err
		}

		var retval multisig.ProposeReturn
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

var mmProposeChangeBeneficiary = &cli.Command{
	Name:      "propose-change-beneficiary",
	Usage:     "Propose a beneficiary address change",
	ArgsUsage: "[beneficiaryAddress quota expiration]",
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
		if cctx.NArg() != 3 {
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

		expiration, err := types.BigFromString(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("parsing expiration: %w", err)
		}

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
		if err != nil {
			return err
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
			NewExpiration:  abi.ChainEpoch(expiration.Int64()),
		}

		sp, err := actors.SerializeParams(params)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ChangeBeneficiary), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Println("Propose Message CID: ", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return xerrors.Errorf("waiting for message to be included in block: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("propose beneficiary change failed")
		}

		return nil
	},
}

var mmConfirmChangeWorker = &cli.Command{
	Name:      "confirm-change-worker",
	Usage:     "Confirm an worker address change",
	ArgsUsage: "[newWorker]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass new worker address")
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

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ConfirmChangeWorkerAddress), nil)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose worker change tx failed!")
			return err
		}

		var retval multisig.ProposeReturn
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

var mmConfirmChangeBeneficiary = &cli.Command{
	Name:      "confirm-change-beneficiary",
	Usage:     "Confirm a beneficiary address change",
	ArgsUsage: "[minerAddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
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

		multisigAddr, sender, minerAddr, err := getInputs(cctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		if mi.PendingBeneficiaryTerm == nil {
			return fmt.Errorf("no pending beneficiary term found for miner %s", minerAddr)
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

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ChangeBeneficiary), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		fmt.Println("Confirm Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return xerrors.Errorf("waiting for message to be included in block: %w", err)
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			return fmt.Errorf("confirm beneficiary change failed with code %d", wait.Receipt.ExitCode)
		}

		updatedMinerInfo, err := api.StateMinerInfo(ctx, minerAddr, types.EmptyTSK)
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

var mmProposeControlSet = &cli.Command{
	Name:      "propose-control-set",
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

		cwp := &miner.ChangeWorkerAddressParams{
			NewWorker:       mi.Worker,
			NewControlAddrs: toSet,
		}

		sp, err := actors.SerializeParams(cwp)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		pcid, err := api.MsigPropose(ctx, multisigAddr, minerAddr, big.Zero(), sender, uint64(builtin.MethodsMiner.ChangeWorkerAddress), sp)
		if err != nil {
			return xerrors.Errorf("proposing message: %w", err)
		}

		_, _ = fmt.Fprintln(cctx.App.Writer, "Propose Message CID:", pcid)

		// wait for it to get mined into a block
		wait, err := api.StateWaitMsg(ctx, pcid, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			_, _ = fmt.Fprintln(cctx.App.Writer, "Propose worker change tx failed!")
			return err
		}

		var retval multisig.ProposeReturn
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
