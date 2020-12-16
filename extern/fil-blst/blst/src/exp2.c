/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "vect.h"
#include "fields.h"

static void reciprocal_fp2(vec384x out, const vec384x inp)
{
    vec384 t0, t1;

    /*
     * |out| = 1/(a + b*i) = a/(a^2+b^2) - b/(a^2+b^2)*i
     */
    sqr_fp(t0, inp[0]);
    sqr_fp(t1, inp[1]);
    add_fp(t0, t0, t1);
    reciprocal_fp(t1, t0);
    mul_fp(out[0], inp[0], t1);
    mul_fp(out[1], inp[1], t1);
    neg_fp(out[1], out[1]);
}

/*
 * |out| = |inp|^|pow|, small footprint, public exponent
 */
static void exp_mont_384x(vec384x out, const vec384x inp, const limb_t *pow,
                          size_t pow_bits, const vec384 p, limb_t n0)
{
    vec384x ret;

    vec_copy(ret, inp, sizeof(ret));  /* |ret| = |inp|^1 */
    --pow_bits; /* most significant bit is accounted for, skip over */
    while (pow_bits--) {
        sqr_mont_384x(ret, ret, p, n0);
        if (is_bit_set(pow, pow_bits))
            mul_mont_384x(ret, ret, inp, p, n0);
    }
    vec_copy(out, ret, sizeof(ret));  /* |out| = |ret| */
}

#ifdef __OPTIMIZE_SIZE__
static void recip_sqrt_fp2_9mod16(vec384x out, const vec384x inp)
{
    static const limb_t BLS_12_381_P_2_minus_9_div_16[] = {
        TO_LIMB_T(0xb26aa00001c718e3), TO_LIMB_T(0xd7ced6b1d76382ea),
        TO_LIMB_T(0x3162c338362113cf), TO_LIMB_T(0x966bf91ed3e71b74),
        TO_LIMB_T(0xb292e85a87091a04), TO_LIMB_T(0x11d68619c86185c7),
        TO_LIMB_T(0xef53149330978ef0), TO_LIMB_T(0x050a62cfd16ddca6),
        TO_LIMB_T(0x466e59e49349e8bd), TO_LIMB_T(0x9e2dc90e50e7046b),
        TO_LIMB_T(0x74bd278eaa22f25e), TO_LIMB_T(0x002a437a4b8c35fc)
    };

    exp_mont_384x(out, inp, BLS_12_381_P_2_minus_9_div_16, 758,
                  BLS12_381_P, p0);
}
#else
static void sqr_n_mul_fp2(vec384x out, const vec384x a, size_t count,
                      const vec384x b)
{
    while(count--) {
        sqr_mont_382x(out, a, BLS12_381_P, p0);
        a = out;
    }
    mul_mont_384x(out, out, b, BLS12_381_P, p0);
}

# define sqr(ret,a)		sqr_fp2(ret,a)
# define mul(ret,a,b)		mul_fp2(ret,a,b)
# define sqr_n_mul(ret,a,n,b)	sqr_n_mul_fp2(ret,a,n,b)

# include "sqrt2-addchain.h"
static void recip_sqrt_fp2_9mod16(vec384x out, const vec384x inp)
{
    RECIP_SQRT_MOD_BLS12_381_P2(out, inp, vec384x);
}
# undef RECIP_SQRT_MOD_BLS12_381_P2

# undef sqr_n_mul
# undef sqr
# undef mul
#endif

static limb_t sqrt_align_fp2(vec384x out, const vec384x ret,
                             const vec384x sqrt, const vec384x inp)
{
    static const vec384x sqrt_minus_1 = { { 0 }, { ONE_MONT_P } };
    static const vec384x sqrt_sqrt_minus_1 = {
      /*
       * "magic" number is ±2^((p-3)/4)%p, which is "1/sqrt(2)",
       * in quotes because 2*"1/sqrt(2)"^2 == -1 mod p, not 1,
       * but it pivots into "complex" plane nevertheless...
       */
      { TO_LIMB_T(0x3e2f585da55c9ad1), TO_LIMB_T(0x4294213d86c18183),
        TO_LIMB_T(0x382844c88b623732), TO_LIMB_T(0x92ad2afd19103e18),
        TO_LIMB_T(0x1d794e4fac7cf0b9), TO_LIMB_T(0x0bd592fc7d825ec8) },
      { TO_LIMB_T(0x7bcfa7a25aa30fda), TO_LIMB_T(0xdc17dec12a927e7c),
        TO_LIMB_T(0x2f088dd86b4ebef1), TO_LIMB_T(0xd1ca2087da74d4a7),
        TO_LIMB_T(0x2da2596696cebc1d), TO_LIMB_T(0x0e2b7eedbbfd87d2) }
    };
    static const vec384x sqrt_minus_sqrt_minus_1 = {
      { TO_LIMB_T(0x7bcfa7a25aa30fda), TO_LIMB_T(0xdc17dec12a927e7c),
        TO_LIMB_T(0x2f088dd86b4ebef1), TO_LIMB_T(0xd1ca2087da74d4a7),
        TO_LIMB_T(0x2da2596696cebc1d), TO_LIMB_T(0x0e2b7eedbbfd87d2) },
      { TO_LIMB_T(0x7bcfa7a25aa30fda), TO_LIMB_T(0xdc17dec12a927e7c),
        TO_LIMB_T(0x2f088dd86b4ebef1), TO_LIMB_T(0xd1ca2087da74d4a7),
        TO_LIMB_T(0x2da2596696cebc1d), TO_LIMB_T(0x0e2b7eedbbfd87d2) }
    };
    vec384x coeff, t0, t1;
    limb_t is_sqrt, flag;

    /*
     * Instead of multiple trial squarings we can perform just one
     * and see if the result is "rotated by multiple of 90°" in
     * relation to |inp|, and "rotate" |ret| accordingly.
     */
    sqr_fp2(t0, sqrt);
    /* "sqrt(|inp|)"^2 = (a + b*i)^2 = (a^2-b^2) + 2ab*i */

    /* (a^2-b^2) + 2ab*i == |inp| ? |ret| is spot on */
    sub_fp2(t1, t0, inp);
    is_sqrt = vec_is_zero(t1, sizeof(t1));
    vec_copy(coeff, BLS12_381_Rx.p2, sizeof(coeff));

    /* -(a^2-b^2) - 2ab*i == |inp| ? "rotate |ret| by 90°" */
    add_fp2(t1, t0, inp);
    vec_select(coeff, sqrt_minus_1, coeff, sizeof(coeff),
               flag = vec_is_zero(t1, sizeof(t1)));
    is_sqrt |= flag;

    /* 2ab - (a^2-b^2)*i == |inp| ? "rotate |ret| by 135°" */
    sub_fp(t1[0], t0[0], inp[1]);
    add_fp(t1[1], t0[1], inp[0]);
    vec_select(coeff, sqrt_sqrt_minus_1, coeff, sizeof(coeff),
               flag = vec_is_zero(t1, sizeof(t1)));
    is_sqrt |= flag;

    /* -2ab + (a^2-b^2)*i == |inp| ? "rotate |ret| by 45°" */
    add_fp(t1[0], t0[0], inp[1]);
    sub_fp(t1[1], t0[1], inp[0]);
    vec_select(coeff, sqrt_minus_sqrt_minus_1, coeff, sizeof(coeff),
               flag = vec_is_zero(t1, sizeof(t1)));
    is_sqrt |= flag;

    /* actual "rotation" */
    mul_fp2(out, ret, coeff);

    return is_sqrt;
}

static limb_t recip_sqrt_fp2(vec384x out, const vec384x inp)
{
    vec384x ret, sqrt;

    recip_sqrt_fp2_9mod16(ret, inp);
    mul_fp2(sqrt, ret, inp);

    /*
     * Now see if |ret| is or can be made 1/sqrt(|inp|)...
     */

    return sqrt_align_fp2(out, ret, sqrt, inp);
}

static limb_t sqrt_fp2(vec384x out, const vec384x inp)
{
    vec384x ret;

    recip_sqrt_fp2_9mod16(ret, inp);
    mul_fp2(ret, ret, inp);

    /*
     * Now see if |ret| is or can be made sqrt(|inp|)...
     */

    return sqrt_align_fp2(out, ret, ret, inp);
}

limb_t blst_fp2_sqrt(vec384x out, const vec384x inp)
{   return sqrt_fp2(out, inp);   }

void blst_fp2_inverse(vec384x out, const vec384x inp)
{   reciprocal_fp2(out, inp);   }

static void blst_fp2_eucl_inverse(vec384x out, const vec384x inp)
{
    vec384 t0, t1;

    /*
     * |out| = 1/(a + b*i) = a/(a^2+b^2) - b/(a^2+b^2)*i
     */
    sqr_fp(t0, inp[0]);
    sqr_fp(t1, inp[1]);
    add_fp(t0, t0, t1);
    eucl_inverse_fp(t1, t0);
    mul_fp(out[0], inp[0], t1);
    mul_fp(out[1], inp[1], t1);
    neg_fp(out[1], out[1]);
}
