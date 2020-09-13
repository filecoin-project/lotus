// +build debug

package main

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/filecoin-project/go-bitfield"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"golang.org/x/xerrors"

	"github.com/urfave/cli/v2"
)

func init() {
	AdvanceBlockCmd = &cli.Command{
		Name: "advance-block",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "submit-posts",
				Usage: "includes a fake PoSt submission in every block created",
			},
		},
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
			msgs, err := api.MpoolSelect(ctx, head.Key(), 1)
			if err != nil {
				return err
			}

			addr, _ := address.NewIDAddress(1000)
			var ticket *types.Ticket
			{
				mi, err := api.StateMinerInfo(ctx, addr, head.Key())
				if err != nil {
					return xerrors.Errorf("StateMinerWorker: %w", err)
				}

				// XXX: This can't be right
				rand, err := api.ChainGetRandomnessFromTickets(ctx, head.Key(), crypto.DomainSeparationTag_TicketProduction, head.Height(), addr.Bytes())
				if err != nil {
					return xerrors.Errorf("failed to get randomness: %w", err)
				}

				t, err := gen.ComputeVRF(ctx, api.WalletSign, mi.Worker, rand)
				if err != nil {
					return xerrors.Errorf("compute vrf failed: %w", err)
				}
				ticket = &types.Ticket{
					VRFProof: t,
				}

				if cctx.Bool("submit-posts") {
					wpmsg, err := makePoStMessage(ctx, api, addr, mi.Worker, head.Key())
					if err == nil {
						msgs = append(msgs, wpmsg)
					} else {
						log.Errorf("Failed to make post msg: %w", err)
					}
				}
			}

			mbi, err := api.MinerGetBaseInfo(ctx, addr, head.Height()+1, head.Key())
			if err != nil {
				return xerrors.Errorf("getting base info: %w", err)
			}

			ep := &types.ElectionProof{}
			ep.WinCount = ep.ComputeWinCount(types.NewInt(1), types.NewInt(1))
			for ep.WinCount == 0 {
				fakeVrf := make([]byte, 8)
				unixNow := uint64(time.Now().UnixNano())
				binary.LittleEndian.PutUint64(fakeVrf, unixNow)

				ep.VRFProof = fakeVrf
				ep.WinCount = ep.ComputeWinCount(types.NewInt(1), types.NewInt(1))
			}

			uts := head.MinTimestamp() + uint64(build.BlockDelaySecs)
			nheight := head.Height() + 1
			blk, err := api.MinerCreateBlock(ctx, &lapi.BlockTemplate{
				addr, head.Key(), ticket, ep, mbi.BeaconEntries, msgs, nheight, uts, gen.ValidWpostForTesting,
			})
			if err != nil {
				return xerrors.Errorf("creating block: %w", err)
			}

			return api.SyncSubmitBlock(ctx, blk)
		},
	}
}

func makePoStMessage(ctx context.Context, api lapi.FullNode, maddr address.Address, waddr address.Address, tsk types.TipSetKey) (*types.SignedMessage, error) {
	di, err := api.StateMinerProvingDeadline(ctx, maddr, tsk)
	if err != nil {
		return nil, err
	}

	partitions, err := api.StateMinerPartitions(ctx, maddr, di.Index, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting partitions: %w", err)
	}

	params := &miner.SubmitWindowedPoStParams{
		Deadline:   di.Index,
		Partitions: make([]miner.PoStPartition, 0, len(partitions)),
		Proofs:     nil,
	}
	for partIdx, _ := range partitions {
		params.Partitions = append(params.Partitions, miner.PoStPartition{
			Index:   uint64(partIdx),
			Skipped: bitfield.New(),
		})
	}

	params.Proofs = gen.ValidWpostForTesting

	commEpoch := di.Open
	commRand, err := api.ChainGetRandomnessFromTickets(ctx, tsk, crypto.DomainSeparationTag_PoStChainCommit, commEpoch, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to get chain randomness for windowPost: %w", err)
	}
	params.ChainCommitEpoch = commEpoch
	params.ChainCommitRand = commRand

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return nil, xerrors.Errorf("could not serialize submit post parameters: %w", aerr)
	}

	msg := &types.Message{
		To:     maddr,
		From:   waddr,
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(0),
	}

	return api.WalletSignMessage(ctx, waddr, msg)
}
