package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/multiformats/go-base32"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var f3ProposalDecodeCmd = &cli.Command{
	Name:        "f3-proposal-decode",
	Description: "Decode F3 proposal identifiers from logs (format: BASE32@EPOCH)",
	Usage:       "lotus-shed f3-proposal-decode <proposal> [proposal2] ...",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() == 0 {
			return fmt.Errorf("at least one proposal must be provided (format: BASE32@EPOCH)")
		}

		ctx := cliutil.ReqContext(cctx)
		api, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return fmt.Errorf("getting api: %w", err)
		}
		defer closer()

		// Get the latest certificate to understand the chain structure
		latestCert, err := api.F3GetLatestCertificate(ctx)
		if err != nil {
			return fmt.Errorf("getting latest certificate: %w", err)
		}

		if latestCert == nil {
			return fmt.Errorf("no certificate available")
		}

		// Process each proposal
		for i := 0; i < cctx.NArg(); i++ {
			proposalStr := cctx.Args().Get(i)
			if err := decodeProposal(ctx, api, proposalStr, latestCert); err != nil {
				fmt.Fprintf(os.Stderr, "Error processing proposal %q: %v\n", proposalStr, err)
				continue
			}
			if i < cctx.NArg()-1 {
				fmt.Println()
			}
		}

		return nil
	},
}

func decodeProposal(ctx context.Context, api v1api.FullNode, proposalStr string, latestCert *certs.FinalityCertificate) error {
	parts := strings.Split(proposalStr, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid proposal format: expected BASE32@EPOCH, got %q", proposalStr)
	}

	base32Str := parts[0]
	epochStr := parts[1]

	epoch, err := strconv.ParseUint(epochStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid epoch: %w", err)
	}

	fmt.Printf("Proposal: %s\n", proposalStr)
	fmt.Printf("  Base32: %s\n", base32Str)
	fmt.Printf("  Epoch: %d\n", epoch)

	// Decode the base32 string
	decoded, err := base32.RawStdEncoding.DecodeString(base32Str)
	if err != nil {
		return fmt.Errorf("decoding base32: %w", err)
	}

	fmt.Printf("  Decoded bytes length: %d\n", len(decoded))
	fmt.Printf("  Decoded bytes (hex): %x\n", decoded)

	// Try to interpret as TipSetKey
	tsk, err := types.TipSetKeyFromBytes(decoded)
	if err == nil {
		cids := tsk.Cids()
		fmt.Printf("  TipSetKey (decoded): %s\n", tsk.String())
		fmt.Printf("  Number of CIDs: %d\n", len(cids))
		for i, c := range cids {
			fmt.Printf("    CID[%d]: %s\n", i, c.String())
		}
	} else {
		fmt.Printf("  Note: Could not decode as TipSetKey: %v\n", err)
		fmt.Printf("  This might be a hash or truncated representation\n")
	}

	// Find the corresponding entry in the certificate's ECChain
	fmt.Printf("\n  Certificate ECChain entry:\n")
	found := false
	for _, ecEntry := range latestCert.ECChain.TipSets {
		if int64(ecEntry.Epoch) == int64(epoch) {
			found = true
			fmt.Printf("    Epoch: %d\n", ecEntry.Epoch)
			
			// Convert gpbft.TipSetKey ([]byte) to types.TipSetKey to get CIDs
			tskFromCert, err := types.TipSetKeyFromBytes(ecEntry.Key)
			if err == nil {
				cids := tskFromCert.Cids()
				fmt.Printf("    TipSetKey CIDs (%d):\n", len(cids))
				for i, c := range cids {
					fmt.Printf("      [%d] %s\n", i, c.String())
				}
			} else {
				fmt.Printf("    TipSetKey (raw bytes, length %d): %x\n", len(ecEntry.Key), ecEntry.Key)
			}
			fmt.Printf("    PowerTable: %s\n", ecEntry.PowerTable.String())

			// Try to match the decoded bytes with the TipSetKey bytes
			if err == nil {
				// Reuse tskFromCert from above
				tskBytes := tskFromCert.Bytes()
				fmt.Printf("\n    TipSetKey bytes length: %d\n", len(tskBytes))
				fmt.Printf("    TipSetKey bytes (hex): %x\n", tskBytes)

				// Check if the decoded bytes match the beginning of the TipSetKey
				if len(decoded) <= len(tskBytes) {
					if len(decoded) > 0 && len(tskBytes) >= len(decoded) {
						match := true
						for i := 0; i < len(decoded); i++ {
							if decoded[i] != tskBytes[i] {
								match = false
								break
							}
						}
						if match {
							fmt.Printf("    ✓ Decoded bytes match the first %d bytes of TipSetKey\n", len(decoded))
						} else {
							fmt.Printf("    ✗ Decoded bytes do not match TipSetKey prefix\n")
						}
					}
				}
			} else {
				// If we couldn't decode earlier, try again here for matching
				tskBytes := ecEntry.Key
				if len(decoded) <= len(tskBytes) {
					if len(decoded) > 0 && len(tskBytes) >= len(decoded) {
						match := true
						for i := 0; i < len(decoded); i++ {
							if decoded[i] != tskBytes[i] {
								match = false
								break
							}
						}
						if match {
							fmt.Printf("    ✓ Decoded bytes match the first %d bytes of TipSetKey\n", len(decoded))
						} else {
							fmt.Printf("    ✗ Decoded bytes do not match TipSetKey prefix\n")
						}
					}
				}
			}
			break
		}
	}

	if !found {
		fmt.Printf("    ⚠ No ECChain entry found for epoch %d\n", epoch)
		fmt.Printf("    Latest certificate instance: %d\n", latestCert.GPBFTInstance)
		fmt.Printf("    Certificate covers epochs: %d to %d\n",
			latestCert.ECChain.TipSets[0].Epoch,
			latestCert.ECChain.TipSets[len(latestCert.ECChain.TipSets)-1].Epoch)
	}

	// Try to get the tipset from the chain
	fmt.Printf("\n  Chain tipset at epoch %d:\n", epoch)
	ts, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(epoch), types.EmptyTSK)
	if err != nil {
		fmt.Printf("    ⚠ Could not get tipset: %v\n", err)
	} else {
		fmt.Printf("    TipSetKey: %s\n", ts.Key().String())
		fmt.Printf("    Height: %d\n", ts.Height())
		fmt.Printf("    Blocks: %d\n", len(ts.Blocks()))
		for i, blk := range ts.Blocks() {
			fmt.Printf("      Block[%d]: %s (miner: %s)\n", i, blk.Cid(), blk.Miner)
		}
	}

	return nil
}
