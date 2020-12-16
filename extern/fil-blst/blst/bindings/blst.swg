// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

%module blst
%rename("%(strip:[blst_])s") "";    // prefix is redundant in named module

%include "exception.i"
#ifdef __cplusplus
%include "std_string.i"
#else
#warning consider using C++ interface
#endif
%include "stdint.i"

%apply const char* { const byte*, const byte[ANY] }

#if defined(SWIGPYTHON) || defined(SWIGPERL)

// some sorcery to allow assignments as output, e.g.
//      hash = p.encode_to(b"foo")
%typemap(in, numinputs=0) OBJECT *OUTPUT($1_basetype temp) { $1 = &temp; };
%typemap(argout) OBJECT *OUTPUT {
# ifdef SWIGPYTHON
    PyObject *obj = SWIG_NewPointerObj(memcpy(malloc(sizeof($1_basetype)),
                                              $1,sizeof($1_basetype)),
                                       $descriptor, SWIG_POINTER_NEW);
    $result = ($result==NULL) ? obj
                              : SWIG_Python_AppendOutput($result, obj);
# else // TODO: figure out more language-specific ways to return multi-values...
    if ($result == NULL)
        $result = SWIG_NewPointerObj(memcpy(malloc(sizeof($1_basetype)),
                                            $1,sizeof($1_basetype)),
                                     $descriptor, SWIG_POINTER_NEW);
# endif
};
%apply OBJECT *OUTPUT {
    blst_p1        *out, blst_p1        *out_pk, blst_p1        *out_sig,
    blst_p1_affine *out, blst_p1_affine *out_pk, blst_p1_affine *out_sig,
    blst_p2        *out, blst_p2        *out_pk, blst_p2        *out_sig,
    blst_p2_affine *out, blst_p2_affine *out_pk, blst_p2_affine *out_sig,
    blst_scalar    *out, blst_scalar    *out_SK,
    blst_fp12      *out
};

#ifdef __cplusplus
%typemap(out) BLST_ERROR {
    if ($1 != BLST_SUCCESS) {
        SWIG_exception(SWIG_ValueError, BLST_ERROR_str[$1]);
        SWIG_fail;
    }
    $result = SWIG_From_int($1);
};
#endif

#elif defined(SWIGJAVA)

%header %{
#ifdef __cplusplus
# define JCALL(func, ...) jenv->func(__VA_ARGS__)
#else
# define JCALL(func, ...) (*jenv)->func(jenv, __VA_ARGS__)
#endif
%}

%include "enums.swg"
%include "arrays_java.i"
%javaconst(1);

%typemap(throws) BLST_ERROR %{
    SWIG_JavaThrowException(jenv, SWIG_JavaRuntimeException,
                                  BLST_ERROR_str[$1]);
%};

// handle input const byte[] more efficiently...
%apply signed char[] { const byte* }
%typemap(in) const byte* %{
    $1 = ($1_ltype)JCALL(GetByteArrayElements, $input, 0);
%};
%typemap(argout)  const byte* ""
%typemap(freearg) const byte* %{
    JCALL(ReleaseByteArrayElements, $input, (jbyte *)$1, JNI_ABORT);
%};

%apply const byte* { const byte[ANY] }
%typemap(in) const byte[ANY] {
    size_t sz = JCALL(GetArrayLength, $input);
    if (sz != $1_dim0) {
        SWIG_JavaThrowException(jenv, SWIG_JavaIndexOutOfBoundsException,
                                      "BLST_ERROR: input size mismatch");
        return $null;
    }
    $1 = ($1_ltype)JCALL(GetByteArrayElements, $input, 0);
};

%apply long long[] { const limb_t* scalar }
%typemap(in) const limb_t* scalar (size_t _global_scalar_sz) %{
    $1 = ($1_ltype)jenv->GetLongArrayElements($input, 0);
    _global_scalar_sz = jenv->GetArrayLength($input);
%};
%typemap(argout)  const limb_t* scalar ""
%typemap(freearg) const limb_t* scalar %{
    jenv->ReleaseLongArrayElements($input, (jlong *)$1, JNI_ABORT);
%};
#ifdef __cplusplus
%typemap(in) size_t nbits %{
    if (($1 = (size_t)$input) > 8*sizeof(jlong)*_global_scalar_sz) {
        SWIG_JavaThrowException(jenv, SWIG_JavaIndexOutOfBoundsException,
                                      "BLST_ERROR: scalar size mismatch");
        $1 = 8*sizeof(jlong)*_global_scalar_sz;
    }
%};
#endif

#endif  // SWIG<language>

// everybody has a way to bundle pointer and buffer size, but C:-(
#if SWIG_VERSION < 0x040000 && defined(SWIGJAVA)
%apply (char *STRING, size_t LENGTH) { (const char *STRING, size_t LENGTH) }
#endif
%apply (const char *STRING, size_t LENGTH) {
       (const byte *msg,    size_t msg_len),
       (const byte *DST,    size_t DST_len),
       (const byte *aug,    size_t aug_len),
       (const byte *IKM,    size_t IKM_len),
       (const byte *info,   size_t info_len)
};

// some sorcery to return byte[] from serialization methods
%typemap(in, numinputs=0) byte out[ANY] (byte temp[$1_dim0]) {
    $1 = temp;
};
%typemap(argout) byte out[ANY] {
#if defined(SWIGPYTHON)
    PyObject *obj = SWIG_FromCharPtrAndSize((char *)$1, $1_dim0);
    $result = ($result==NULL) ? obj
                              : SWIG_Python_AppendOutput($result, obj);
#elif defined(SWIGJAVA)
    $result = JCALL(NewByteArray, $1_dim0);
    if ($result != $null) {
        JCALL(SetByteArrayRegion, $result, 0, $1_dim0, (const jbyte *)$1);
    }
#else // TODO: figure out more language-specific ways to return multi-values...
    if ($result == NULL)
        $result = SWIG_FromCharPtrAndSize((char *)$1, $1_dim0);
#endif
};
%typemap(freearg) byte out[ANY] ""
#ifdef SWIGJAVA
%typemap(jni)     byte out[ANY] "jbyteArray"
%typemap(jtype)   byte out[ANY] "byte[]"
%typemap(jstype)  byte out[ANY] "byte[]"
%typemap(javaout) byte out[ANY] "{ return $jnicall; }"
#endif
%apply byte out[ANY] {
    void to_bendian,    void blst_bendian_from_scalar,
    void to_lendian,    void blst_lendian_from_scalar,
    void serialize,     void blst_p1_serialize, void blst_p1_affine_serialize,
                        void blst_p2_serialize, void blst_p2_affine_serialize,
    void compress,      void blst_p1_compress,  void blst_p1_affine_compress,
                        void blst_p2_compress,  void blst_p2_affine_compress,
    void blst_sk_to_pk2_in_g1,  void blst_sign_pk2_in_g1,
    void blst_sk_to_pk2_in_g2,  void blst_sign_pk2_in_g2
};

#ifdef __cplusplus
%apply const std::string& { const std::string* };

#pragma SWIG nowarn=509,516

%ignore nullptr;
%ignore None;
%catches(BLST_ERROR) P1(const byte* in);
%catches(BLST_ERROR) P1_Affine(const byte* in);
%catches(BLST_ERROR) aggregate(const P1_Affine& in);

%catches(BLST_ERROR) P2(const byte* in);
%catches(BLST_ERROR) P2_Affine(const byte* in);
%catches(BLST_ERROR) aggregate(const P2_Affine& in);

typedef enum {
    BLST_SUCCESS = 0,
    BLST_BAD_ENCODING,
    BLST_POINT_NOT_ON_CURVE,
    BLST_POINT_NOT_IN_GROUP,
    BLST_AGGR_TYPE_MISMATCH,
    BLST_VERIFY_FAIL,
} BLST_ERROR;

%include "blst.hpp"

extern const blst::P1_Affine BLS12_381_G1;
extern const blst::P1_Affine BLS12_381_NEG_G1;
extern const blst::P2_Affine BLS12_381_G2;
extern const blst::P2_Affine BLS12_381_NEG_G2;

#else
%ignore blst_fr;
%ignore blst_fp;
%ignore blst_fp2;
%ignore blst_fp6;
%ignore blst_scalar_from_uint32;
%ignore blst_scalar_from_uint64;
%ignore blst_uint32_from_scalar;
%ignore blst_uint64_from_scalar;
%ignore blst_pairing_init;
%ignore blst_pairing_get_dst;

%include "blst.h"
%include "blst_aux.h"
%extend blst_pairing {
    blst_pairing(bool hash_or_encode, const byte *DST DEFNULL,
                                      size_t DST_len DEFNULL)
    {   void *ret = malloc(blst_pairing_sizeof());
        if (DST_len != 0) {
            void *dst = malloc(DST_len);
            memcpy(dst, DST, DST_len);
            blst_pairing_init(ret, hash_or_encode, dst, DST_len);
        } else {
            blst_pairing_init(ret, hash_or_encode, NULL, 0);
        }
        return ret;
    }
    ~blst_pairing()
    {   void *dst = (void *)blst_pairing_get_dst($self);
        if (dst != NULL) free(dst);
        free($self);
    }
};
#endif

%begin %{
#ifdef __cplusplus
# include "blst.hpp"
using namespace blst;
#else
# include "blst.h"
#endif

static const char *const BLST_ERROR_str [] = {
    "BLST_ERROR: success",
    "BLST_ERROR: bad point encoding",
    "BLST_ERROR: point is not on curve",
    "BLST_ERROR: point is not in group",
    "BLST_ERROR: context type mismatch",
    "BLST_ERROR: verify failed",
};

#define SWIG_PYTHON_STRICT_BYTE_CHAR
%}

#if defined(SWIGPYTHON) || defined(SWIGPERL)
%include "cdata.i"
#endif
