// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#ifndef __UTIL_H__
#define __UTIL_H__

#include <vector>
#include <random>
#include "fil_blst_internal.hpp"

void print_blst_scalar(const blst_scalar* s, const char* name);
void print_blst_fr(const blst_fr *p, const char* name);
void print_blst_fp(const blst_fp p, const char* name);
void print_blst_fp2(const blst_fp2 p, const char* name);
void print_blst_fp6(const blst_fp6 p, const char* name);
void print_blst_fp12(blst_fp12 p, const char* name);
void print_blst_p1(const blst_p1* p, const char* name);
void print_blst_p1_affine(const blst_p1_affine* p_aff, const char* name);
void print_blst_p2(const blst_p2* p, const char* name);
void print_blst_p2_affine(const blst_p2_affine* p_aff, const char* name);
void print_fr(const blst_fr *fr);
void print_proof(PROOF* p);
void print_proofs(PROOF* p, size_t num);
void print_vk(VERIFYING_KEY_OPAQUE blob);
BLST_ERROR read_test_file(PROOF* proof, std::vector<blst_scalar>& inputs,
                          const char* filename);
BLST_ERROR read_batch_test_file(BATCH_PROOF& bp, const char* filename);
BLST_ERROR read_batch_test_file_bytes(std::vector<uint8_t> &proof,
                                      std::vector<blst_scalar> &inputs,
                                      const char* filename);
void delete_batch_test_data(BATCH_PROOF& bp);
void generate_random_scalars(uint64_t num_proofs, std::vector<blst_scalar>& z,
                             std::mt19937_64& gen,
                             std::uniform_int_distribution<limb_t>& dist);
bool verify_batch_proof_cpp(BATCH_PROOF& bp, VERIFYING_KEY* vk,
                            blst_scalar *rand_z, size_t nbits);

#endif  // __UTIL_H__
