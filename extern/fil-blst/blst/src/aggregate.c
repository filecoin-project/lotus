/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * Usage pattern on single-processor system is
 *
 * blst_pairing_init(ctx, hash_or_encode, DST);
 * blst_pairing_aggregate_pk_in_g1(ctx, PK[0], aggregated_signature, msg[0]);
 * blst_pairing_aggregate_pk_in_g1(ctx, PK[1], NULL, msg[1]);
 * ...
 * blst_pairing_commit(ctx);
 * blst_pairing_finalverify(ctx, NULL);
 *
 ***********************************************************************
 * Usage pattern on multi-processor system is
 *
 *   blst_pairing_init(pk[0], hash_or_encode, DST);
 *   blst_pairing_init(pk[1], hash_or_encode, DST);
 *   ...
 * start threads each processing an N/nthreads slice of PKs and messages:
 *     blst_pairing_aggregate_pk_in_g1(pk[i], PK[i*n+0], NULL, msg[i*n+0]);
 *     blst_pairing_aggregate_pk_in_g1(pk[i], PK[i*n+1], NULL, msg[i*n+1]);
 *     ...
 *     blst_pairing_commit(pkx);
 *   ...
 * meanwhile in main thread
 *   blst_fp12 gtsig;
 *   blst_aggregated_in_g2(&gtsig, aggregated_signature);
 * join threads and merge their contexts:
 *   blst_pairing_merge(pk[0], pk[1]);
 *   blst_pairing_merge(pk[0], pk[2]);
 *   ...
 *   blst_pairing_finalverify(pk[0], gtsig);
 */

#ifndef N_MAX
# define N_MAX 8
#endif

typedef union { POINTonE1 e1; POINTonE2 e2; } AggregatedSignature;
typedef struct {
    unsigned int ctrl;
    unsigned int nelems;
    const void *DST;
    size_t DST_len;
    vec384fp12 GT;
    AggregatedSignature AggrSign;
    POINTonE2_affine Q[N_MAX];
    POINTonE1_affine P[N_MAX];
} PAIRING;

enum { AGGR_UNDEFINED      = 0,
       AGGR_MIN_SIG        = 1,
       AGGR_MIN_PK         = 2,
       AGGR_SIGN_SET       = 0x10,
       AGGR_GT_SET         = 0x20,
       AGGR_HASH_OR_ENCODE = 0x40 };
#define MIN_SIG_OR_PK (AGGR_MIN_SIG | AGGR_MIN_PK)

size_t blst_pairing_sizeof(void)
{   return (sizeof(PAIRING) + 7) & ~(size_t)7;   }

void blst_pairing_init(PAIRING *ctx, int hash_or_encode,
                       const void *DST, size_t DST_len)
{
    ctx->ctrl = AGGR_UNDEFINED | (hash_or_encode ? AGGR_HASH_OR_ENCODE : 0);
    ctx->nelems = 0;
    ctx->DST = DST;
    ctx->DST_len = DST_len;
}

const void *blst_pairing_get_dst(const PAIRING *ctx)
{   return ctx->DST;   }

#define FROM_AFFINE(out,in) do { \
    vec_copy((out)->X, in->X, 2*sizeof(in->X)), \
    vec_select((out)->Z, in->X, BLS12_381_Rx.p, sizeof(in->X), \
                         vec_is_zero(in->X, 2*sizeof(in->X))); } while(0)

/*
 * Optional |nbits|-wide |scalar| is used to facilitate multiple aggregated
 * signature vetification as discussed at
 * https://ethresear.ch/t/fast-verification-of-multiple-bls-signatures/5407.
 * Usage pattern is not finalized yet, because (sig != NULL) is better and
 * will be handled separately...
 */
static BLST_ERROR PAIRING_Aggregate_PK_in_G2(PAIRING *ctx,
                                             const POINTonE2_affine *PK,
                                             const POINTonE1_affine *sig,
                                             const limb_t *scalar, size_t nbits,
                                             const void *msg, size_t msg_len,
                                             const void *aug, size_t aug_len)
{
    if (ctx->ctrl & AGGR_MIN_PK)
        return BLST_AGGR_TYPE_MISMATCH;

    ctx->ctrl |= AGGR_MIN_SIG;

    if (sig != NULL) {
        POINTonE1 *S = &ctx->AggrSign.e1;

        if (!POINTonE1_in_G1((const POINTonE1 *)sig))
            return BLST_POINT_NOT_IN_GROUP;

        if (ctx->ctrl & AGGR_SIGN_SET) {
            if (nbits != 0 && scalar != NULL) {
                POINTonE1 P[1];

                FROM_AFFINE(P, sig);
                POINTonE1_mult_w5(P, P, scalar, nbits);
                POINTonE1_dadd(S, S, P, NULL);
            } else {
                POINTonE1_dadd_affine(S, S, (const POINTonE1 *)sig);
            }
        } else {
            ctx->ctrl |= AGGR_SIGN_SET;
            FROM_AFFINE(S, sig);
            if (nbits != 0 && scalar != NULL)
                POINTonE1_mult_w5(S, S, scalar, nbits);
        }
    }

    if (PK != NULL) {
        unsigned int n;
        POINTonE1 H[1];

        if (ctx->ctrl & AGGR_HASH_OR_ENCODE)
            Hash_to_G1(H, msg, msg_len, ctx->DST, ctx->DST_len, aug, aug_len);
        else
            Encode_to_G1(H, msg, msg_len, ctx->DST, ctx->DST_len, aug, aug_len);

        if (nbits != 0 && scalar != NULL)
            POINTonE1_mult_w5(H, H, scalar, nbits);

        POINTonE1_from_Jacobian(H, H);

        n = ctx->nelems;
        vec_copy(ctx->Q + n, PK, sizeof(POINTonE2_affine));
        vec_copy(ctx->P + n, H, sizeof(POINTonE1_affine));
        if (++n == N_MAX) {
            if (ctx->ctrl & AGGR_GT_SET) {
                vec384fp12 GT;
                miller_loop_n(GT, ctx->Q, ctx->P, n);
                mul_fp12(ctx->GT, ctx->GT, GT);
            } else {
                miller_loop_n(ctx->GT, ctx->Q, ctx->P, n);
                ctx->ctrl |= AGGR_GT_SET;
            }
            n = 0;
        }
        ctx->nelems = n;
    }

    return BLST_SUCCESS;
}

BLST_ERROR blst_pairing_aggregate_pk_in_g2(PAIRING *ctx,
                                           const POINTonE2_affine *PK,
                                           const POINTonE1_affine *signature,
                                           const void *msg, size_t msg_len,
                                           const void *aug, size_t aug_len)
{   return PAIRING_Aggregate_PK_in_G2(ctx, PK, signature, NULL, 0,
                                      msg, msg_len, aug, aug_len);
}

BLST_ERROR blst_pairing_mul_n_aggregate_pk_in_g2(PAIRING *ctx,
                                                 const POINTonE2_affine *PK,
                                                 const POINTonE1_affine *sig,
                                                 const limb_t *scalar,
                                                 size_t nbits,
                                                 const void *msg,
                                                 size_t msg_len,
                                                 const void *aug,
                                                 size_t aug_len)
{   return PAIRING_Aggregate_PK_in_G2(ctx, PK, sig, scalar, nbits,
                                      msg, msg_len, aug, aug_len);
}

static BLST_ERROR PAIRING_Aggregate_PK_in_G1(PAIRING *ctx,
                                             const POINTonE1_affine *PK,
                                             const POINTonE2_affine *sig,
                                             const limb_t *scalar, size_t nbits,
                                             const void *msg, size_t msg_len,
                                             const void *aug, size_t aug_len)
{
    if (ctx->ctrl & AGGR_MIN_SIG)
        return BLST_AGGR_TYPE_MISMATCH;

    ctx->ctrl |= AGGR_MIN_PK;

    if (sig != NULL) {
        POINTonE2 *S = &ctx->AggrSign.e2;

        if (!POINTonE2_in_G2((const POINTonE2 *)sig))
            return BLST_POINT_NOT_IN_GROUP;

        if (ctx->ctrl & AGGR_SIGN_SET) {
            if (nbits != 0 && scalar != NULL) {
                POINTonE2 P[1];

                FROM_AFFINE(P, sig);
                POINTonE2_mult_w5(P, P, scalar, nbits);
                POINTonE2_dadd(S, S, P, NULL);
            } else {
                POINTonE2_dadd_affine(S, S, (const POINTonE2 *)sig);
            }
        } else {
            ctx->ctrl |= AGGR_SIGN_SET;
            FROM_AFFINE(S, sig);
            if (nbits != 0 && scalar != NULL)
                POINTonE2_mult_w5(S, S, scalar, nbits);
        }
    }

    if (PK != NULL) {
        unsigned int n;
        POINTonE2 H[1];

        if (ctx->ctrl & AGGR_HASH_OR_ENCODE)
            Hash_to_G2(H, msg, msg_len, ctx->DST, ctx->DST_len, aug, aug_len);
        else
            Encode_to_G2(H, msg, msg_len, ctx->DST, ctx->DST_len, aug, aug_len);

        POINTonE2_from_Jacobian(H, H);

        if (nbits != 0 && scalar != NULL) {
            POINTonE1 pk[1];

            FROM_AFFINE(pk, PK);
            POINTonE1_mult_w5(pk, pk, scalar, nbits);
            POINTonE1_from_Jacobian(pk, pk);
            PK = (const POINTonE1_affine *)pk;
        }

        n = ctx->nelems;
        vec_copy(ctx->Q + n, H, sizeof(POINTonE2_affine));
        vec_copy(ctx->P + n, PK, sizeof(POINTonE1_affine));
        if (++n == N_MAX) {
            if (ctx->ctrl & AGGR_GT_SET) {
                vec384fp12 GT;
                miller_loop_n(GT, ctx->Q, ctx->P, n);
                mul_fp12(ctx->GT, ctx->GT, GT);
            } else {
                miller_loop_n(ctx->GT, ctx->Q, ctx->P, n);
                ctx->ctrl |= AGGR_GT_SET;
            }
            n = 0;
        }
        ctx->nelems = n;
    }

    return BLST_SUCCESS;
}

BLST_ERROR blst_pairing_aggregate_pk_in_g1(PAIRING *ctx,
                                           const POINTonE1_affine *PK,
                                           const POINTonE2_affine *signature,
                                           const void *msg, size_t msg_len,
                                           const void *aug, size_t aug_len)
{   return PAIRING_Aggregate_PK_in_G1(ctx, PK, signature, NULL, 0,
                                      msg, msg_len, aug, aug_len);
}

BLST_ERROR blst_pairing_mul_n_aggregate_pk_in_g1(PAIRING *ctx,
                                                 const POINTonE1_affine *PK,
                                                 const POINTonE2_affine *sig,
                                                 const limb_t *scalar,
                                                 size_t nbits,
                                                 const void *msg,
                                                 size_t msg_len,
                                                 const void *aug,
                                                 size_t aug_len)
{   return PAIRING_Aggregate_PK_in_G1(ctx, PK, sig, scalar, nbits,
                                      msg, msg_len, aug, aug_len);
}

static void PAIRING_Commit(PAIRING *ctx)
{
    unsigned int n;

    if ((n = ctx->nelems) != 0) {
        if (ctx->ctrl & AGGR_GT_SET) {
            vec384fp12 GT;
            miller_loop_n(GT, ctx->Q, ctx->P, n);
            mul_fp12(ctx->GT, ctx->GT, GT);
        } else {
            miller_loop_n(ctx->GT, ctx->Q, ctx->P, n);
            ctx->ctrl |= AGGR_GT_SET;
        }
        ctx->nelems = 0;
    }
}

void blst_pairing_commit(PAIRING *ctx)
{   PAIRING_Commit(ctx);   }

BLST_ERROR blst_pairing_merge(PAIRING *ctx, const PAIRING *ctx1)
{
    if ((ctx->ctrl & MIN_SIG_OR_PK) != AGGR_UNDEFINED
        && (ctx->ctrl & ctx1->ctrl & MIN_SIG_OR_PK) == 0)
        return BLST_AGGR_TYPE_MISMATCH;

    /* context producers are expected to have called blst_pairing_commit */
    if (ctx->nelems || ctx1->nelems)
        return BLST_AGGR_TYPE_MISMATCH;

    switch (ctx->ctrl & MIN_SIG_OR_PK) {
        case AGGR_MIN_SIG:
            if (ctx->ctrl & ctx1->ctrl & AGGR_SIGN_SET) {
                POINTonE1_dadd(&ctx->AggrSign.e1, &ctx->AggrSign.e1,
                                                  &ctx1->AggrSign.e1, NULL);
            } else if (ctx1->ctrl & AGGR_SIGN_SET) {
                ctx->ctrl |= AGGR_SIGN_SET;
                vec_copy(&ctx->AggrSign.e1, &ctx1->AggrSign.e1,
                         sizeof(ctx->AggrSign.e1));
            }
            break;
        case AGGR_MIN_PK:
            if (ctx->ctrl & ctx1->ctrl & AGGR_SIGN_SET) {
                POINTonE2_dadd(&ctx->AggrSign.e2, &ctx->AggrSign.e2,
                                                  &ctx1->AggrSign.e2, NULL);
            } else if (ctx1->ctrl & AGGR_SIGN_SET) {
                ctx->ctrl |= AGGR_SIGN_SET;
                vec_copy(&ctx->AggrSign.e2, &ctx1->AggrSign.e2,
                         sizeof(ctx->AggrSign.e2));
            }
            break;
        case AGGR_UNDEFINED:
            vec_copy(ctx, ctx1, sizeof(*ctx));
            return BLST_SUCCESS;
        default:
            return BLST_AGGR_TYPE_MISMATCH;
    }

    if (ctx->ctrl & ctx1->ctrl & AGGR_GT_SET) {
        mul_fp12(ctx->GT, ctx->GT, ctx1->GT);
    } else if (ctx1->ctrl & AGGR_GT_SET) {
        ctx->ctrl |= AGGR_GT_SET;
        vec_copy(ctx->GT, ctx1->GT, sizeof(ctx->GT));
    }

    return BLST_SUCCESS;
}

static limb_t PAIRING_FinalVerify(const PAIRING *ctx, const vec384fp12 GTsig)
{
    vec384fp12 GT;

    if (!(ctx->ctrl & AGGR_GT_SET))
        return 0;

    if (GTsig != NULL) {
        vec_copy(GT, GTsig, sizeof(GT));
    } else if (ctx->ctrl & AGGR_SIGN_SET) {
        AggregatedSignature AggrSign;

        switch (ctx->ctrl & MIN_SIG_OR_PK) {
            case AGGR_MIN_SIG:
                POINTonE1_from_Jacobian(&AggrSign.e1, &ctx->AggrSign.e1);
                miller_loop_n(GT, (const POINTonE2_affine *)&BLS12_381_G2,
                                  (const POINTonE1_affine *)&AggrSign.e1, 1);
                break;
            case AGGR_MIN_PK:
                POINTonE2_from_Jacobian(&AggrSign.e2, &ctx->AggrSign.e2);
                miller_loop_n(GT, (const POINTonE2_affine *)&AggrSign.e2,
                                  (const POINTonE1_affine *)&BLS12_381_G1, 1);
                break;
            default:
                return 0;
        }
    } else {
        return 0;
    }

    conjugate_fp12(GT);
    mul_fp12(GT, GT, ctx->GT);
    final_exp(GT, GT);

    /* return GT==1 */
    return vec_is_equal(GT[0][0], BLS12_381_Rx.p2, sizeof(GT[0][0])) &
           vec_is_zero(GT[0][1], sizeof(GT) - sizeof(GT[0][0]));
}

limb_t blst_pairing_finalverify(const PAIRING *ctx, const vec384fp12 GTsig)
{   return PAIRING_FinalVerify(ctx, GTsig);   }

/*
 * PAIRING context-free entry points.
 *
 * To perform FastAggregateVerify, aggregate all public keys and
 * signatures with corresponding blst_aggregate_in_g{12}, convert
 * result to affine and call suitable blst_core_verify_pk_in_g{12}
 * or blst_aggregated_in_g{12}...
 */
BLST_ERROR blst_aggregate_in_g1(POINTonE1 *out, const POINTonE1 *in,
                                                const unsigned char *zwire)
{
    POINTonE1_affine P[1];

    if (zwire[0] & 0x40) {      /* infinity? */
        if (in == NULL)
            vec_zero(out, sizeof(*out));
        return BLST_SUCCESS;
    }

    if (zwire[0] & 0x80) {      /* compressed? */
        BLST_ERROR ret = POINTonE1_Uncompress(P, zwire);
        if (ret != BLST_SUCCESS)
            return ret;
    } else {
        POINTonE1_Deserialize_BE(P, zwire);
        if (!POINTonE1_affine_on_curve(P))
            return BLST_POINT_NOT_ON_CURVE;
    }

    if (!POINTonE1_in_G1((POINTonE1 *)P))
        return BLST_POINT_NOT_IN_GROUP;

    if (in == NULL) {
        vec_copy(out->X, P->X, 2*sizeof(out->X));
        vec_copy(out->Z, BLS12_381_Rx.p, sizeof(out->Z));
    } else {
        POINTonE1_dadd_affine(out, in, (POINTonE1 *)P);
    }

    return BLST_SUCCESS;
}

BLST_ERROR blst_aggregate_in_g2(POINTonE2 *out, const POINTonE2 *in,
                                                const unsigned char *zwire)
{
    POINTonE2_affine P[1];

    if (zwire[0] & 0x40) {      /* infinity? */
        if (in == NULL)
            vec_zero(out, sizeof(*out));
        return BLST_SUCCESS;
    }

    if (zwire[0] & 0x80) {      /* compressed? */
        BLST_ERROR ret = POINTonE2_Uncompress(P, zwire);
        if (ret != BLST_SUCCESS)
            return ret;
    } else {
        POINTonE2_Deserialize_BE(P, zwire);
        if (!POINTonE2_affine_on_curve(P))
            return BLST_POINT_NOT_ON_CURVE;
    }

    if (!POINTonE2_in_G2((POINTonE2 *)P))
        return BLST_POINT_NOT_IN_GROUP;

    if (in == NULL) {
        vec_copy(out->X, P->X, 2*sizeof(out->X));
        vec_copy(out->Z, BLS12_381_Rx.p, sizeof(out->Z));
    } else {
        POINTonE2_dadd_affine(out, in, (POINTonE2 *)P);
    }
    return BLST_SUCCESS;
}

void blst_aggregated_in_g1(vec384fp12 ret, const POINTonE1_affine *sig)
{   miller_loop_n(ret, (const POINTonE2_affine *)&BLS12_381_G2, sig, 1);   }

void blst_aggregated_in_g2(vec384fp12 ret, const POINTonE2_affine *sig)
{   miller_loop_n(ret, sig, (const POINTonE1_affine *)&BLS12_381_G1, 1);   }

BLST_ERROR blst_core_verify_pk_in_g1(const POINTonE1_affine *pk,
                                     const POINTonE2_affine *signature,
                                     int hash_or_encode,
                                     const void *msg, size_t msg_len,
                                     const void *DST, size_t DST_len,
                                     const void *aug, size_t aug_len)
{
    PAIRING ctx;
    BLST_ERROR ret;

    ctx.ctrl = AGGR_UNDEFINED | (hash_or_encode ? AGGR_HASH_OR_ENCODE : 0);
    ctx.nelems = 0;
    ctx.DST = DST;
    ctx.DST_len = DST_len;

    ret = PAIRING_Aggregate_PK_in_G1(&ctx, pk, signature, NULL, 0,
                                     msg, msg_len, aug, aug_len);
    if (ret != BLST_SUCCESS)
        return ret;

    PAIRING_Commit(&ctx);

    return PAIRING_FinalVerify(&ctx, NULL) ? BLST_SUCCESS : BLST_VERIFY_FAIL;
}

BLST_ERROR blst_core_verify_pk_in_g2(const POINTonE2_affine *pk,
                                     const POINTonE1_affine *signature,
                                     int hash_or_encode,
                                     const void *msg, size_t msg_len,
                                     const void *DST, size_t DST_len,
                                     const void *aug, size_t aug_len)
{
    PAIRING ctx;
    BLST_ERROR ret;

    ctx.ctrl = AGGR_UNDEFINED | (hash_or_encode ? AGGR_HASH_OR_ENCODE : 0);
    ctx.nelems = 0;
    ctx.DST = DST;
    ctx.DST_len = DST_len;

    ret = PAIRING_Aggregate_PK_in_G2(&ctx, pk, signature, NULL, 0,
                                     msg, msg_len, aug, aug_len);
    if (ret != BLST_SUCCESS)
        return ret;

    PAIRING_Commit(&ctx);

    return PAIRING_FinalVerify(&ctx, NULL) ? BLST_SUCCESS : BLST_VERIFY_FAIL;
}
