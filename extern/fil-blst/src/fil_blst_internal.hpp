// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#ifndef __FIL_BLST_INTERNAL_HPP__
#define __FIL_BLST_INTERNAL_HPP__

#include "blst.h"

struct MultiscalarPrecomp {
  size_t   num_points;
  size_t   window_size;
  uint64_t window_mask;
  size_t   table_entries;
  blst_p1_affine** tables;
  bool     own_memory;

  MultiscalarPrecomp() {
    own_memory = true;
  }
  ~MultiscalarPrecomp() {
    if (own_memory) {
      for (size_t i = 0; i < num_points; i++) {
        delete [] tables[i];
      }
      delete [] tables;
      tables = NULL;
    }
  }
  MultiscalarPrecomp at_point(size_t idx) {
    MultiscalarPrecomp subset = *this;
    subset.num_points -= idx;
    subset.tables = &tables[idx];
    subset.own_memory = false;
    return subset;
  }
};
  
struct VERIFYING_KEY {
  // Verification key stored values
  blst_p1_affine alpha_g1;
  blst_p1_affine beta_g1;
  blst_p2_affine beta_g2;
  blst_p2_affine gamma_g2;
  blst_p1_affine delta_g1;
  blst_p2_affine delta_g2;
  std::vector<blst_p1_affine> ic;

  // Precomputations
  blst_fp12 alpha_g1_beta_g2;
  blst_fp6  delta_q_lines[68];
  blst_fp6  gamma_q_lines[68];
  blst_fp6  neg_delta_q_lines[68];
  blst_fp6  neg_gamma_q_lines[68];

  MultiscalarPrecomp *multiscalar;

  VERIFYING_KEY() {
    multiscalar = NULL;
  }
  ~VERIFYING_KEY() {
    if (multiscalar != NULL) {
      delete multiscalar;
    }
  }
};

struct VERIFYING_KEY_OPAQUE {
  uint8_t *u8;
};

typedef struct {
  blst_p1_affine a_g1;
  blst_p2_affine b_g2;
  blst_p1_affine c_g1;
} PROOF;

typedef struct {
  uint64_t      num_proofs;
  uint64_t      input_lengths;
  PROOF*        proofs;
  blst_scalar** public_inputs;
} BATCH_PROOF;

// Read verification key file and perform basic precomputations
BLST_ERROR read_vk_file(VERIFYING_KEY** vk, std::string filename);

bool verify_batch_proof_inner(PROOF *proofs, size_t num_proofs,
                              blst_scalar *public_inputs, size_t num_inputs,
                              VERIFYING_KEY& vk,
                              blst_scalar *rand_z, size_t nbits);


#endif  // __FIL_BLST_INTERNAL_H__
