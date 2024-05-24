// Package cl was borrowed from the go-opencl library which is more complex and
// doesn't compile well for our needs.
package cl

// #include "cl.h"
import "C"
import (
	"fmt"
	"unsafe"
)

const maxPlatforms = 32

type Platform struct {
	id C.cl_platform_id
}

// Obtain the list of platforms available.
func GetPlatforms() ([]*Platform, error) {
	var platformIds [maxPlatforms]C.cl_platform_id
	var nPlatforms C.cl_uint
	err := C.clGetPlatformIDs(C.cl_uint(maxPlatforms), &platformIds[0], &nPlatforms)
	if err == -1001 { // No platforms found
		return nil, nil
	}
	if err != C.CL_SUCCESS {
		return nil, toError(err)
	}
	platforms := make([]*Platform, nPlatforms)
	for i := 0; i < int(nPlatforms); i++ {
		platforms[i] = &Platform{id: platformIds[i]}
	}
	return platforms, nil
}

const maxDeviceCount = 64

type DeviceType uint

const (
	DeviceTypeAll DeviceType = C.CL_DEVICE_TYPE_ALL
)

type Device struct {
	id C.cl_device_id
}

func (p *Platform) GetAllDevices() ([]*Device, error) {
	var deviceIds [maxDeviceCount]C.cl_device_id
	var numDevices C.cl_uint
	var platformId C.cl_platform_id
	if p != nil {
		platformId = p.id
	}
	if err := C.clGetDeviceIDs(platformId, C.cl_device_type(DeviceTypeAll), C.cl_uint(maxDeviceCount), &deviceIds[0], &numDevices); err != C.CL_SUCCESS {
		return nil, toError(err)
	}
	if numDevices > maxDeviceCount {
		numDevices = maxDeviceCount
	}
	devices := make([]*Device, numDevices)
	for i := 0; i < int(numDevices); i++ {
		devices[i] = &Device{id: deviceIds[i]}
	}
	return devices, nil
}

func toError(code C.cl_int) error {
	return ErrOther(code)
}

type ErrOther int

func (e ErrOther) Error() string {
	return fmt.Sprintf("OpenCL: error %d", int(e))
}

// Size of global device memory in bytes.
func (d *Device) GlobalMemSize() int64 {
	val, _ := d.getInfoUlong(C.CL_DEVICE_GLOBAL_MEM_SIZE, true)
	return val
}

func (d *Device) getInfoUlong(param C.cl_device_info, panicOnError bool) (int64, error) {
	var val C.cl_ulong
	if err := C.clGetDeviceInfo(d.id, param, C.size_t(unsafe.Sizeof(val)), unsafe.Pointer(&val), nil); err != C.CL_SUCCESS {
		if panicOnError {
			panic("Should never fail")
		}
		return 0, toError(err)
	}
	return int64(val), nil
}
