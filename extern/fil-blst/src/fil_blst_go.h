/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef __FIL_BLST_GO_HPP__
#define __FIL_BLST_GO_HPP__

#include <stdint.h>
#include <stddef.h>

typedef uint8_t byte;

void threadpool_init(size_t thread_count);
int verify_window_post_go(uint8_t *randomness, uint64_t sector_mask, 
                          uint8_t *sector_comm_r, uint64_t *sector_ids,
                          size_t num_sectors,
                          size_t challenge_count,
                          uint8_t *proof, size_t num_proofs,
                          char *vkfile);

#endif  // __FIL_BLST_GO_HPP__
