// Package cl was borrowed from the go-opencl library which is more complex and
// doesn't compile well for our needs.
package cl

// #include "cl.h"
import "C"
import (
	"errors"
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
	if err := C.clGetPlatformIDs(C.cl_uint(maxPlatforms), &platformIds[0], &nPlatforms); err != C.CL_SUCCESS {
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

var errorMap = map[C.cl_int]string{
	C.CL_SUCCESS:                                   "nil",
	C.CL_DEVICE_NOT_FOUND:                          "ErrDeviceNotFound",
	C.CL_DEVICE_NOT_AVAILABLE:                      "ErrDeviceNotAvailable",
	C.CL_COMPILER_NOT_AVAILABLE:                    "ErrCompilerNotAvailable",
	C.CL_MEM_OBJECT_ALLOCATION_FAILURE:             "ErrMemObjectAllocationFailure",
	C.CL_OUT_OF_RESOURCES:                          "ErrOutOfResources",
	C.CL_OUT_OF_HOST_MEMORY:                        "ErrOutOfHostMemory",
	C.CL_PROFILING_INFO_NOT_AVAILABLE:              "ErrProfilingInfoNotAvailable",
	C.CL_MEM_COPY_OVERLAP:                          "ErrMemCopyOverlap",
	C.CL_IMAGE_FORMAT_MISMATCH:                     "ErrImageFormatMismatch",
	C.CL_IMAGE_FORMAT_NOT_SUPPORTED:                "ErrImageFormatNotSupported",
	C.CL_BUILD_PROGRAM_FAILURE:                     "ErrBuildProgramFailure",
	C.CL_MAP_FAILURE:                               "ErrMapFailure",
	C.CL_MISALIGNED_SUB_BUFFER_OFFSET:              "ErrMisalignedSubBufferOffset",
	C.CL_EXEC_STATUS_ERROR_FOR_EVENTS_IN_WAIT_LIST: "ErrExecStatusErrorForEventsInWaitList",
	C.CL_INVALID_VALUE:                             "ErrInvalidValue",
	C.CL_INVALID_DEVICE_TYPE:                       "ErrInvalidDeviceType",
	C.CL_INVALID_PLATFORM:                          "ErrInvalidPlatform",
	C.CL_INVALID_DEVICE:                            "ErrInvalidDevice",
	C.CL_INVALID_CONTEXT:                           "ErrInvalidContext",
	C.CL_INVALID_QUEUE_PROPERTIES:                  "ErrInvalidQueueProperties",
	C.CL_INVALID_COMMAND_QUEUE:                     "ErrInvalidCommandQueue",
	C.CL_INVALID_HOST_PTR:                          "ErrInvalidHostPtr",
	C.CL_INVALID_MEM_OBJECT:                        "ErrInvalidMemObject",
	C.CL_INVALID_IMAGE_FORMAT_DESCRIPTOR:           "ErrInvalidImageFormatDescriptor",
	C.CL_INVALID_IMAGE_SIZE:                        "ErrInvalidImageSize",
	C.CL_INVALID_SAMPLER:                           "ErrInvalidSampler",
	C.CL_INVALID_BINARY:                            "ErrInvalidBinary",
	C.CL_INVALID_BUILD_OPTIONS:                     "ErrInvalidBuildOptions",
	C.CL_INVALID_PROGRAM:                           "ErrInvalidProgram",
	C.CL_INVALID_PROGRAM_EXECUTABLE:                "ErrInvalidProgramExecutable",
	C.CL_INVALID_KERNEL_NAME:                       "ErrInvalidKernelName",
	C.CL_INVALID_KERNEL_DEFINITION:                 "ErrInvalidKernelDefinition",
	C.CL_INVALID_KERNEL:                            "ErrInvalidKernel",
	C.CL_INVALID_ARG_INDEX:                         "ErrInvalidArgIndex",
	C.CL_INVALID_ARG_VALUE:                         "ErrInvalidArgValue",
	C.CL_INVALID_ARG_SIZE:                          "ErrInvalidArgSize",
	C.CL_INVALID_KERNEL_ARGS:                       "ErrInvalidKernelArgs",
	C.CL_INVALID_WORK_DIMENSION:                    "ErrInvalidWorkDimension",
	C.CL_INVALID_WORK_GROUP_SIZE:                   "ErrInvalidWorkGroupSize",
	C.CL_INVALID_WORK_ITEM_SIZE:                    "ErrInvalidWorkItemSize",
	C.CL_INVALID_GLOBAL_OFFSET:                     "ErrInvalidGlobalOffset",
	C.CL_INVALID_EVENT_WAIT_LIST:                   "ErrInvalidEventWaitList",
	C.CL_INVALID_EVENT:                             "ErrInvalidEvent",
	C.CL_INVALID_OPERATION:                         "ErrInvalidOperation",
	C.CL_INVALID_GL_OBJECT:                         "ErrInvalidGlObject",
	C.CL_INVALID_BUFFER_SIZE:                       "ErrInvalidBufferSize",
	C.CL_INVALID_MIP_LEVEL:                         "ErrInvalidMipLevel",
	C.CL_INVALID_GLOBAL_WORK_SIZE:                  "ErrInvalidGlobalWorkSize",
	C.CL_INVALID_PROPERTY:                          "ErrInvalidProperty",
}

func toError(code C.cl_int) error {
	if err, ok := errorMap[code]; ok {
		return errors.New(err)
	}
	return ErrOther(code)
}

type ErrOther int

func (e ErrOther) Error() string {
	return fmt.Sprintf("cl: error %d", int(e))
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
