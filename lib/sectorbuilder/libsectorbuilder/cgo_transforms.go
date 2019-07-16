package libsectorbuilder

import (
	"unsafe"
)

// #cgo LDFLAGS: -L${SRCDIR}/../lib -lsector_builder_ffi
// #cgo pkg-config: ${SRCDIR}/../lib/pkgconfig/sector_builder_ffi.pc
// #include "../include/sector_builder_ffi.h"
import "C"

// SingleProofPartitionProofLen denotes the number of bytes in a proof generated
// with a single partition. The number of bytes in a proof increases linearly
// with the number of partitions used when creating that proof.
const SingleProofPartitionProofLen = 192

func cPoStProofs(src [][]byte) (C.uint8_t, *C.uint8_t, C.size_t) {
	proofSize := len(src[0])

	flattenedLen := C.size_t(proofSize * len(src))

	// flattening the byte slice makes it easier to copy into the C heap
	flattened := make([]byte, flattenedLen)
	for idx, proof := range src {
		copy(flattened[(proofSize*idx):(proofSize*(1+idx))], proof[:])
	}

	proofPartitions := proofSize / SingleProofPartitionProofLen

	return C.uint8_t(proofPartitions), (*C.uint8_t)(C.CBytes(flattened)), flattenedLen
}

func cUint64s(src []uint64) (*C.uint64_t, C.size_t) {
	srcCSizeT := C.size_t(len(src))

	// allocate array in C heap
	cUint64s := C.malloc(srcCSizeT * C.sizeof_uint64_t)

	// create a Go slice backed by the C-array
	pp := (*[1 << 30]C.uint64_t)(cUint64s)
	for i, v := range src {
		pp[i] = C.uint64_t(v)
	}

	return (*C.uint64_t)(cUint64s), srcCSizeT
}

func cSectorClass(sectorSize uint64, poRepProofPartitions uint8, poStProofPartitions uint8) (C.sector_builder_ffi_FFISectorClass, error) {
	return C.sector_builder_ffi_FFISectorClass{
		sector_size:            C.uint64_t(sectorSize),
		porep_proof_partitions: C.uint8_t(poRepProofPartitions),
		post_proof_partitions:  C.uint8_t(poStProofPartitions),
	}, nil
}

func goBytes(src *C.uint8_t, size C.size_t) []byte {
	return C.GoBytes(unsafe.Pointer(src), C.int(size))
}

func goStagedSectorMetadata(src *C.sector_builder_ffi_FFIStagedSectorMetadata, size C.size_t) ([]StagedSectorMetadata, error) {
	sectors := make([]StagedSectorMetadata, size)
	if src == nil || size == 0 {
		return sectors, nil
	}

	sectorPtrs := (*[1 << 30]C.sector_builder_ffi_FFIStagedSectorMetadata)(unsafe.Pointer(src))[:size:size]
	for i := 0; i < int(size); i++ {
		sectors[i] = StagedSectorMetadata{
			SectorID: uint64(sectorPtrs[i].sector_id),
		}
	}

	return sectors, nil
}

func goPieceMetadata(src *C.sector_builder_ffi_FFIPieceMetadata, size C.size_t) ([]PieceMetadata, error) {
	ps := make([]PieceMetadata, size)
	if src == nil || size == 0 {
		return ps, nil
	}

	ptrs := (*[1 << 30]C.sector_builder_ffi_FFIPieceMetadata)(unsafe.Pointer(src))[:size:size]
	for i := 0; i < int(size); i++ {
		ps[i] = PieceMetadata{
			Key:  C.GoString(ptrs[i].piece_key),
			Size: uint64(ptrs[i].num_bytes),
		}
	}

	return ps, nil
}

func goPoStProofs(partitions C.uint8_t, src *C.uint8_t, size C.size_t) ([][]byte, error) {
	tmp := goBytes(src, size)

	arraySize := len(tmp)
	chunkSize := int(partitions) * SingleProofPartitionProofLen

	out := make([][]byte, arraySize/chunkSize)
	for i := 0; i < len(out); i++ {
		out[i] = append([]byte{}, tmp[i*chunkSize:(i+1)*chunkSize]...)
	}

	return out, nil
}

func goUint64s(src *C.uint64_t, size C.size_t) []uint64 {
	out := make([]uint64, size)
	if src != nil {
		copy(out, (*(*[1 << 30]uint64)(unsafe.Pointer(src)))[:size:size])
	}
	return out
}
