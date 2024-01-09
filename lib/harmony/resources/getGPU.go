//go:build !darwin
// +build !darwin

package resources

import (
	"strings"

	ffi "github.com/filecoin-project/filecoin-ffi"
)

func getGPUDevices() float64 { // GPU boolean
	gpus, err := ffi.GetGPUDevices()
	if err != nil {
		logger.Errorf("getting gpu devices failed: %+v", err)
	}
	all := strings.ToLower(strings.Join(gpus, ","))
	if len(gpus) > 1 || strings.Contains(all, "ati") || strings.Contains(all, "nvidia") {
		return float64(len(gpus))
	}
	return 0
}
