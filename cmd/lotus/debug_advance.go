// +build debug

package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/miner"
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
			pending, err := api.MpoolPending(ctx, head)
			if err != nil {
				return err
			}

			msgs, err := miner.SelectMessages(ctx, api.StateGetActor, head, pending)
			if len(msgs) > build.BlockMessageLimit {
				log.Error("SelectMessages returned too many messages: ", len(msgs))
				msgs = msgs[:build.BlockMessageLimit]
			}

			addr, _ := address.NewIDAddress(101)
			var ticket *types.Ticket
			{
				vrfBase := head.MinTicket().VRFProof
				ret, err := api.StateCall(ctx, &types.Message{
					From:   addr,
					To:     addr,
					Method: actors.MAMethods.GetWorkerAddr,
				}, head)
				if err != nil {
					return xerrors.Errorf("failed to get miner worker addr: %w", err)
				}

				if ret.ExitCode != 0 {
					return xerrors.Errorf("failed to get miner worker addr (exit code %d)", ret.ExitCode)
				}

				w, err := address.NewFromBytes(ret.Return)
				if err != nil {
					return xerrors.Errorf("GetWorkerAddr returned malformed address: %w", err)
				}
				t, err := gen.ComputeVRF(ctx, api.WalletSign, w, addr, gen.DSepTicket, vrfBase)
				if err != nil {
					return xerrors.Errorf("compute vrf failed: %w", err)
				}
				ticket = &types.Ticket{
					VRFProof: t,
				}

			}

			epostp := &types.EPostProof{
				Proof: []byte("valid proof"),
				Candidates: []types.EPostTicket{
					{
						ChallengeIndex: 0,
						SectorID:       1,
					},
				},
			}

			{
				r, err := api.ChainGetRandomness(ctx, head.Key(), int64(head.Height()+1)-build.EcRandomnessLookback)
				if err != nil {
					return xerrors.Errorf("chain get randomness: %w", err)
				}
				mworker, err := api.StateMinerWorker(ctx, addr, head)
				if err != nil {
					return xerrors.Errorf("failed to get miner worker: %w", err)
				}

				vrfout, err := gen.ComputeVRF(ctx, api.WalletSign, mworker, addr, gen.DSepElectionPost, r)
				if err != nil {
					return xerrors.Errorf("failed to compute VRF: %w", err)
				}
				epostp.PostRand = vrfout
			}

			uts := head.MinTimestamp() + uint64(build.BlockDelay)
			nheight := head.Height() + 1
			blk, err := api.MinerCreateBlock(ctx, addr, head, ticket, epostp, msgs, nheight, uts)
			if err != nil {
				return xerrors.Errorf("creating block: %w", err)
			}

			return api.SyncSubmitBlock(ctx, blk)
		},
	}
}
