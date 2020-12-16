# blst [![Crates.io](https://img.shields.io/crates/v/blst.svg)](https://crates.io/crates/blst)

The `blst` crate provides a rust interface to the blst BLS12-381 signature library.

## Build
[bindgen](https://github.com/rust-lang/rust-bindgen) is used to generate FFI bindings to blst.h. Then [build.rs](https://github.com/supranational/blst/blob/master/bindings/rust/build.rs) invokes C compiler to compile everything into libblst.a within the rust target build area. On Linux it's possible to choose compiler by setting `CC` environment variable.

Everything can be built and run with the typical cargo commands:

```
cargo test
cargo bench
```

If the test or target application crashes with an "illegal instruction" exception [after copying to an older system], set `CFLAGS` environment variable to `‑D__BLST_PORTABLE__` prior clean build. Alternatively, if you compile on an older Intel system, but will execute the binary on a newer one, consider instead `‑D__ADX__` for better performance.

## Usage
There are two primary modes of operation that can be chosen based on declaration path:

For minimal-pubkey-size operations:
```
use blst::min_pk::*
```

For minimal-signature-size operations:
```
use blst::min_sig::*
```

There are five structs with inherent implementations that provide the BLS12-381 signature functionality.
```
SecretKey
PublicKey
AggregatePublicKey
Signature
AggregateSignature
```

A simple example for generating a key, signing a message, and verifying the message:
```
let mut ikm = [0u8; 32];
rng.fill_bytes(&mut ikm);

let sk = SecretKey::key_gen(&ikm, &[]).unwrap();
let pk = sk.sk_to_pk();

let dst = b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_";
let msg = b"blst is such a blast";
let sig = sk.sign(msg, dst, &[]);

let err = sig.verify(msg, dst, &[], &pk);
assert_eq!(err, BLST_ERROR::BLST_SUCCESS);
```

See the tests in src/lib.rs and benchmarks in benches/blst_benches.rs for further examples of usage.
