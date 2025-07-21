package storiface

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

var log = logging.Logger("resources")

type Resources struct {
	MinMemory uint64 `envname:"MIN_MEMORY"` // What Must be in RAM for decent perf
	MaxMemory uint64 `envname:"MAX_MEMORY"` // Memory required (swap + ram; peak memory usage during task execution)

	// GPUUtilization specifies the number of GPUs a task can use
	GPUUtilization float64 `envname:"GPU_UTILIZATION"`

	// MaxParallelism specifies the number of CPU cores when GPU is NOT in use
	MaxParallelism int `envname:"MAX_PARALLELISM"` // -1 = multithread

	// MaxParallelismGPU specifies the number of CPU cores when GPU is in use
	MaxParallelismGPU int `envname:"MAX_PARALLELISM_GPU"` // when 0, inherits MaxParallelism

	BaseMinMemory uint64 `envname:"BASE_MIN_MEMORY"` // What Must be in RAM for decent perf (shared between threads)

	MaxConcurrent int `envname:"MAX_CONCURRENT"` // Maximum number of tasks of this type that can be scheduled on a worker (0=default, no limit)
}

/*
Percent of threads to allocate to parallel tasks

12  * 0.92 = 11
16  * 0.92 = 14
24  * 0.92 = 22
32  * 0.92 = 29
64  * 0.92 = 58
128 * 0.92 = 117
*/
var ParallelNum uint64 = 92
var ParallelDenom uint64 = 100

func (r Resources) Threads(wcpus uint64, gpus int) uint64 {
	// TODO: Take NUMA into account

	mp := r.MaxParallelism

	if r.GPUUtilization > 0 && gpus > 0 && r.MaxParallelismGPU != 0 { // task can use GPUs and worker has some
		mp = r.MaxParallelismGPU
	}

	if mp == -1 {
		n := (wcpus * ParallelNum) / ParallelDenom
		if n == 0 {
			return wcpus
		}
		return n
	}

	return uint64(mp)
}

var ResourceTable = map[sealtasks.TaskType]map[abi.RegisteredSealProof]Resources{
	sealtasks.TTAddPiece: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 8 << 30,
			MinMemory: 8 << 30,

			MaxParallelism: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 4 << 30,
			MinMemory: 4 << 30,

			MaxParallelism: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTPreCommit1: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 128 << 30,
			MinMemory: 112 << 30,

			MaxParallelism: 1,

			BaseMinMemory: 10 << 20,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 64 << 30,
			MinMemory: 56 << 30,

			MaxParallelism: 1,

			BaseMinMemory: 10 << 20,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 768 << 20,

			MaxParallelism: 1,

			BaseMinMemory: 1 << 20,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTPreCommit2: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 30 << 30,
			MinMemory: 30 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 15 << 30,
			MinMemory: 15 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MaxParallelism: -1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: -1,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: -1,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTCommit1: { // Very short (~100ms), so params are very light
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTCommit2: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 190 << 30, // TODO: Confirm
			MinMemory: 60 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 64 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 150 << 30, // TODO: ~30G of this should really be BaseMaxMemory
			MinMemory: 30 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 32 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MaxParallelism: 1, // This is fine
			GPUUtilization: 1.0,

			BaseMinMemory: 10 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTFetch: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			MaxParallelism: 0,
			GPUUtilization: 0,

			BaseMinMemory: 0,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			MaxParallelism: 0,
			GPUUtilization: 0,

			BaseMinMemory: 0,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			MaxParallelism: 0,
			GPUUtilization: 0,

			BaseMinMemory: 0,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			MaxParallelism: 0,
			GPUUtilization: 0,

			BaseMinMemory: 0,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			MaxParallelism: 0,
			GPUUtilization: 0,

			BaseMinMemory: 0,
		},
	},
	// TODO: this should ideally be the actual replica update proof types
	// TODO: actually measure this (and all the other replica update work)
	sealtasks.TTReplicaUpdate: { // copied from addpiece
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 8 << 30,
			MinMemory: 8 << 30,

			MaxParallelism:    1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 4 << 30,
			MinMemory: 4 << 30,

			MaxParallelism:    1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTProveReplicaUpdate1: { // copied from commit1
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTProveReplicaUpdate2: { // copied from commit2
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 190 << 30, // TODO: Confirm
			MinMemory: 60 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 64 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 150 << 30, // TODO: ~30G of this should really be BaseMaxMemory
			MinMemory: 30 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 32 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MaxParallelism: 1, // This is fine
			GPUUtilization: 1.0,

			BaseMinMemory: 10 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTGenerateWindowPoSt: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 120 << 30, // TODO: Confirm
			MinMemory: 60 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 64 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 96 << 30,
			MinMemory: 30 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 32 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MaxParallelism: 1, // This is fine
			GPUUtilization: 1.0,

			BaseMinMemory: 10 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTGenerateWinningPoSt: {
		abi.RegisteredSealProof_StackedDrg64GiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 64 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg32GiBV1: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MaxParallelism:    -1,
			MaxParallelismGPU: 6,
			GPUUtilization:    1.0,

			BaseMinMemory: 32 << 30, // params
		},
		abi.RegisteredSealProof_StackedDrg512MiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1, // This is fine
			GPUUtilization: 1.0,

			BaseMinMemory: 10 << 30,
		},
		abi.RegisteredSealProof_StackedDrg2KiBV1: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredSealProof_StackedDrg8MiBV1: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			MaxParallelism: 1,
			GPUUtilization: 1.0,

			BaseMinMemory: 8 << 20,
		},
	},
}

func init() {
	ResourceTable[sealtasks.TTUnseal] = ResourceTable[sealtasks.TTPreCommit1] // TODO: measure accurately
	ResourceTable[sealtasks.TTRegenSectorKey] = ResourceTable[sealtasks.TTReplicaUpdate]

	// DataCid doesn't care about sector proof type; Use 32G AddPiece resource definition
	ResourceTable[sealtasks.TTDataCid] = map[abi.RegisteredSealProof]Resources{}
	for proof := range ResourceTable[sealtasks.TTAddPiece] {
		ResourceTable[sealtasks.TTDataCid][proof] = ResourceTable[sealtasks.TTAddPiece][abi.RegisteredSealProof_StackedDrg32GiBV1]
	}

	// V1_1 and SynthethicpoRep is the same as V1
	for _, m := range ResourceTable {
		m[abi.RegisteredSealProof_StackedDrg2KiBV1_1] = m[abi.RegisteredSealProof_StackedDrg2KiBV1]
		m[abi.RegisteredSealProof_StackedDrg2KiBV1_1_Feat_SyntheticPoRep] = m[abi.RegisteredSealProof_StackedDrg2KiBV1]
		m[abi.RegisteredSealProof_StackedDrg8MiBV1_1] = m[abi.RegisteredSealProof_StackedDrg8MiBV1]
		m[abi.RegisteredSealProof_StackedDrg8MiBV1_1_Feat_SyntheticPoRep] = m[abi.RegisteredSealProof_StackedDrg8MiBV1]
		m[abi.RegisteredSealProof_StackedDrg512MiBV1_1] = m[abi.RegisteredSealProof_StackedDrg512MiBV1]
		m[abi.RegisteredSealProof_StackedDrg512MiBV1_1_Feat_SyntheticPoRep] = m[abi.RegisteredSealProof_StackedDrg512MiBV1]
		m[abi.RegisteredSealProof_StackedDrg32GiBV1_1] = m[abi.RegisteredSealProof_StackedDrg32GiBV1]
		m[abi.RegisteredSealProof_StackedDrg32GiBV1_1_Feat_SyntheticPoRep] = m[abi.RegisteredSealProof_StackedDrg32GiBV1]
		m[abi.RegisteredSealProof_StackedDrg64GiBV1_1] = m[abi.RegisteredSealProof_StackedDrg64GiBV1]
		m[abi.RegisteredSealProof_StackedDrg64GiBV1_1_Feat_SyntheticPoRep] = m[abi.RegisteredSealProof_StackedDrg64GiBV1]
	}
}

func ParseResourceEnv(lookup func(key, def string) (string, bool)) (map[sealtasks.TaskType]map[abi.RegisteredSealProof]Resources, error) {
	out := map[sealtasks.TaskType]map[abi.RegisteredSealProof]Resources{}

	for taskType, defTT := range ResourceTable {
		out[taskType] = map[abi.RegisteredSealProof]Resources{}

		for spt, defRes := range defTT {
			r := defRes // copy

			spsz, err := spt.SectorSize()
			if err != nil {
				return nil, xerrors.Errorf("getting sector size: %w", err)
			}
			shortSize := strings.TrimSuffix(spsz.ShortString(), "iB")

			rr := reflect.ValueOf(&r)
			for i := 0; i < rr.Elem().Type().NumField(); i++ {
				f := rr.Elem().Type().Field(i)

				envname := f.Tag.Get("envname")
				if envname == "" {
					return nil, xerrors.Errorf("no envname for field '%s'", f.Name)
				}

				envval, found := lookup(taskType.Short()+"_"+shortSize+"_"+envname, fmt.Sprint(rr.Elem().Field(i).Interface()))
				if !found {
					// see if a non-size-specific envvar is set
					envval, found = lookup(taskType.Short()+"_"+envname, fmt.Sprint(rr.Elem().Field(i).Interface()))
					if !found {
						// special multicore SDR handling
						if (taskType == sealtasks.TTPreCommit1 || taskType == sealtasks.TTUnseal) && envname == "MAX_PARALLELISM" {
							v, ok := rr.Elem().Field(i).Addr().Interface().(*int)
							if !ok {
								// can't happen, but let's not panic
								return nil, xerrors.Errorf("res.MAX_PARALLELISM is not int (!?): %w", err)
							}
							*v, err = getSDRThreads(lookup)
							if err != nil {
								return nil, err
							}
						}

						continue
					}

				} else {
					if !taskType.SectorSized() {
						log.Errorw("sector-size independent task resource var specified with sector-sized envvar", "env", taskType.Short()+"_"+shortSize+"_"+envname, "use", taskType.Short()+"_"+envname)
					}
				}

				v := rr.Elem().Field(i).Addr().Interface()
				switch fv := v.(type) {
				case *uint64:
					*fv, err = strconv.ParseUint(envval, 10, 64)
				case *int:
					*fv, err = strconv.Atoi(envval)
				case *float64:
					*fv, err = strconv.ParseFloat(envval, 64)
				default:
					return nil, xerrors.Errorf("unknown resource field type")
				}
			}

			out[taskType][spt] = r
		}
	}

	return out, nil
}

func getSDRThreads(lookup func(key, def string) (string, bool)) (_ int, err error) {
	producers := 0

	if v, _ := lookup("FIL_PROOFS_USE_MULTICORE_SDR", ""); v == "1" {
		producers = 3

		if penv, found := lookup("FIL_PROOFS_MULTICORE_SDR_PRODUCERS", ""); found {
			producers, err = strconv.Atoi(penv)
			if err != nil {
				return 0, xerrors.Errorf("parsing (atoi) FIL_PROOFS_MULTICORE_SDR_PRODUCERS: %w", err)
			}
		}
	}

	// producers + the one core actually doing the work
	return producers + 1, nil
}
