package miner

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
)

// fakePartition holds sectors 1-40: 3-4 faulty (4 recovering), 5-6 unproven
// (6 also faulty) and 7 terminated.
type fakePartition struct{}

func sectorSet(sns ...uint64) (bitfield.BitField, error) { return bitfield.NewFromSet(sns), nil }

func (fakePartition) AllSectors() (bitfield.BitField, error) {
	all := make([]uint64, 40)
	for i := range all {
		all[i] = uint64(i + 1)
	}
	return sectorSet(all...)
}
func (fakePartition) FaultySectors() (bitfield.BitField, error)     { return sectorSet(3, 4, 6) }
func (fakePartition) RecoveringSectors() (bitfield.BitField, error) { return sectorSet(4) }
func (p fakePartition) LiveSectors() (bitfield.BitField, error) {
	all, _ := p.AllSectors()
	return bitfield.SubtractBitField(all, bitfield.NewFromSet([]uint64{7}))
}
func (fakePartition) ActiveSectors() (bitfield.BitField, error)   { panic("unused") }
func (fakePartition) UnprovenSectors() (bitfield.BitField, error) { return sectorSet(5, 6) }

func TestUpgradeSkipsSummary(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requested    []abi.SectorNumber
		stoppedEarly bool
		verbose      bool
		expect       string
	}{
		{
			name: "every sector in scope",
			expect: "skipped 12 sectors (already at full QA power): 10, 11, 12, 13, 14, 15, 16, 17, 18, 19 and 2 more\n" +
				"skipped 1 sector (expired): 8\n" +
				"skipped 3 sectors (faulty or recovering, not active): 3, 4, 6\n" +
				"skipped 1 sector (unproven, not active until its first Window PoSt): 5\n",
		},
		{
			name:    "verbose lists every sector",
			verbose: true,
			expect: "skipped 12 sectors (already at full QA power): 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21\n" +
				"skipped 1 sector (expired): 8\n" +
				"skipped 3 sectors (faulty or recovering, not active): 3, 4, 6\n" +
				"skipped 1 sector (unproven, not active until its first Window PoSt): 5\n",
		},
		{
			name:      "requested sectors",
			requested: []abi.SectorNumber{1, 4, 5, 7, 10, 99},
			expect: "--sectors: 1 of 6 requested sectors can be upgraded\n" +
				"skipped 1 sector (already at full QA power): 10\n" +
				"skipped 1 sector (faulty or recovering, not active): 4\n" +
				"skipped 1 sector (unproven, not active until its first Window PoSt): 5\n" +
				"skipped 1 sector (terminated): 7\n" +
				"skipped 1 sector (requested, not found on this miner): 99\n",
		},
		{
			name:         "requested sectors past --max-sectors",
			requested:    []abi.SectorNumber{1, 99},
			stoppedEarly: true,
			expect: "--sectors: 1 of 2 requested sectors can be upgraded\n" +
				"skipped 1 sector (requested, not found before the --max-sectors limit): 99\n",
		},
		{
			name:      "no requested sector eligible",
			requested: []abi.SectorNumber{3, 21},
			expect: "--sectors: 0 of 2 requested sectors can be upgraded\n" +
				"skipped 1 sector (already at full QA power): 21\n" +
				"skipped 1 sector (faulty or recovering, not active): 3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var skips upgradeSkips
			if tc.requested != nil {
				skips.requested = make(map[abi.SectorNumber]bool)
				for _, sn := range tc.requested {
					skips.requested[sn] = false
				}
			}
			require.NoError(t, skips.recordInactive(fakePartition{}))
			// Active sectors as the command's traversal classifies them: 1-2, 9 and
			// 22-40 upgradable, 8 expired, 10-21 at full QA power.
			for sn := abi.SectorNumber(1); sn <= 40; sn++ {
				if !skips.selected(sn) || (sn >= 3 && sn <= 7) {
					continue
				}
				switch {
				case sn == 8:
					skips.skip(sn, skipExpired)
				case sn >= 10 && sn <= 21:
					skips.skip(sn, skipFullQaPower)
				default:
					skips.include(sn)
				}
			}
			skips.finish(tc.stoppedEarly)

			var out bytes.Buffer
			skips.write(&out, tc.verbose)
			require.Equal(t, tc.expect, out.String())
		})
	}
}
