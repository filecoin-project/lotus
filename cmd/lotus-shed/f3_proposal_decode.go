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

	// Try to get the tipset from the chain (canonical chain)
	fmt.Printf("\n  Canonical chain tipset at epoch %d:\n", epoch)
	canonicalTS, err := api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(epoch), types.EmptyTSK)
	if err != nil {
		fmt.Printf("    ⚠ Could not get tipset: %v\n", err)
	} else {
		fmt.Printf("    TipSetKey: %s\n", canonicalTS.Key().String())
		fmt.Printf("    Height: %d\n", canonicalTS.Height())
		fmt.Printf("    Blocks: %d\n", len(canonicalTS.Blocks()))
		for i, blk := range canonicalTS.Blocks() {
			fmt.Printf("      Block[%d]: %s (miner: %s)\n", i, blk.Cid(), blk.Miner)
		}
	}

	// Check if the proposed TipSetKey matches the canonical chain
	fmt.Printf("\n  Reorg Detection:\n")
	var proposedTSK types.TipSetKey
	proposedTSKValid := false
	
	// Try to get the proposed TipSetKey from the decoded bytes
	if tsk != nil && err == nil {
		proposedTSK = tsk
		proposedTSKValid = true
	} else if found {
		// Try to get it from the certificate entry
		if tskFromCert, err := types.TipSetKeyFromBytes(ecEntry.Key); err == nil {
			// Check if decoded bytes match certificate entry
			tskBytes := tskFromCert.Bytes()
			if len(decoded) > 0 && len(tskBytes) >= len(decoded) {
				match := true
				for i := 0; i < len(decoded); i++ {
					if decoded[i] != tskBytes[i] {
						match = false
						break
					}
				}
				if match {
					proposedTSK = tskFromCert
					proposedTSKValid = true
				}
			}
		}
	}

	if proposedTSKValid && canonicalTS != nil {
		if proposedTSK.Equals(canonicalTS.Key()) {
			fmt.Printf("    ✓ Proposal matches canonical chain\n")
			fmt.Printf("    ✓ No reorg detected\n")
		} else {
			fmt.Printf("    ⚠ REORG DETECTED!\n")
			fmt.Printf("    Proposed TipSetKey: %s\n", proposedTSK.String())
			fmt.Printf("    Canonical TipSetKey: %s\n", canonicalTS.Key().String())
			
			// Try to load the proposed tipset to see if it exists
			proposedTS, err := api.ChainGetTipSet(ctx, proposedTSK)
			if err != nil {
				fmt.Printf("    ⚠ Proposed tipset not found in chain store: %v\n", err)
				fmt.Printf("    This suggests the proposal is for a different chain/fork\n")
			} else {
				fmt.Printf("    Proposed tipset exists in chain store:\n")
				fmt.Printf("      Height: %d\n", proposedTS.Height())
				fmt.Printf("      Blocks: %d\n", len(proposedTS.Blocks()))
				for i, blk := range proposedTS.Blocks() {
					fmt.Printf("        Block[%d]: %s (miner: %s)\n", i, blk.Cid(), blk.Miner)
				}
				
				// Find where the chains diverge
				fmt.Printf("\n    Finding common ancestor...\n")
				head, err := api.ChainHead(ctx)
				if err == nil {
					// Walk backwards from both tipsets to find common ancestor
					canonicalWalk := canonicalTS
					proposedWalk := proposedTS
					
					canonicalChain := []*types.TipSet{canonicalTS}
					proposedChain := []*types.TipSet{proposedTS}
					
					maxDepth := 100 // Limit depth to avoid infinite loops
					depth := 0
					
					for !canonicalWalk.Key().Equals(proposedWalk.Key()) && depth < maxDepth {
						if canonicalWalk.Height() > proposedWalk.Height() {
							canonicalWalk, err = api.ChainGetTipSet(ctx, canonicalWalk.Parents())
							if err != nil {
								break
							}
							canonicalChain = append(canonicalChain, canonicalWalk)
						} else if proposedWalk.Height() > canonicalWalk.Height() {
							proposedWalk, err = api.ChainGetTipSet(ctx, proposedWalk.Parents())
							if err != nil {
								break
							}
							proposedChain = append(proposedChain, proposedWalk)
						} else {
							canonicalWalk, err = api.ChainGetTipSet(ctx, canonicalWalk.Parents())
							if err != nil {
								break
							}
							proposedWalk, err = api.ChainGetTipSet(ctx, proposedWalk.Parents())
							if err != nil {
								break
							}
							canonicalChain = append(canonicalChain, canonicalWalk)
							proposedChain = append(proposedChain, proposedWalk)
						}
						depth++
					}
					
					if canonicalWalk.Key().Equals(proposedWalk.Key()) {
						fmt.Printf("    Common ancestor found at epoch %d: %s\n", 
							canonicalWalk.Height(), canonicalWalk.Key().String())
						fmt.Printf("    Reorg depth: %d epochs\n", len(canonicalChain)-1)
						fmt.Printf("    Canonical chain from ancestor:\n")
						for i := len(canonicalChain) - 1; i >= 0; i-- {
							if i < len(canonicalChain)-1 {
								fmt.Printf("      → ")
							} else {
								fmt.Printf("      ")
							}
							fmt.Printf("Epoch %d: %s\n", canonicalChain[i].Height(), canonicalChain[i].Key().String())
						}
						fmt.Printf("    Proposed chain from ancestor:\n")
						for i := len(proposedChain) - 1; i >= 0; i-- {
							if i < len(proposedChain)-1 {
								fmt.Printf("      → ")
							} else {
								fmt.Printf("      ")
							}
							fmt.Printf("Epoch %d: %s\n", proposedChain[i].Height(), proposedChain[i].Key().String())
					} else {
						fmt.Printf("    ⚠ Could not find common ancestor (reached max depth or error)\n")
					}
				}
			}
		}
	} else if !proposedTSKValid {
		fmt.Printf("    ⚠ Could not decode proposed TipSetKey for comparison\n")
	} else if canonicalTS == nil {
		fmt.Printf("    ⚠ Could not get canonical chain tipset for comparison\n")
	}

	return nil
}
