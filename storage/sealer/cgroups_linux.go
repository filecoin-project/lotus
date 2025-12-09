//go:build linux

package sealer

import (
	"bufio"
	"bytes"
	"math"
	"os"
	"path/filepath"

	"github.com/containerd/cgroups"
	cgroupv2 "github.com/containerd/cgroups/v2"
)

func cgroupV2MountPoint() (string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := bytes.Fields(scanner.Bytes())
		if len(fields) >= 9 && bytes.Equal(fields[8], []byte("cgroup2")) {
			return string(fields[4]), nil
		}
	}
	return "", cgroups.ErrMountPointNotExist
}

func cgroupV1Mem() (memoryMax, memoryUsed, swapMax, swapUsed uint64, err error) {
	path := cgroups.NestedPath("")
	if pid := os.Getpid(); pid == 1 {
		path = cgroups.RootPath
	}
	c, err := cgroups.Load(cgroups.SingleSubsystem(cgroups.V1, cgroups.Memory), path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	stats, err := c.Stat(cgroups.IgnoreNotExist)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if stats.Memory == nil {
		return 0, 0, 0, 0, nil
	}
	if stats.Memory.Usage != nil {
		memoryMax = stats.Memory.Usage.Limit
		// Exclude cached files
		memoryUsed = stats.Memory.Usage.Usage - stats.Memory.InactiveFile - stats.Memory.ActiveFile
	}
	if stats.Memory.Swap != nil {
		swapMax = stats.Memory.Swap.Limit
		swapUsed = stats.Memory.Swap.Usage
	}
	return memoryMax, memoryUsed, swapMax, swapUsed, nil
}

func cgroupV2MemFromPath(mp, path string) (memoryMax, memoryUsed, swapMax, swapUsed uint64, err error) {
	c, err := cgroupv2.LoadManager(mp, path)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	stats, err := c.Stat()
	if err != nil {
		return 0, 0, 0, 0, err
	}

	if stats.Memory != nil {
		memoryMax = stats.Memory.UsageLimit
		// Exclude memory used caching files
		memoryUsed = stats.Memory.Usage - stats.Memory.File
		swapMax = stats.Memory.SwapLimit
		swapUsed = stats.Memory.SwapUsage
	}

	return memoryMax, memoryUsed, swapMax, swapUsed, nil
}

func cgroupV2Mem() (memoryMax, memoryUsed, swapMax, swapUsed uint64, err error) {
	memoryMax = math.MaxUint64
	swapMax = math.MaxUint64

	path, err := cgroupv2.PidGroupPath(os.Getpid())
	if err != nil {
		return 0, 0, 0, 0, err
	}

	mp, err := cgroupV2MountPoint()
	if err != nil {
		return 0, 0, 0, 0, err
	}

	for path != "/" {
		cgMemoryMax, cgMemoryUsed, cgSwapMax, cgSwapUsed, err := cgroupV2MemFromPath(mp, path)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if cgMemoryMax != 0 && cgMemoryMax < memoryMax {
			log.Debugf("memory limited by cgroup %s: %v", path, cgMemoryMax)
			memoryMax = cgMemoryMax
			memoryUsed = cgMemoryUsed
		}
		if cgSwapMax != 0 && cgSwapMax < swapMax {
			log.Debugf("swap limited by cgroup %s: %v", path, cgSwapMax)
			swapMax = cgSwapMax
			swapUsed = cgSwapUsed
		}
		path = filepath.Dir(path)
	}

	return memoryMax, memoryUsed, swapMax, swapUsed, nil
}
