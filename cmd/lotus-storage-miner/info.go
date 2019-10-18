package main

import (
	"context"
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	sectorstate "github.com/filecoin-project/go-sectorbuilder/sealing_state"
)

var infoCmd = &cli.Command{
	Name:  "info",
	Usage: "Print storage miner info",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("Miner: %s\n", maddr)

		pow, err := api.StateMinerPower(ctx, maddr, nil)
		if err != nil {
			return err
		}

		percI := types.BigDiv(types.BigMul(pow.MinerPower, types.NewInt(1000)), pow.TotalPower)
		fmt.Printf("Power: %s / %s (%0.2f%%)\n", pow.MinerPower, pow.TotalPower, float64(percI.Int64())/1000*100)

		sinfo, err := sectorsInfo(ctx, nodeApi)
		if err != nil {
			return err
		}

		fmt.Println("Sealed Sectors:\t", sinfo.SealedCount)
		fmt.Println("Sealing Sectors:\t", sinfo.SealingCount)
		fmt.Println("Pending Sectors:\t", sinfo.PendingCount)
		fmt.Println("Failed Sectors:\t", sinfo.FailedCount)

		// TODO: grab actr state / info
		//  * Sector size
		//  * Sealed sectors (count / bytes)
		//  * Power
		return nil
	},
}

type SectorsInfo struct {
	TotalCount   int
	SealingCount int
	FailedCount  int
	SealedCount  int
	PendingCount int
}

func sectorsInfo(ctx context.Context, napi api.StorageMiner) (*SectorsInfo, error) {
	sectors, err := napi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	out := SectorsInfo{
		TotalCount: len(sectors),
	}
	for _, s := range sectors {
		st, err := napi.SectorsStatus(ctx, s)
		if err != nil {
			return nil, err
		}

		switch st.State {
		case sectorstate.Sealed:
			out.SealedCount++
		case sectorstate.Pending:
			out.PendingCount++
		case sectorstate.Sealing:
			out.SealingCount++
		case sectorstate.Failed:
			out.FailedCount++
		case sectorstate.Unknown:
		}
	}

	return &out, nil
}
