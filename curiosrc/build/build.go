package build

// IsOpencl is set to the value of FFI_USE_OPENCL
var IsOpencl string

// Format: 8 HEX then underscore then ISO8701 date
// Ex: 4c5e98f28_2024-05-17T18:42:27-04:00
// NOTE: git date for repeatabile builds.
var Commit string
