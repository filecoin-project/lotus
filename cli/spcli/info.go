package spcli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

func InfoCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Print miner actor info",
		Action: func(cctx *cli.Context) error {
			api, closer, err := cliutil.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			ctx := cliutil.ReqContext(cctx)

			ts, err := lcli.LoadTipSet(ctx, cctx, api)
			if err != nil {
				return err
			}

			addr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}
			mi, err := api.StateMinerInfo(ctx, addr, ts.Key())
			if err != nil {
				return err
			}

			availableBalance, err := api.StateMinerAvailableBalance(ctx, addr, ts.Key())
			if err != nil {
				return xerrors.Errorf("getting miner available balance: %w", err)
			}
			fmt.Printf("Available Balance: %s\n", types.FIL(availableBalance))
			fmt.Printf("Owner:\t%s\n", mi.Owner)
			fmt.Printf("Worker:\t%s\n", mi.Worker)
			for i, controlAddress := range mi.ControlAddresses {
				fmt.Printf("Control %d: \t%s\n", i, controlAddress)
			}
			if mi.Beneficiary != address.Undef {
				fmt.Printf("Beneficiary:\t%s\n", mi.Beneficiary)
				if mi.Beneficiary != mi.Owner {
					fmt.Printf("Beneficiary Quota:\t%s\n", mi.BeneficiaryTerm.Quota)
					fmt.Printf("Beneficiary Used Quota:\t%s\n", mi.BeneficiaryTerm.UsedQuota)
					fmt.Printf("Beneficiary Expiration:\t%s\n", mi.BeneficiaryTerm.Expiration)
				}
			}
			if mi.PendingBeneficiaryTerm != nil {
				fmt.Printf("Pending Beneficiary Term:\n")
				fmt.Printf("New Beneficiary:\t%s\n", mi.PendingBeneficiaryTerm.NewBeneficiary)
				fmt.Printf("New Quota:\t%s\n", mi.PendingBeneficiaryTerm.NewQuota)
				fmt.Printf("New Expiration:\t%s\n", mi.PendingBeneficiaryTerm.NewExpiration)
				fmt.Printf("Approved By Beneficiary:\t%t\n", mi.PendingBeneficiaryTerm.ApprovedByBeneficiary)
				fmt.Printf("Approved By Nominee:\t%t\n", mi.PendingBeneficiaryTerm.ApprovedByNominee)
			}

			fmt.Printf("PeerID:\t%s\n", mi.PeerId)
			fmt.Printf("Multiaddrs:\t")
			for _, addr := range mi.Multiaddrs {
				a, err := multiaddr.NewMultiaddrBytes(addr)
				if err != nil {
					return xerrors.Errorf("undecodable listen address: %w", err)
				}
				fmt.Printf("%s ", a)
			}
			fmt.Println()
			fmt.Printf("Consensus Fault End:\t%d\n", mi.ConsensusFaultElapsed)

			fmt.Printf("SectorSize:\t%s (%d)\n", types.SizeStr(types.NewInt(uint64(mi.SectorSize))), mi.SectorSize)
			pow, err := api.StateMinerPower(ctx, addr, ts.Key())
			if err != nil {
				return err
			}

			fmt.Printf("Byte Power:   %s / %s (%0.4f%%)\n",
				color.BlueString(types.SizeStr(pow.MinerPower.RawBytePower)),
				types.SizeStr(pow.TotalPower.RawBytePower),
				types.BigDivFloat(
					types.BigMul(pow.MinerPower.RawBytePower, big.NewInt(100)),
					pow.TotalPower.RawBytePower,
				),
			)

			fmt.Printf("Actual Power: %s / %s (%0.4f%%)\n",
				color.GreenString(types.DeciStr(pow.MinerPower.QualityAdjPower)),
				types.DeciStr(pow.TotalPower.QualityAdjPower),
				types.BigDivFloat(
					types.BigMul(pow.MinerPower.QualityAdjPower, big.NewInt(100)),
					pow.TotalPower.QualityAdjPower,
				),
			)

			fmt.Println()

			cd, err := api.StateMinerProvingDeadline(ctx, addr, ts.Key())
			if err != nil {
				return xerrors.Errorf("getting miner info: %w", err)
			}

			fmt.Printf("Proving Period Start:\t%s\n", cliutil.EpochTime(cd.CurrentEpoch, cd.PeriodStart))

			return nil
		},
	}
}
