/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

package filblst

// -march=native is a little better since it uses sha-ni instructions. However,
// using -D__ADX__ to maintain cross system compatibility.

// #cgo CFLAGS: -I${SRCDIR}/../blst/bindings -I${SRCDIR}/../blst/src -I${SRCDIR}/../blst/build -D__BLST_CGO__ -mno-avx -D__ADX__ -O3
// #cgo CPPFLAGS: -I${SRCDIR}/../blst/bindings -I${SRCDIR}/../blst/src -I${SRCDIR}/../blst/build -D__BLST_CGO__ -mno-avx -D__ADX__ -O3
// #cgo LDFLAGS: -L${SRCDIR}/../blst -lblst
// #include <fil_blst_go.h>
import "C"
import (
	"fmt"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/pkg/errors"

	_ "github.com/supranational/blst/bindings/go"
)

const BLST_SCALAR_BYTES = 256 / 8

// Print bytes in big-endian order
func printBytes(val []byte, name string) {
	fmt.Printf("%s = ", name)
	for i := len(val); i > 0; i-- {
		fmt.Printf("%02x", val[i-1])
	}
	fmt.Printf("\n")
}

// Configure the number of threads used by the library
func Init(num_threads uint64) {
	C.threadpool_init(C.size_t(num_threads))
}

func VerifyWindowPoSt(info abi.WindowPoStVerifyInfo,
	vkFile string) (bool, error) {

	// Types defined in specs-actors/actors/abi/sector.go
	numSectors, err := info.Proofs[0].PoStProof.WindowPoStPartitionSectors()
	if err != nil {
		return false, err
	}

	if len(info.Proofs) > 1 {
		return false, fmt.Errorf("Batch window proofs are not yet supported")
	}

	// 3fffffff for 32G
	// 7fffffff for 64G
	// sectorMask := sectorSize/NODE_SIZE - 1
	// TODO: where should we get the vk file name?
	typ, _ := info.Proofs[0].PoStProof.RegisteredSealProof()
	var sectorMask uint64
	if typ == abi.RegisteredSealProof_StackedDrg32GiBV1 {
		sectorMask = 0x3fffffff
	} else if typ == abi.RegisteredSealProof_StackedDrg64GiBV1 {
		sectorMask = 0x7fffffff
	} else {
		return false, fmt.Errorf("Unsupported proof type: %d", typ)
	}

	commRs := make([]byte, 0, len(info.ChallengedSectors)*BLST_SCALAR_BYTES)
	sectorIDs := make([]C.size_t, 0, len(info.ChallengedSectors))
	for _, v := range info.ChallengedSectors {
		commR, err := commcid.CIDToReplicaCommitmentV1(v.SealedCID)
		if err != nil {
			return false, errors.Wrap(err, "failed to transform sealed CID to CommR")
		}
		commRs = append(commRs, commR...)
		sectorIDs = append(sectorIDs, C.size_t(v.SectorNumber))
	}

	// TODO: How do we get this in Go?
	// rust-fil-proofs/filecoin-proofs/src/constants.rs
	WINDOW_POST_CHALLENGE_COUNT := uint64(10)

	res := C.verify_window_post_go(
		(*C.byte)(&info.Randomness[0]), C.size_t(sectorMask),
		(*C.byte)(&commRs[0]), &sectorIDs[0], C.size_t(numSectors),
		C.size_t(WINDOW_POST_CHALLENGE_COUNT),
		(*C.byte)(&info.Proofs[0].ProofBytes[0]), C.size_t(len(info.Proofs)),
		C.CString(vkFile))
	if res == 1 {
		return true, nil
	}
	return false, nil
}
