// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#include <fstream>
#include <iostream>
#include <cstring>

#include "util.hpp"
#include "fil_blst.h"

static void print_value(unsigned int *value, size_t n, const char* name) {
    if (name != NULL)
        printf("%s = 0x", name);
    else
        printf("0x");
    while (n--)
        printf("%08x", value[n]);
    printf("\n");
}

// static void print_bytes(unsigned char *value, size_t n, const char* name) {
//     if (name != NULL)
//         printf("%s = 0x", name);
//     else
//         printf("0x");
//     while (n--)
//         printf("%02x", value[n]);
//     printf("\n");
// }

/* Print helper functions */
void print_blst_scalar(const blst_scalar* s, const char* name) {
    unsigned int value[256/32];
    blst_uint32_from_scalar(value, s);
    print_value(value, 256/32, name);
}

void print_blst_fr(const blst_fr *p, const char* name) {
    union { blst_fr fr;
            blst_scalar s;
            unsigned int v[256/32]; } value;
    blst_fr_from(&value.fr, p);
    //blst_uint32_from_scalar(value.v, &value.s);
    print_value(value.v, 256/32, name);
}

void print_blst_fp(const blst_fp p, const char* name) {
    unsigned int value[384/32];
    blst_uint32_from_fp(value, &p);
    print_value(value, 384/32, name);
}

void print_blst_fp2(const blst_fp2 p, const char* name) {
    if (name != NULL)
        printf("%s:\n", name);

    print_blst_fp(p.fp[0], "    0");
    print_blst_fp(p.fp[1], "    1");
}

void print_blst_fp6(const blst_fp6 p, const char* name) {
    if (name != NULL)
        printf("%s:\n", name);

    print_blst_fp2(p.fp2[0], "    0");
    print_blst_fp2(p.fp2[1], "    1");
    print_blst_fp2(p.fp2[2], "    2");
}

void print_blst_fp12(blst_fp12 p, const char* name) {
    if (name != NULL)
        printf("%s:\n", name);

    print_blst_fp6(p.fp6[0], "    0");
    print_blst_fp6(p.fp6[1], "    1");
}

void print_blst_p1(const blst_p1* p, const char* name) {
    blst_p1_affine p_aff;

    if (name != NULL)
        printf("%s:\n", name);

    blst_p1_to_affine(&p_aff, p);
    print_blst_fp(p_aff.x, "  x");

    print_blst_fp(p_aff.y, "  y");
    printf("\n");
}

void print_blst_p1_affine(const blst_p1_affine* p_aff, const char* name) {
    if (name != NULL)
        printf("%s:\n", name);

    print_blst_fp(p_aff->x, "  x");
    print_blst_fp(p_aff->y, "  y");
    printf("\n");
}

void print_blst_p2(const blst_p2* p, const char* name) {
    blst_p2_affine  p_aff;
    blst_p2_to_affine(&p_aff, p);

    if (name != NULL)
        printf("%s:\n", name);
    print_blst_fp2(p_aff.x, "  x");
    print_blst_fp2(p_aff.y, "  y");
    printf("\n");
}

void print_blst_p2_affine(const blst_p2_affine* p_aff, const char* name) {
    if (name != NULL)
        printf("%s:\n", name);
    print_blst_fp2(p_aff->x, "  x");
    print_blst_fp2(p_aff->y, "  y");
    printf("\n");
}

void print_fr(const blst_fr *fr) {
  print_blst_fr(fr, "blst fr");
}

void print_proof(PROOF* p) {
  print_blst_p1_affine(&p->a_g1, "Proof A");
  print_blst_p2_affine(&p->b_g2, "Proof B");
  print_blst_p1_affine(&p->c_g1, "Proof C");
}

void print_proofs(PROOF* p, size_t num) {
  for (size_t i = 0; i < num; ++i) {
    print_proof(&p[i]);
  }
}

void print_vk(VERIFYING_KEY_OPAQUE blob) {
  VERIFYING_KEY *vk = (VERIFYING_KEY *)blob.u8;
  print_blst_p1_affine(&vk->alpha_g1, "alpha_g1");
  print_blst_p1_affine(&vk->beta_g1, "beta_g1");
  print_blst_p2_affine(&vk->beta_g2, "beta_g2");
  print_blst_p2_affine(&vk->gamma_g2, "gamma_g2");
  print_blst_p1_affine(&vk->delta_g1, "delta_g1");
  print_blst_p2_affine(&vk->delta_g2, "delta_g2");
  print_blst_fp12(vk->alpha_g1_beta_g2, "alpha_g1_beta_g2");
  printf("ic_len = %lu\n", vk->ic.size());
  for (size_t i = 0; i < 2; i++) {
    print_blst_p1_affine(&vk->ic[i], "  ic");
  }
}


// TODO: remove entirely - need to update winning test file
// Read sample test file from collected data to get proof and public inputs
BLST_ERROR read_test_file(PROOF* proof, std::vector<blst_scalar>& inputs,
                          const char* filename) {
  std::ifstream test_file;
  test_file.open(filename, std::ios::binary | std::ios::in);

  if (!test_file) {
    std::cout << "read_test_file read error: " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  unsigned char g1_bytes[48];
  unsigned char g2_bytes[96];
  unsigned char fr_bytes[32];
  uint64_t      in_len_rd;
  uint64_t      in_len;
  BLST_ERROR    err;

  test_file.read((char*) g1_bytes, sizeof(g1_bytes));
  err = blst_p1_uncompress(&proof->a_g1, g1_bytes);
  if (err != BLST_SUCCESS) return err;

  test_file.read((char*) g2_bytes, sizeof(g2_bytes));
  err = blst_p2_uncompress(&proof->b_g2, g2_bytes);
  if (err != BLST_SUCCESS) return err;

  test_file.read((char*) g1_bytes, sizeof(g1_bytes));
  err = blst_p1_uncompress(&proof->c_g1, g1_bytes);
  if (err != BLST_SUCCESS) return err;

  test_file.read((char*) &in_len_rd, sizeof(in_len_rd));
  in_len = ((in_len_rd << 8)  & 0xFF00FF00FF00FF00ULL ) |
           ((in_len_rd >> 8)  & 0x00FF00FF00FF00FFULL );
  in_len = ((in_len    << 16) & 0xFFFF0000FFFF0000ULL ) |
           ((in_len    >> 16) & 0x0000FFFF0000FFFFULL );
  in_len = (in_len << 32) | (in_len >> 32);

  inputs.reserve(in_len);

  while (in_len--) {
    blst_scalar cur_input;
    test_file.read((char*) fr_bytes, sizeof(fr_bytes));
    blst_scalar_from_bendian(&cur_input, fr_bytes);
    inputs.push_back(cur_input);
  }

  if (!test_file) {
    std::cout << "read_test_file read too much " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  test_file.close();

  return err;
}

// Read batch test file from collected data to get proof and public inputs
BLST_ERROR read_batch_test_file(BATCH_PROOF& bp, const char* filename) {
  std::ifstream test_file;
  test_file.open(filename, std::ios::binary | std::ios::in);

  if (!test_file) {
    std::cout << "read_batch_test_file read error: " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  uint64_t      num_proofs;
  test_file.read((char*) &num_proofs, sizeof(num_proofs));
  num_proofs = ((num_proofs << 8)  & 0xFF00FF00FF00FF00ULL ) |
               ((num_proofs >> 8)  & 0x00FF00FF00FF00FFULL );
  num_proofs = ((num_proofs    << 16) & 0xFFFF0000FFFF0000ULL ) |
               ((num_proofs    >> 16) & 0x0000FFFF0000FFFFULL );
  num_proofs = (num_proofs << 32) | (num_proofs >> 32);

  uint64_t      in_len;
  test_file.read((char*) &in_len, sizeof(in_len));
  in_len = ((in_len << 8)  & 0xFF00FF00FF00FF00ULL ) |
           ((in_len >> 8)  & 0x00FF00FF00FF00FFULL );
  in_len = ((in_len << 16) & 0xFFFF0000FFFF0000ULL ) |
           ((in_len >> 16) & 0x0000FFFF0000FFFFULL );
  in_len = (in_len  << 32) | (in_len >> 32);

  // Discard verification key
  uint64_t vk_file_len = ((in_len + 1) * 96) + 4 + (3 * 96) + (3 * 192);
  unsigned char* vk_bytes = new unsigned char[vk_file_len];
  test_file.read((char*) vk_bytes, vk_file_len);
  delete[] vk_bytes;

  unsigned char g1_bytes[48];
  unsigned char g2_bytes[96];
  unsigned char fr_bytes[32];
  BLST_ERROR    err;

  bp.num_proofs    = num_proofs;
  bp.input_lengths = in_len;
  bp.proofs         = new PROOF[num_proofs];
  bp.public_inputs  = new blst_scalar*[num_proofs];
  for (uint64_t i = 0; i < num_proofs; ++i) {
    test_file.read((char*) g1_bytes, sizeof(g1_bytes));
    err = blst_p1_uncompress(&bp.proofs[i].a_g1, g1_bytes);
    if (err != BLST_SUCCESS) return err;

    test_file.read((char*) g2_bytes, sizeof(g2_bytes));
    err = blst_p2_uncompress(&bp.proofs[i].b_g2, g2_bytes);
    if (err != BLST_SUCCESS) return err;

    test_file.read((char*) g1_bytes, sizeof(g1_bytes));
    err = blst_p1_uncompress(&bp.proofs[i].c_g1, g1_bytes);
    if (err != BLST_SUCCESS) return err;

    bp.public_inputs[i] = new blst_scalar[in_len];

    for (uint64_t j = 0; j < in_len; ++j) {
      test_file.read((char*) fr_bytes, sizeof(fr_bytes));
      blst_scalar_from_bendian(&bp.public_inputs[i][j],
                               fr_bytes);
    }
  }

  if (!test_file) {
    std::cout << "read_batch_test_file read too much " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  test_file.close();

  return err;
}

// Read batch test file from collected data to get proof and public inputs
BLST_ERROR read_batch_test_file_bytes(std::vector<uint8_t> &proof,
                                      std::vector<blst_scalar> &inputs,
                                      const char* filename) {
  std::ifstream test_file;
  test_file.open(filename, std::ios::binary | std::ios::in);

  if (!test_file) {
    std::cout << "read_batch_test_file read error: " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  uint64_t      num_proofs;
  test_file.read((char*) &num_proofs, sizeof(num_proofs));
  num_proofs = ((num_proofs << 8)  & 0xFF00FF00FF00FF00ULL ) |
               ((num_proofs >> 8)  & 0x00FF00FF00FF00FFULL );
  num_proofs = ((num_proofs    << 16) & 0xFFFF0000FFFF0000ULL ) |
               ((num_proofs    >> 16) & 0x0000FFFF0000FFFFULL );
  num_proofs = (num_proofs << 32) | (num_proofs >> 32);

  uint64_t      in_len;
  test_file.read((char*) &in_len, sizeof(in_len));
  in_len = ((in_len << 8)  & 0xFF00FF00FF00FF00ULL ) |
           ((in_len >> 8)  & 0x00FF00FF00FF00FFULL );
  in_len = ((in_len << 16) & 0xFFFF0000FFFF0000ULL ) |
           ((in_len >> 16) & 0x0000FFFF0000FFFFULL );
  in_len = (in_len  << 32) | (in_len >> 32);

  // Discard verification key
  uint64_t vk_file_len = ((in_len + 1) * 96) + 4 + (3 * 96) + (3 * 192);
  unsigned char* vk_bytes = new unsigned char[vk_file_len];
  test_file.read((char*) vk_bytes, vk_file_len);
  delete[] vk_bytes;

  unsigned char proof_bytes[192];
  unsigned char fr_bytes[32];
  BLST_ERROR    err = BLST_SUCCESS;

  for (uint64_t i = 0; i < num_proofs; ++i) {
    test_file.read((char*) proof_bytes, sizeof(proof_bytes));
    for (size_t j = 0; j < sizeof(proof_bytes); j++) {
      proof.push_back(proof_bytes[j]);
    }

    for (uint64_t j = 0; j < in_len; ++j) {
      test_file.read((char*) fr_bytes, sizeof(fr_bytes));
      blst_scalar fr;
      blst_scalar_from_bendian(&fr, fr_bytes);
      inputs.push_back(fr);
    }
  }

  if (!test_file) {
    std::cout << "read_batch_test_file read too much " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  test_file.close();

  return err;
}

void delete_batch_test_data(BATCH_PROOF& bp) {
  delete[] bp.proofs;
  for (uint64_t i = 0; i < bp.num_proofs; ++i) {
    delete[] bp.public_inputs[i];
  }

  delete[] bp.public_inputs;
}

void generate_random_scalars(uint64_t num_proofs, std::vector<blst_scalar>& z,
                             std::mt19937_64& gen,
                             std::uniform_int_distribution<limb_t>& dist) {
  z.reserve(num_proofs);
  blst_scalar cur_rand;
  memset(&cur_rand, 0, sizeof(blst_fr));
  while(num_proofs--) {
    // 128 bit random value
    for (size_t i = 0; i < 128 / 8 / sizeof(limb_t); ++i) {
      cur_rand.l[i] = dist(gen);
      //std::cout << "Gen " << std::dec << num_proofs << ":" << i
      //          << std::hex << cur_rand.l[i] << std::endl;
    }
    // TODO - is this necessary to be in Montgomery form?
    //blst_fr_to(&cur_rand, &cur_rand);
    z.push_back(cur_rand);
  }
}

bool verify_batch_proof_cpp(BATCH_PROOF& bp, VERIFYING_KEY* vk,
                            blst_scalar *rand_z, size_t nbits) {
  // TODO: modify BATCH_PROOF?
  std::vector<blst_scalar> flat_pis;
  for (size_t i = 0; i < bp.num_proofs; i++) {
    for (size_t j = 0; j < bp.input_lengths; j++) {
      flat_pis.push_back(bp.public_inputs[i][j]);
    }
  }
  return verify_batch_proof_inner(bp.proofs, bp.num_proofs,
                                  flat_pis.data(), bp.input_lengths,
                                  *vk, rand_z, nbits);
}
