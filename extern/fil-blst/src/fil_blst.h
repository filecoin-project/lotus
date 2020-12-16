// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#ifndef __FIL_BLST_H__
#define __FIL_BLST_H__

#include "blst.h"

#ifdef __cplusplus
extern "C" {
void threadpool_init(size_t thread_count);
bool verify_batch_proof_c(uint8_t *proof_bytes, size_t num_proofs,
                          blst_scalar *public_inputs, size_t num_inputs,
                          blst_scalar *rand_z, size_t nbits,
                          uint8_t *vk_path, size_t vk_len);
int verify_window_post_go(uint8_t *randomness, uint64_t sector_mask, 
                          uint8_t *sector_comm_r, uint64_t *sector_ids,
                          size_t num_sectors,
                          size_t challenge_count,
                          uint8_t *proof_bytes, size_t num_proofs,
                          char *vkfile);
}
#endif  // __cplusplus

#endif  // __FIL_BLST_H__
