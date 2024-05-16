package ffiselect

import (
	"bytes"
	"context"
	"encoding/gob"
	"os"
	"os/exec"
	"strconv"
	"strings"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/curiosrc/build"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
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
	Err error
}

// This is not the one you're looking for.
type FFICall struct {
	Fn   string
	Args []interface{}
}

func call(layers []string, fn string, args ...interface{}) ([]interface{}, error) {
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
	if len(layers) > 0 {
		commandAry = append(commandAry, "--layers="+strings.Join(layers, ","))
	}
	cmd := exec.Command(p, commandAry...)

	// Set Visible Devices for CUDA and OpenCL
	cmd.Env = append(os.Environ(),
		func(isCuda bool) string {
			if isCuda {
				return "CUDA_VISIBLE_DEVICES=" + dOrdinal
			}
			return "GPU_DEVICE_ORDINAL=" + dOrdinal
		}(IsCuda))

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	outFile, err := os.CreateTemp("", "out")
	if err != nil {
		return nil, err
	}
	cmd.ExtraFiles = []*os.File{outFile}
	var gobber bytes.Buffer
	err = gob.NewEncoder(&gobber).Encode(FFICall{
		Fn:   fn,
		Args: args,
	})
	if err != nil {
		return nil, err
	}
	err = cmd.Run()
	if err != nil {
		return nil, err
	}
	var ve ValErr
	err = gob.NewDecoder(outFile).Decode(&ve)
	if err != nil {
		return nil, err
	}
	return ve.Val, ve.Err
}

// FUTURE?: be snazzy and generate + reflect all FFIWrapper methods.
type CurioFFIWrap struct {
	Layers []string
}

func (fcw *CurioFFIWrap) GenerateSingleVanillaProof(
	replica ffi.PrivateSectorInfo,
	challenges []uint64,
) ([]byte, error) {
	res, err := call(fcw.Layers, "GenerateSingleVanillaProof", replica, challenges)
	if err != nil {
		return nil, err
	}
	return res[0].([]byte), res[1].(error)
}

func (fcw *CurioFFIWrap) GenerateWinningPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, vanillas [][]byte) ([]proof.PoStProof, error) {
	res, err := call(fcw.Layers, "GenerateWinningPoStWithVanilla", proofType, minerID, randomness, vanillas)
	if err != nil {
		return nil, err
	}
	return res[0].([]proof.PoStProof), res[1].(error)
}

func (fcw *CurioFFIWrap) GenerateWindowPoStWithVanilla(ctx context.Context, proofType abi.RegisteredPoStProof, minerID abi.ActorID, randomness abi.PoStRandomness, proofs [][]byte, partitionIdx int) (proof.PoStProof, error) {
	res, err := call(fcw.Layers, "GenerateWindowPoStWithVanilla", proofType, minerID, randomness, proofs, partitionIdx)
	if err != nil {
		return proof.PoStProof{}, err
	}
	return res[0].(proof.PoStProof), res[1].(error)
}

func (fcw *CurioFFIWrap) SealCommit2(ctx context.Context, sector storiface.SectorRef, phase1Out storiface.Commit1Out) (storiface.Proof, error) {
	res, err := call(fcw.Layers, "SealCommit2", sector, phase1Out)
	if err != nil {
		return storiface.Proof{}, err
	}
	return res[0].(storiface.Proof), res[1].(error)
}

func (fcw *CurioFFIWrap) SealPreCommit2(ctx context.Context, sector storiface.SectorRef, phase1Out storiface.PreCommit1Out) (storiface.SectorCids, error) {
	res, err := call(fcw.Layers, "SealPreCommit2", sector, phase1Out)
	if err != nil {
		return storiface.SectorCids{}, err
	}
	return res[0].(storiface.SectorCids), res[1].(error)
}

func (fcw *CurioFFIWrap) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, postProofType abi.RegisteredPoStProof, sectorInfo []proof.ExtendedSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, []abi.SectorID, error) {
	res, err := call(fcw.Layers, "GenerateWindowPoSt", minerID, postProofType, sectorInfo, randomness)
	if err != nil {
		return nil, nil, err
	}
	return res[0].([]proof.PoStProof), res[1].([]abi.SectorID), res[2].(error)
}
