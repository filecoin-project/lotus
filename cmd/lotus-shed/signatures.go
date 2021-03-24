package main

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/lib/sigs"

	"github.com/filecoin-project/go-address"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var signaturesCmd = &cli.Command{
	Name:  "signatures",
	Usage: "tools involving signatures",
	Subcommands: []*cli.Command{
		sigsVerifyVoteCmd,
	},
}

var sigsVerifyVoteCmd = &cli.Command{
	Name:        "verify-vote",
	Description: "can be used to verify signed votes being submitted for FILPolls",
	Usage:       "<FIPnumber> <signingAddress> <signature>",
	Action: func(cctx *cli.Context) error {

		if cctx.Args().Len() != 3 {
			return xerrors.Errorf("usage: verify-vote <FIPnumber> <signingAddress> <signature>")
		}

		fip, err := strconv.ParseInt(cctx.Args().First(), 10, 64)
		if err != nil {
			return xerrors.Errorf("couldn't parse FIP number: %w", err)
		}

		addr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("couldn't parse signing address: %w", err)
		}

		sigBytes, err := hex.DecodeString(cctx.Args().Get(2))
		if err != nil {
			return xerrors.Errorf("couldn't parse sig: %w", err)
		}

		var sig crypto.Signature
		if err := sig.UnmarshalBinary(sigBytes); err != nil {
			return xerrors.Errorf("couldn't unmarshal sig: %w", err)
		}

		switch fip {
		case 14:
			approve := []byte("7 - Approve")

			if sigs.Verify(&sig, addr, approve) == nil {
				fmt.Println("valid vote for approving FIP-0014")
				return nil
			}

			reject := []byte("7 - Reject")
			if sigs.Verify(&sig, addr, reject) == nil {
				fmt.Println("valid vote for rejecting FIP-0014")
				return nil
			}

			return xerrors.Errorf("invalid vote for FIP-0014!")
		default:
			return xerrors.Errorf("unrecognized FIP number")
		}
	},
}
