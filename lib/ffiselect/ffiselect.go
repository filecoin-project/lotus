package ffiselect

import (
	"bytes"
	"encoding/gob"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/curiosrc/build"
	"github.com/filecoin-project/lotus/lib/ffiselect/ffidirect"
)

var IsCuda = build.IsOpencl != "1"

// Get all devices from ffi
var ch chan string

func init() {
	devices, err := ffi.GetGPUDevices()
	if err != nil {
		panic(err)
	}
	ch = make(chan string, len(devices))
	for i := 0; i < len(devices); i++ {
		ch <- strconv.Itoa(i)
	}
}

type ValErr struct {
	Val []interface{}
	Err string
}

// This is not the one you're looking for.
type FFICall struct {
	Fn   string
	Args []interface{}
}

func subStrInSet(set []string, sub string) bool {
	return lo.Reduce(set, func(agg bool, item string, _ int) bool { return agg || strings.Contains(item, "RUST_LOG") }, false)
}

func call(logctx []any, fn string, args ...interface{}) ([]interface{}, error) {
	// get dOrdinal
	dOrdinal := <-ch
	defer func() {
		ch <- dOrdinal
	}()

	p, err := os.Executable()
	if err != nil {
		return nil, err
	}

	commandAry := []string{"ffi"}
	cmd := exec.Command(p, commandAry...)

	// Set Visible Devices for CUDA and OpenCL
	cmd.Env = append(os.Environ(),
		func(isCuda bool) string {
			if isCuda {
				return "CUDA_VISIBLE_DEVICES=" + dOrdinal
			}
			return "GPU_DEVICE_ORDINAL=" + dOrdinal
		}(IsCuda))
	tmpDir, err := os.MkdirTemp("", "rust-fil-proofs")
	if err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Env, "TMPDIR="+tmpDir)

	if !subStrInSet(cmd.Env, "RUST_LOG") {
		cmd.Env = append(cmd.Env, "RUST_LOG=debug")
	}
	if !subStrInSet(cmd.Env, "FIL_PROOFS_USE_GPU_COLUMN_BUILDER") {
		cmd.Env = append(cmd.Env, "FIL_PROOFS_USE_GPU_COLUMN_BUILDER=1")
	}
	if !subStrInSet(cmd.Env, "FIL_PROOFS_USE_GPU_TREE_BUILDER") {
		cmd.Env = append(cmd.Env, "FIL_PROOFS_USE_GPU_TREE_BUILDER=1")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	lw := NewLogWriter(logctx, os.Stderr)

	cmd.Stderr = lw
	cmd.Stdout = os.Stdout
	outFile, err := os.CreateTemp("", "out")
	if err != nil {
		return nil, err
	}
	cmd.ExtraFiles = []*os.File{outFile}
	var encArgs bytes.Buffer
	err = gob.NewEncoder(&encArgs).Encode(FFICall{
		Fn:   fn,
		Args: args,
	})
	if err != nil {
		return nil, xerrors.Errorf("subprocess caller cannot encode: %w", err)
	}

	cmd.Stdin = &encArgs
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	// seek to start
	outFile.Seek(0, io.SeekStart)

	var ve ValErr
	err = gob.NewDecoder(outFile).Decode(&ve)
	if err != nil {
		return nil, xerrors.Errorf("subprocess caller cannot decode: %w", err)
	}
	if ve.Err != "" {
		return nil, xerrors.Errorf("subprocess failure: %s", ve.Err)
	}
	if ve.Val[len(ve.Val)-1].(ffidirect.ErrorString) != "" {
		return nil, xerrors.Errorf("subprocess call error: %s", ve.Val[len(ve.Val)-1].(ffidirect.ErrorString))
	}
	return ve.Val, nil
}

///////////Funcs reachable by the GPU selector.///////////
// NOTE: Changes here MUST also change ffi-direct.go

func GenerateSinglePartitionWindowPoStWithVanilla(
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	proofs [][]byte,
	partitionIndex uint,
) (*ffi.PartitionProof, error) {
	logctx := []any{"spid", minerID, "proof_count", len(proofs), "partition_index", partitionIndex}

	val, err := call(logctx, "GenerateSinglePartitionWindowPoStWithVanilla", proofType, minerID, randomness, proofs, partitionIndex)
	if err != nil {
		return nil, err
	}
	return val[0].(*ffi.PartitionProof), nil
}
func SealPreCommitPhase2(
	sid abi.SectorID,
	phase1Output []byte,
	cacheDirPath string,
	sealedSectorPath string,
) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	logctx := []any{"sector", sid}

	val, err := call(logctx, "SealPreCommitPhase2", phase1Output, cacheDirPath, sealedSectorPath)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}
	return val[0].(cid.Cid), val[1].(cid.Cid), nil
}

func SealCommitPhase2(
	phase1Output []byte,
	sectorNum abi.SectorNumber,
	minerID abi.ActorID,
) ([]byte, error) {
	logctx := []any{"sector", abi.SectorID{Miner: minerID, Number: sectorNum}}

	val, err := call(logctx, "SealCommitPhase2", phase1Output, sectorNum, minerID)
	if err != nil {
		return nil, err
	}

	return val[0].([]byte), nil
}

func GenerateWinningPoStWithVanilla(
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	proofs [][]byte,
) ([]proof.PoStProof, error) {
	logctx := []any{"proof_type", proofType, "miner_id", minerID}

	val, err := call(logctx, "GenerateWinningPoStWithVanilla", proofType, minerID, randomness, proofs)
	if err != nil {
		return nil, err
	}
	return val[0].([]proof.PoStProof), nil
}

func SelfTest(val1 int, val2 cid.Cid) (int, cid.Cid, error) {
	val, err := call([]any{"selftest", "true"}, "SelfTest", val1, val2)
	if err != nil {
		return 0, cid.Undef, err
	}
	return val[0].(int), val[1].(cid.Cid), nil
}

// //////////////////////////

func init() {
	gob.Register(ValErr{})
	gob.Register(FFICall{})
	gob.Register(cid.Cid{})
	gob.Register(abi.RegisteredPoStProof(0))
	gob.Register(abi.ActorID(0))
	gob.Register(abi.PoStRandomness{})
	gob.Register(abi.SectorNumber(0))
	gob.Register(ffi.PartitionProof{})
	gob.Register(proof.PoStProof{})
	gob.Register(abi.RegisteredPoStProof(0))
}
