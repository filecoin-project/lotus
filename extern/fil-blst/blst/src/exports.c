/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * Why this file? Overall goal is to ensure that all internal calls
 * remain internal after linking application. This is to both
 *
 * a) minimize possibility of external name conflicts (since all
 *    non-blst-prefixed and [assembly subroutines] remain static);
 * b) preclude possibility of unintentional internal reference
 *    overload in shared library context (one can achieve same
 *    effect with -Bsymbolic, but we don't want to rely on end-user
 *    to remember to use it);
 */

#include "fields.h"

/*
 * BLS12-381-specifc Fr shortcuts to assembly.
 */
void blst_fr_add(vec256 ret, const vec256 a, const vec256 b)
{   add_mod_256(ret, a, b, BLS12_381_r);   }

void blst_fr_sub(vec256 ret, const vec256 a, const vec256 b)
{   sub_mod_256(ret, a, b, BLS12_381_r);   }

void blst_fr_mul_by_3(vec256 ret, const vec256 a)
{   mul_by_3_mod_256(ret, a, BLS12_381_r);   }

void blst_fr_lshift(vec256 ret, const vec256 a, size_t count)
{   lshift_mod_256(ret, a, count, BLS12_381_r);   }

void blst_fr_rshift(vec256 ret, const vec256 a, size_t count)
{   rshift_mod_256(ret, a, count, BLS12_381_r);   }

void blst_fr_mul(vec256 ret, const vec256 a, const vec256 b)
{   mul_mont_sparse_256(ret, a, b, BLS12_381_r, r0);   }

void blst_fr_sqr(vec256 ret, const vec256 a)
{   sqr_mont_sparse_256(ret, a, BLS12_381_r, r0);   }

void blst_fr_cneg(vec256 ret, const vec256 a, size_t flag)
{   cneg_mod_256(ret, a, flag, BLS12_381_r);   }

void blst_fr_to(vec256 ret, const vec256 a)
{   mul_mont_sparse_256(ret, a, BLS12_381_rRR, BLS12_381_r, r0);   }

void blst_fr_from(vec256 ret, const vec256 a)
{   from_mont_256(ret, a, BLS12_381_r, r0);   }

void blst_fr_eucl_inverse(vec256 ret, const vec256 a)
{   eucl_inverse_mod_256(ret, a, BLS12_381_r, BLS12_381_rRR);   }

/*
 * BLS12-381-specifc Fp shortcuts to assembly.
 */
void blst_fp_add(vec384 ret, const vec384 a, const vec384 b)
{   add_fp(ret, a, b);   }

void blst_fp_sub(vec384 ret, const vec384 a, const vec384 b)
{   sub_fp(ret, a, b);   }

void blst_fp_mul_by_3(vec384 ret, const vec384 a)
{   mul_by_3_fp(ret, a);   }

void blst_fp_mul_by_8(vec384 ret, const vec384 a)
{   mul_by_8_fp(ret, a);   }

void blst_fp_lshift(vec384 ret, const vec384 a, size_t count)
{   lshift_fp(ret, a, count);   }

void blst_fp_mul(vec384 ret, const vec384 a, const vec384 b)
{   mul_fp(ret, a, b);   }

void blst_fp_sqr(vec384 ret, const vec384 a)
{   sqr_fp(ret, a);   }

void blst_fp_cneg(vec384 ret, const vec384 a, size_t flag)
{   cneg_fp(ret, a, flag);   }

void blst_fp_eucl_inverse(vec384 ret, const vec384 a)
{   eucl_inverse_fp(ret, a);   }

void blst_fp_to(vec384 ret, const vec384 a)
{   mul_fp(ret, a, BLS12_381_RR);   }

void blst_fp_from(vec384 ret, const vec384 a)
{   from_fp(ret, a);   }

/*
 * Fp serialization/deserialization.
 */
void blst_fp_from_uint32(vec384 ret, const unsigned int a[12])
{
    if (sizeof(limb_t) == 8) {
        int i;
        for (i = 0; i < 6; i++)
            ret[i] = a[2*i] | ((limb_t)a[2*i+1] << 32);
        a = (const unsigned int *)ret;
    }
    mul_fp(ret, (const limb_t *)a, BLS12_381_RR);
}

void blst_uint32_from_fp(unsigned int ret[12], const vec384 a)
{
    if (sizeof(limb_t) == 4) {
        from_fp((limb_t *)ret, a);
    } else {
        vec384 out;
        int i;

        from_fp(out, a);
        for (i = 0; i < 6; i++) {
            limb_t limb = out[i];
            ret[2*i]   = (unsigned int)limb;
            ret[2*i+1] = (unsigned int)(limb >> 32);
        }
    }
}

void blst_fp_from_uint64(vec384 ret, const unsigned long long a[6])
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if (sizeof(limb_t) == 4 && !is_endian.little) {
        int i;
        for (i = 0; i < 6; i++) {
            unsigned long long limb = a[i];
            ret[2*i]   = (limb_t)limb;
            ret[2*i+1] = (limb_t)(limb >> 32);
        }
        a = (const unsigned long long *)ret;
    }
    mul_fp(ret, (const limb_t *)a, BLS12_381_RR);
}

void blst_uint64_from_fp(unsigned long long ret[6], const vec384 a)
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if (sizeof(limb_t) == 8 || is_endian.little) {
        from_fp((limb_t *)ret, a);
    } else {
        vec384 out;
        int i;

        from_fp(out, a);
        for (i = 0; i < 6; i++)
            ret[i] = out[2*i] | ((unsigned long long)out[2*i+1] << 32);
    }
}

void blst_fp_from_bendian(vec384 ret, const unsigned char a[48])
{
    vec384 out;

    limbs_from_be_bytes(out, a, sizeof(vec384));
    mul_fp(ret, out, BLS12_381_RR);
}

void blst_bendian_from_fp(unsigned char ret[48], const vec384 a)
{
    vec384 out;

    from_fp(out, a);
    be_bytes_from_limbs(ret, out, sizeof(vec384));
}

void blst_fp_from_lendian(vec384 ret, const unsigned char a[48])
{
    vec384 out;

    limbs_from_le_bytes(out, a, sizeof(vec384));
    mul_fp(ret, out, BLS12_381_RR);
}

void blst_lendian_from_fp(unsigned char ret[48], const vec384 a)
{
    vec384 out;

    from_fp(out, a);
    le_bytes_from_limbs(ret, out, sizeof(vec384));
}

/*
 * BLS12-381-specifc Fp2 shortcuts to assembly.
 */
void blst_fp2_add(vec384x ret, const vec384x a, const vec384x b)
{   add_fp2(ret, a, b);   }

void blst_fp2_sub(vec384x ret, const vec384x a, const vec384x b)
{   sub_fp2(ret, a, b);   }

void blst_fp2_mul_by_3(vec384x ret, const vec384x a)
{   mul_by_3_fp2(ret, a);   }

void blst_fp2_mul_by_8(vec384x ret, const vec384x a)
{   mul_by_8_fp2(ret, a);   }

void blst_fp2_lshift(vec384x ret, const vec384x a, size_t count)
{   lshift_fp2(ret, a, count);    }

void blst_fp2_mul(vec384x ret, const vec384x a, const vec384x b)
{   mul_fp2(ret, a, b);   }

void blst_fp2_sqr(vec384x ret, const vec384x a)
{   sqr_fp2(ret, a);   }

void blst_fp2_cneg(vec384x ret, const vec384x a, size_t flag)
{   cneg_fp2(ret, a, flag);   }

/*
 * BLS12-381-specifc point operations.
 */
void blst_p1_add(POINTonE1 *out, const POINTonE1 *a, const POINTonE1 *b)
{   POINTonE1_add(out, a, b);   }

void blst_p1_add_or_double(POINTonE1 *out, const POINTonE1 *a,
                                           const POINTonE1 *b)
{   POINTonE1_dadd(out, a, b, NULL);   }

void blst_p1_add_affine(POINTonE1 *out, const POINTonE1 *a, const POINTonE1 *b)
{   POINTonE1_add_affine(out, a, b);   }

void blst_p1_add_or_double_affine(POINTonE1 *out, const POINTonE1 *a,
                                                  const POINTonE1 *b)
{   POINTonE1_dadd_affine(out, a, b);   }

void blst_p1_double(POINTonE1 *out, const POINTonE1 *a)
{   POINTonE1_double(out, a);   }

void blst_p1_mult(POINTonE1 *out, const POINTonE1 *a,
                                  const limb_t *scalar, size_t nbits)
{   POINTonE1_mult_w5(out, a, scalar, nbits);   }

limb_t blst_p1_affine_is_equal(const POINTonE1_affine *a,
                               const POINTonE1_affine *b)
{   return vec_is_equal(a, b, sizeof(*a));   }

void blst_p2_add(POINTonE2 *out, const POINTonE2 *a, const POINTonE2 *b)
{   POINTonE2_add(out, a, b);   }

void blst_p2_add_or_double(POINTonE2 *out, const POINTonE2 *a,
                                           const POINTonE2 *b)
{   POINTonE2_dadd(out, a, b, NULL);   }

void blst_p2_add_affine(POINTonE2 *out, const POINTonE2 *a, const POINTonE2 *b)
{   POINTonE2_add_affine(out, a, b);   }

void blst_p2_add_or_double_affine(POINTonE2 *out, const POINTonE2 *a,
                                                  const POINTonE2 *b)
{   POINTonE2_dadd_affine(out, a, b);   }

void blst_p2_double(POINTonE2 *out, const POINTonE2 *a)
{   POINTonE2_double(out, a);   }

void blst_p2_mult(POINTonE2 *out, const POINTonE2 *a,
                                  const limb_t *scalar, size_t nbits)
{   POINTonE2_mult_w5(out, a, scalar, nbits);   }

limb_t blst_p2_affine_is_equal(const POINTonE2_affine *a,
                               const POINTonE2_affine *b)
{   return vec_is_equal(a, b, sizeof(*a));   }

/*
 * Scalar serialization/deseriazation
 */
#ifdef __UINTPTR_TYPE__
typedef __UINTPTR_TYPE__ uptr_t;
#else
typedef const void *uptr_t;
#endif

void blst_scalar_from_uint32(vec256 ret, const unsigned int a[8])
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if ((uptr_t)ret==(uptr_t)a && (sizeof(limb_t)==4 || is_endian.little))
        return;

    if (sizeof(limb_t)==4) {
        vec_copy(ret, a, sizeof(vec256));
    } else {
        int i;
        for (i = 0; i < 4; i++)
            ret[i] = a[2*i] | ((limb_t)a[2*i+1] << 32);
    }
}

void blst_uint32_from_scalar(unsigned int ret[8], const vec256 a)
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if ((uptr_t)ret==(uptr_t)a && (sizeof(limb_t)==4 || is_endian.little))
        return;

    if (sizeof(limb_t)==4) {
        vec_copy(ret, a, sizeof(vec256));
    } else {
        int i;
        for (i = 0; i < 4; i++) {
            limb_t limb = a[i];
            ret[2*i]   = (unsigned int)limb;
            ret[2*i+1] = (unsigned int)(limb >> 32);
        }
    }
}

void blst_scalar_from_uint64(vec256 ret, const unsigned long long a[4])
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if (sizeof(limb_t)==8 || is_endian.little) {
        if ((uptr_t)ret != (uptr_t)a)
            vec_copy(ret, a, sizeof(vec256));
    } else {
        int i;
        for (i = 0; i < 4; i++) {
            unsigned long long limb = a[i];
            ret[2*i]   = (limb_t)limb;
            ret[2*i+1] = (limb_t)(limb >> 32);
        }
    }
}

void blst_uint64_from_scalar(unsigned long long ret[4], const vec256 a)
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if (sizeof(limb_t)==8 || is_endian.little) {
        if ((uptr_t)ret != (uptr_t)a)
            vec_copy(ret, a, sizeof(vec256));
    } else {
        int i;
        for (i = 0; i < 4; i++)
            ret[i] = a[2*i] | ((unsigned long long)a[2*i+1] << 32);
    }
}

void blst_scalar_from_bendian(vec256 ret, const unsigned char a[32])
{
    vec256 out;
    limbs_from_be_bytes(out, a, sizeof(out));
    vec_copy(ret, out, sizeof(out));
}

void blst_bendian_from_scalar(unsigned char ret[32], const vec256 a)
{
    vec256 out;
    vec_copy(out, a, sizeof(out));
    be_bytes_from_limbs(ret, out, sizeof(out));
}

void blst_scalar_from_lendian(vec256 ret, const unsigned char a[32])
{
    vec256 out;
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if ((uptr_t)ret==(uptr_t)a && is_endian.little)
        return;

    limbs_from_le_bytes(out, a, sizeof(out));
    vec_copy(ret, out, sizeof(out));
}

void blst_lendian_from_scalar(unsigned char ret[32], const vec256 a)
{
    vec256 out;
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if ((uptr_t)ret==(uptr_t)a && is_endian.little)
        return;

    vec_copy(out, a, sizeof(out));
    le_bytes_from_limbs(ret, out, sizeof(out));
}

limb_t blst_scalar_fr_check(const vec256 a)
{
    vec256 zero = { 0 };

    add_mod_256(zero, zero, a, BLS12_381_r);
    return vec_is_equal(zero, a, sizeof(zero));
}

void blst_fr_from_uint64(vec256 ret, const unsigned long long a[4])
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if (sizeof(limb_t) == 4 && !is_endian.little) {
        int i;
        for (i = 0; i < 4; i++) {
            unsigned long long limb = a[i];
            ret[2*i]   = (limb_t)limb;
            ret[2*i+1] = (limb_t)(limb >> 32);
        }
        a = (const unsigned long long *)ret;
    }
    mul_mont_sparse_256(ret, (const limb_t *)a, BLS12_381_rRR, BLS12_381_r, r0);
}

void blst_uint64_from_fr(unsigned long long ret[4], const vec256 a)
{
    const union {
        long one;
        char little;
    } is_endian = { 1 };

    if (sizeof(limb_t) == 8 || is_endian.little) {
        from_mont_256((limb_t *)ret, a, BLS12_381_r, r0);
    } else {
        vec256 out;
        int i;

        from_mont_256(out, a, BLS12_381_r, r0);
        for (i = 0; i < 4; i++)
            ret[i] = out[2*i] | ((unsigned long long)out[2*i+1] << 32);
    }
}

void blst_fr_from_scalar(vec256 ret, const vec256 a)
{   mul_mont_sparse_256(ret, a, BLS12_381_rRR, BLS12_381_r, r0);   }

void blst_scalar_from_fr(vec256 ret, const vec256 a)
{   from_mont_256(ret, a, BLS12_381_r, r0);   }

/*
 * Test facilitator
 */
static unsigned char nibble(unsigned char c)
{
    if (c >= '0' && c <= '9')
        return c - '0';
    else if (c >= 'a' && c <= 'f')
        return 10 + c - 'a';
    else if (c >= 'A' && c <= 'F')
        return 10 + c - 'A';
    else
        return 16;
}

static void limbs_from_hexascii(limb_t *ret, size_t sz,
                                const unsigned char *hex)
{
    size_t len;
    limb_t limb = 0;

    if (hex[0]=='0' && (hex[1]=='x' || hex[1]=='X'))
        hex += 2;

    for (len = 0; len<2*sz && nibble(hex[len])<16; len++) ;

    vec_zero(ret, sz);

    while(len--) {
        limb <<= 4;
        limb |= nibble(*hex++);
        if (len % (2*sizeof(limb_t)) == 0)
            ret[len / (2*sizeof(limb_t))] = limb;
    }
}

void blst_scalar_from_hexascii(vec256 ret, const unsigned char *hex)
{   limbs_from_hexascii(ret, sizeof(vec256), hex);   }

void blst_fp_from_hexascii(vec384 ret, const unsigned char *hex)
{
    limbs_from_hexascii(ret, sizeof(vec384), hex);
    mul_fp(ret, ret, BLS12_381_RR);
}
