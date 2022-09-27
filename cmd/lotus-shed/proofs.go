package main

import (
	"encoding/hex"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	lcli "github.com/filecoin-project/lotus/cli"
)

var proofsCmd = &cli.Command{
	Name: "proofs",
	Subcommands: []*cli.Command{
		verifySealProofCmd,
	},
}

var verifySealProofCmd = &cli.Command{
	Name:        "verify-seal",
	ArgsUsage:   "<commr> <commd> <proof>",
	Description: "Verify a seal proof with manual inputs",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "ticket",
		},
		&cli.StringFlag{
			Name: "proof-rand",
		},
		&cli.StringFlag{
			Name: "miner",
		},
		&cli.Uint64Flag{
			Name: "sector-id",
		},
		&cli.Int64Flag{
			Name: "proof-type",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 3 {
			return lcli.IncorrectNumArgs(cctx)
		}

		commr, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		commd, err := cid.Decode(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		proof, err := hex.DecodeString(cctx.Args().Get(2))
		if err != nil {
			return fmt.Errorf("failed to decode hex proof input: %w", err)
		}

		maddr, err := address.NewFromString(cctx.String("miner"))
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(maddr)
		if err != nil {
			return err
		}

		ticket, err := hex.DecodeString(cctx.String("ticket"))
		if err != nil {
			return err
		}

		proofRand, err := hex.DecodeString(cctx.String("proof-rand"))
		if err != nil {
			return err
		}

		snum := abi.SectorNumber(cctx.Uint64("sector-id"))

		ok, err := ffi.VerifySeal(prooftypes.SealVerifyInfo{
			SectorID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: snum,
			},
			SealedCID:             commr,
			SealProof:             abi.RegisteredSealProof(cctx.Int64("proof-type")),
			Proof:                 proof,
			DealIDs:               nil,
			Randomness:            ticket,
			InteractiveRandomness: proofRand,
			UnsealedCID:           commd,
		})
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("invalid proof")
		}

		fmt.Println("proof valid!")
		return nil
	},
}
