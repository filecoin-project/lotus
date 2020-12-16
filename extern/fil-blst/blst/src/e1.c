/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "point.h"
#include "fields.h"
#include "errors.h"

/*
 * y^2 = x^3 + B
 */
static const vec384 B_E1 = {        /* (4 << 384) % P */
    TO_LIMB_T(0xaa270000000cfff3), TO_LIMB_T(0x53cc0032fc34000a),
    TO_LIMB_T(0x478fe97a6b0a807f), TO_LIMB_T(0xb1d37ebee6ba24d7),
    TO_LIMB_T(0x8ec9733bbf78ab2f), TO_LIMB_T(0x09d645513d83de7e)
};

const POINTonE1 BLS12_381_G1 = {    /* generator point [in Montgomery] */
  /* (0x17f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905
   *    a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb << 384) % P */
  { TO_LIMB_T(0x5cb38790fd530c16), TO_LIMB_T(0x7817fc679976fff5),
    TO_LIMB_T(0x154f95c7143ba1c1), TO_LIMB_T(0xf0ae6acdf3d0e747),
    TO_LIMB_T(0xedce6ecc21dbf440), TO_LIMB_T(0x120177419e0bfb75) },
  /* (0x08b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af6
   *    00db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e1 << 384) % P */
  { TO_LIMB_T(0xbaac93d50ce72271), TO_LIMB_T(0x8c22631a7918fd8e),
    TO_LIMB_T(0xdd595f13570725ce), TO_LIMB_T(0x51ac582950405194),
    TO_LIMB_T(0x0e1c8c3fad0059c0), TO_LIMB_T(0x0bbc3efc5008a26a) },
  { ONE_MONT_P }
};

const POINTonE1 BLS12_381_NEG_G1 = { /* negative generator [in Montgomery] */
  /* (0x17f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905
   *    a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb << 384) % P */
  { TO_LIMB_T(0x5cb38790fd530c16), TO_LIMB_T(0x7817fc679976fff5),
    TO_LIMB_T(0x154f95c7143ba1c1), TO_LIMB_T(0xf0ae6acdf3d0e747),
    TO_LIMB_T(0xedce6ecc21dbf440), TO_LIMB_T(0x120177419e0bfb75) },
  /* (0x114d1d6855d545a8aa7d76c8cf2e21f267816aef1db507c9
   *    6655b9d5caac42364e6f38ba0ecb751bad54dcd6b939c2ca << 384) % P */
  { TO_LIMB_T(0xff526c2af318883a), TO_LIMB_T(0x92899ce4383b0270),
    TO_LIMB_T(0x89d7738d9fa9d055), TO_LIMB_T(0x12caf35ba344c12a),
    TO_LIMB_T(0x3cff1b76964b5317), TO_LIMB_T(0x0e44d2ede9774430) },
  { ONE_MONT_P }
};

#if 1
void mul_by_b_onE1(vec384 out, const vec384 in);
void mul_by_4b_onE1(vec384 out, const vec384 in);
#else
static inline void mul_by_b_onE1(vec384 out, const vec384 in)
{   lshift_mod_384(out, in, 2, BLS12_381_P);   }

static inline void mul_by_4b_onE1(vec384 out, const vec384 in)
{   lshift_mod_384(out, in, 4, BLS12_381_P);   }
#endif

static void POINTonE1_cneg(POINTonE1 *p, limb_t cbit)
{   cneg_fp(p->Y, p->Y, cbit);   }

static void POINTonE1_from_Jacobian(POINTonE1 *out, const POINTonE1 *in)
{
    vec384 Z, ZZ;
    limb_t inf = vec_is_zero(in->Z, sizeof(in->Z));

    reciprocal_fp(Z, in->Z);                            /* 1/Z   */

    sqr_fp(ZZ, Z);
    mul_fp(out->X, in->X, ZZ);                          /* X = X/Z^2 */

    mul_fp(ZZ, ZZ, Z);
    mul_fp(out->Y, in->Y, ZZ);                          /* Y = Y/Z^3 */

    vec_select(out->Z, in->Z, BLS12_381_G1.Z,
                       sizeof(BLS12_381_G1.Z), inf);    /* Z = inf ? 0 : 1 */
}

static void POINTonE1_to_affine(POINTonE1_affine *out, const POINTonE1 *in)
{
    POINTonE1 p;

    if (!vec_is_equal(in->Z, BLS12_381_Rx.p, sizeof(in->Z))) {
        POINTonE1_from_Jacobian(&p, in);
        in = &p;
    }
    vec_copy(out, in, sizeof(*out));
}

static limb_t POINTonE1_affine_on_curve(const POINTonE1_affine *p)
{
    vec384 XXX, YY;

    sqr_fp(XXX, p->X);
    mul_fp(XXX, XXX, p->X);                             /* X^3 */
    add_fp(XXX, XXX, B_E1);                             /* X^3 + B */

    sqr_fp(YY, p->Y);                                   /* Y^2 */

    return vec_is_equal(XXX, YY, sizeof(XXX)) | vec_is_zero(p, sizeof(*p));
}

static limb_t POINTonE1_on_curve(const POINTonE1 *p)
{
    vec384 XXX, YY, BZ6;
    limb_t inf = vec_is_zero(p->Z, sizeof(p->Z));

    sqr_fp(BZ6, p->Z);
    mul_fp(BZ6, BZ6, p->Z);
    sqr_fp(BZ6, BZ6);                                   /* Z^6 */
    mul_by_b_onE1(BZ6, BZ6);                            /* B*Z^6 */

    sqr_fp(XXX, p->X);
    mul_fp(XXX, XXX, p->X);                             /* X^3 */
    add_fp(XXX, XXX, BZ6);                              /* X^3 + B*Z^6 */

    sqr_fp(YY, p->Y);                                   /* Y^2 */

    return vec_is_equal(XXX, YY, sizeof(XXX)) | inf;
}

static limb_t POINTonE1_affine_Serialize_BE(unsigned char out[96],
                                            const POINTonE1_affine *in)
{
    vec384 temp;

    from_fp(temp, in->X);
    be_bytes_from_limbs(out, temp, sizeof(temp));

    from_fp(temp, in->Y);
    be_bytes_from_limbs(out + 48, temp, sizeof(temp));

    return sgn0_pty_mod_384(temp, BLS12_381_P);
}

void blst_p1_affine_serialize(unsigned char out[96],
                              const POINTonE1_affine *in)
{
    if (vec_is_zero(in->X, 2*sizeof(in->X))) {
        vec_zero(out, 96);
        out[0] = 0x40;    /* infinitiy bit */
    }

    (void)POINTonE1_affine_Serialize_BE(out, in);
}

static limb_t POINTonE1_Serialize_BE(unsigned char out[96],
                                     const POINTonE1 *in)
{
    POINTonE1 p;

    if (!vec_is_equal(in->Z, BLS12_381_Rx.p, sizeof(in->Z))) {
        POINTonE1_from_Jacobian(&p, in);
        in = &p;
    }

    return POINTonE1_affine_Serialize_BE(out, (const POINTonE1_affine *)in);
}

static void POINTonE1_Serialize(unsigned char out[96], const POINTonE1 *in)
{
    limb_t inf = vec_is_zero(in->Z, sizeof(in->Z));

    if (inf) {
        vec_zero(out, 96);
        out[0] = 0x40;    /* infinitiy bit */
    } else {
        (void)POINTonE1_Serialize_BE(out, in);
    }
}

void blst_p1_serialize(unsigned char out[96], const POINTonE1 *in)
{   POINTonE1_Serialize(out, in);   }

static limb_t POINTonE1_affine_Compress_BE(unsigned char out[48],
                                           const POINTonE1_affine *in)
{
    vec384 temp;

    from_fp(temp, in->X);
    be_bytes_from_limbs(out, temp, sizeof(temp));

    return sgn0_pty_mont_384(in->Y, BLS12_381_P, p0);
}

void blst_p1_affine_compress(unsigned char out[48], const POINTonE1_affine *in)
{
    if (vec_is_zero(in->X, 2*sizeof(in->X))) {
        vec_zero(out, 48);
        out[0] = 0xc0;    /* compressed and infinitiy bits */
    } else {
        unsigned char sign = (unsigned char)POINTonE1_affine_Compress_BE(out,
                                                                         in);
        out[0] |= 0x80 | ((sign & 2) << 4);
    }
}

static limb_t POINTonE1_Compress_BE(unsigned char out[48],
                                    const POINTonE1 *in)
{
    POINTonE1 p;

    if (!vec_is_equal(in->Z, BLS12_381_Rx.p, sizeof(in->Z))) {
        POINTonE1_from_Jacobian(&p, in);
        in = &p;
    }

    return POINTonE1_affine_Compress_BE(out, (const POINTonE1_affine *)in);
}

void blst_p1_compress(unsigned char out[48], const POINTonE1 *in)
{
    limb_t inf = vec_is_zero(in->Z, sizeof(in->Z));

    if (inf) {
        vec_zero(out, 48);
        out[0] = 0xc0;    /* compressed and infinitiy bits */
    } else {
        unsigned char sign = (unsigned char)POINTonE1_Compress_BE(out, in);
        out[0] |= 0x80 | ((sign & 2) << 4);
    }
}

static limb_t POINTonE1_Uncompress_BE(POINTonE1_affine *out,
                                      const unsigned char in[48])
{
    POINTonE1_affine ret;
    vec384 temp;

    limbs_from_be_bytes(ret.X, in, sizeof(ret.X));
    /* clear top 3 bits in case caller was conveying some information there */
    ret.X[sizeof(ret.X)/sizeof(limb_t)-1] &= ((limb_t)0-1) >> 3;
    add_fp(temp, ret.X, ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.X, sizeof(temp)))
        return (limb_t)0 - BLST_BAD_ENCODING;
    mul_fp(ret.X, ret.X, BLS12_381_RR);

    sqr_fp(ret.Y, ret.X);
    mul_fp(ret.Y, ret.Y, ret.X);
    add_fp(ret.Y, ret.Y, B_E1);                         /* X^3 + B */
    if (!sqrt_fp(ret.Y, ret.Y))
        return (limb_t)0 - BLST_POINT_NOT_ON_CURVE;

    vec_copy(out, &ret, sizeof(ret));

    return sgn0_pty_mont_384(out->Y, BLS12_381_P, p0);
}

static BLST_ERROR POINTonE1_Uncompress(POINTonE1_affine *out,
                                       const unsigned char in[48])
{
    unsigned char in0 = in[0];
    limb_t sgn0_pty;

    if ((in0 & 0x80) == 0)      /* compressed bit */
        return BLST_BAD_ENCODING;

    if (in0 & 0x40) {           /* infinity bit */
        if (byte_is_zero(in0 & 0x3f) & bytes_are_zero(in+1, 47)) {
            vec_zero(out, sizeof(*out));
            return BLST_SUCCESS;
        } else {
            return BLST_BAD_ENCODING;
        }
    }

    sgn0_pty = POINTonE1_Uncompress_BE(out, in);

    if (sgn0_pty > 3)
        return (BLST_ERROR)(0 - sgn0_pty); /* POINT_NOT_ON_CURVE */

    sgn0_pty >>= 1; /* skip over parity bit */
    sgn0_pty ^= (in0 & 0x20) >> 5;
    cneg_fp(out->Y, out->Y, sgn0_pty);

    /* (0,±2) is not in group, but application might want to ignore? */
    return vec_is_zero(out->X, sizeof(out->X)) ? BLST_POINT_NOT_IN_GROUP
                                               : BLST_SUCCESS;
}

BLST_ERROR blst_p1_uncompress(POINTonE1_affine *out, const unsigned char in[48])
{   return POINTonE1_Uncompress(out, in);   }

static BLST_ERROR POINTonE1_Deserialize_BE(POINTonE1_affine *out,
                                           const unsigned char in[96])
{
    POINTonE1_affine ret;
    vec384 temp;

    limbs_from_be_bytes(ret.X, in, sizeof(ret.X));
    limbs_from_be_bytes(ret.Y, in + 48, sizeof(ret.Y));

    /* clear top 3 bits in case caller was conveying some information there */
    ret.X[sizeof(ret.X)/sizeof(limb_t)-1] &= ((limb_t)0-1) >> 3;
    add_fp(temp, ret.X, ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.X, sizeof(temp)))
        return BLST_BAD_ENCODING;

    add_fp(temp, ret.Y, ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.Y, sizeof(temp)))
        return BLST_BAD_ENCODING;

    mul_fp(ret.X, ret.X, BLS12_381_RR);
    mul_fp(ret.Y, ret.Y, BLS12_381_RR);

    if (!POINTonE1_affine_on_curve(&ret))
        return BLST_POINT_NOT_ON_CURVE;

    vec_copy(out, &ret, sizeof(ret));

    /* (0,±2) is not in group, but application might want to ignore? */
    return vec_is_zero(out->X, sizeof(out->X)) ? BLST_POINT_NOT_IN_GROUP
                                               : BLST_SUCCESS;
}

BLST_ERROR blst_p1_deserialize(POINTonE1_affine *out,
                               const unsigned char in[96])
{
    unsigned char in0 = in[0];

    if ((in0 & 0xe0) == 0)
        return POINTonE1_Deserialize_BE(out, in);

    if (in0 & 0x80)             /* compressed bit */
        return POINTonE1_Uncompress(out, in);

    if (in0 & 0x40) {           /* infinity bit */
        if (byte_is_zero(in0 & 0x3f) & bytes_are_zero(in+1, 95)) {
            vec_zero(out, sizeof(*out));
            return BLST_SUCCESS;
        }
    }

    return BLST_BAD_ENCODING;
}

void blst_sk_to_pk_in_g1(POINTonE1 *out, const vec256 SK)
{   POINTonE1_mult_w5(out, &BLS12_381_G1, SK, 255);   }

void blst_sign_pk_in_g2(POINTonE1 *out, const POINTonE1 *msg, const vec256 SK)
{   POINTonE1_mult_w5(out, msg, SK, 255);   }

void blst_sk_to_pk2_in_g1(unsigned char out[96], POINTonE1_affine *PK,
                          const vec256 SK)
{
    POINTonE1 P[1];

    POINTonE1_mult_w5(P, &BLS12_381_G1, SK, 255);
    POINTonE1_from_Jacobian(P, P);
    if (PK != NULL)
        vec_copy(PK, P, sizeof(*PK));
    if (out != NULL) {
        limb_t sgn0_pty = POINTonE1_Serialize_BE(out, P);
        out[0] |= (sgn0_pty & 2) << 4;      /* pre-decorate */
        out[0] |= vec_is_zero(P->Z, sizeof(P->Z)) << 6;
    }
}

void blst_sign_pk2_in_g2(unsigned char out[96], POINTonE1_affine *sig,
                         const POINTonE1 *hash, const vec256 SK)
{
    POINTonE1 P[1];

    POINTonE1_mult_w5(P, hash, SK, 255);
    POINTonE1_from_Jacobian(P, P);
    if (sig != NULL)
        vec_copy(sig, P, sizeof(*sig));
    if (out != NULL) {
        limb_t sgn0_pty = POINTonE1_Serialize_BE(out, P);
        out[0] |= (sgn0_pty & 2) << 4;      /* pre-decorate */
        out[0] |= vec_is_zero(P->Z, sizeof(P->Z)) << 6;
    }
}

void blst_p1_cneg(POINTonE1 *a, size_t cbit)
{   POINTonE1_cneg(a, cbit);   }

void blst_p1_from_jacobian(POINTonE1 *out, const POINTonE1 *a)
{   POINTonE1_from_Jacobian(out, a);   }

void blst_p1_to_affine(POINTonE1_affine *out, const POINTonE1 *a)
{   POINTonE1_to_affine(out, a);   }

void blst_p1_from_affine(POINTonE1 *out, const POINTonE1_affine *a)
{
    vec_copy(out, a, sizeof(*a));
    vec_select(out->Z, a->X, BLS12_381_Rx.p, sizeof(out->Z),
                       vec_is_zero(a, sizeof(*a)));
}

limb_t blst_p1_on_curve(const POINTonE1 *p)
{   return POINTonE1_on_curve(p);   }

limb_t blst_p1_affine_on_curve(const POINTonE1_affine *p)
{   return POINTonE1_affine_on_curve(p);   }

#include "ec_ops.h"
POINT_DADD_IMPL(POINTonE1, 384, fp)
POINT_DADD_AFFINE_IMPL_A0(POINTonE1, 384, fp, BLS12_381_Rx.p)
POINT_ADD_IMPL(POINTonE1, 384, fp)
POINT_ADD_AFFINE_IMPL(POINTonE1, 384, fp, BLS12_381_Rx.p)
POINT_DOUBLE_IMPL_A0(POINTonE1, 384, fp)
POINT_IS_EQUAL_IMPL(POINTonE1, 384, fp)

limb_t blst_p1_is_equal(const POINTonE1 *a, const POINTonE1 *b)
{   return POINTonE1_is_equal(a, b);   }

#include "ec_mult.h"
POINT_MULT_SCALAR_W5_IMPL(POINTonE1)
POINT_AFFINE_MULT_SCALAR_IMPL(POINTonE1)

DECLARE_PRIVATE_POINTXZ(POINTonE1, 384)
POINT_LADDER_PRE_IMPL(POINTonE1, 384, fp)
POINT_LADDER_STEP_IMPL_A0(POINTonE1, 384, fp, onE1)
POINT_LADDER_POST_IMPL_A0(POINTonE1, 384, fp, onE1)
POINT_MULT_SCALAR_LADDER_IMPL(POINTonE1)

limb_t blst_p1_is_inf(const POINTonE1 *p)
{   return vec_is_zero(p->Z, sizeof(p->Z));   }

const POINTonE1 *blst_p1_generator()
{   return &BLS12_381_G1;   }

limb_t blst_p1_affine_is_inf(const POINTonE1_affine *p)
{   return vec_is_zero(p, sizeof(*p));   }

const POINTonE1_affine *blst_p1_affine_generator()
{   return (const POINTonE1_affine *)&BLS12_381_G1;   }
