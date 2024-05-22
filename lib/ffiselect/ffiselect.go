package ffiselect

import (
	"bytes"
	"encoding/gob"
	"os"
	"os/exec"
	"reflect"
	"strconv"

	"github.com/ipfs/go-cid"
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
	tmpDir, err := os.MkdirTemp("", "rust-fil-proofs")
	if err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Env, "TMPDIR="+tmpDir)
	defer func() { _ = os.RemoveAll(tmpDir) }()

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
		return nil, xerrors.Errorf("subprocess caller cannot encode: %w", err)
	}
	err = cmd.Run()
	if err != nil {
		return nil, err
	}
	var ve ValErr
	err = gob.NewDecoder(outFile).Decode(&ve)
	if err != nil {
		return nil, xerrors.Errorf("subprocess caller cannot decode: %w", err)
	}
	if ve.Val[len(ve.Val)-1].(error) != nil {
		return nil, ve.Val[len(ve.Val)-1].(error)
	}
	return ve.Val, nil
}

///////////Funcs reachable by the GPU selector.///////////
// NOTE: Changes here MUST also change ffi-direct.go

type FFISelect struct{}

func (FFISelect) GenerateSinglePartitionWindowPoStWithVanilla(
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	proofs [][]byte,
	partitionIndex uint,
) (*ffi.PartitionProof, error) {
	val, err := call("GenerateSinglePartitionWindowPoStWithVanilla", proofType, minerID, randomness, proofs, partitionIndex)
	if err != nil {
		return nil, err
	}
	return val[0].(*ffi.PartitionProof), nil
}
func (FFISelect) SealPreCommitPhase2(
	phase1Output []byte,
	cacheDirPath string,
	sealedSectorPath string,
) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	val, err := call("SealPreCommitPhase2", phase1Output, cacheDirPath, sealedSectorPath)
	if err != nil {
		return cid.Undef, cid.Undef, err
	}
	return val[0].(cid.Cid), val[1].(cid.Cid), nil
}

func (FFISelect) SealCommitPhase2(
	phase1Output []byte,
	sectorNum abi.SectorNumber,
	minerID abi.ActorID,
) ([]byte, error) {
	val, err := call("SealCommitPhase2", phase1Output, sectorNum, minerID)
	if err != nil {
		return nil, err
	}

	return val[0].([]byte), nil
}

func (FFISelect) GenerateWinningPoStWithVanilla(
	proofType abi.RegisteredPoStProof,
	minerID abi.ActorID,
	randomness abi.PoStRandomness,
	proofs [][]byte,
) ([]proof.PoStProof, error) {
	val, err := call("GenerateWinningPoStWithVanilla", proofType, minerID, randomness, proofs)
	if err != nil {
		return nil, err
	}
	return val[0].([]proof.PoStProof), nil
}

func (FFISelect) SelfTest(val1 int, val2 cid.Cid) (int, cid.Cid, error) {
	val, err := call("SelfTest", val1, val2)
	if err != nil {
		return 0, cid.Undef, err
	}
	return val[0].(int), val[1].(cid.Cid), nil
}

// //////////////////////////

func init() {
	registeredTypes := []any{
		ValErr{},
		FFICall{},
		cid.Cid{},
		abi.RegisteredPoStProof(0),
		abi.ActorID(0),
		abi.PoStRandomness{},
		abi.SectorNumber(0),
		ffi.PartitionProof{},
		proof.PoStProof{},
		abi.RegisteredPoStProof(0),
	}
	var registeredTypeNames = make(map[string]struct{})

	//Ensure all methods are implemented:
	// This is designed to fail for happy-path runs
	//  and should never actually impact curio users.
	for _, t := range registeredTypes {
		gob.Register(t)
		registeredTypeNames[reflect.TypeOf(t).PkgPath()+"."+reflect.TypeOf(t).Name()] = struct{}{}
	}

	from := reflect.TypeOf(FFISelect{})
	to := reflect.TypeOf(ffidirect.FFI{})
	for m := 0; m < from.NumMethod(); m++ {
		fm := from.Method(m)
		tm, ok := to.MethodByName(fm.Name)
		if !ok {
			panic("ffiSelect: method not found: " + fm.Name)
		}
		ff, tf := fm.Func, tm.Func
		//Ensure they have the same types of input and output args:
		if ff.Type().NumIn() != tf.Type().NumIn() || ff.Type().NumOut() != tf.Type().NumOut() {
			panic("ffiSelect: mismatched number of args or return values: " + fm.Name)
		}
		for i := 1; i < ff.Type().NumIn(); i++ { // skipping first arg (struct type)
			in := ff.Type().In(i)
			nm := in.PkgPath() + "." + in.Name()
			if in != tf.Type().In(i) {
				panic("ffiSelect: mismatched arg types: " + nm + " Number: " + strconv.Itoa(i) + " " + in.String() + " != " + tf.Type().In(i).String())
			}
			if _, ok := registeredTypeNames[nm]; in.PkgPath() != "" && !ok { // built-ins ok
				panic("ffiSelect: unregistered type: " + nm + " from " + fm.Name + " arg: " + strconv.Itoa(i))
			}
		}
		for i := 0; i < ff.Type().NumOut(); i++ {
			out := ff.Type().Out(i)
			nm := out.PkgPath() + "." + out.Name()
			if out != tf.Type().Out(i) {
				panic("ffiSelect: mismatched return types: " + nm + " Number: " + strconv.Itoa(i) + " " + out.String() + " != " + tf.Type().Out(i).String())
			}
			if _, ok := registeredTypeNames[nm]; out.PkgPath() != "" && !ok { // built-ins ok
				panic("ffiSelect: unregistered type: " + nm + " from " + fm.Name + " arg: " + strconv.Itoa(i))
			}
		}
	}
}
