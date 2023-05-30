package storiface

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	stabi "github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

func TestListResourceVars(t *testing.T) {
	seen := map[string]struct{}{}
	_, err := ParseResourceEnv(func(key, def string) (string, bool) {
		_, s := seen[key]
		if !s && def != "" {
			fmt.Printf("%s=%s\n", key, def)
			seen[key] = struct{}{}
		}

		return "", false
	})

	require.NoError(t, err)
}

func TestListResourceOverride(t *testing.T) {
	rt, err := ParseResourceEnv(func(key, def string) (string, bool) {
		if key == "UNS_2K_MAX_PARALLELISM" {
			return "2", true
		}
		if key == "PC2_2K_GPU_UTILIZATION" {
			return "0.4", true
		}
		if key == "PC2_2K_MAX_MEMORY" {
			return "2222", true
		}

		return "", false
	})

	require.NoError(t, err)
	require.Equal(t, 2, rt[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
	require.Equal(t, 0.4, rt[sealtasks.TTPreCommit2][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].GPUUtilization)
	require.Equal(t, uint64(2222), rt[sealtasks.TTPreCommit2][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxMemory)

	// check that defaults don't get mutated
	require.Equal(t, 1, ResourceTable[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
}

func TestListResourceSDRMulticoreOverride(t *testing.T) {
	rt, err := ParseResourceEnv(func(key, def string) (string, bool) {
		if key == "FIL_PROOFS_USE_MULTICORE_SDR" {
			return "1", true
		}

		return "", false
	})

	require.NoError(t, err)
	require.Equal(t, 4, rt[sealtasks.TTPreCommit1][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
	require.Equal(t, 4, rt[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)

	rt, err = ParseResourceEnv(func(key, def string) (string, bool) {
		if key == "FIL_PROOFS_USE_MULTICORE_SDR" {
			return "1", true
		}
		if key == "FIL_PROOFS_MULTICORE_SDR_PRODUCERS" {
			return "9000", true
		}

		return "", false
	})

	require.NoError(t, err)
	require.Equal(t, 9001, rt[sealtasks.TTPreCommit1][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
	require.Equal(t, 9001, rt[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
}

func TestUnsizedSetAll(t *testing.T) {
	rt, err := ParseResourceEnv(func(key, def string) (string, bool) {
		if key == "UNS_MAX_PARALLELISM" {
			return "2", true
		}

		return "", false
	})

	require.NoError(t, err)
	require.Equal(t, 2, rt[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
	require.Equal(t, 2, rt[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg32GiBV1].MaxParallelism)
	require.Equal(t, 2, rt[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg8MiBV1].MaxParallelism)

	// check that defaults don't get mutated
	require.Equal(t, 1, ResourceTable[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
}

func TestUnsizedNotPreferred(t *testing.T) {
	rt, err := ParseResourceEnv(func(key, def string) (string, bool) {
		if key == "DC_MAX_PARALLELISM" {
			return "2", true
		}

		// test should also print a warning for DataCid as it's not sector-size dependent
		if key == "DC_64G_MAX_PARALLELISM" {
			return "1", true
		}

		return "", false
	})

	require.NoError(t, err)
	require.Equal(t, 2, rt[sealtasks.TTDataCid][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
	require.Equal(t, 2, rt[sealtasks.TTDataCid][stabi.RegisteredSealProof_StackedDrg32GiBV1].MaxParallelism)
	require.Equal(t, 1, rt[sealtasks.TTDataCid][stabi.RegisteredSealProof_StackedDrg64GiBV1_1].MaxParallelism)

	// check that defaults don't get mutated
	require.Equal(t, 1, ResourceTable[sealtasks.TTUnseal][stabi.RegisteredSealProof_StackedDrg2KiBV1_1].MaxParallelism)
}
