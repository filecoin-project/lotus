// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#include <vector>
#include <fstream>
#include <iostream>
#include <iomanip>
#include <string.h>
#include <thread>
#include <atomic>
#include <map>
#include <xmmintrin.h>

#include "thread_pool.hpp"
#include "fil_blst.h"
#include "fil_blst_internal.hpp"
#include "util.hpp"

static const size_t  SCALAR_SIZE         = 256;
static const size_t  P1_COMPRESSED_BYTES = 48;
static const size_t  P2_COMPRESSED_BYTES = 96;
static const size_t  PROOF_BYTES         = 192;
static const blst_p1 G1_INFINITY         = {{ 0 }, { 0 }, { 0 }};


#if (defined(__x86_64__) || defined(__x86_64) || defined(_M_X64)) && \
     defined(__SHA__)   /* -msha */
# define blst_sha256_block blst_sha256_block_data_order_shaext
#elif defined(__aarch64__) && defined(__ARM_FEATURE_CRYPTO)
# define blst_sha256_block blst_sha256_block_armv8
#else
# define blst_sha256_block blst_sha256_block_data_order
#endif

typedef std::chrono::high_resolution_clock Clock;
static ThreadPool *da_pool = NULL;

extern "C" {
  void blst_sha256_block(unsigned int *h, const void *inp, size_t blocks);
  void threadpool_init(size_t thread_count);
}

// Singleton to construct and delete state
struct Config {
  std::map<std::string, VERIFYING_KEY *> vk_cache;
  Config() {
    threadpool_init(0);
  }
  ~Config() {
    if (da_pool != NULL) {
      delete da_pool;
    }
    for (auto it = vk_cache.begin(); it != vk_cache.end(); it++) {
      delete it->second;
    }
  }
};
Config zkconfig;

// Initialize and allocate threads in the thread pool.
void threadpool_init(size_t thread_count) {
  if (da_pool != NULL) {
    delete da_pool;
  }
  if (thread_count == 0 || thread_count > std::thread::hardware_concurrency()) {
    thread_count = std::thread::hardware_concurrency();
  }
  // We currently require a minimum of 4 threads
  if (thread_count < 4) {
    thread_count = 4;
  }
  da_pool = new ThreadPool(thread_count);
}

// Convenience function for multiplying an affine point
static void blst_p1_mult_affine(blst_p1 *out, const blst_p1_affine *p,
                                const blst_scalar *scalar, size_t nbits) {
  blst_p1 proj;
  blst_p1_from_affine(&proj, p);
  blst_p1_mult(out, &proj, scalar, nbits);
}

// Fp12 pow using square and multiply
// TODO: A small window might be faster
static void blst_fp12_pow(blst_fp12 *ret, blst_fp12 *_a, blst_scalar *b) {
  uint64_t b64[4];
  blst_uint64_from_scalar(b64, b);

  // TODO
  //blst_fp12 a = *_a;
  blst_fp12 a;
  memcpy(&a, _a, sizeof(a));
  
  bool first = true;
  for (int i = 0; i < 256; i++) {
    unsigned limb = i / 64;
    int bit = (b64[limb] >> (i % 64)) & 0x1;
    if (bit) {
      if (first) {
        first = false;
        // assign
        memcpy(ret, &a, sizeof(blst_fp12));
      } else {
        blst_fp12_mul(ret, ret, &a);
      }
    }
    blst_fp12_sqr(&a, &a);
  }
}

static uint8_t make_fr_safe(uint8_t in) {
  return in & 0x7f;
}
static uint64_t make_fr_safe_u64(uint64_t in) {
  return in & 0x7fffffffffffffffUL;
}

// Precompute tables for fixed bases
static MultiscalarPrecomp *precompute_fixed_window(
                                 std::vector<blst_p1_affine> &points,
                                 size_t window_size) {
  MultiscalarPrecomp *ms = new MultiscalarPrecomp();

  ms->num_points    = points.size();
  ms->window_size   = window_size;
  ms->window_mask   = (1UL << window_size) - 1;
  ms->table_entries = (1UL << window_size) - 1;
  ms->tables        = new blst_p1_affine*[ms->num_points];

  da_pool->parMap(ms->num_points,
                    [&points, &ms](size_t tid) {
    blst_p1_affine *table = new blst_p1_affine[ms->table_entries];
    ms->tables[tid] = table;
    
    memcpy(&(table[0]), &(points[tid]), sizeof(blst_p1_affine));
    blst_p1 curPrecompPoint;
    blst_p1_from_affine(&curPrecompPoint, &points[tid]);
      
    for (limb_t j = 2; j <= ms->table_entries; j++) {
      blst_p1_add_or_double_affine(&curPrecompPoint, &curPrecompPoint,
                                   &points[tid]);
      blst_p1_to_affine(&(table[j - 1]), &curPrecompPoint);
    }
  });
  
  return ms;
}

// Multipoint scalar multiplication
// Only supports window sizes that evenly divide a limb and nbits!!
static void multiscalar(blst_p1* result, blst_scalar* k,
                        MultiscalarPrecomp *precompTable,
                        int num_points, size_t nbits) {
  // TODO: support more bit sizes
  if (nbits % precompTable->window_size != 0 ||
      sizeof(uint64_t) * 8 % precompTable->window_size != 0) {
    printf("Unsupported multiscalar window size!\n");
    exit(1);
  }
  
  memcpy(result, &G1_INFINITY, sizeof(blst_p1));

  // nbits must be evenly divided by window_size!
  const size_t num_windows = ((nbits +
                               precompTable->window_size - 1) /
                              precompTable->window_size);
  int idx;

  // This version prefetches the next window and computes on the previous
  // window. 
  for (int i = num_windows - 1; i >= 0; i--) {
    const int bits_per_limb = sizeof(uint64_t) * 8;
    int limb = (i * precompTable->window_size) / bits_per_limb;
    int window_in_limb = i % (bits_per_limb / precompTable->window_size);
    
    for (size_t l = 0; l < precompTable->window_size; ++l) {
      blst_p1_double(result, result);
    }
    int prev_idx = 0;
    blst_p1_affine *prev_table = 0;
    blst_p1_affine *table = 0;
    for (int m = 0; m < num_points; ++m) {
      idx = (k[m].l[limb] >> (window_in_limb * precompTable->window_size)) &
        precompTable->window_mask;
      if (idx > 0) {
        table = precompTable->tables[m];
        __builtin_prefetch(&table[idx - 1]);
      }
      if (prev_idx > 0 && m > 0) {
        blst_p1_add_or_double_affine(result, result,
                                     &prev_table[prev_idx - 1]);
      } 
      prev_idx = idx;
      prev_table = table;
    }
    // Perform the final addition
    if (prev_idx > 0) {
      blst_p1_add_or_double_affine(result, result,
                                   &prev_table[prev_idx - 1]);
    }
  }
}

// A function for requesting scalar inputs for multiscalar.
// - s   - Should be populated with the scalar value
// - idx - The index of the scalar being requested
typedef std::function<void (blst_scalar *s, uint64_t idx)> ScalarGetter;

// Perform a threaded multiscalar multiplication and accumulation
// - k or getter should be non-null
static void par_multiscalar(size_t max_threads, blst_p1* result,
                            blst_scalar* k, ScalarGetter *getter,
                            MultiscalarPrecomp *precompTable,
                            size_t num_points, size_t nbits) {
  // The granularity of work, in points. When a thread gets work it will
  // gather chunk_size points, perform muliscalar on them, and accumulate
  // the result. This is more efficient than evenly dividing the work among
  // threads because threads sometimes get preempted. When that happens
  // these long pole threads hold up progress across the board resulting in
  // occasional long delays. 
  size_t chunk_size = 16; // TUNEABLE
  if (num_points > 1024) {
    chunk_size = 256;
  }

  memset(result, 0, sizeof(*result));
  size_t num_threads = std::min(max_threads,
                                (num_points + chunk_size - 1) / chunk_size);
  std::vector<blst_p1> acc_intermediates(num_threads);
  memset(acc_intermediates.data(), 0, sizeof(blst_p1)*num_threads);

  // Work item counter - each thread will take work by incrementing
  std::atomic<size_t> work(0);

  da_pool->parMap(num_threads, [&acc_intermediates,
                                &k, &getter, &precompTable, &work,
                                num_threads, num_points, chunk_size, nbits]
                  (size_t tid) {
    // Temporary storage for scalars
    std::vector<blst_scalar> scalar_storage(chunk_size);
    // Thread result accumulation
    blst_p1 thr_result;
    memset(&thr_result, 0, sizeof(thr_result));
    
    while (true) {
      size_t i = work++;
      size_t start_idx = i * chunk_size;
      if (start_idx >= num_points) {
        break;
      }

      size_t end_idx = start_idx + chunk_size;
      if (end_idx > num_points) {
        end_idx = num_points;
      }
      size_t num_items = end_idx - start_idx;
      
      blst_scalar *scalars;
      if (k != NULL) {
        scalars = &k[start_idx];
      } else {
        memset(scalar_storage.data(), 0, sizeof(blst_scalar)*chunk_size);
        for (size_t i = start_idx; i < end_idx; i++) {
          (*getter)(&scalar_storage[i - start_idx], i);
        }
        scalars = scalar_storage.data();
      }
      MultiscalarPrecomp subset = precompTable->at_point(start_idx);
      blst_p1 acc;
      multiscalar(&acc, scalars,
                  &subset, num_items, nbits);
      blst_p1_add_or_double(&thr_result, &thr_result, &acc);
    }
    memcpy(&acc_intermediates[tid], &thr_result, sizeof(thr_result));
  });
  // Accumulate thread results
  for (size_t i = 0; i < num_threads; i++) {
    blst_p1_add_or_double(result, result, &acc_intermediates[i]);
  }
}

// Read verification key file and perform basic precomputations
// TODO: make static?
BLST_ERROR read_vk_file(VERIFYING_KEY** ret, std::string filename) {
  auto search = zkconfig.vk_cache.find(filename);
  if (search != zkconfig.vk_cache.end()) {
    *ret = search->second;
    return BLST_SUCCESS;
  }
  VERIFYING_KEY* vk = new VERIFYING_KEY();
  *ret = vk;
  
  std::ifstream vk_file;
  vk_file.open(filename, std::ios::binary | std::ios::in);

  if (!vk_file) {
    std::cout << "read_vk_file read error: " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  unsigned char g1_bytes[96];
  unsigned char g2_bytes[192];
  uint32_t      ic_len_rd;
  uint32_t      ic_len;
  BLST_ERROR    err;

  // TODO: do we need to group check these? 
  
  vk_file.read((char*) g1_bytes, sizeof(g1_bytes));
  err = blst_p1_deserialize(&vk->alpha_g1, g1_bytes);
  if (err != BLST_SUCCESS) return err;

  vk_file.read((char*) g1_bytes, sizeof(g1_bytes));
  err = blst_p1_deserialize(&vk->beta_g1, g1_bytes);
  if (err != BLST_SUCCESS) return err;

  vk_file.read((char*) g2_bytes, sizeof(g2_bytes));
  err = blst_p2_deserialize(&vk->beta_g2, g2_bytes);
  if (err != BLST_SUCCESS) return err;

  vk_file.read((char*) g2_bytes, sizeof(g2_bytes));
  err = blst_p2_deserialize(&vk->gamma_g2, g2_bytes);
  if (err != BLST_SUCCESS) return err;

  vk_file.read((char*) g1_bytes, sizeof(g1_bytes));
  err = blst_p1_deserialize(&vk->delta_g1, g1_bytes);
  if (err != BLST_SUCCESS) return err;

  vk_file.read((char*) g2_bytes, sizeof(g2_bytes));
  err = blst_p2_deserialize(&vk->delta_g2, g2_bytes);
  if (err != BLST_SUCCESS) return err;

  vk_file.read((char*) &ic_len_rd, sizeof(ic_len_rd));
  ic_len = ((ic_len_rd >> 24) & 0xff)       |
           ((ic_len_rd << 8)  & 0xff0000)   |
           ((ic_len_rd >> 8)  & 0xff00)     |
           ((ic_len_rd << 24) & 0xff000000);

  vk->ic.reserve(ic_len);

  while (ic_len--) {
    blst_p1_affine cur_ic_aff;
    vk_file.read((char*) g1_bytes, sizeof(g1_bytes));
    err = blst_p1_deserialize(&cur_ic_aff, g1_bytes);
    if (err != BLST_SUCCESS) return err;
    vk->ic.push_back(cur_ic_aff);
  }

  if (!vk_file) {
    std::cout << "read_vk_file read too much " << filename << std::endl;
    return BLST_BAD_ENCODING;
  }

  vk_file.close();

  blst_miller_loop(&vk->alpha_g1_beta_g2, &vk->beta_g2, &vk->alpha_g1);
  blst_final_exp(&vk->alpha_g1_beta_g2, &vk->alpha_g1_beta_g2);
  blst_precompute_lines(vk->delta_q_lines, &vk->delta_g2);
  blst_precompute_lines(vk->gamma_q_lines, &vk->gamma_g2);

  blst_p2        neg_delta_g2;
  blst_p2_affine neg_delta_g2_aff;
  blst_p2_from_affine(&neg_delta_g2, &vk->delta_g2);
  blst_p2_cneg(&neg_delta_g2, 1);
  blst_p2_to_affine(&neg_delta_g2_aff, &neg_delta_g2);
  blst_precompute_lines(vk->neg_delta_q_lines, &neg_delta_g2_aff);

  blst_p2        neg_gamma_g2;
  blst_p2_affine neg_gamma_g2_aff;
  blst_p2_from_affine(&neg_gamma_g2, &vk->gamma_g2);
  blst_p2_cneg(&neg_gamma_g2, 1);
  blst_p2_to_affine(&neg_gamma_g2_aff, &neg_gamma_g2);
  blst_precompute_lines(vk->neg_gamma_q_lines, &neg_gamma_g2_aff);

  const size_t WINDOW_SIZE   = 8;
  vk->multiscalar = precompute_fixed_window(vk->ic, WINDOW_SIZE);

  if (err == BLST_SUCCESS) {
    zkconfig.vk_cache[filename] = vk;
  }
  return err;
}

// Verify batch proofs individually
static bool verify_batch_proof_ind(PROOF *proofs, size_t num_proofs,
                                   blst_scalar *public_inputs,
                                   ScalarGetter *getter,
                                   size_t num_inputs,
                                   VERIFYING_KEY& vk) {
  if ((num_inputs + 1) != vk.ic.size()) {
    return false;
  }
  
  bool result(true);
  for (uint64_t j = 0; j < num_proofs; ++j) {
    // Start the two independent miller loops
    blst_fp12 ml_a_b;
    blst_fp12 ml_all;
    std::vector<std::mutex> thread_complete(2);
    for (size_t i = 0; i < 2; i++) {
      thread_complete[i].lock();
    }
    da_pool->schedule([j, &ml_a_b, &proofs, &thread_complete]() {
      blst_miller_loop(&ml_a_b, &proofs[j].b_g2, &proofs[j].a_g1);
      thread_complete[0].unlock();
    });
    da_pool->schedule([j, &ml_all, &vk, &proofs, &thread_complete]() {
      blst_miller_loop_lines(&ml_all, vk.neg_delta_q_lines, &proofs[j].c_g1);
      thread_complete[1].unlock();
    });

    // Multiscalar
    blst_scalar *scalars_addr = NULL;
    if (public_inputs != NULL) {
      scalars_addr = &public_inputs[j * num_inputs];
    }
    blst_p1 acc;
    MultiscalarPrecomp subset = vk.multiscalar->at_point(1);
    par_multiscalar(da_pool->size(), &acc,
                    scalars_addr, getter,
                    &subset, num_inputs, 256);
    blst_p1_add_or_double_affine(&acc, &acc, &vk.ic[0]);
    blst_p1_affine acc_aff;
    blst_p1_to_affine(&acc_aff, &acc);

    // acc miller loop
    blst_fp12 ml_acc;
    blst_miller_loop_lines(&ml_acc, vk.neg_gamma_q_lines, &acc_aff);

    // Gather the threaded miller loops
    for (size_t i = 0; i < 2; i++) {
      thread_complete[i].lock();
    }
    blst_fp12_mul(&ml_acc, &ml_acc, &ml_a_b);
    blst_fp12_mul(&ml_all, &ml_all, &ml_acc);
    blst_final_exp(&ml_all, &ml_all);
    
    result &= blst_fp12_is_equal(&ml_all, &vk.alpha_g1_beta_g2);
  }
  return result;
}

// Verify batch proofs
// TODO: make static?
bool verify_batch_proof_inner(PROOF *proofs, size_t num_proofs,
                              blst_scalar *public_inputs, size_t num_inputs,
                              VERIFYING_KEY& vk,
                              blst_scalar *rand_z, size_t nbits) {
  // TODO: best size for this? 
  if (num_proofs < 2) {
    return verify_batch_proof_ind(proofs, num_proofs,
                                  public_inputs, NULL, num_inputs, vk);
  }

  if ((num_inputs + 1) != vk.ic.size()) {
    return false;
  }

  // Names for the threads
  const int ML_D_THR     = 0;
  const int ACC_AB_THR   = 1;
  const int Y_THR        = 2;
  const int ML_G_THR     = 3;
  const int WORK_THREADS = 4;
  
  std::vector<std::mutex> thread_complete(WORK_THREADS);
  for (size_t i = 0; i < WORK_THREADS; i++) {
    thread_complete[i].lock();
  }

  // This is very fast and needed by two threads so can live here
  // accum_y = sum(zj)
  blst_fr accum_y; // used in multi add and below
  memcpy(&accum_y, &rand_z[0], sizeof(blst_fr));
  for (uint64_t j = 1; j < num_proofs; ++j) {
    blst_fr_add(&accum_y, &accum_y, (blst_fr *)&rand_z[j]);
  }

  // THREAD 3
  blst_fp12 ml_g;
  da_pool->schedule([num_proofs, num_inputs, &accum_y,
                       &vk, &public_inputs, &rand_z, &ml_g,
                       &thread_complete, WORK_THREADS]() {
    auto scalar_getter = [num_proofs, num_inputs,
                          &accum_y, &rand_z, &public_inputs]
      (blst_scalar *s, size_t idx) {
      if (idx == 0) {
        memcpy(s, &accum_y, sizeof(blst_scalar));
      } else {
        idx--;
        // sum(zj * aj,i)
        blst_fr cur_sum, cur_mul, pi_mont, rand_mont;
        blst_fr_to(&rand_mont, (blst_fr *)&rand_z[0]);
        blst_fr_to(&pi_mont, (blst_fr *)&public_inputs[idx]);
        blst_fr_mul(&cur_sum, &rand_mont, &pi_mont);
        for (uint64_t j = 1; j < num_proofs; ++j) {
          blst_fr_to(&rand_mont, (blst_fr *)&rand_z[j]);
          blst_fr_to(&pi_mont, (blst_fr *)&public_inputs[j * num_inputs + idx]);
          blst_fr_mul(&cur_mul, &rand_mont, &pi_mont);
          blst_fr_add(&cur_sum, &cur_sum, &cur_mul);
        }
        
        blst_fr_from((blst_fr *)s, &cur_sum);
      }
    };
    ScalarGetter getter(scalar_getter);
    
    // sum_i(accum_g * psi)
    blst_p1 acc_g_psi;
    par_multiscalar(da_pool->size(),
                    &acc_g_psi, NULL, &getter,
                    vk.multiscalar, num_inputs + 1, 256);
    blst_p1_affine acc_g_psi_aff;
    blst_p1_to_affine(&acc_g_psi_aff, &acc_g_psi);

    // ml(acc_g_psi, vk.gamma)
    blst_miller_loop_lines(&ml_g, &(vk.gamma_q_lines[0]), &acc_g_psi_aff);

    thread_complete[ML_G_THR].unlock();
  });

  // THREAD 1
  blst_fp12 ml_d;
  da_pool->schedule([num_proofs, nbits,
                       &proofs, &rand_z, &vk, &thread_complete,
                       &ml_d]() {
    blst_p1 acc_d;
    std::vector<blst_p1_affine> points;
    points.resize(num_proofs);
    for (size_t i = 0; i < num_proofs; i++) {
      memcpy(&points[i], &proofs[i].c_g1, sizeof(blst_p1_affine));
    }
    MultiscalarPrecomp *pre = precompute_fixed_window(points, 1);
    multiscalar(&acc_d, rand_z,
                pre, num_proofs, nbits);
    delete pre;
    
    blst_p1_affine acc_d_aff;
    blst_p1_to_affine(&acc_d_aff, &acc_d);
    blst_miller_loop_lines(&ml_d, &(vk.delta_q_lines[0]), &acc_d_aff);

    thread_complete[ML_D_THR].unlock();
  });
  
  
  // THREAD 2
  blst_fp12 acc_ab;
  da_pool->schedule([num_proofs, nbits,
                       &vk, &proofs, &rand_z,
                       &acc_ab,
                       &thread_complete, WORK_THREADS]() {
    std::vector<blst_fp12> accum_ab_mls;
    accum_ab_mls.resize(num_proofs);
    da_pool->parMap(num_proofs, [num_proofs, &proofs, &rand_z,
                                   &accum_ab_mls, nbits]
                      (size_t j) {
      blst_p1 mul_a;
      blst_p1_mult_affine(&mul_a, &proofs[j].a_g1, &rand_z[j], nbits);
      blst_p1_affine acc_a_aff;
      blst_p1_to_affine(&acc_a_aff, &mul_a);
      
      blst_p2 cur_neg_b;
      blst_p2_affine cur_neg_b_aff;
      blst_p2_from_affine(&cur_neg_b, &proofs[j].b_g2);
      blst_p2_cneg(&cur_neg_b, 1);
      blst_p2_to_affine(&cur_neg_b_aff, &cur_neg_b);
      
      blst_miller_loop(&accum_ab_mls[j], &cur_neg_b_aff, &acc_a_aff);
    }, da_pool->size() - WORK_THREADS);

    // accum_ab = mul_j(ml((zj*proof_aj), -proof_bj))
    memcpy(&acc_ab, &accum_ab_mls[0], sizeof(acc_ab));
    for (uint64_t j = 1; j < num_proofs; ++j) {
      blst_fp12_mul(&acc_ab, &acc_ab, &accum_ab_mls[j]);
    }

    thread_complete[ACC_AB_THR].unlock();
  });

  // THREAD 0
  blst_fp12 y;
  da_pool->schedule([&y, &accum_y, &vk, &thread_complete]() {
    // -accum_y
    blst_fr accum_y_neg;
    blst_fr_cneg(&accum_y_neg, &accum_y, 1);
    
    // Y^-accum_y
    blst_fp12_pow(&y, &vk.alpha_g1_beta_g2, (blst_scalar *)&accum_y_neg);

    thread_complete[Y_THR].unlock();
  });

  blst_fp12 ml_all;
  thread_complete[ML_D_THR].lock();
  thread_complete[ACC_AB_THR].lock();
  blst_fp12_mul(&ml_all, &acc_ab, &ml_d);

  thread_complete[ML_G_THR].lock();
  blst_fp12_mul(&ml_all, &ml_all, &ml_g);
  blst_final_exp(&ml_all, &ml_all);

  thread_complete[Y_THR].lock();
  bool res = blst_fp12_is_equal(&ml_all, &y);
  
  return res;
}

// External entry point for proof verification
// - proof_bytes   - proof(s) in byte form, 192 bytes per proof
// - num proofs    - number of proofs provided
// - public_inputs - flat array of inputs for all proofs
// - num_inputs    - number of public inputs per proof (all same size)
// - rand_z        - random scalars for combining proofs
// - nbits         - bit size of the scalars. all unused bits must be zero.
// - vk_path       - path to the verifying key in the file system
// - vk_len        - length of vk_path, in bytes, not including null termination
// TODO: change public_inputs to scalar
bool verify_batch_proof_c(uint8_t *proof_bytes, size_t num_proofs,
                          blst_scalar *public_inputs, size_t num_inputs,
                          blst_scalar *rand_z, size_t nbits,
                          uint8_t *vk_path, size_t vk_len) {
  std::vector<PROOF> proofs;
  proofs.resize(num_proofs);

  for (size_t i = 0; i < num_inputs * num_proofs; i++) {
    (public_inputs)[i].l[3] = make_fr_safe_u64((public_inputs)[i].l[3]);
  }

  // Decompress and group check in parallel
  std::atomic<bool> ok(true);
  da_pool->parMap(num_proofs * 3, [num_proofs, proof_bytes, &proofs, &ok]
                    (size_t i) {
    // Work on all G2 points first since they are more expensive. Avoid
    // having a long pole due to g2 starting late.
    size_t c = i / num_proofs;
    size_t p = i % num_proofs;
    PROOF *proof = &proofs[p];
    size_t offset = PROOF_BYTES * p;
    switch(c) {
    case 0:
      if (blst_p2_uncompress(&proof->b_g2, proof_bytes + offset +
                             P1_COMPRESSED_BYTES) !=
          BLST_SUCCESS) {
        ok = false;
      }
      if (!blst_p2_affine_in_g2(&proof->b_g2)) {
        ok = false;
      }
      break;
    case 1:
      if (blst_p1_uncompress(&proof->a_g1, proof_bytes + offset) !=
          BLST_SUCCESS) {
        ok = false;
      }
      if (!blst_p1_affine_in_g1(&proof->a_g1)) {
        ok = false;
      }
      break;
    case 2:
      if (blst_p1_uncompress(&proof->c_g1, proof_bytes + offset +
                             P1_COMPRESSED_BYTES + P2_COMPRESSED_BYTES) !=
          BLST_SUCCESS) {
        ok = false;
      }
      if (!blst_p1_affine_in_g1(&proof->c_g1)) {
        ok = false;
      }
      break;
    }
  });
  if (!ok) {
    return false;
  }
  
  VERIFYING_KEY *vk;
  std::string vk_str((const char *)vk_path, vk_len);
  read_vk_file(&vk, vk_str);
  
  int res = verify_batch_proof_inner(proofs.data(), num_proofs,
                                     public_inputs, num_inputs,
                                     *vk,
                                     rand_z, nbits);
  return res;
}

// Window post leaf challenge
uint64_t generate_leaf_challenge(uint8_t *buf,
                                 uint64_t sector_id,
                                 uint64_t leaf_challenge_index,
                                 uint64_t sector_mask) {
  memcpy(buf + 32, &sector_id,  sizeof(sector_id));
  memcpy(buf + 40, &leaf_challenge_index, sizeof(leaf_challenge_index));

  unsigned int h[8] = { 0x6a09e667U, 0xbb67ae85U, 0x3c6ef372U, 0xa54ff53aU,
                        0x510e527fU, 0x9b05688cU, 0x1f83d9abU, 0x5be0cd19U };
  blst_sha256_block(h, buf, 1);

  unsigned int swap_h0 = ((h[0] & 0xFF000000) >> 24) |
                         ((h[0] & 0x00FF0000) >> 8)  |
                         ((h[0] & 0x0000FF00) << 8)  |
                         ((h[0] & 0x000000FF) << 24);

  return swap_h0 & sector_mask;
}

extern "C" {
int verify_window_post_go(uint8_t *randomness, uint64_t sector_mask, 
                          uint8_t *sector_comm_r, uint64_t *sector_ids,
                          size_t num_sectors,
                          size_t challenge_count,
                          uint8_t *proof_bytes, size_t num_proofs,
                          char *vkfile) {
  if (num_proofs > 1) {
    printf("WARNING: only single proof supported for window verify!\n");
    return 0;
  }

  randomness[31] = make_fr_safe(randomness[31]);

  PROOF proof;
  blst_p1_uncompress(&proof.a_g1, proof_bytes);
  blst_p2_uncompress(&proof.b_g2, proof_bytes + P1_COMPRESSED_BYTES);
  blst_p1_uncompress(&proof.c_g1, proof_bytes +
                     P1_COMPRESSED_BYTES + P2_COMPRESSED_BYTES);

  bool result = blst_p1_affine_in_g1(&proof.a_g1);
  result &= blst_p2_affine_in_g2(&proof.b_g2);
  result &= blst_p1_affine_in_g1(&proof.c_g1);
  if (!result) {
    return 0;
  }
  
  // Set up the sha buffer for generating leaf nodes
  unsigned char base_buf[64] = {0};
  memcpy(base_buf, randomness, 32);
  base_buf[48] = 0x80; // Padding
  base_buf[62] = 0x01; // Length = 0x180 = 384b
  base_buf[63] = 0x80;

  // Parallel leaf node generation
  auto scalar_getter = [randomness, sector_ids, sector_comm_r,
                        challenge_count, num_sectors, sector_mask,
                        base_buf](blst_scalar *s, size_t idx) {
    uint64_t sector = idx / (challenge_count + 1);
    uint64_t challenge_num = idx % (challenge_count + 1);
    
    unsigned char buf[64];
    memcpy(buf, base_buf, sizeof(buf));

    if (challenge_num == 0) {
      int top_byte = sector * 32 + 31;
      sector_comm_r[top_byte] = make_fr_safe(sector_comm_r[top_byte]);
      blst_scalar_from_lendian(s, &sector_comm_r[sector * 32]);
    } else {
      challenge_num--; // Decrement to account for comm_r
      uint64_t challenge_idx = sector * challenge_count + challenge_num;
      uint64_t challenge = generate_leaf_challenge(buf,
                                                   sector_ids[sector],
                                                   challenge_idx, sector_mask);
      uint64_t a[4];
      memset(a, 0, 4 * sizeof(uint64_t));
      a[0] = challenge;
      blst_scalar_from_uint64(s, a);
    }
  };
  ScalarGetter getter(scalar_getter);

  uint64_t inputs_size = num_sectors * (challenge_count + 1);

  VERIFYING_KEY *vk;
  read_vk_file(&vk, std::string(vkfile));
  
  bool res = verify_batch_proof_ind(&proof, 1, NULL, &getter, inputs_size, *vk);
  return res;
}
}
