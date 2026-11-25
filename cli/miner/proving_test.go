package miner

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
	mineractor "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestProvingCheckOrdersSectors(t *testing.T) {
	wasNoColor := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = wasNoColor })

	const deadline = 3
	minerAddr, err := address.NewIDAddress(1000)
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		args []string
		rows string
	}{
		{
			name: "all sectors",
			args: []string{"lotus-miner", "check", "3"},
			rows: "3 0 2 good 3 1 3 bad (sealed file missing) 3 1 10 good 3 0 100 bad (cache missing)",
		},
		{
			name: "only bad",
			args: []string{"lotus-miner", "check", "--only-bad", "3"},
			rows: "3 1 3 bad (sealed file missing) 3 0 100 bad (cache missing)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fullNode := mocks.NewMockFullNode(gomock.NewController(t))
			fullNode.EXPECT().StateMinerInfo(gomock.Any(), minerAddr, types.EmptyTSK).Return(api.MinerInfo{}, nil)
			parts := []api.Partition{
				{LiveSectors: bitfield.NewFromSet([]uint64{2, 100})},
				{LiveSectors: bitfield.NewFromSet([]uint64{3, 10})},
			}
			fullNode.EXPECT().StateMinerPartitions(gomock.Any(), minerAddr, uint64(deadline), types.EmptyTSK).Return(parts, nil)
			fullNode.EXPECT().StateMinerSectors(gomock.Any(), minerAddr, &parts[0].LiveSectors, types.EmptyTSK).
				Return([]*mineractor.SectorOnChainInfo{{SectorNumber: 100}, {SectorNumber: 2}}, nil)
			fullNode.EXPECT().StateMinerSectors(gomock.Any(), minerAddr, &parts[1].LiveSectors, types.EmptyTSK).
				Return([]*mineractor.SectorOnChainInfo{{SectorNumber: 10}, {SectorNumber: 3}}, nil)

			var checked []abi.SectorNumber
			storageMiner := &api.StorageMinerStruct{Internal: api.StorageMinerMethods{
				ActorAddress: func(context.Context) (address.Address, error) { return minerAddr, nil },
				CheckProvable: func(_ context.Context, _ abi.RegisteredPoStProof, sectors []storiface.SectorRef) (map[abi.SectorNumber]string, error) {
					bad := make(map[abi.SectorNumber]string)
					for _, sector := range sectors {
						checked = append(checked, sector.ID.Number)
						switch sector.ID.Number {
						case 3:
							bad[3] = "sealed file missing"
						case 100:
							bad[100] = "cache missing"
						}
					}
					return bad, nil
				},
			}}

			var output bytes.Buffer
			app := cli.NewApp()
			app.Writer = &output
			app.Commands = []*cli.Command{provingCheckProvableCmd}
			app.Metadata = map[string]interface{}{
				"testnode-full":    fullNode,
				"testnode-storage": storageMiner,
			}

			require.NoError(t, app.Run(tc.args))
			require.ElementsMatch(t, []abi.SectorNumber{2, 3, 10, 100}, checked)
			require.Equal(t, "deadline partition sector status "+tc.rows, strings.Join(strings.Fields(output.String()), " "))
		})
	}
}
