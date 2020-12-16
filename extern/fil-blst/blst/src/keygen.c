/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "consts.h"
#include "sha256.h"

typedef struct {
    SHA256_CTX ctx;
    unsigned int h_ipad[8];
    unsigned int h_opad[8];
    union { limb_t l[64/sizeof(limb_t)]; unsigned char c[64]; } tail;
} HMAC_SHA256_CTX;

static void HMAC_init(HMAC_SHA256_CTX *ctx, const void *K, size_t K_len)
{
    size_t i;

    if (K == NULL) {            /* reuse h_ipad and h_opad */
        sha256_hcopy(ctx->ctx.h, ctx->h_ipad);
        ctx->ctx.N = 64;
        vec_zero(ctx->ctx.buf, sizeof(ctx->ctx.buf));
        ctx->ctx.off = 0;

        return;
    }

    vec_zero(ctx->tail.c, sizeof(ctx->tail));
    if (K_len > 64) {
        sha256_init(&ctx->ctx);
        sha256_update(&ctx->ctx, K, K_len);
        sha256_final(ctx->tail.c, &ctx->ctx);
    } else {
        sha256_bcopy(ctx->tail.c, K, K_len);
    }

    for (i = 0; i < 64/sizeof(limb_t); i++)
        ctx->tail.l[i] ^= (limb_t)0x3636363636363636;

    sha256_init(&ctx->ctx);
    sha256_update(&ctx->ctx, ctx->tail.c, 64);
    sha256_hcopy(ctx->h_ipad, ctx->ctx.h);

    for (i = 0; i < 64/sizeof(limb_t); i++)
        ctx->tail.l[i] ^= (limb_t)(0x3636363636363636 ^ 0x5c5c5c5c5c5c5c5c);

    sha256_init_h(ctx->h_opad);
    sha256_block_data_order(ctx->h_opad, ctx->tail.c, 1);

    vec_zero(ctx->tail.c, sizeof(ctx->tail));
    ctx->tail.c[32] = 0x80;
    ctx->tail.c[62] = 3;        /* (64+32)*8 in big endian */
    ctx->tail.c[63] = 0;
}

static void HMAC_update(HMAC_SHA256_CTX *ctx, const unsigned char *inp,
                                              size_t len)
{   sha256_update(&ctx->ctx, inp, len);   }

static void HMAC_final(unsigned char md[32], HMAC_SHA256_CTX *ctx)
{
    sha256_final(ctx->tail.c, &ctx->ctx);
    sha256_hcopy(ctx->ctx.h, ctx->h_opad);
    sha256_block_data_order(ctx->ctx.h, ctx->tail.c, 1);
    sha256_emit(md, ctx->ctx.h);
}

static void HKDF_Extract(unsigned char PRK[32],
                         const void *salt, size_t salt_len,
                         const void *IKM,  size_t IKM_len,
                         HMAC_SHA256_CTX *ctx)
{
    unsigned char zero[1] = { 0 };

    HMAC_init(ctx, salt != NULL ? salt : zero, salt_len);
    HMAC_update(ctx, IKM, IKM_len);
#ifndef HKDF_TESTMODE
    /* Section 2.3 KeyGen in BLS-signature draft */
    HMAC_update(ctx, zero, 1);
#endif
    HMAC_final(PRK, ctx);
}

static void HKDF_Expand(unsigned char *OKM, size_t L,
                        const unsigned char PRK[32],
                        const void *info, size_t info_len,
                        HMAC_SHA256_CTX *ctx)
{
#if !defined(__STDC_VERSION__) || __STDC_VERSION__<199901
    unsigned char *info_prime = alloca(info_len + 2 + 1);
#else
    unsigned char info_prime[info_len + 2 + 1];
#endif

    HMAC_init(ctx, PRK, 32);

    if (info_len != 0)
        sha256_bcopy(info_prime, info, info_len);
#ifndef HKDF_TESTMODE
    /* Section 2.3 KeyGen in BLS-signature draft */
    info_prime[info_len + 0] = (unsigned char)(L >> 8);
    info_prime[info_len + 1] = (unsigned char)(L);
    info_len += 2;
#endif
    info_prime[info_len] = 1;   /* counter */
    HMAC_update(ctx, info_prime, info_len + 1);
    HMAC_final(ctx->tail.c, ctx);
    while (L > 32) {
        sha256_hcopy((unsigned int *)OKM, (const unsigned int *)ctx->tail.c);
        OKM += 32; L -= 32;
        ++info_prime[info_len]; /* counter */
        HMAC_init(ctx, NULL, 0);
        HMAC_update(ctx, ctx->tail.c, 32);
        HMAC_update(ctx, info_prime, info_len + 1);
        HMAC_final(ctx->tail.c, ctx);
    }
    sha256_bcopy(OKM, ctx->tail.c, L);
}

#ifndef HKDF_TESTMODE
void blst_keygen(vec256 SK, const void *IKM, size_t IKM_len,
                            const void *info, size_t info_len)
{
    struct {
        HMAC_SHA256_CTX ctx;
        unsigned char PRK[32], OKM[48];
        vec512 key;
    } scratch;
    size_t i;
    volatile limb_t *p;

    /*
     * Vet |info| since some callers were caught to be sloppy, e.g.
     * SWIG-4.0-generated Python wrapper...
     */
    info_len = info==NULL ? 0 : info_len;

    /* PRK = HKDF-Extract("BLS-SIG-KEYGEN-SALT-", IKM || I2OSP(0, 1)) */
    HKDF_Extract(scratch.PRK, "BLS-SIG-KEYGEN-SALT-", 20,
                              IKM, IKM_len, &scratch.ctx);

    /* OKM = HKDF-Expand(PRK, key_info || I2OSP(L, 2), L) */
    HKDF_Expand(scratch.OKM, sizeof(scratch.OKM), scratch.PRK,
                info, info_len, &scratch.ctx);

    /* SK = OS2IP(OKM) mod r */
    vec_zero(scratch.key, sizeof(scratch.key));
    limbs_from_be_bytes(scratch.key, scratch.OKM, sizeof(scratch.OKM));
    redc_mont_256(SK, scratch.key, BLS12_381_r, r0);
    mul_mont_sparse_256(SK, SK, BLS12_381_rRR, BLS12_381_r, r0);

    /*
     * scrub the stack just in case next callee inadvertently flashes
     * a fragment across application boundary...
     */
    for(p = (limb_t *)&scratch, i = 0; i < sizeof(scratch)/sizeof(limb_t); i++)
        p[i] = 0;
}
#endif
