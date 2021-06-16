package main

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-state-types/abi"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

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
