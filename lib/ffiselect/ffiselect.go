package ffiselect

import (
	"bytes"
	"encoding/gob"
	"os"
	"os/exec"
	"strconv"

	ffi "github.com/filecoin-project/filecoin-ffi"
)

// TODO set this appropriately based on the compile-time environment variables
// THEN  build-out the FFI caller.
var isCuda = false

// Get all devices from ffi
var devices = []string{}
var ch chan int

func init() {
	var err error
	devices, err = ffi.GetGPUDevices()
	if err != nil {
		panic(err)
	}
	ch = make(chan int, len(devices))
	for i := 0; i < len(devices); i++ {
		ch <- i
	}
}

type ValErr struct {
	Val interface{}
	Err error
}

func Call(fn string, args ...interface{}) (interface{}, error) {
	// get dOrdinal
	dOrdinal := <-ch
	defer func() {
		ch <- dOrdinal
	}()

	p, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(p, "ffi", "fn")

	// Set Visible Devices for CUDA and OpenCL
	cmd.Env = append(os.Environ(),
		func(isCuda bool) string {
			if isCuda {
				return "CUDA_VISIBLE_DEVICES=" + devices[dOrdinal]
			}
			return "GPU_DEVICE_ORDINAL=" + strconv.Itoa(dOrdinal)
		}(isCuda))

	cmd.Stderr = os.Stderr
	var gobber bytes.Buffer
	err = gob.NewEncoder(&gobber).Encode(args)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = &gobber
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return nil, err
	}
	var ve ValErr
	err = gob.NewDecoder(&out).Decode(&ve)
	if err != nil {
		return nil, err
	}
	return ve.Val, ve.Err
}
