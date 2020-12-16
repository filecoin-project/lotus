/*
 * Copyright Supranational LLC
 * Licensed under the Apache License, Version 2.0, see LICENSE for details.
 * SPDX-License-Identifier: Apache-2.0
 */
#ifndef __BLST_HPP__
#define __BLST_HPP__

#include <string>
#include <cstring>

#if __cplusplus >= 201703L
# include <string_view>
# ifndef app__string_view
#  define app__string_view std::string_view // std::basic_string_view<byte>
# endif
#endif

namespace blst {

#if __cplusplus >= 201703L
static const app__string_view None;
#endif

#if __cplusplus < 201103L
# ifdef __GNUG__
#  define nullptr __null
# else
#  define nullptr 0
# endif
#endif

#include "blst.h"

class P1_Affine;
class P1;
class P2_Affine;
class P2;
class Pairing;

/*
 * As for SecretKey being struct and not class, and lack of constructors
 * with one accepting for example |IKM|. We can't make assumptions about
 * application's policy toward handling secret key material. Hence it's
 * argued that application is entitled for transparent structure, not
 * opaque or semi-opaque class. And in the context it's appropriate not
 * to "entice" developers with idiomatic constructors:-) Though this
 * doesn't really apply to SWIG-assisted interfaces...
 */
struct SecretKey {
#ifdef SWIG
private:
#endif
    blst_scalar key;

#ifdef SWIG
public:
#endif
    void keygen(const byte* IKM, size_t IKM_len,
                const std::string& info = "")
    {   blst_keygen(&key, IKM, IKM_len,
                    reinterpret_cast<const byte *>(info.data()),
                    info.size());
    }
#if __cplusplus >= 201703L
    void keygen(const app__string_view IKM, // string_view by value, cool!
                const std::string& info = "")
    {   keygen(reinterpret_cast<const byte *>(IKM.data()),
               IKM.size(), info);
    }
#endif
    void from_bendian(const byte in[32]) { blst_scalar_from_bendian(&key, in); }
    void from_lendian(const byte in[32]) { blst_scalar_from_lendian(&key, in); }

    void to_bendian(byte out[32]) const
    {   blst_bendian_from_scalar(out, &key);   }
    void to_lendian(byte out[32]) const
    {   blst_lendian_from_scalar(out, &key);   }
};

class P1_Affine {
private:
    blst_p1_affine point;

public:
    P1_Affine() { memset(&point, 0, sizeof(point)); }
    P1_Affine(const byte *in)
    {   BLST_ERROR err = blst_p1_deserialize(&point, in);
        if (err != BLST_SUCCESS)
            throw err;
    }
    P1_Affine(const P1& jacobian);

    P1 to_jacobian() const;
    void serialize(byte out[96]) const
    {   blst_p1_affine_serialize(out, &point);   }
    void compress(byte out[48]) const
    {   blst_p1_affine_compress(out, &point);   }
    bool on_curve() const { return blst_p1_affine_on_curve(&point); }
    bool in_group() const { return blst_p1_affine_in_g1(&point);    }
    bool is_inf() const   { return blst_p1_affine_is_inf(&point);   }
    BLST_ERROR core_verify(const P2_Affine& pk, bool hash_or_encode,
                           const byte* msg, size_t msg_len,
                           const std::string& DST = "",
                           const byte* aug = nullptr, size_t aug_len = 0) const;
#if __cplusplus >= 201703L
    BLST_ERROR core_verify(const P2_Affine& pk, bool hash_or_encode,
                           const app__string_view msg,
                           const std::string& DST = "",
                           const app__string_view aug = None) const;
#endif
    static const P1_Affine& generator()
    {
        return *reinterpret_cast<const P1_Affine*>(blst_p1_affine_generator());
    }

private:
    friend class Pairing;
    friend class P2_Affine;
    friend class PT;
    friend class P1;
    operator const blst_p1_affine*() const { return &point; }
};

class P1 {
private:
    blst_p1 point;

public:
    P1() { memset(&point, 0, sizeof(point)); }
    P1(SecretKey& sk) { blst_sk_to_pk_in_g1(&point, &sk.key); }
    P1(const byte *in)
    {   blst_p1_affine a;
        BLST_ERROR err = blst_p1_deserialize(&a, in);
        if (err != BLST_SUCCESS)
            throw err;
        blst_p1_from_affine(&point, &a);
    }
    P1(const P1_Affine& affine) { blst_p1_from_affine(&point, affine); }

    P1_Affine to_affine() const         { P1_Affine ret(*this); return ret;  }
    void serialize(byte out[96]) const  { blst_p1_serialize(out, &point);    }
    void compress(byte out[48]) const   { blst_p1_compress(out, &point);     }
    bool is_inf() const                 { return blst_p1_is_inf(&point);     }
    void aggregate(const P1_Affine& in)
    {   if (blst_p1_affine_in_g1(in))
            blst_p1_add_or_double_affine(&point, &point, in);
        else
            throw BLST_POINT_NOT_IN_GROUP;
    }
    P1* sign_with(SecretKey& sk)
    {   blst_p1_mult(&point, &point, &sk.key, 255); return this;   }
    P1* hash_to(const byte* msg, size_t msg_len,
                const std::string& DST = "",
                const byte* aug = nullptr, size_t aug_len = 0)
    {   blst_hash_to_g1(&point, msg, msg_len,
                        reinterpret_cast<const byte *>(DST.data()),
                        DST.size(), aug, aug_len);
        return this;
    }
    P1* encode_to(const byte* msg, size_t msg_len,
                  const std::string& DST = "",
                  const byte* aug = nullptr, size_t aug_len = 0)
    {   blst_encode_to_g1(&point, msg, msg_len,
                          reinterpret_cast<const byte *>(DST.data()),
                          DST.size(), aug, aug_len);
        return this;
    }
#if __cplusplus >= 201703L
    P1* hash_to(const app__string_view msg, const std::string& DST = "",
                const app__string_view aug = None)
    {   return hash_to(reinterpret_cast<const byte *>(msg.data()),
                       msg.size(), DST,
                       reinterpret_cast<const byte *>(aug.data()),
                       aug.size());
    }
    P1* encode_to(const app__string_view msg, const std::string& DST = "",
                  const app__string_view aug = None)
    {   return encode_to(reinterpret_cast<const byte *>(msg.data()),
                         msg.size(), DST,
                         reinterpret_cast<const byte *>(aug.data()),
                         aug.size());
    }
#endif
    P1* cneg(bool flag)
    {   blst_p1_cneg(&point, flag); return this;   }
    P1* add(const P1& a)
    {   blst_p1_add_or_double(&point, &point, a); return this;   }
    P1* add(const P1_Affine &a)
    {   blst_p1_add_or_double_affine(&point, &point, a); return this;   }
    P1* dbl()
    {   blst_p1_double(&point, &point); return this;   }
    static P1 add(const P1& a, const P1& b)
    {   P1 ret; blst_p1_add_or_double(&ret.point, a, b); return ret;   }
    static P1 add(const P1& a, const P1_Affine& b)
    {   P1 ret; blst_p1_add_or_double_affine(&ret.point, a, b); return ret;   }
    static P1 dbl(const P1& a)
    {   P1 ret; blst_p1_double(&ret.point, a); return ret;   }
    static const P1& generator()
    {   return *reinterpret_cast<const P1*>(blst_p1_generator());   }

private:
    friend class P1_Affine;
    operator const blst_p1*() const { return &point; }
};

class P2_Affine {
private:
    blst_p2_affine point;

public:
    P2_Affine() { memset(&point, 0, sizeof(point)); }
    P2_Affine(const byte *in)
    {   BLST_ERROR err = blst_p2_deserialize(&point, in);
        if (err != BLST_SUCCESS)
            throw err;
    }
    P2_Affine(const P2& jacobian);

    P2 to_jacobian() const;
    void serialize(byte out[192]) const
    {   blst_p2_affine_serialize(out, &point);   }
    void compress(byte out[96]) const
    {   blst_p2_affine_compress(out, &point);   }
    bool on_curve() const { return blst_p2_affine_on_curve(&point); }
    bool in_group() const { return blst_p2_affine_in_g2(&point);    }
    bool is_inf() const   { return blst_p2_affine_is_inf(&point);   }
    BLST_ERROR core_verify(const P1_Affine& pk, bool hash_or_encode,
                           const byte* msg, size_t msg_len,
                           const std::string& DST = "",
                           const byte* aug = nullptr, size_t aug_len = 0) const;
#if __cplusplus >= 201703L
    BLST_ERROR core_verify(const P1_Affine& pk, bool hash_or_encode,
                           const app__string_view msg,
                           const std::string& DST = "",
                           const app__string_view aug = None) const;
#endif
    static const P2_Affine& generator()
    {
        return *reinterpret_cast<const P2_Affine*>(blst_p2_affine_generator());
    }

private:
    friend class Pairing;
    friend class P1_Affine;
    friend class PT;
    friend class P2;
    operator const blst_p2_affine*() const { return &point; }
};

class P2 {
private:
    blst_p2 point;

public:
    P2() { memset(&point, 0, sizeof(point)); }
    P2(SecretKey& sk) { blst_sk_to_pk_in_g2(&point, &sk.key); }
    P2(const byte *in)
    {   blst_p2_affine a;
        BLST_ERROR err = blst_p2_deserialize(&a, in);
        if (err != BLST_SUCCESS)
            throw err;
        blst_p2_from_affine(&point, &a);
    }
    P2(const P2_Affine& affine) { blst_p2_from_affine(&point, affine); }

    P2_Affine to_affine() const         { P2_Affine ret(*this); return ret; }
    void serialize(byte out[192]) const { blst_p2_serialize(out, &point);   }
    void compress(byte out[96]) const   { blst_p2_compress(out, &point);    }
    bool is_inf() const                 { return blst_p2_is_inf(&point);    }
    void aggregate(const P2_Affine& in)
    {   if (blst_p2_affine_in_g2(in))
            blst_p2_add_or_double_affine(&point, &point, in);
        else
            throw BLST_POINT_NOT_IN_GROUP;
    }
    P2* sign_with(SecretKey& sk)
    {   blst_p2_mult(&point, &point, &sk.key, 255); return this;   }
    P2* hash_to(const byte* msg, size_t msg_len,
                const std::string& DST = "",
                const byte* aug = nullptr, size_t aug_len = 0)
    {   blst_hash_to_g2(&point, msg, msg_len,
                        reinterpret_cast<const byte *>(DST.data()),
                        DST.size(), aug, aug_len);
        return this;
    }
    P2* encode_to(const byte* msg, size_t msg_len,
                  const std::string& DST = "",
                  const byte* aug = nullptr, size_t aug_len = 0)
    {   blst_encode_to_g2(&point, msg, msg_len,
                          reinterpret_cast<const byte *>(DST.data()),
                          DST.size(), aug, aug_len);
        return this;
    }
#if __cplusplus >= 201703L
    P2* hash_to(const app__string_view msg, const std::string& DST = "",
                const app__string_view aug = None)
    {   return hash_to(reinterpret_cast<const byte *>(msg.data()),
                       msg.size(), DST,
                       reinterpret_cast<const byte *>(aug.data()),
                       aug.size());
    }
    P2* encode_to(const app__string_view msg, const std::string& DST = "",
                  const app__string_view aug = None)
    {   return encode_to(reinterpret_cast<const byte *>(msg.data()),
                         msg.size(), DST,
                         reinterpret_cast<const byte *>(aug.data()),
                         aug.size());
    }
#endif
    P2* cneg(bool flag)
    {   blst_p2_cneg(&point, flag); return this;   }
    P2* add(const P2& a)
    {   blst_p2_add_or_double(&point, &point, a); return this;   }
    P2* add(const P2_Affine &a)
    {   blst_p2_add_or_double_affine(&point, &point, a); return this;   }
    P2* dbl()
    {   blst_p2_double(&point, &point); return this;   }
    static P2 add(const P2& a, const P2& b)
    {   P2 ret; blst_p2_add_or_double(&ret.point, a, b); return ret;   }
    static P2 add(const P2& a, const P2_Affine& b)
    {   P2 ret; blst_p2_add_or_double_affine(&ret.point, a, b); return ret;   }
    static P2 dbl(const P2& a)
    {   P2 ret; blst_p2_double(&ret.point, a); return ret;   }
    static const P2& generator()
    {   return *reinterpret_cast<const P2*>(blst_p2_generator());   }

private:
    friend class P2_Affine;
    operator const blst_p2*() const { return &point; }
};

inline P1_Affine::P1_Affine(const P1& jacobian)
{   blst_p1_to_affine(&point, jacobian);   }
inline P2_Affine::P2_Affine(const P2& jacobian)
{   blst_p2_to_affine(&point, jacobian);   }

inline P1 P1_Affine::to_jacobian() const { P1 ret(*this); return ret; }
inline P2 P2_Affine::to_jacobian() const { P2 ret(*this); return ret; }

inline BLST_ERROR P1_Affine::core_verify(const P2_Affine& pk,
                                         bool hash_or_encode,
                                         const byte* msg, size_t msg_len,
                                         const std::string& DST,
                                         const byte* aug, size_t aug_len) const
{   return blst_core_verify_pk_in_g2(pk, &point, hash_or_encode,
                                     msg, msg_len,
                                     reinterpret_cast<const byte *>(DST.data()),
                                     DST.size(), aug, aug_len);
}
inline BLST_ERROR P2_Affine::core_verify(const P1_Affine& pk,
                                         bool hash_or_encode,
                                         const byte* msg, size_t msg_len,
                                         const std::string& DST,
                                         const byte* aug, size_t aug_len) const
{   return blst_core_verify_pk_in_g1(pk, &point, hash_or_encode,
                                     msg, msg_len,
                                     reinterpret_cast<const byte *>(DST.data()),
                                     DST.size(), aug, aug_len);
}
#if __cplusplus >= 201703L
inline BLST_ERROR P1_Affine::core_verify(const P2_Affine& pk,
                                         bool hash_or_encode,
                                         const app__string_view msg,
                                         const std::string& DST,
                                         const app__string_view aug) const
{   return core_verify(pk, hash_or_encode,
                       reinterpret_cast<const byte *>(msg.data()),
                       msg.size(), DST,
                       reinterpret_cast<const byte *>(aug.data()),
                       aug.size());
}
inline BLST_ERROR P2_Affine::core_verify(const P1_Affine& pk,
                                         bool hash_or_encode,
                                         const app__string_view msg,
                                         const std::string& DST,
                                         const app__string_view aug) const
{   return core_verify(pk, hash_or_encode,
                       reinterpret_cast<const byte *>(msg.data()),
                       msg.size(), DST,
                       reinterpret_cast<const byte *>(aug.data()),
                       aug.size());
}
#endif

class PT {
private:
    blst_fp12 value;

public:
    PT(const P1_Affine& p) { blst_aggregated_in_g1(&value, p); }
    PT(const P2_Affine& p) { blst_aggregated_in_g2(&value, p); }

private:
    friend class Pairing;
    operator const blst_fp12*() const { return &value; }
};

class Pairing {
private:
    operator blst_pairing*()
    {   return reinterpret_cast<blst_pairing *>(this);   }
    operator const blst_pairing*() const
    {   return reinterpret_cast<const blst_pairing *>(this);   }

    void init(bool hash_or_encode, const byte* DST, size_t DST_len)
    {   // Copy DST to heap, std::string can be volatile, especially in SWIG:-(
        byte *dst = new byte[DST_len];
        memcpy(dst, DST, DST_len);
        blst_pairing_init(*this, hash_or_encode, dst, DST_len);
    }

public:
#ifndef SWIG
    void* operator new(size_t)
    {   return new uint64_t[blst_pairing_sizeof()/sizeof(uint64_t)];   }
    void operator delete(void *ptr)
    {   delete[] static_cast<uint64_t*>(ptr);   }
#endif

    Pairing(bool hash_or_encode, const byte* DST, size_t DST_len)
    {   init(hash_or_encode, DST, DST_len);   }
    Pairing(bool hash_or_encode, const std::string& DST)
    {   init(hash_or_encode, reinterpret_cast<const byte*>(DST.data()),
                             DST.size());
    }
#if __cplusplus >= 201703L
    Pairing(bool hash_or_encode, const app__string_view DST)
    {   init(hash_or_encode, reinterpret_cast<const byte*>(DST.data()),
                             DST.size());
    }
#endif
    ~Pairing() { delete[] blst_pairing_get_dst(*this); }

    BLST_ERROR aggregate(const P1_Affine* pk, const P2_Affine* sig,
                         const byte* msg, size_t msg_len,
                         const byte* aug = nullptr, size_t aug_len = 0)
    {   return blst_pairing_aggregate_pk_in_g1(*this, *pk, *sig,
                         msg, msg_len, aug, aug_len);
    }
    BLST_ERROR aggregate(const P2_Affine* pk, const P1_Affine* sig,
                         const byte* msg, size_t msg_len,
                         const byte* aug = nullptr, size_t aug_len = 0)
    {   return blst_pairing_aggregate_pk_in_g2(*this, *pk, *sig,
                         msg, msg_len, aug, aug_len);
    }
    BLST_ERROR mul_n_aggregate(const P1_Affine* pk, const P2_Affine* sig,
                               const limb_t* scalar, size_t nbits,
                               const byte* msg, size_t msg_len,
                               const byte* aug = nullptr, size_t aug_len = 0)
    {   return blst_pairing_mul_n_aggregate_pk_in_g1(*this, *pk, *sig,
                               scalar, nbits, msg, msg_len, aug, aug_len);
    }
    BLST_ERROR mul_n_aggregate(const P2_Affine* pk, const P1_Affine* sig,
                               const limb_t* scalar, size_t nbits,
                               const byte* msg, size_t msg_len,
                               const byte* aug = nullptr, size_t aug_len = 0)
    {   return blst_pairing_mul_n_aggregate_pk_in_g2(*this, *pk, *sig,
                               scalar, nbits, msg, msg_len, aug, aug_len);
    }
#if __cplusplus >= 201703L
    BLST_ERROR aggregate(const P1_Affine* pk, const P2_Affine* sig,
                         const app__string_view msg,
                         const app__string_view aug = None)
    {   return aggregate(pk, sig,
                         reinterpret_cast<const byte *>(msg.data()),
                         msg.size(),
                         reinterpret_cast<const byte *>(aug.data()),
                         aug.size());
    }
    BLST_ERROR aggregate(const P2_Affine* pk, const P1_Affine* sig,
                         const app__string_view msg,
                         const app__string_view aug = None)
    {   return aggregate(pk, sig,
                         reinterpret_cast<const byte *>(msg.data()),
                         msg.size(),
                         reinterpret_cast<const byte *>(aug.data()),
                         aug.size());
    }
    BLST_ERROR mul_n_aggregate(const P1_Affine* pk, const P2_Affine* sig,
                               const limb_t* scalar, size_t nbits,
                               const app__string_view msg,
                               const app__string_view aug = None)
    {   return mul_n_aggregate(pk, sig, scalar, nbits,
                               reinterpret_cast<const byte *>(msg.data()),
                               msg.size(),
                               reinterpret_cast<const byte *>(aug.data()),
                               aug.size());
    }
    BLST_ERROR mul_n_aggregate(const P2_Affine* pk, const P1_Affine* sig,
                               const limb_t* scalar, size_t nbits,
                               const app__string_view msg,
                               const app__string_view aug = None)
    {   return mul_n_aggregate(pk, sig, scalar, nbits,
                               reinterpret_cast<const byte *>(msg.data()),
                               msg.size(),
                               reinterpret_cast<const byte *>(aug.data()),
                               aug.size());
    }
#endif
    void commit()
    {   blst_pairing_commit(*this);   }
    BLST_ERROR merge(const Pairing* ctx)
    {   return blst_pairing_merge(*this, *ctx);   }
    bool finalverify(const PT* sig = nullptr) const
    {   return blst_pairing_finalverify(*this, *sig);   }
};

} // namespace blst

#endif
