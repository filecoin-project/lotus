package kit

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestUnmanagedMiner is a miner that's not managed by the storage/
// infrastructure, all tasks must be manually executed, managed and scheduled by
// the test or test kit.
type TestUnmanagedMiner struct {
	t       *testing.T
	options nodeOpts

	ActorAddr address.Address
	OwnerKey  *key.Key
	FullNode  *TestFullNode
	Libp2p    struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}
}

func (tm *TestUnmanagedMiner) StartWindowedPostLoop(ctx context.Context, sectorNumber abi.SectorNumber, proofType abi.RegisteredSealProof) {
	tm.t.Helper()

	go func() {
		for ctx.Err() == nil {
			currentEpoch, nextPost, err := tm.manualOnboardingCalculateNextPostEpoch(ctx, sectorNumber)
			if err != nil {
				errCh <- err
				return
			}
			if ctx.Err() != nil {
				return
			}
			nextPost += 5 // give a little buffer
			fmt.Printf("WindowPoST(%d) Waiting %d until epoch %d to submit PoSt\n", sectorNumber, nextPost-currentEpoch, nextPost)

			// Create channel to listen for chain head
			heads, err := tm.FullNode.ChainNotify(ctx)
			if err != nil {
				errCh <- err
				return
			}
			// Wait for nextPost epoch
			for chg := range heads {
				var ts *types.TipSet
				for _, c := range chg {
					if c.Type != "apply" {
						continue
					}
					ts = c.Val
					if ts.Height() >= nextPost {
						break
					}
				}
				if ctx.Err() != nil {
					return
				}
				if ts != nil && ts.Height() >= nextPost {
					break
				}
			}
			if ctx.Err() != nil {
				return
			}

			err = manualOnboardingSubmitWindowPost(ctx, withMockProofs, client, miner, sectorNumber, cacheDirPath, sealedSectorPath, sealedCid, proofType)
			if err != nil {
				errCh <- err
				return
			}

			// signal first post is done
			select {
			case <-first:
			default:
				close(first)
			}
		}
	}()
}

func (tm *TestUnmanagedMiner) manualOnboardingCalculateNextPostEpoch(
	ctx context.Context,
	sectorNumber abi.SectorNumber,
) (abi.ChainEpoch, abi.ChainEpoch, error) {

	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get sector partition: %w", err)
	}

	fmt.Printf("WindowPoST(%d): SectorPartition: %+v\n", sectorNumber, sp)

	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get proving deadline: %w", err)
	}

	fmt.Printf("WindowPoST(%d): ProvingDeadline: %+v\n", sectorNumber, di)

	// periodStart tells us the first epoch of the current proving period (24h)
	// although it may be in the future if we don't need to submit post in this period
	periodStart := di.PeriodStart
	if di.PeriodStart < di.CurrentEpoch && sp.Deadline <= di.Index {
		// the deadline we want has past in this current proving period, so wait till the next one
		periodStart += di.WPoStProvingPeriod
	}
	provingEpoch := periodStart + (di.WPoStProvingPeriod/abi.ChainEpoch(di.WPoStPeriodDeadlines))*abi.ChainEpoch(sp.Deadline)

	return di.CurrentEpoch, provingEpoch, nil
}

func (tm *TestUnmanagedMiner) manualOnboardingSubmitWindowPost(
	ctx context.Context,
	withMockProofs bool,
	sectorNumber abi.SectorNumber,
	cacheDirPath, sealedSectorPath string,
	sealedCid cid.Cid,
	proofType abi.RegisteredSealProof,
) error {
	fmt.Printf("WindowPoST(%d): Running WindowPoSt ...\n", sectorNumber)

	head, err := tm.FullNode.ChainHead(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain head: %w", err)
	}

	sp, err := tm.FullNode.StateSectorPartition(ctx, tm.ActorAddr, sectorNumber, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get sector partition: %w", err)
	}

	// We should be up to the deadline we care about
	di, err := tm.FullNode.StateMinerProvingDeadline(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get proving deadline: %w", err)
	}
	fmt.Printf("WindowPoST(%d): SectorPartition: %+v, ProvingDeadline: %+v\n", sectorNumber, sp, di)
	if di.Index != sp.Deadline {
		return fmt.Errorf("sector %d is not in the deadline %d, but %d", sectorNumber, sp.Deadline, di.Index)
	}

	var proofBytes []byte
	if withMockProofs {
		proofBytes = []byte{0xde, 0xad, 0xbe, 0xef}
	} else {
		proofBytes, err = manualOnboardingGenerateWindowPost(ctx, tm.FullNode, cacheDirPath, sealedSectorPath, tm.ActorAddr, sectorNumber, sealedCid, proofType)
		if err != nil {
			return fmt.Errorf("failed to generate window post: %w", err)
		}
	}

	fmt.Printf("WindowedPoSt(%d) Submitting ...\n", sectorNumber)

	chainRandomnessEpoch := di.Challenge
	chainRandomness, err := tm.FullNode.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, chainRandomnessEpoch, nil, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get chain randomness: %w", err)
	}

	minerInfo, err := tm.FullNode.StateMinerInfo(ctx, tm.ActorAddr, head.Key())
	if err != nil {
		return fmt.Errorf("failed to get miner info: %w", err)
	}

	r, err := manualOnboardingSubmitMessage(ctx, tm.FullNode, miner, &miner14.SubmitWindowedPoStParams{
		ChainCommitEpoch: chainRandomnessEpoch,
		ChainCommitRand:  chainRandomness,
		Deadline:         sp.Deadline,
		Partitions:       []miner14.PoStPartition{{Index: sp.Partition}},
		Proofs:           []proof.PoStProof{{PoStProof: minerInfo.WindowPoStProofType, ProofBytes: proofBytes}},
	}, 0, builtin.MethodsMiner.SubmitWindowedPoSt)
	if err != nil {
		return fmt.Errorf("failed to submit PoSt: %w", err)
	}
	if !r.Receipt.ExitCode.IsSuccess() {
		return fmt.Errorf("submitting PoSt failed: %s", r.Receipt.ExitCode)
	}

	if !withMockProofs {
		// Dispute the PoSt to confirm the validity of the PoSt since PoSt acceptance is optimistic
		if err := manualOnboardingDisputeWindowPost(ctx, client, miner, sectorNumber); err != nil {
			return fmt.Errorf("failed to dispute PoSt: %w", err)
		}
	}
	return nil
}

func (tm *TestUnmanagedMiner) manualOnboardingSubmitMessage(
	ctx context.Context,
	params cbg.CBORMarshaler,
	value uint64,
	method abi.MethodNum,
) (*api.MsgLookup, error) {
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return nil, fmt.Errorf("failed to serialize params: %w", aerr)
	}

	m, err := tm.FullNode.MpoolPushMessage(ctx, &types.Message{
		To:     tm.ActorAddr,
		From:   tm.OwnerKey.Address,
		Value:  types.FromFil(value),
		Method: method,
		Params: enc,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to push message: %w", err)
	}

	return tm.FullNode.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
}
