/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */

#include "vect.h"

/*
 * Following are some reference C implementations to assist new
 * assembly modules development, as starting-point stand-ins and for
 * cross-checking. In order to "polyfil" specific subroutine redefine
 * it on compiler command line, e.g. -Dmul_mont_384x=_mul_mont_384x.
 */

#ifdef lshift_mod_384
void lshift_mod_384(vec384x ret, const vec384x a, size_t n, const vec384 p)
{
    while(n--)
        add_mod_384(ret, a, a, mod), a = ret;
}
#endif

#ifdef mul_by_8_mod_384
void mul_by_8_mod_384(vec384 ret, const vec384 a, const vec384 mod)
{   lshift_mod_384(ret, a, 3, mod);   }
#endif

#ifdef mul_by_3_mod_384
void mul_by_3_mod_384(vec384 ret, const vec384 a, const vec384 mod)
{
    vec384 t;

    add_mod_384(t, a, a, mod);
    add_mod_384(ret, t, a, mod);
}
#endif

#ifdef mul_by_3_mod_384x
void mul_by_3_mod_384x(vec384x ret, const vec384x a, const vec384 mod)
{
    mul_by_3_mod_384(ret[0], a[0], mod);
    mul_by_3_mod_384(ret[1], a[1], mod);
}
#endif

#ifdef mul_by_8_mod_384x
void mul_by_8_mod_384x(vec384 ret, const vec384 a, const vec384 mod)
{
    mul_by_8_mod_384(ret[0], a[0], mod);
    mul_by_8_mod_384(ret[1], a[1], mod);
}
#endif

#ifdef mul_by_1_plus_i_mod_384x
void mul_by_1_plus_i_mod_384x(vec384x ret, const vec384x a, const vec384 mod)
{
    vec384 t;

    add_mod_384(t, a[0], a[1], mod);
    sub_mod_384(ret[0], a[0], a[1], mod);
    vec_copy(ret[1], t, sizeof(t));
}
#endif

#ifdef add_mod_384x
void add_mod_384x(vec384x ret, const vec384x a, const vec384x b,
                  const vec384 mod)
{
    add_mod_384(ret[0], a[0], b[0], mod);
    add_mod_384(ret[1], a[1], b[1], mod);
}
#endif

#ifdef sub_mod_384x
void sub_mod_384x(vec384x ret, const vec384x a, const vec384x b,
                  const vec384 mod)
{
    sub_mod_384(ret[0], a[0], b[0], mod);
    sub_mod_384(ret[1], a[1], b[1], mod);
}
#endif

#ifdef lshift_mod_384x
void lshift_mod_384x(vec384x ret, const vec384x a, size_t n, const vec384 p)
{
    lshift_mod_384(ret[0], a[0], n, p);
    lshift_mod_384(ret[1], a[1], n, p);
}
#endif

#if defined(mul_mont_384x) && !(defined(__ADX__) && !defined(__BLST_PORTABLE__))
void mul_mont_384x(vec384x ret, const vec384x a, const vec384x b,
                   const vec384 mod, limb_t n0)
{
    vec768 t0, t1, t2;
    vec384 aa, bb;

    mul_384(t0, a[0], b[0]);
    mul_384(t1, a[1], b[1]);

    add_mod_384(aa, a[0], a[1], mod);
    add_mod_384(bb, b[0], b[1], mod);
    mul_384(t2, aa, bb);
    sub_mod_384x384(t2, t2, t0, mod);
    sub_mod_384x384(t2, t2, t1, mod);

    sub_mod_384x384(t0, t0, t1, mod);

    redc_mont_384(ret[0], t0, mod, n0);
    redc_mont_384(ret[1], t2, mod, n0);
}
#endif

#if defined(sqr_mont_384x) && !(defined(__ADX__) && !defined(__BLST_PORTABLE__))
void sqr_mont_384x(vec384x ret, const vec384x a, const vec384 mod, limb_t n0)
{
    vec384 t0, t1;

    add_mod_384(t0, a[0], a[1], mod);
    sub_mod_384(t1, a[0], a[1], mod);

    mul_mont_384(ret[1], a[0], a[1], mod, n0);
    add_mod_384(ret[1], ret[1], ret[1], mod);

    mul_mont_384(ret[0], t0, t1, mod, n0);
}
#endif
