//go:build !linux

package sealer

func cgroupV1Mem() (memoryMax, memoryUsed, swapMax, swapUsed uint64, err error) {
	return 0, 0, 0, 0, nil
}

func cgroupV2Mem() (memoryMax, memoryUsed, swapMax, swapUsed uint64, err error) {
	return 0, 0, 0, 0, nil
}
