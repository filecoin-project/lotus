// +build debug

package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"golang.org/x/xerrors"

	"gopkg.in/urfave/cli.v2"
)

func init() {
	AdvanceBlockCmd = &cli.Command{
		Name: "advance-block",
		Action: func(cctx *cli.Context) error {
			api, closer, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			ctx := lcli.ReqContext(cctx)
			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}
			pending, err := api.MpoolPending(ctx, head.Key())
			if err != nil {
				return err
			}

			msgs, err := miner.SelectMessages(ctx, api.StateGetActor, head, pending)
			if err != nil {
				return err
			}
			if len(msgs) > build.BlockMessageLimit {
				log.Error("SelectMessages returned too many messages: ", len(msgs))
				msgs = msgs[:build.BlockMessageLimit]
			}

			addr, _ := address.NewIDAddress(1000)
			var ticket *types.Ticket
			{
				w, err := api.StateMinerWorker(ctx, addr, head.Key())
				if err != nil {
					return xerrors.Errorf("StateMinerWorker: %w", err)
				}

				rand, err := api.ChainGetRandomness(ctx, head.Key(), crypto.DomainSeparationTag_TicketProduction, head.Height(), addr.Bytes())
				if err != nil {
					return xerrors.Errorf("failed to get randomness: %w", err)
				}

				t, err := gen.ComputeVRF(ctx, api.WalletSign, w, rand)
				if err != nil {
					return xerrors.Errorf("compute vrf failed: %w", err)
				}
				ticket = &types.Ticket{
					VRFProof: t,
				}

			}

			epostp := &types.EPostProof{
				Proofs: []abi.PoStProof{{ProofBytes: []byte("valid proof")}},
				Candidates: []types.EPostTicket{
					{
						ChallengeIndex: 0,
						SectorID:       1,
					},
				},
			}

			{
				r, err := api.ChainGetRandomness(ctx, head.Key(), crypto.DomainSeparationTag_ElectionPoStChallengeSeed, (head.Height()+1)-build.EcRandomnessLookback, addr.Bytes())
				if err != nil {
					return xerrors.Errorf("chain get randomness: %w", err)
				}
				mworker, err := api.StateMinerWorker(ctx, addr, head.Key())
				if err != nil {
					return xerrors.Errorf("failed to get miner worker: %w", err)
				}

				vrfout, err := gen.ComputeVRF(ctx, api.WalletSign, mworker, r)
				if err != nil {
					return xerrors.Errorf("failed to compute VRF: %w", err)
				}
				epostp.PostRand = vrfout
			}

			uts := head.MinTimestamp() + uint64(build.BlockDelay)
			nheight := head.Height() + 1
			blk, err := api.MinerCreateBlock(ctx, addr, head.Key(), ticket, epostp, nil, msgs, nheight, uts)
			if err != nil {
				return xerrors.Errorf("creating block: %w", err)
			}

			return api.SyncSubmitBlock(ctx, blk)
		},
	}
}
