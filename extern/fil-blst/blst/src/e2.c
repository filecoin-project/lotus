/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "point.h"
#include "fields.h"

/*
 * y^2 = x^3 + B
 */
static const vec384x B_E2 = {       /* 4 + 4*i */
  { TO_LIMB_T(0xaa270000000cfff3), TO_LIMB_T(0x53cc0032fc34000a),
    TO_LIMB_T(0x478fe97a6b0a807f), TO_LIMB_T(0xb1d37ebee6ba24d7),
    TO_LIMB_T(0x8ec9733bbf78ab2f), TO_LIMB_T(0x09d645513d83de7e) },
  { TO_LIMB_T(0xaa270000000cfff3), TO_LIMB_T(0x53cc0032fc34000a),
    TO_LIMB_T(0x478fe97a6b0a807f), TO_LIMB_T(0xb1d37ebee6ba24d7),
    TO_LIMB_T(0x8ec9733bbf78ab2f), TO_LIMB_T(0x09d645513d83de7e) }
};

const POINTonE2 BLS12_381_G2 = {    /* generator point [in Montgomery] */
{ /* (0x024aa2b2f08f0a91260805272dc51051c6e47ad4fa403b02
        b4510b647ae3d1770bac0326a805bbefd48056c8c121bdb8 << 384) % P */
  { TO_LIMB_T(0xf5f28fa202940a10), TO_LIMB_T(0xb3f5fb2687b4961a),
    TO_LIMB_T(0xa1a893b53e2ae580), TO_LIMB_T(0x9894999d1a3caee9),
    TO_LIMB_T(0x6f67b7631863366b), TO_LIMB_T(0x058191924350bcd7) },
  /* (0x13e02b6052719f607dacd3a088274f65596bd0d09920b61a
        b5da61bbdc7f5049334cf11213945d57e5ac7d055d042b7e << 384) % P */
  { TO_LIMB_T(0xa5a9c0759e23f606), TO_LIMB_T(0xaaa0c59dbccd60c3),
    TO_LIMB_T(0x3bb17e18e2867806), TO_LIMB_T(0x1b1ab6cc8541b367),
    TO_LIMB_T(0xc2b6ed0ef2158547), TO_LIMB_T(0x11922a097360edf3) }
},
{ /* (0x0ce5d527727d6e118cc9cdc6da2e351aadfd9baa8cbdd3a7
        6d429a695160d12c923ac9cc3baca289e193548608b82801 << 384) % P */
  { TO_LIMB_T(0x4c730af860494c4a), TO_LIMB_T(0x597cfa1f5e369c5a),
    TO_LIMB_T(0xe7e6856caa0a635a), TO_LIMB_T(0xbbefb5e96e0d495f),
    TO_LIMB_T(0x07d3a975f0ef25a2), TO_LIMB_T(0x0083fd8e7e80dae5) },
  /* (0x0606c4a02ea734cc32acd2b02bc28b99cb3e287e85a763af
        267492ab572e99ab3f370d275cec1da1aaa9075ff05f79be << 384) % P */
  { TO_LIMB_T(0xadc0fc92df64b05d), TO_LIMB_T(0x18aa270a2b1461dc),
    TO_LIMB_T(0x86adac6a3be4eba0), TO_LIMB_T(0x79495c4ec93da33a),
    TO_LIMB_T(0xe7175850a43ccaed), TO_LIMB_T(0x0b2bc2a163de1bf2) },
},
{ { ONE_MONT_P }, { 0 } }
};

const POINTonE2 BLS12_381_NEG_G2 = { /* negative generator [in Montgomery] */
{ /* (0x024aa2b2f08f0a91260805272dc51051c6e47ad4fa403b02
        b4510b647ae3d1770bac0326a805bbefd48056c8c121bdb8 << 384) % P */
  { TO_LIMB_T(0xf5f28fa202940a10), TO_LIMB_T(0xb3f5fb2687b4961a),
    TO_LIMB_T(0xa1a893b53e2ae580), TO_LIMB_T(0x9894999d1a3caee9),
    TO_LIMB_T(0x6f67b7631863366b), TO_LIMB_T(0x058191924350bcd7) },
  /* (0x13e02b6052719f607dacd3a088274f65596bd0d09920b61a
        b5da61bbdc7f5049334cf11213945d57e5ac7d055d042b7e << 384) % P */
  { TO_LIMB_T(0xa5a9c0759e23f606), TO_LIMB_T(0xaaa0c59dbccd60c3),
    TO_LIMB_T(0x3bb17e18e2867806), TO_LIMB_T(0x1b1ab6cc8541b367),
    TO_LIMB_T(0xc2b6ed0ef2158547), TO_LIMB_T(0x11922a097360edf3) }
},
{ /* (0x0d1b3cc2c7027888be51d9ef691d77bcb679afda66c73f17
        f9ee3837a55024f78c71363275a75d75d86bab79f74782aa << 384) % P */
  { TO_LIMB_T(0x6d8bf5079fb65e61), TO_LIMB_T(0xc52f05df531d63a5),
    TO_LIMB_T(0x7f4a4d344ca692c9), TO_LIMB_T(0xa887959b8577c95f),
    TO_LIMB_T(0x4347fe40525c8734), TO_LIMB_T(0x197d145bbaff0bb5) },
  /* (0x13fa4d4a0ad8b1ce186ed5061789213d993923066dddaf10
        40bc3ff59f825c78df74f2d75467e25e0f55f8a00fa030ed << 384) % P */
  { TO_LIMB_T(0x0c3e036d209afa4e), TO_LIMB_T(0x0601d8f4863f9e23),
    TO_LIMB_T(0xe0832636bacc0a84), TO_LIMB_T(0xeb2def362a476f84),
    TO_LIMB_T(0x64044f659f0ee1e9), TO_LIMB_T(0x0ed54f48d5a1caa7) }
},
{ { ONE_MONT_P }, { 0 } }
};

#if 1
void mul_by_b_onE2(vec384x out, const vec384x in);
void mul_by_4b_onE2(vec384x out, const vec384x in);
#else
static void mul_by_b_onE2(vec384x out, const vec384x in)
{
    sub_mod_384(out[0], in[0], in[1], BLS12_381_P);
    add_mod_384(out[1], in[0], in[1], BLS12_381_P);
    lshift_mod_384(out[0], out[0], 2, BLS12_381_P);
    lshift_mod_384(out[1], out[1], 2, BLS12_381_P);
}

static void mul_by_4b_onE2(vec384x out, const vec384x in)
{
    sub_mod_384(out[0], in[0], in[1], BLS12_381_P);
    add_mod_384(out[1], in[0], in[1], BLS12_381_P);
    lshift_mod_384(out[0], out[0], 4, BLS12_381_P);
    lshift_mod_384(out[1], out[1], 4, BLS12_381_P);
}
#endif

static void POINTonE2_cneg(POINTonE2 *p, limb_t cbit)
{
    cneg_fp2(p->Y, p->Y, cbit);
}

static void POINTonE2_from_Jacobian(POINTonE2 *out, const POINTonE2 *in)
{
    vec384x Z, ZZ;
    limb_t inf = vec_is_zero(in->Z, sizeof(in->Z));

    reciprocal_fp2(Z, in->Z);                           /* 1/Z */

    sqr_fp2(ZZ, Z);
    mul_fp2(out->X, in->X, ZZ);                         /* X = X/Z^2 */

    mul_fp2(ZZ, ZZ, Z);
    mul_fp2(out->Y, in->Y, ZZ);                         /* Y = Y/Z^3 */

    vec_select(out->Z, in->Z, BLS12_381_G2.Z,
                       sizeof(BLS12_381_G2.Z), inf);    /* Z = inf ? 0 : 1 */
}

static void POINTonE2_to_affine(POINTonE2_affine *out, const POINTonE2 *in)
{
    POINTonE2 p;

    if (!vec_is_equal(in->Z, BLS12_381_Rx.p2, sizeof(in->Z))) {
        POINTonE2_from_Jacobian(&p, in);
        in = &p;
    }
    vec_copy(out, in, sizeof(*out));
}

static limb_t POINTonE2_affine_on_curve(const POINTonE2_affine *p)
{
    vec384x XXX, YY;

    sqr_fp2(XXX, p->X);
    mul_fp2(XXX, XXX, p->X);                            /* X^3 */
    add_fp2(XXX, XXX, B_E2);                            /* X^3 + B */

    sqr_fp2(YY, p->Y);                                  /* Y^2 */

    return vec_is_equal(XXX, YY, sizeof(XXX)) | vec_is_zero(p, sizeof(*p));
}

static limb_t POINTonE2_on_curve(const POINTonE2 *p)
{
    vec384x XXX, YY, BZ6;
    limb_t inf = vec_is_zero(p->Z, sizeof(p->Z));

    sqr_fp2(BZ6, p->Z);
    mul_fp2(BZ6, BZ6, p->Z);
    sqr_fp2(XXX, BZ6);                                  /* Z^6 */
    mul_by_b_onE2(BZ6, XXX);                            /* B*Z^6 */

    sqr_fp2(XXX, p->X);
    mul_fp2(XXX, XXX, p->X);                            /* X^3 */
    add_fp2(XXX, XXX, BZ6);                             /* X^3 + B*Z^6 */

    sqr_fp2(YY, p->Y);                                  /* Y^2 */

    return vec_is_equal(XXX, YY, sizeof(XXX)) | inf;
}

static limb_t POINTonE2_affine_Serialize_BE(unsigned char out[192],
                                            const POINTonE2_affine *in)
{
    vec384x temp;

    from_fp(temp[1], in->X[1]);
    be_bytes_from_limbs(out, temp[1], sizeof(temp[1]));
    from_fp(temp[0], in->X[0]);
    be_bytes_from_limbs(out + 48, temp[0], sizeof(temp[0]));

    from_fp(temp[1], in->Y[1]);
    be_bytes_from_limbs(out + 96, temp[1], sizeof(temp[1]));
    from_fp(temp[0], in->Y[0]);
    be_bytes_from_limbs(out + 144, temp[0], sizeof(temp[0]));

    return sgn0_pty_mod_384x(temp, BLS12_381_P);
}

void blst_p2_affine_serialize(unsigned char out[192],
                              const POINTonE2_affine *in)
{
    if (vec_is_zero(in->X, 2*sizeof(in->X))) {
        vec_zero(out, 192);
        out[0] = 0x40;    /* infinitiy bit */
    }

    (void)POINTonE2_affine_Serialize_BE(out, in);
}

static limb_t POINTonE2_Serialize_BE(unsigned char out[192],
                                     const POINTonE2 *in)
{
    POINTonE2 p;

    if (!vec_is_equal(in->Z, BLS12_381_Rx.p2, sizeof(in->Z))) {
        POINTonE2_from_Jacobian(&p, in);
        in = &p;
    }

    return POINTonE2_affine_Serialize_BE(out, (const POINTonE2_affine *)in);
}

static void POINTonE2_Serialize(unsigned char out[192], const POINTonE2 *in)
{
    limb_t inf = vec_is_zero(in->Z, sizeof(in->Z));

    if (inf) {
        vec_zero(out, 192);
        out[0] = 0x40;    /* infinitiy bit */
    } else {
        (void)POINTonE2_Serialize_BE(out, in);
    }
}

void blst_p2_serialize(unsigned char out[192], const POINTonE2 *in)
{   POINTonE2_Serialize(out, in);   }

static limb_t POINTonE2_affine_Compress_BE(unsigned char out[96],
                                           const POINTonE2_affine *in)
{
    vec384 temp;

    from_fp(temp, in->X[1]);
    be_bytes_from_limbs(out, temp, sizeof(temp));
    from_fp(temp, in->X[0]);
    be_bytes_from_limbs(out + 48, temp, sizeof(temp));

    return sgn0_pty_mont_384x(in->Y, BLS12_381_P, p0);
}

void blst_p2_affine_compress(unsigned char out[96], const POINTonE2_affine *in)
{
    if (vec_is_zero(in->X, 2*sizeof(in->X))) {
        vec_zero(out, 96);
        out[0] = 0xc0;    /* compressed and infinitiy bits */
    } else {
        unsigned char sign = (unsigned char)POINTonE2_affine_Compress_BE(out,
                                                                         in);
        out[0] |= 0x80 | ((sign & 2) << 4);
    }
}

static limb_t POINTonE2_Compress_BE(unsigned char out[96],
                                    const POINTonE2 *in)
{
    POINTonE2 p;

    if (!vec_is_equal(in->Z, BLS12_381_Rx.p, sizeof(in->Z))) {
        POINTonE2_from_Jacobian(&p, in);
        in = &p;
    }

    return POINTonE2_affine_Compress_BE(out, (const POINTonE2_affine *)in);
}

void blst_p2_compress(unsigned char out[96], const POINTonE2 *in)
{
    limb_t inf = vec_is_zero(in->Z, sizeof(in->Z));

    if (inf) {
        vec_zero(out, 96);
        out[0] = 0xc0;    /* compressed and infinitiy bits */
    } else {
        unsigned char sign = (unsigned char)POINTonE2_Compress_BE(out, in);
        out[0] |= 0x80 | ((sign & 2) << 4);
    }
}

static limb_t POINTonE2_Uncompress_BE(POINTonE2_affine *out,
                                      const unsigned char in[96])
{
    POINTonE2_affine ret;
    vec384 temp;

    limbs_from_be_bytes(ret.X[1], in, sizeof(ret.X[1]));
    limbs_from_be_bytes(ret.X[0], in + 48, sizeof(ret.X[0]));

    /* clear top 3 bits in case caller was conveying some information there */
    ret.X[1][sizeof(ret.X[1])/sizeof(limb_t)-1] &= ((limb_t)0-1) >> 3;
    add_fp(temp, ret.X[1], ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.X[1], sizeof(temp)))
        return (limb_t)0 - BLST_BAD_ENCODING;

    add_fp(temp, ret.X[0], ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.X[0], sizeof(temp)))
        return (limb_t)0 - BLST_BAD_ENCODING;

    mul_fp(ret.X[0], ret.X[0], BLS12_381_RR);
    mul_fp(ret.X[1], ret.X[1], BLS12_381_RR);

    sqr_fp2(ret.Y, ret.X);
    mul_fp2(ret.Y, ret.Y, ret.X);
    add_fp2(ret.Y, ret.Y, B_E2);                        /* X^3 + B */
    if (!sqrt_fp2(ret.Y, ret.Y))
        return (limb_t)0 - BLST_POINT_NOT_ON_CURVE;

    vec_copy(out, &ret, sizeof(ret));

    return sgn0_pty_mont_384x(out->Y, BLS12_381_P, p0);
}

static BLST_ERROR POINTonE2_Uncompress(POINTonE2_affine *out,
                                       const unsigned char in[96])
{
    unsigned char in0 = in[0];
    limb_t sgn0_pty;

    if ((in0 & 0x80) == 0)      /* compressed bit */
        return BLST_BAD_ENCODING;

    if (in0 & 0x40) {           /* infinity bit */
        if (byte_is_zero(in0 & 0x3f) & bytes_are_zero(in+1, 95)) {
            vec_zero(out, sizeof(*out));
            return BLST_SUCCESS;
        } else {
            return BLST_BAD_ENCODING;
        }
    }

    sgn0_pty = POINTonE2_Uncompress_BE(out, in);

    if (sgn0_pty > 3)
        return (BLST_ERROR)(0 - sgn0_pty); /* POINT_NOT_ON_CURVE */

    sgn0_pty >>= 1; /* skip over parity bit */
    sgn0_pty ^= (in0 & 0x20) >> 5;
    cneg_fp2(out->Y, out->Y, sgn0_pty);

    return BLST_SUCCESS;
}

BLST_ERROR blst_p2_uncompress(POINTonE2_affine *out, const unsigned char in[96])
{   return POINTonE2_Uncompress(out, in);   }

static BLST_ERROR POINTonE2_Deserialize_BE(POINTonE2_affine *out,
                                           const unsigned char in[192])
{
    POINTonE2_affine ret;
    vec384 temp;

    limbs_from_be_bytes(ret.X[1], in, sizeof(ret.X[1]));
    limbs_from_be_bytes(ret.X[0], in + 48, sizeof(ret.X[0]));
    limbs_from_be_bytes(ret.Y[1], in + 96, sizeof(ret.Y[1]));
    limbs_from_be_bytes(ret.Y[0], in + 144, sizeof(ret.Y[0]));

    /* clear top 3 bits in case caller was conveying some information there */
    ret.X[1][sizeof(ret.X[1])/sizeof(limb_t)-1] &= ((limb_t)0-1) >> 3;
    add_fp(temp, ret.X[1], ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.X[1], sizeof(temp)))
        return BLST_BAD_ENCODING;

    add_fp(temp, ret.X[0], ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.X[0], sizeof(temp)))
        return BLST_BAD_ENCODING;

    add_fp(temp, ret.Y[1], ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.Y[1], sizeof(temp)))
        return BLST_BAD_ENCODING;

    add_fp(temp, ret.Y[0], ZERO_384);  /* less than modulus? */
    if (!vec_is_equal(temp, ret.Y[0], sizeof(temp)))
        return BLST_BAD_ENCODING;

    mul_fp(ret.X[0], ret.X[0], BLS12_381_RR);
    mul_fp(ret.X[1], ret.X[1], BLS12_381_RR);
    mul_fp(ret.Y[0], ret.Y[0], BLS12_381_RR);
    mul_fp(ret.Y[1], ret.Y[1], BLS12_381_RR);

    if (!POINTonE2_affine_on_curve(&ret))
        return BLST_POINT_NOT_ON_CURVE;

    vec_copy(out, &ret, sizeof(ret));

    return BLST_SUCCESS;
}

int blst_p2_deserialize(POINTonE2_affine *out, const unsigned char in[192])
{
    unsigned char in0 = in[0];

    if ((in0 & 0xe0) == 0)
        return POINTonE2_Deserialize_BE(out, in);

    if (in0 & 0x80)             /* compressed bit */
        return POINTonE2_Uncompress(out, in);

    if (in0 & 0x40) {           /* infinity bit */
        if (byte_is_zero(in0 & 0x3f) & bytes_are_zero(in+1, 191)) {
            vec_zero(out, sizeof(*out));
            return BLST_SUCCESS;
        }
    }

    return BLST_BAD_ENCODING;
}

void blst_sk_to_pk_in_g2(POINTonE2 *out, const vec256 SK)
{   POINTonE2_mult_w5(out, &BLS12_381_G2, SK, 255);   }

void blst_sign_pk_in_g1(POINTonE2 *out, const POINTonE2 *msg, const vec256 SK)
{   POINTonE2_mult_w5(out, msg, SK, 255);   }

void blst_sk_to_pk2_in_g2(unsigned char out[192], POINTonE2_affine *PK,
                          const vec256 SK)
{
    POINTonE2 P[1];

    POINTonE2_mult_w5(P, &BLS12_381_G2, SK, 255);
    POINTonE2_from_Jacobian(P, P);
    if (PK != NULL)
        vec_copy(PK, P, sizeof(*PK));
    if (out != NULL) {
        limb_t sgn0_pty = POINTonE2_Serialize_BE(out, P);
        out[0] |= (sgn0_pty & 2) << 4;      /* pre-decorate */
        out[0] |= vec_is_zero(P->Z, sizeof(P->Z)) << 6;
    }
}

void blst_sign_pk2_in_g1(unsigned char out[192], POINTonE2_affine *sig,
                         const POINTonE2 *hash, const vec256 SK)
{
    POINTonE2 P[1];

    POINTonE2_mult_w5(P, hash, SK, 255);
    POINTonE2_from_Jacobian(P, P);
    if (sig != NULL)
        vec_copy(sig, P, sizeof(*sig));
    if (out != NULL) {
        limb_t sgn0_pty = POINTonE2_Serialize_BE(out, P);
        out[0] |= (sgn0_pty & 2) << 4;      /* pre-decorate */
        out[0] |= vec_is_zero(P->Z, sizeof(P->Z)) << 6;
    }
}

void blst_p2_cneg(POINTonE2 *a, size_t cbit)
{   POINTonE2_cneg(a, cbit);   }

void blst_p2_from_jacobian(POINTonE2 *out, const POINTonE2 *a)
{   POINTonE2_from_Jacobian(out, a);   }

void blst_p2_to_affine(POINTonE2_affine *out, const POINTonE2 *a)
{   POINTonE2_to_affine(out, a);   }

void blst_p2_from_affine(POINTonE2 *out, const POINTonE2_affine *a)
{
    vec_copy(out, a, sizeof(*a));
    vec_select(out->Z, a->X, BLS12_381_Rx.p2, sizeof(out->Z),
                       vec_is_zero(a, sizeof(*a)));
}

limb_t blst_p2_on_curve(const POINTonE2 *p)
{   return POINTonE2_on_curve(p);   }

limb_t blst_p2_affine_on_curve(const POINTonE2_affine *p)
{   return POINTonE2_affine_on_curve(p);   }

#include "ec_ops.h"
POINT_DADD_IMPL(POINTonE2, 384x, fp2)
POINT_DADD_AFFINE_IMPL_A0(POINTonE2, 384x, fp2, BLS12_381_Rx.p2)
POINT_ADD_IMPL(POINTonE2, 384x, fp2)
POINT_ADD_AFFINE_IMPL(POINTonE2, 384x, fp2, BLS12_381_Rx.p2)
POINT_DOUBLE_IMPL_A0(POINTonE2, 384x, fp2)
POINT_IS_EQUAL_IMPL(POINTonE2, 384x, fp2)

limb_t blst_p2_is_equal(const POINTonE2 *a, const POINTonE2 *b)
{   return POINTonE2_is_equal(a, b);   }

#include "ec_mult.h"
POINT_MULT_SCALAR_W5_IMPL(POINTonE2)
POINT_AFFINE_MULT_SCALAR_IMPL(POINTonE2)

DECLARE_PRIVATE_POINTXZ(POINTonE2, 384x)
POINT_LADDER_PRE_IMPL(POINTonE2, 384x, fp2)
POINT_LADDER_STEP_IMPL_A0(POINTonE2, 384x, fp2, onE2)
POINT_LADDER_POST_IMPL_A0(POINTonE2, 384x, fp2, onE2)
POINT_MULT_SCALAR_LADDER_IMPL(POINTonE2)

limb_t blst_p2_is_inf(const POINTonE2 *p)
{   return vec_is_zero(p->Z, sizeof(p->Z));   }

const POINTonE2 *blst_p2_generator()
{   return &BLS12_381_G2;   }

limb_t blst_p2_affine_is_inf(const POINTonE2_affine *p)
{   return vec_is_zero(p, sizeof(*p));   }

const POINTonE2_affine *blst_p2_affine_generator()
{   return (const POINTonE2_affine *)&BLS12_381_G2;   }
