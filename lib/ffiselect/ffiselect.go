package ffiselect

import (
	"bytes"
	"encoding/gob"
	"os"
	"os/exec"
	"strconv"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/curiosrc/build"
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

func call(fn string, args ...interface{}) ([]interface{}, error) {
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
}

func (CurioFFIWrap) GenerateWindowPoSt(
	minerID abi.ActorID,
	privateSectorInfo ffi.SortedPrivateSectorInfo,
	randomness abi.PoStRandomness,
) ([]proof.PoStProof, []abi.SectorNumber, error) {
	val, err := call("GenerateWindowPoSt", minerID, privateSectorInfo, randomness)
	if err != nil {
		return nil, nil, err
	}
	return val[0].([]proof.PoStProof), val[1].([]abi.SectorNumber), val[2].(error)
}
func (CurioFFIWrap) GenerateWinningPoSt(minerID abi.ActorID, privateSectorInfo ffi.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	val, err := call("GenerateWinningPoSt", minerID, privateSectorInfo, randomness)
	if err != nil {
		return nil, err
	}
	return val[0].([]proof.PoStProof), val[1].(error)
}

func (CurioFFIWrap) SealPreCommitPhase2(phase1Output []byte, cacheDirPath string, sealedSectorPath string) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	val, err := call("SealPreCommitPhase2", phase1Output, cacheDirPath, sealedSectorPath)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}
	return val[0].(cid.Cid), val[1].(cid.Cid), val[2].(error)
}

func (CurioFFIWrap) SealCommitPhase2(phase1Output []byte, sectorNum abi.SectorNumber, minerID abi.ActorID) ([]byte, error) {
	val, err := call("SealCommitPhase2", phase1Output, sectorNum, minerID)
	if err != nil {
		return nil, err
	}
	return val[0].([]byte), val[1].(error)
}
