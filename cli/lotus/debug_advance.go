//go:build debug

package lotus

import (
	"encoding/binary"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
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

			uts := head.MinTimestamp() + uint64(buildconstants.BlockDelaySecs)
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
