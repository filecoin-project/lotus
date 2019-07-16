// +build !windows

package sectorbuilder

import (
	"time"
	"unsafe"

	logging "github.com/ipfs/go-log"
	"github.com/pkg/errors"
)

// #cgo LDFLAGS: -L${SRCDIR}/../lib -lsector_builder_ffi
// #cgo pkg-config: ${SRCDIR}/../lib/pkgconfig/sector_builder_ffi.pc
// #include "../include/sector_builder_ffi.h"
import "C"

var log = logging.Logger("libsectorbuilder") // nolint: deadcode

func elapsed(what string) func() {
	start := time.Now()
	return func() {
		log.Debugf("%s took %v\n", what, time.Since(start))
	}
}

// StagedSectorMetadata is a sector into which we write user piece-data before
// sealing. Note: SectorID is unique across all staged and sealed sectors for a
// storage miner actor.
type StagedSectorMetadata struct {
	SectorID uint64
}

// SectorSealingStatus communicates how far along in the sealing process a
// sector has progressed.
type SectorSealingStatus struct {
	SectorID       uint64
	SealStatusCode uint8           // Sealed = 0, Pending = 1, Failed = 2, Sealing = 3
	SealErrorMsg   string          // will be nil unless SealStatusCode == 2
	CommD          [32]byte        // will be empty unless SealStatusCode == 0
	CommR          [32]byte        // will be empty unless SealStatusCode == 0
	CommRStar      [32]byte        // will be empty unless SealStatusCode == 0
	Proof          []byte          // will be empty unless SealStatusCode == 0
	Pieces         []PieceMetadata // will be empty unless SealStatusCode == 0
}

// PieceMetadata represents a piece stored by the sector builder.
type PieceMetadata struct {
	Key            string
	Size           uint64
	InclusionProof []byte
}

// VerifySeal returns true if the sealing operation from which its inputs were
// derived was valid, and false if not.
func VerifySeal(
	sectorSize uint64,
	commR [32]byte,
	commD [32]byte,
	commRStar [32]byte,
	proverID [31]byte,
	sectorID [31]byte,
	proof []byte,
) (bool, error) {
	defer elapsed("VerifySeal")()

	commDCBytes := C.CBytes(commD[:])
	defer C.free(commDCBytes)

	commRCBytes := C.CBytes(commR[:])
	defer C.free(commRCBytes)

	commRStarCBytes := C.CBytes(commRStar[:])
	defer C.free(commRStarCBytes)

	proofCBytes := C.CBytes(proof[:])
	defer C.free(proofCBytes)

	proverIDCBytes := C.CBytes(proverID[:])
	defer C.free(proverIDCBytes)

	sectorIDCbytes := C.CBytes(sectorID[:])
	defer C.free(sectorIDCbytes)

	// a mutable pointer to a VerifySealResponse C-struct
	resPtr := (*C.sector_builder_ffi_VerifySealResponse)(unsafe.Pointer(C.sector_builder_ffi_verify_seal(
		C.uint64_t(sectorSize),
		(*[32]C.uint8_t)(commRCBytes),
		(*[32]C.uint8_t)(commDCBytes),
		(*[32]C.uint8_t)(commRStarCBytes),
		(*[31]C.uint8_t)(proverIDCBytes),
		(*[31]C.uint8_t)(sectorIDCbytes),
		(*C.uint8_t)(proofCBytes),
		C.size_t(len(proof)),
	)))
	defer C.sector_builder_ffi_destroy_verify_seal_response(resPtr)

	if resPtr.status_code != 0 {
		return false, errors.New(C.GoString(resPtr.error_msg))
	}

	return bool(resPtr.is_valid), nil
}

// VerifyPoSt returns true if the PoSt-generation operation from which its
// inputs were derived was valid, and false if not.
func VerifyPoSt(
	sectorSize uint64,
	sortedCommRs [][32]byte,
	challengeSeed [32]byte,
	proofs [][]byte,
	faults []uint64,
) (bool, error) {
	defer elapsed("VerifyPoSt")()

	// validate verification request
	if len(proofs) == 0 {
		return false, errors.New("must provide at least one proof to verify")
	}

	// CommRs must be provided to C.verify_post in the same order that they were
	// provided to the C.generate_post
	commRs := sortedCommRs

	// flattening the byte slice makes it easier to copy into the C heap
	flattened := make([]byte, 32*len(commRs))
	for idx, commR := range commRs {
		copy(flattened[(32*idx):(32*(1+idx))], commR[:])
	}

	// copy bytes from Go to C heap
	flattenedCommRsCBytes := C.CBytes(flattened)
	defer C.free(flattenedCommRsCBytes)

	challengeSeedCBytes := C.CBytes(challengeSeed[:])
	defer C.free(challengeSeedCBytes)

	proofPartitions, proofsPtr, proofsLen := cPoStProofs(proofs)
	defer C.free(unsafe.Pointer(proofsPtr))

	// allocate fixed-length array of uint64s in C heap
	faultsPtr, faultsSize := cUint64s(faults)
	defer C.free(unsafe.Pointer(faultsPtr))

	// a mutable pointer to a VerifyPoStResponse C-struct
	resPtr := (*C.sector_builder_ffi_VerifyPoStResponse)(unsafe.Pointer(C.sector_builder_ffi_verify_post(
		C.uint64_t(sectorSize),
		proofPartitions,
		(*C.uint8_t)(flattenedCommRsCBytes),
		C.size_t(len(flattened)),
		(*[32]C.uint8_t)(challengeSeedCBytes),
		proofsPtr,
		proofsLen,
		faultsPtr,
		faultsSize,
	)))
	defer C.sector_builder_ffi_destroy_verify_post_response(resPtr)

	if resPtr.status_code != 0 {
		return false, errors.New(C.GoString(resPtr.error_msg))
	}

	return bool(resPtr.is_valid), nil
}

// GetMaxUserBytesPerStagedSector returns the number of user bytes that will fit
// into a staged sector. Due to bit-padding, the number of user bytes that will
// fit into the staged sector will be less than number of bytes in sectorSize.
func GetMaxUserBytesPerStagedSector(sectorSize uint64) uint64 {
	defer elapsed("GetMaxUserBytesPerStagedSector")()

	return uint64(C.sector_builder_ffi_get_max_user_bytes_per_staged_sector(C.uint64_t(sectorSize)))
}

// InitSectorBuilder allocates and returns a pointer to a sector builder.
func InitSectorBuilder(
	sectorSize uint64,
	poRepProofPartitions uint8,
	poStProofPartitions uint8,
	lastUsedSectorID uint64,
	metadataDir string,
	proverID [31]byte,
	sealedSectorDir string,
	stagedSectorDir string,
	maxNumOpenStagedSectors uint8,
) (unsafe.Pointer, error) {
	defer elapsed("InitSectorBuilder")()

	cMetadataDir := C.CString(metadataDir)
	defer C.free(unsafe.Pointer(cMetadataDir))

	proverIDCBytes := C.CBytes(proverID[:])
	defer C.free(proverIDCBytes)

	cStagedSectorDir := C.CString(stagedSectorDir)
	defer C.free(unsafe.Pointer(cStagedSectorDir))

	cSealedSectorDir := C.CString(sealedSectorDir)
	defer C.free(unsafe.Pointer(cSealedSectorDir))

	class, err := cSectorClass(sectorSize, poRepProofPartitions, poStProofPartitions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sector class")
	}

	resPtr := (*C.sector_builder_ffi_InitSectorBuilderResponse)(unsafe.Pointer(C.sector_builder_ffi_init_sector_builder(
		class,
		C.uint64_t(lastUsedSectorID),
		cMetadataDir,
		(*[31]C.uint8_t)(proverIDCBytes),
		cSealedSectorDir,
		cStagedSectorDir,
		C.uint8_t(maxNumOpenStagedSectors),
	)))
	defer C.sector_builder_ffi_destroy_init_sector_builder_response(resPtr)

	if resPtr.status_code != 0 {
		return nil, errors.New(C.GoString(resPtr.error_msg))
	}

	return unsafe.Pointer(resPtr.sector_builder), nil
}

// DestroySectorBuilder deallocates the sector builder associated with the
// provided pointer. This function will panic if the provided pointer is null
// or if the sector builder has been previously deallocated.
func DestroySectorBuilder(sectorBuilderPtr unsafe.Pointer) {
	defer elapsed("DestroySectorBuilder")()

	C.sector_builder_ffi_destroy_sector_builder((*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr))
}

// AddPiece writes the given piece into an unsealed sector and returns the id
// of that sector.
func AddPiece(
	sectorBuilderPtr unsafe.Pointer,
	pieceKey string,
	pieceSize uint64,
	piecePath string,
) (sectorID uint64, retErr error) {
	defer elapsed("AddPiece")()

	cPieceKey := C.CString(pieceKey)
	defer C.free(unsafe.Pointer(cPieceKey))

	cPiecePath := C.CString(piecePath)
	defer C.free(unsafe.Pointer(cPiecePath))

	resPtr := (*C.sector_builder_ffi_AddPieceResponse)(unsafe.Pointer(C.sector_builder_ffi_add_piece(
		(*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr),
		cPieceKey,
		C.uint64_t(pieceSize),
		cPiecePath,
	)))
	defer C.sector_builder_ffi_destroy_add_piece_response(resPtr)

	if resPtr.status_code != 0 {
		return 0, errors.New(C.GoString(resPtr.error_msg))
	}

	return uint64(resPtr.sector_id), nil
}

// ReadPieceFromSealedSector produces a byte buffer containing the piece
// associated with the provided key. If the key is not associated with any piece
// yet sealed into a sector, an error will be returned.
func ReadPieceFromSealedSector(sectorBuilderPtr unsafe.Pointer, pieceKey string) ([]byte, error) {
	defer elapsed("ReadPieceFromSealedSector")()

	cPieceKey := C.CString(pieceKey)
	defer C.free(unsafe.Pointer(cPieceKey))

	resPtr := (*C.sector_builder_ffi_ReadPieceFromSealedSectorResponse)(unsafe.Pointer(C.sector_builder_ffi_read_piece_from_sealed_sector((*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr), cPieceKey)))
	defer C.sector_builder_ffi_destroy_read_piece_from_sealed_sector_response(resPtr)

	if resPtr.status_code != 0 {
		return nil, errors.New(C.GoString(resPtr.error_msg))
	}

	return goBytes(resPtr.data_ptr, resPtr.data_len), nil
}

// SealAllStagedSectors schedules sealing of all staged sectors.
func SealAllStagedSectors(sectorBuilderPtr unsafe.Pointer) error {
	defer elapsed("SealAllStagedSectors")()

	resPtr := (*C.sector_builder_ffi_SealAllStagedSectorsResponse)(unsafe.Pointer(C.sector_builder_ffi_seal_all_staged_sectors((*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr))))
	defer C.sector_builder_ffi_destroy_seal_all_staged_sectors_response(resPtr)

	if resPtr.status_code != 0 {
		return errors.New(C.GoString(resPtr.error_msg))
	}

	return nil
}

// GetAllStagedSectors returns a slice of all staged sector metadata for the sector builder.
func GetAllStagedSectors(sectorBuilderPtr unsafe.Pointer) ([]StagedSectorMetadata, error) {
	defer elapsed("GetAllStagedSectors")()

	resPtr := (*C.sector_builder_ffi_GetStagedSectorsResponse)(unsafe.Pointer(C.sector_builder_ffi_get_staged_sectors((*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr))))
	defer C.sector_builder_ffi_destroy_get_staged_sectors_response(resPtr)

	if resPtr.status_code != 0 {
		return nil, errors.New(C.GoString(resPtr.error_msg))
	}

	meta, err := goStagedSectorMetadata((*C.sector_builder_ffi_FFIStagedSectorMetadata)(unsafe.Pointer(resPtr.sectors_ptr)), resPtr.sectors_len)
	if err != nil {
		return nil, err
	}

	return meta, nil
}

// GetSectorSealingStatusByID produces sector sealing status (staged, sealing in
// progress, sealed, failed) for the provided sector id if it exists, otherwise
// an error.
func GetSectorSealingStatusByID(sectorBuilderPtr unsafe.Pointer, sectorID uint64) (SectorSealingStatus, error) {
	defer elapsed("GetSectorSealingStatusByID")()

	resPtr := (*C.sector_builder_ffi_GetSealStatusResponse)(unsafe.Pointer(C.sector_builder_ffi_get_seal_status((*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr), C.uint64_t(sectorID))))
	defer C.sector_builder_ffi_destroy_get_seal_status_response(resPtr)

	if resPtr.status_code != 0 {
		return SectorSealingStatus{}, errors.New(C.GoString(resPtr.error_msg))
	}

	if resPtr.seal_status_code == C.Failed {
		return SectorSealingStatus{SealStatusCode: 2, SealErrorMsg: C.GoString(resPtr.seal_error_msg)}, nil
	} else if resPtr.seal_status_code == C.Pending {
		return SectorSealingStatus{SealStatusCode: 1}, nil
	} else if resPtr.seal_status_code == C.Sealing {
		return SectorSealingStatus{SealStatusCode: 3}, nil
	} else if resPtr.seal_status_code == C.Sealed {
		commRSlice := goBytes(&resPtr.comm_r[0], 32)
		var commR [32]byte
		copy(commR[:], commRSlice)

		commDSlice := goBytes(&resPtr.comm_d[0], 32)
		var commD [32]byte
		copy(commD[:], commDSlice)

		commRStarSlice := goBytes(&resPtr.comm_r_star[0], 32)
		var commRStar [32]byte
		copy(commRStar[:], commRStarSlice)

		proof := goBytes(resPtr.proof_ptr, resPtr.proof_len)

		ps, err := goPieceMetadata(resPtr.pieces_ptr, resPtr.pieces_len)
		if err != nil {
			return SectorSealingStatus{}, errors.Wrap(err, "failed to marshal from string to cid")
		}

		return SectorSealingStatus{
			SectorID:       sectorID,
			SealStatusCode: 0,
			CommD:          commD,
			CommR:          commR,
			CommRStar:      commRStar,
			Proof:          proof,
			Pieces:         ps,
		}, nil
	} else {
		// unknown
		return SectorSealingStatus{}, errors.New("unexpected seal status")
	}
}

// GeneratePoSt produces a proof-of-spacetime for the provided replica commitments.
func GeneratePoSt(
	sectorBuilderPtr unsafe.Pointer,
	sortedCommRs [][32]byte,
	challengeSeed [32]byte,
) ([][]byte, []uint64, error) {
	defer elapsed("GeneratePoSt")()

	// flattening the byte slice makes it easier to copy into the C heap
	commRs := sortedCommRs
	flattened := make([]byte, 32*len(commRs))
	for idx, commR := range commRs {
		copy(flattened[(32*idx):(32*(1+idx))], commR[:])
	}

	// copy the Go byte slice into C memory
	cflattened := C.CBytes(flattened)
	defer C.free(cflattened)

	challengeSeedPtr := unsafe.Pointer(&(challengeSeed)[0])

	// a mutable pointer to a GeneratePoStResponse C-struct
	resPtr := (*C.sector_builder_ffi_GeneratePoStResponse)(unsafe.Pointer(C.sector_builder_ffi_generate_post((*C.sector_builder_ffi_SectorBuilder)(sectorBuilderPtr), (*C.uint8_t)(cflattened), C.size_t(len(flattened)), (*[32]C.uint8_t)(challengeSeedPtr))))
	defer C.sector_builder_ffi_destroy_generate_post_response(resPtr)

	if resPtr.status_code != 0 {
		return nil, nil, errors.New(C.GoString(resPtr.error_msg))
	}

	proofs, err := goPoStProofs(resPtr.proof_partitions, resPtr.flattened_proofs_ptr, resPtr.flattened_proofs_len)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to convert to []PoStProof")
	}

	return proofs, goUint64s(resPtr.faults_ptr, resPtr.faults_len), nil
}
