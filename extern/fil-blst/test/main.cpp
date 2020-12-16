// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0
#include <vector>
#include <fstream>
#include <iostream>
#include <iomanip>
#include <string.h>
#include <chrono>

#include "fil_blst.h"
#include "util.hpp"

typedef std::chrono::high_resolution_clock Clock;

const char* vk_files[] = {
   // seal verification key
   "/var/tmp/filecoin-proof-parameters/v27-stacked-proof-of-replication-merkletree-poseidon_hasher-8-8-0-sha256_hasher-82a357d2f2ca81dc61bb45f4a762807aedee1b0a53fd6c4e77b46a01bfef7820.vk",
   // winning verification key
   "/var/tmp/filecoin-proof-parameters/v27-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-8-0-559e581f022bb4e4ec6e719e563bf0e026ad6de42e56c18714a2c692b1b88d7e.vk",
   // window verification key
   "/var/tmp/filecoin-proof-parameters/v27-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-8-0-0377ded656c6f524f1618760bffe4e0a1c51d5a70c4509eedae8a27555733edc.vk"
};

int main() {
  BLST_ERROR err;
  
  VERIFYING_KEY *seal_vk;
  err = read_vk_file(&seal_vk, std::string(vk_files[0]));
  if (err != BLST_SUCCESS) {
    std::cout << "seal read_vk_file err " << err << std::endl;
    return 1;
  }

  VERIFYING_KEY *winning_vk;
  err = read_vk_file(&winning_vk, std::string(vk_files[1]));
  if (err != BLST_SUCCESS) {
    std::cout << "winning read_vk_file err " << err << std::endl;
    return 1;
  }

  VERIFYING_KEY *window_vk;
  err = read_vk_file(&window_vk, std::string(vk_files[2]));
  if (err != BLST_SUCCESS) {
    std::cout << "window read_vk_file err " << err << std::endl;
    return 1;
  }

  bool result;
  std::mt19937_64 gen(1);
  std::uniform_int_distribution<limb_t>
    dist(0, std::numeric_limits<limb_t>::max());
  std::vector<blst_scalar> rand_z;
  
  PROOF winning_proof;
  std::vector<blst_scalar> public_inputs;
  err = read_test_file(&winning_proof, public_inputs,
                       "test/tests/winning32.prf");
  if (err != BLST_SUCCESS) {
    std::cout << "winning read_test_file err " << err << std::endl;
    return 1;
  }

  auto start = Clock::now();
  // This is a bit ugly, but we don't have a winning proof serialized in
  // the batch format.
  BATCH_PROOF winning_proofs;
  winning_proofs.num_proofs = 1;
  winning_proofs.input_lengths = public_inputs.size();
  winning_proofs.proofs = &winning_proof;
  blst_scalar *inputs_ptr = public_inputs.data();
  winning_proofs.public_inputs = &inputs_ptr;
  winning_proofs.num_proofs = 1;
  result = verify_batch_proof_cpp(winning_proofs, winning_vk, rand_z.data(),
                                  128);
  auto end = Clock::now();
  uint64_t dt = std::chrono::duration_cast<
    std::chrono::microseconds>(end - start).count();
  printf("Verifying Winning Proof took %5lu us, result %d\n", dt, result);


  // Seal
  BATCH_PROOF seal_batch_proof;
  err = read_batch_test_file(seal_batch_proof, "test/tests/seal32_x10.prf");
  if (err != BLST_SUCCESS) {
    std::cout << "seal read_batch_test_file err " << err << std::endl;
    return 1;
  }

  rand_z.clear();
  generate_random_scalars(seal_batch_proof.num_proofs, rand_z, gen, dist);
  start = Clock::now();
  result = verify_batch_proof_cpp(seal_batch_proof, seal_vk, rand_z.data(),
                                  128);
  end = Clock::now();
  dt = std::chrono::duration_cast<
    std::chrono::microseconds>(end - start).count();
  printf("Verifying Seal    Proof took %5lu us, result %d\n", dt, result);
  delete_batch_test_data(seal_batch_proof);

  // Seal including proof decompression
  std::vector<uint8_t> proof;
  std::vector<blst_scalar> inputs;
  err = read_batch_test_file_bytes(proof, inputs,
                                   "test/tests/seal32_x10.prf");
  if (err != BLST_SUCCESS) {
    std::cout << "seal read_batch_test_file_bytes err " << err << std::endl;
    return 1;
  }
  start = Clock::now();
  size_t num_proofs = proof.size() / 192;
  size_t num_inputs = inputs.size() / num_proofs;
  result = verify_batch_proof_c(proof.data(), num_proofs,
                                inputs.data(), num_inputs,
                                rand_z.data(), 128,
                                (uint8_t *)vk_files[0],
                                strlen(vk_files[0]));
  end = Clock::now();
  dt = std::chrono::duration_cast<
    std::chrono::microseconds>(end - start).count();
  printf("Verifying Seal    Proof took %5lu us, result %d\n", dt, result);
  
  // Window
  BATCH_PROOF window_proofs;
  err = read_batch_test_file(window_proofs, "test/tests/window.prf");
  if (err != BLST_SUCCESS) {
    std::cout << "window read_test_file err " << err << std::endl;
    return 1;
  }
  rand_z.clear();
  generate_random_scalars(window_proofs.num_proofs, rand_z, gen, dist);

  start = Clock::now();
  result = verify_batch_proof_cpp(window_proofs, window_vk, rand_z.data(),
                                  128);
  end = Clock::now();
  dt = std::chrono::duration_cast<
    std::chrono::microseconds>(end - start).count();
  printf("Verifying Window  Proof took %5lu us, result %d\n", dt, result);
  delete_batch_test_data(window_proofs);

  return 0;
}
