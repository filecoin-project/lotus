// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#![allow(non_upper_case_globals)]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]

use std::any::Any;
use std::mem::{transmute, MaybeUninit};
use std::sync::{atomic::*, mpsc::channel, Arc, Mutex, Once};
use std::{ptr, slice};
use threadpool::ThreadPool;

fn da_pool() -> ThreadPool {
    static INIT: Once = Once::new();
    static mut POOL: *const Mutex<ThreadPool> = 0 as *const Mutex<ThreadPool>;

    INIT.call_once(|| {
        let pool = Mutex::new(ThreadPool::default());
        unsafe { POOL = transmute(Box::new(pool)) };
    });
    unsafe { (*POOL).lock().unwrap().clone() }
}

include!("bindings.rs");

impl PartialEq for blst_p1 {
    fn eq(&self, other: &Self) -> bool {
        unsafe { blst_p1_is_equal(self, other) }
    }
}

impl PartialEq for blst_p1_affine {
    fn eq(&self, other: &Self) -> bool {
        unsafe { blst_p1_affine_is_equal(self, other) }
    }
}

impl PartialEq for blst_p2 {
    fn eq(&self, other: &Self) -> bool {
        unsafe { blst_p2_is_equal(self, other) }
    }
}

impl PartialEq for blst_p2_affine {
    fn eq(&self, other: &Self) -> bool {
        unsafe { blst_p2_affine_is_equal(self, other) }
    }
}

impl Default for blst_fp12 {
    fn default() -> Self {
        unsafe { *blst_fp12_one() }
    }
}

impl PartialEq for blst_fp12 {
    fn eq(&self, other: &Self) -> bool {
        unsafe { blst_fp12_is_equal(self, other) }
    }
}

#[derive(Debug)]
pub struct Pairing {
    v: Box<[u64]>,
}

impl Pairing {
    pub fn new(hash_or_encode: bool, dst: &[u8]) -> Self {
        let v: Vec<u64> = vec![0; unsafe { blst_pairing_sizeof() } / 8];
        let mut obj = Self {
            v: v.into_boxed_slice(),
        };
        obj.init(hash_or_encode, dst);
        obj
    }

    pub fn init(&mut self, hash_or_encode: bool, dst: &[u8]) {
        unsafe {
            blst_pairing_init(
                self.ctx(),
                hash_or_encode,
                dst.as_ptr(),
                dst.len(),
            )
        }
    }
    fn ctx(&mut self) -> *mut blst_pairing {
        self.v.as_mut_ptr() as *mut blst_pairing
    }
    fn const_ctx(&self) -> *const blst_pairing {
        self.v.as_ptr() as *const blst_pairing
    }

    pub fn aggregate(
        &mut self,
        pk: &dyn Any,
        sig: &dyn Any,
        msg: &[u8],
        aug: &[u8],
    ) -> BLST_ERROR {
        if pk.is::<blst_p1_affine>() {
            unsafe {
                blst_pairing_aggregate_pk_in_g1(
                    self.ctx(),
                    match pk.downcast_ref::<blst_p1_affine>() {
                        Some(pk) => pk,
                        None => ptr::null(),
                    },
                    match sig.downcast_ref::<blst_p2_affine>() {
                        Some(sig) => sig,
                        None => ptr::null(),
                    },
                    msg.as_ptr(),
                    msg.len(),
                    aug.as_ptr(),
                    aug.len(),
                )
            }
        } else if pk.is::<blst_p2_affine>() {
            unsafe {
                blst_pairing_aggregate_pk_in_g2(
                    self.ctx(),
                    match pk.downcast_ref::<blst_p2_affine>() {
                        Some(pk) => pk,
                        None => ptr::null(),
                    },
                    match sig.downcast_ref::<blst_p1_affine>() {
                        Some(sig) => sig,
                        None => ptr::null(),
                    },
                    msg.as_ptr(),
                    msg.len(),
                    aug.as_ptr(),
                    aug.len(),
                )
            }
        } else {
            panic!("whaaaa?")
        }
    }

    pub fn mul_n_aggregate(
        &mut self,
        pk: &dyn Any,
        sig: &dyn Any,
        scalar: &[limb_t],
        nbits: usize,
        msg: &[u8],
        aug: &[u8],
    ) -> BLST_ERROR {
        if pk.is::<blst_p1_affine>() {
            unsafe {
                blst_pairing_mul_n_aggregate_pk_in_g1(
                    self.ctx(),
                    match pk.downcast_ref::<blst_p1_affine>() {
                        Some(pk) => pk,
                        None => ptr::null(),
                    },
                    match sig.downcast_ref::<blst_p2_affine>() {
                        Some(sig) => sig,
                        None => ptr::null(),
                    },
                    scalar.as_ptr(),
                    nbits,
                    msg.as_ptr(),
                    msg.len(),
                    aug.as_ptr(),
                    aug.len(),
                )
            }
        } else if pk.is::<blst_p2_affine>() {
            unsafe {
                blst_pairing_mul_n_aggregate_pk_in_g2(
                    self.ctx(),
                    match pk.downcast_ref::<blst_p2_affine>() {
                        Some(pk) => pk,
                        None => ptr::null(),
                    },
                    match sig.downcast_ref::<blst_p1_affine>() {
                        Some(sig) => sig,
                        None => ptr::null(),
                    },
                    scalar.as_ptr(),
                    nbits,
                    msg.as_ptr(),
                    msg.len(),
                    aug.as_ptr(),
                    aug.len(),
                )
            }
        } else {
            panic!("whaaaa?")
        }
    }

    pub fn aggregated(gtsig: &mut blst_fp12, sig: &dyn Any) {
        if sig.is::<blst_p1_affine>() {
            unsafe {
                blst_aggregated_in_g1(
                    gtsig,
                    sig.downcast_ref::<blst_p1_affine>().unwrap(),
                )
            }
        } else if sig.is::<blst_p2_affine>() {
            unsafe {
                blst_aggregated_in_g2(
                    gtsig,
                    sig.downcast_ref::<blst_p2_affine>().unwrap(),
                )
            }
        } else {
            panic!("whaaaa?")
        }
    }

    pub fn commit(&mut self) {
        unsafe { blst_pairing_commit(self.ctx()) }
    }

    pub fn merge(&mut self, ctx1: &Self) -> BLST_ERROR {
        unsafe { blst_pairing_merge(self.ctx(), ctx1.const_ctx()) }
    }

    pub fn finalverify(&self, gtsig: Option<&blst_fp12>) -> bool {
        unsafe {
            blst_pairing_finalverify(
                self.const_ctx(),
                match gtsig {
                    Some(gtsig) => gtsig,
                    None => ptr::null(),
                },
            )
        }
    }
}

// TODO - add group checks after deserialization

pub fn print_bytes(bytes: &[u8], name: &str) {
    print!("{} ", name);
    for b in bytes.iter() {
        print!("{:02x}", b);
    }
    println!();
}

macro_rules! sig_variant_impl {
    (
        $name:expr,
        $pk:ty,
        $pk_aff:ty,
        $sig:ty,
        $sig_aff:ty,
        $sk_to_pk:ident,
        $hash_or_encode:expr,
        $hash_or_encode_to:ident,
        $sign:ident,
        $pk_eq:ident,
        $sig_eq:ident,
        $verify:ident,
        $pk_in_group:ident,
        $pk_to_aff:ident,
        $pk_from_aff:ident,
        $pk_ser:ident,
        $pk_comp:ident,
        $pk_deser:ident,
        $pk_uncomp:ident,
        $pk_comp_size:expr,
        $pk_ser_size:expr,
        $sig_in_group:ident,
        $sig_to_aff:ident,
        $sig_from_aff:ident,
        $sig_ser:ident,
        $sig_comp:ident,
        $sig_deser:ident,
        $sig_uncomp:ident,
        $sig_comp_size:expr,
        $sig_ser_size:expr,
        $pk_add_or_dbl:ident,
        $pk_add_or_dbl_aff:ident,
        $sig_add_or_dbl:ident,
        $sig_add_or_dbl_aff:ident,
    ) => {
        /// Secret Key
        #[derive(Default, Debug, Clone)]
        pub struct SecretKey {
            pub value: blst_scalar,
        }

        impl SecretKey {
            /// Deterministically generate a secret key from key material
            pub fn key_gen(
                ikm: &[u8],
                key_info: &[u8],
            ) -> Result<Self, BLST_ERROR> {
                if ikm.len() < 32 {
                    return Err(BLST_ERROR::BLST_BAD_ENCODING);
                }
                let mut sk = SecretKey::default();
                unsafe {
                    blst_keygen(
                        &mut sk.value,
                        ikm.as_ptr(),
                        ikm.len(),
                        key_info.as_ptr(),
                        key_info.len(),
                    );
                }
                Ok(sk)
            }

            // sk_to_pk
            pub fn sk_to_pk(&self) -> PublicKey {
                // TODO - would the user like the serialized/compressed pk as well?
                let mut pk_aff = PublicKey::default();
                //let mut pk_ser = [0u8; $pk_ser_size];

                unsafe {
                    $sk_to_pk(
                        //pk_ser.as_mut_ptr(),
                        ptr::null_mut(),
                        &mut pk_aff.point,
                        &self.value,
                    );
                }
                pk_aff
            }

            // Sign
            pub fn sign(
                &self,
                msg: &[u8],
                dst: &[u8],
                aug: &[u8],
            ) -> Signature {
                // TODO - would the user like the serialized/compressed sig as well?
                let mut q = <$sig>::default();
                let mut sig_aff = <$sig_aff>::default();
                //let mut sig_ser = [0u8; $sig_ser_size];
                unsafe {
                    $hash_or_encode_to(
                        &mut q,
                        msg.as_ptr(),
                        msg.len(),
                        dst.as_ptr(),
                        dst.len(),
                        aug.as_ptr(),
                        aug.len(),
                    );
                    $sign(ptr::null_mut(), &mut sig_aff, &q, &self.value);
                }
                Signature { point: sig_aff }
            }

            // TODO - formally speaking application is entitled to have
            // ultimate control over secret key storage, which means that
            // corresponding serialization/deserialization subroutines
            // should accept reference to where to store the result, as
            // opposite to returning one.

            // serialize
            pub fn serialize(&self) -> [u8; 32] {
                let mut sk_out = [0; 32];
                unsafe {
                    blst_bendian_from_scalar(sk_out.as_mut_ptr(), &self.value);
                }
                sk_out
            }

            // deserialize
            pub fn deserialize(sk_in: &[u8]) -> Result<Self, BLST_ERROR> {
                let mut sk = blst_scalar::default();
                unsafe {
                    blst_scalar_from_bendian(&mut sk, sk_in.as_ptr());
                    if !blst_scalar_fr_check(&sk) {
                        return Err(BLST_ERROR::BLST_BAD_ENCODING);
                    }
                }
                Ok(Self { value: sk })
            }

            pub fn to_bytes(&self) -> [u8; 32] {
                SecretKey::serialize(&self)
            }

            pub fn from_bytes(sk_in: &[u8]) -> Result<Self, BLST_ERROR> {
                SecretKey::deserialize(sk_in)
            }
        }

        #[derive(Default, Debug, Clone, Copy)]
        pub struct PublicKey {
            pub point: $pk_aff,
        }

        impl PublicKey {
            // Core operations

            // key_validate
            pub fn key_validate(key: &[u8]) -> Result<Self, BLST_ERROR> {
                let pk = PublicKey::from_bytes(key)?;
                let err: bool;
                unsafe {
                    err = $pk_in_group(&pk.point);
                }
                if err != true {
                    return Err(BLST_ERROR::BLST_POINT_NOT_IN_GROUP);
                }
                Ok(pk)
            }

            pub fn from_aggregate(agg_pk: &AggregatePublicKey) -> Self {
                let mut pk_aff = <$pk_aff>::default();
                unsafe {
                    $pk_to_aff(&mut pk_aff, &agg_pk.point);
                }
                Self { point: pk_aff }
            }

            // Serdes

            pub fn compress(&self) -> [u8; $pk_comp_size] {
                let mut pk = <$pk>::default();
                let mut pk_comp = [0u8; $pk_comp_size]; // TODO - no need to initialize
                unsafe {
                    $pk_from_aff(&mut pk, &self.point);
                    $pk_comp(pk_comp.as_mut_ptr(), &pk);
                }
                pk_comp
            }

            pub fn serialize(&self) -> [u8; $pk_ser_size] {
                let mut pk = <$pk>::default();
                let mut pk_out = [0u8; $pk_ser_size];
                unsafe {
                    $pk_from_aff(&mut pk, &self.point);
                    $pk_ser(pk_out.as_mut_ptr(), &pk);
                }
                pk_out
            }

            pub fn uncompress(pk_comp: &[u8]) -> Result<Self, BLST_ERROR> {
                if pk_comp.len() == $pk_comp_size {
                    let mut pk = <$pk_aff>::default();
                    let err = unsafe { $pk_uncomp(&mut pk, pk_comp.as_ptr()) };
                    if err != BLST_ERROR::BLST_SUCCESS {
                        return Err(err);
                    }
                    Ok(Self { point: pk })
                } else {
                    Err(BLST_ERROR::BLST_BAD_ENCODING)
                }
            }

            pub fn deserialize(pk_in: &[u8]) -> Result<Self, BLST_ERROR> {
                if pk_in.len() == $pk_ser_size && (pk_in[0] & 0x80) == 0
                    || pk_in.len() == $pk_comp_size && (pk_in[0] & 0x80) != 0
                {
                    let mut pk = <$pk_aff>::default();
                    let err = unsafe { $pk_deser(&mut pk, pk_in.as_ptr()) };
                    if err != BLST_ERROR::BLST_SUCCESS {
                        return Err(err);
                    }
                    Ok(Self { point: pk })
                } else {
                    Err(BLST_ERROR::BLST_BAD_ENCODING)
                }
            }

            pub fn from_bytes(pk_in: &[u8]) -> Result<Self, BLST_ERROR> {
                if (pk_in[0] & 0x80) == 0 {
                    // Not compressed
                    PublicKey::deserialize(pk_in)
                } else {
                    // compressed
                    PublicKey::uncompress(pk_in)
                }
            }

            pub fn to_bytes(&self) -> [u8; $pk_comp_size] {
                self.compress()
            }
        }

        impl PartialEq for PublicKey {
            fn eq(&self, other: &Self) -> bool {
                unsafe { $pk_eq(&self.point, &other.point) }
            }
        }

        #[derive(Debug, Clone, Copy)]
        pub struct AggregatePublicKey {
            pub point: $pk,
        }

        impl AggregatePublicKey {
            pub fn from_public_key(pk: &PublicKey) -> Self {
                let mut agg_pk = <$pk>::default();
                unsafe {
                    $pk_from_aff(&mut agg_pk, &pk.point);
                }
                Self { point: agg_pk }
            }

            pub fn to_public_key(&self) -> PublicKey {
                let mut pk = <$pk_aff>::default();
                unsafe {
                    $pk_to_aff(&mut pk, &self.point);
                }
                PublicKey { point: pk }
            }

            // Aggregate
            pub fn aggregate(pks: &[&PublicKey]) -> Self {
                // TODO - handle case of zero length array? What to return then?
                let mut agg_pk = AggregatePublicKey::from_public_key(pks[0]);
                for s in pks.iter().skip(1) {
                    unsafe {
                        $pk_add_or_dbl_aff(
                            &mut agg_pk.point,
                            &agg_pk.point,
                            &s.point,
                        );
                    }
                }
                agg_pk
            }

            pub fn aggregate_serialized(
                pks: &[&[u8]],
            ) -> Result<Self, BLST_ERROR> {
                // TODO - handle case of zero length array?
                // TODO - subgroup check
                // TODO - threading
                let mut pk = PublicKey::from_bytes(pks[0])?;
                let mut agg_pk = AggregatePublicKey::from_public_key(&pk);
                for s in pks.iter().skip(1) {
                    pk = PublicKey::from_bytes(s)?;
                    unsafe {
                        // TODO - does this need add_or_double?
                        $pk_add_or_dbl_aff(
                            &mut agg_pk.point,
                            &agg_pk.point,
                            &pk.point,
                        );
                    }
                }
                Ok(agg_pk)
            }

            pub fn add_aggregate(&mut self, agg_pk: &AggregatePublicKey) {
                unsafe {
                    $pk_add_or_dbl(&mut self.point, &self.point, &agg_pk.point);
                }
            }

            pub fn add_public_key(&mut self, pk: &PublicKey) {
                unsafe {
                    $pk_add_or_dbl_aff(&mut self.point, &self.point, &pk.point);
                }
            }
        }

        #[derive(Debug, Clone, Copy)]
        pub struct Signature {
            pub point: $sig_aff,
        }

        impl Signature {
            pub fn verify(
                &self,
                msg: &[u8],
                dst: &[u8],
                aug: &[u8],
                pk: &PublicKey,
            ) -> BLST_ERROR {
                let aug_msg = [aug, msg].concat();
                self.aggregate_verify(&[aug_msg.as_slice()], dst, &[pk])
            }

            pub fn aggregate_verify(
                &self,
                msgs: &[&[u8]],
                dst: &[u8],
                pks: &[&PublicKey],
            ) -> BLST_ERROR {
                let n_elems = pks.len();
                if msgs.len() != n_elems {
                    return BLST_ERROR::BLST_VERIFY_FAIL;
                }

                // TODO - check msg uniqueness?
                // TODO - since already in object form, any need to subgroup check?

                let pool = da_pool();
                let (tx, rx) = channel();
                let counter = Arc::new(AtomicUsize::new(0));
                let valid = Arc::new(AtomicBool::new(true));

                // Bypass 'lifetime limitations by brute force. It works,
                // because we explicitly join the threads...
                let raw_pks = unsafe {
                    transmute::<*const &PublicKey, usize>(pks.as_ptr())
                };
                let raw_msgs =
                    unsafe { transmute::<*const &[u8], usize>(msgs.as_ptr()) };
                let dst =
                    unsafe { slice::from_raw_parts(dst.as_ptr(), dst.len()) };

                let n_workers = std::cmp::min(pool.max_count(), n_elems);
                for _ in 0..n_workers {
                    let tx = tx.clone();
                    let counter = counter.clone();
                    let valid = valid.clone();

                    pool.execute(move || {
                        let mut pairing = Pairing::new($hash_or_encode, dst);
                        // reconstruct input slices...
                        let msgs = unsafe {
                            slice::from_raw_parts(
                                transmute::<usize, *const &[u8]>(raw_msgs),
                                n_elems,
                            )
                        };
                        let pks = unsafe {
                            slice::from_raw_parts(
                                transmute::<usize, *const &PublicKey>(raw_pks),
                                n_elems,
                            )
                        };

                        while valid.load(Ordering::Relaxed) {
                            let work = counter.fetch_add(1, Ordering::Relaxed);
                            if work >= n_elems {
                                break;
                            }
                            if pairing.aggregate(
                                &pks[work].point,
                                &unsafe { ptr::null::<$sig_aff>().as_ref() },
                                &msgs[work],
                                &[],
                            ) != BLST_ERROR::BLST_SUCCESS
                            {
                                valid.store(false, Ordering::Relaxed);
                                break;
                            }
                        }
                        if valid.load(Ordering::Relaxed) {
                            pairing.commit();
                        }
                        tx.send(pairing).expect("disaster");
                    });
                }

                let mut gtsig = blst_fp12::default();
                if valid.load(Ordering::Relaxed) {
                    Pairing::aggregated(&mut gtsig, &self.point);
                }

                let mut acc = rx.recv().unwrap();
                for _ in 1..n_workers {
                    acc.merge(&rx.recv().unwrap());
                }

                if valid.load(Ordering::Relaxed)
                    && acc.finalverify(Some(&gtsig))
                {
                    BLST_ERROR::BLST_SUCCESS
                } else {
                    BLST_ERROR::BLST_VERIFY_FAIL
                }
            }

            pub fn fast_aggregate_verify(
                &self,
                msg: &[u8],
                dst: &[u8],
                pks: &[&PublicKey],
            ) -> BLST_ERROR {
                let agg_pk = AggregatePublicKey::aggregate(pks);
                let pk = agg_pk.to_public_key();
                self.aggregate_verify(&[msg], dst, &[&pk])
            }

            pub fn fast_aggregate_verify_pre_aggregated(
                &self,
                msg: &[u8],
                dst: &[u8],
                pk: &PublicKey,
            ) -> BLST_ERROR {
                self.aggregate_verify(&[msg], dst, &[pk])
            }

            // https://ethresear.ch/t/fast-verification-of-multiple-bls-signatures/5407
            pub fn verify_multiple_aggregate_signatures(
                msgs: &[&[u8]],
                dst: &[u8],
                pks: &[&PublicKey],
                sigs: &[&Signature],
                rands: &[blst_scalar],
                rand_bits: usize,
            ) -> BLST_ERROR {
                let n_elems = pks.len();
                if msgs.len() != n_elems
                    || sigs.len() != n_elems
                    || rands.len() != n_elems
                {
                    return BLST_ERROR::BLST_VERIFY_FAIL;
                }

                // TODO - check msg uniqueness?
                // TODO - since already in object form, any need to subgroup check?

                let pool = da_pool();
                let (tx, rx) = channel();
                let counter = Arc::new(AtomicUsize::new(0));
                let valid = Arc::new(AtomicBool::new(true));

                // Bypass 'lifetime limitations by brute force. It works,
                // because we explicitly join the threads...
                let raw_pks = unsafe {
                    transmute::<*const &PublicKey, usize>(pks.as_ptr())
                };
                let raw_sigs = unsafe {
                    transmute::<*const &Signature, usize>(sigs.as_ptr())
                };
                let raw_rands = unsafe {
                    transmute::<*const blst_scalar, usize>(rands.as_ptr())
                };
                let raw_msgs =
                    unsafe { transmute::<*const &[u8], usize>(msgs.as_ptr()) };
                let dst =
                    unsafe { slice::from_raw_parts(dst.as_ptr(), dst.len()) };

                let n_workers = std::cmp::min(pool.max_count(), n_elems);
                for _ in 0..n_workers {
                    let tx = tx.clone();
                    let counter = counter.clone();
                    let valid = valid.clone();

                    pool.execute(move || {
                        let mut pairing = Pairing::new($hash_or_encode, dst);
                        // reconstruct input slices...
                        let rands = unsafe {
                            slice::from_raw_parts(
                                transmute::<usize, *const blst_scalar>(
                                    raw_rands,
                                ),
                                n_elems,
                            )
                        };
                        let msgs = unsafe {
                            slice::from_raw_parts(
                                transmute::<usize, *const &[u8]>(raw_msgs),
                                n_elems,
                            )
                        };
                        let sigs = unsafe {
                            slice::from_raw_parts(
                                transmute::<usize, *const &Signature>(raw_sigs),
                                n_elems,
                            )
                        };
                        let pks = unsafe {
                            slice::from_raw_parts(
                                transmute::<usize, *const &PublicKey>(raw_pks),
                                n_elems,
                            )
                        };

                        // TODO - engage multi-point mul-n-add for larger
                        // amount of inputs...
                        while valid.load(Ordering::Relaxed) {
                            let work = counter.fetch_add(1, Ordering::Relaxed);
                            if work >= n_elems {
                                break;
                            }

                            if pairing.mul_n_aggregate(
                                &pks[work].point,
                                &sigs[work].point,
                                &rands[work].l,
                                rand_bits,
                                msgs[work],
                                &[],
                            ) != BLST_ERROR::BLST_SUCCESS
                            {
                                valid.store(false, Ordering::Relaxed);
                                break;
                            }
                        }
                        if valid.load(Ordering::Relaxed) {
                            pairing.commit();
                        }
                        tx.send(pairing).expect("disaster");
                    });
                }

                let mut acc = rx.recv().unwrap();
                for _ in 1..n_workers {
                    acc.merge(&rx.recv().unwrap());
                }

                if valid.load(Ordering::Relaxed) && acc.finalverify(None) {
                    BLST_ERROR::BLST_SUCCESS
                } else {
                    BLST_ERROR::BLST_VERIFY_FAIL
                }
            }

            pub fn from_aggregate(agg_sig: &AggregateSignature) -> Self {
                let mut sig_aff = <$sig_aff>::default();
                unsafe {
                    $sig_to_aff(&mut sig_aff, &agg_sig.point);
                }
                Self { point: sig_aff }
            }

            pub fn compress(&self) -> [u8; $sig_comp_size] {
                let mut sig_comp = [0; $sig_comp_size];
                unsafe {
                    let mut sig = MaybeUninit::<$sig>::uninit();
                    $sig_from_aff(sig.as_mut_ptr(), &self.point);
                    $sig_comp(sig_comp.as_mut_ptr(), sig.as_ptr());
                    //$sig_comp(sig_comp.as_mut_ptr(), &self.point);
                }
                sig_comp
            }

            pub fn serialize(&self) -> [u8; $sig_ser_size] {
                let mut sig_out = [0; $sig_ser_size];
                unsafe {
                    let mut sig = MaybeUninit::<$sig>::uninit();
                    $sig_from_aff(sig.as_mut_ptr(), &self.point);
                    $sig_ser(sig_out.as_mut_ptr(), sig.as_ptr());
                    //$sig_ser(sig_out.as_mut_ptr(), &self.point);
                }
                sig_out
            }

            pub fn uncompress(sig_comp: &[u8]) -> Result<Self, BLST_ERROR> {
                if sig_comp.len() == $sig_comp_size {
                    let mut sig = <$sig_aff>::default();
                    let err =
                        unsafe { $sig_uncomp(&mut sig, sig_comp.as_ptr()) };
                    if err != BLST_ERROR::BLST_SUCCESS {
                        return Err(err);
                    }
                    Ok(Self { point: sig })
                } else {
                    Err(BLST_ERROR::BLST_BAD_ENCODING)
                }
            }

            pub fn deserialize(sig_in: &[u8]) -> Result<Self, BLST_ERROR> {
                if sig_in.len() == $sig_ser_size && (sig_in[0] & 0x80) == 0
                    || sig_in.len() == $sig_comp_size && (sig_in[0] & 0x80) != 0
                {
                    let mut sig = <$sig_aff>::default();
                    let err = unsafe { $sig_deser(&mut sig, sig_in.as_ptr()) };
                    if err != BLST_ERROR::BLST_SUCCESS {
                        return Err(err);
                    }
                    Ok(Self { point: sig })
                } else {
                    Err(BLST_ERROR::BLST_BAD_ENCODING)
                }
            }

            pub fn from_bytes(sig_in: &[u8]) -> Result<Self, BLST_ERROR> {
                if (sig_in[0] & 0x80) == 0 {
                    // Not compressed
                    Signature::deserialize(sig_in)
                } else {
                    // compressed
                    Signature::uncompress(sig_in)
                }
            }

            pub fn to_bytes(&self) -> [u8; $sig_comp_size] {
                self.compress()
            }
        }

        impl PartialEq for Signature {
            fn eq(&self, other: &Self) -> bool {
                unsafe { $sig_eq(&self.point, &other.point) }
            }
        }

        #[derive(Debug, Clone, Copy)]
        pub struct AggregateSignature {
            pub point: $sig,
        }

        impl AggregateSignature {
            pub fn from_signature(sig: &Signature) -> Self {
                let mut agg_sig = <$sig>::default();
                unsafe {
                    $sig_from_aff(&mut agg_sig, &sig.point);
                }
                Self { point: agg_sig }
            }

            pub fn to_signature(&self) -> Signature {
                let mut sig = <$sig_aff>::default();
                unsafe {
                    $sig_to_aff(&mut sig, &self.point);
                }
                Signature { point: sig }
            }

            // Aggregate
            pub fn aggregate(sigs: &[&Signature]) -> Self {
                // TODO - handle case of zero length array?
                let mut agg_sig = AggregateSignature::from_signature(sigs[0]);
                for s in sigs.iter().skip(1) {
                    unsafe {
                        // TODO - does this need add_or_double?
                        $sig_add_or_dbl_aff(
                            &mut agg_sig.point,
                            &agg_sig.point,
                            &s.point,
                        );
                    }
                }
                agg_sig
            }

            pub fn aggregate_serialized(
                sigs: &[&[u8]],
            ) -> Result<Self, BLST_ERROR> {
                // TODO - handle case of zero length array?
                // TODO - subgroup check
                // TODO - threading
                let mut sig = Signature::from_bytes(sigs[0])?;
                let mut agg_sig = AggregateSignature::from_signature(&sig);
                for s in sigs.iter().skip(1) {
                    unsafe {
                        sig = Signature::from_bytes(s)?;
                        // TODO - does this need add_or_double?
                        $sig_add_or_dbl_aff(
                            &mut agg_sig.point,
                            &agg_sig.point,
                            &sig.point,
                        );
                    }
                }
                Ok(agg_sig)
            }

            pub fn add_aggregate(&mut self, agg_sig: &AggregateSignature) {
                unsafe {
                    $sig_add_or_dbl(
                        &mut self.point,
                        &self.point,
                        &agg_sig.point,
                    );
                }
            }

            pub fn add_signature(&mut self, sig: &Signature) {
                unsafe {
                    $sig_add_or_dbl_aff(
                        &mut self.point,
                        &self.point,
                        &sig.point,
                    );
                }
            }
        }

        #[cfg(test)]
        mod tests {
            use super::*;
            use rand::{RngCore, SeedableRng};
            use rand_chacha::ChaCha20Rng;

            // Testing only - do not use for production
            pub fn gen_random_key(
                rng: &mut rand_chacha::ChaCha20Rng,
            ) -> SecretKey {
                let mut ikm = [0u8; 32];
                rng.fill_bytes(&mut ikm);

                let mut sk = <blst_scalar>::default();
                unsafe {
                    blst_keygen(&mut sk, ikm.as_ptr(), 32, ptr::null(), 0);
                }
                SecretKey { value: sk }
            }

            #[test]
            fn test_sign() {
                let ikm: [u8; 32] = [
                    0x93, 0xad, 0x7e, 0x65, 0xde, 0xad, 0x05, 0x2a, 0x08, 0x3a,
                    0x91, 0x0c, 0x8b, 0x72, 0x85, 0x91, 0x46, 0x4c, 0xca, 0x56,
                    0x60, 0x5b, 0xb0, 0x56, 0xed, 0xfe, 0x2b, 0x60, 0xa6, 0x3c,
                    0x48, 0x99,
                ];

                let sk = SecretKey::key_gen(&ikm, &[]).unwrap();
                let pk = sk.sk_to_pk();

                let dst = b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_";
                let msg = b"hello foo";
                let sig = sk.sign(msg, dst, &[]);

                let err = sig.verify(msg, dst, &[], &pk);
                assert_eq!(err, BLST_ERROR::BLST_SUCCESS);
            }

            #[test]
            fn test_aggregate() {
                let num_msgs = 10;
                let dst = b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_";

                let seed = [0u8; 32];
                let mut rng = ChaCha20Rng::from_seed(seed);

                let sks: Vec<_> =
                    (0..num_msgs).map(|_| gen_random_key(&mut rng)).collect();
                let pks =
                    sks.iter().map(|sk| sk.sk_to_pk()).collect::<Vec<_>>();
                let pks_refs: Vec<&PublicKey> =
                    pks.iter().map(|pk| pk).collect();
                let pks_rev: Vec<&PublicKey> =
                    pks.iter().rev().map(|pk| pk).collect();

                let pk_comp = pks[0].compress();
                let pk_uncomp = PublicKey::uncompress(&pk_comp);
                assert_eq!(pk_uncomp.is_ok(), true);

                let mut msgs: Vec<Vec<u8>> = vec![vec![]; num_msgs];
                for i in 0..num_msgs {
                    let msg_len = (rng.next_u64() & 0x3F) + 1;
                    msgs[i] = vec![0u8; msg_len as usize];
                    rng.fill_bytes(&mut msgs[i]);
                }

                let msgs_refs: Vec<&[u8]> =
                    msgs.iter().map(|m| m.as_slice()).collect();

                let sigs = sks
                    .iter()
                    .zip(msgs.iter())
                    .map(|(sk, m)| (sk.sign(m, dst, &[])))
                    .collect::<Vec<Signature>>();

                let mut errs = sigs
                    .iter()
                    .zip(msgs.iter())
                    .zip(pks.iter())
                    .map(|((s, m), pk)| (s.verify(m, dst, &[], pk)))
                    .collect::<Vec<BLST_ERROR>>();
                assert_eq!(errs, vec![BLST_ERROR::BLST_SUCCESS; num_msgs]);

                // Swap message/public key pairs to create bad signature
                errs = sigs
                    .iter()
                    .zip(msgs.iter())
                    .zip(pks.iter().rev())
                    .map(|((s, m), pk)| (s.verify(m, dst, &[], pk)))
                    .collect::<Vec<BLST_ERROR>>();
                assert_ne!(errs, vec![BLST_ERROR::BLST_SUCCESS; num_msgs]);

                let sig_refs =
                    sigs.iter().map(|s| s).collect::<Vec<&Signature>>();
                let agg = AggregateSignature::aggregate(&sig_refs);

                let agg_sig = agg.to_signature();
                let mut result =
                    agg_sig.aggregate_verify(&msgs_refs, dst, &pks_refs);
                assert_eq!(result, BLST_ERROR::BLST_SUCCESS);

                // Swap message/public key pairs to create bad signature
                result = agg_sig.aggregate_verify(&msgs_refs, dst, &pks_rev);
                assert_ne!(result, BLST_ERROR::BLST_SUCCESS);
            }

            #[test]
            fn test_multiple_agg_sigs() {
                let dst = b"BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_";
                let num_pks_per_sig = 10;
                let num_sigs = 10;

                let seed = [0u8; 32];
                let mut rng = ChaCha20Rng::from_seed(seed);

                let mut msgs: Vec<Vec<u8>> = vec![vec![]; num_sigs];
                let mut sigs: Vec<Signature> = Vec::with_capacity(num_sigs);
                let mut pks: Vec<PublicKey> = Vec::with_capacity(num_sigs);
                let mut rands: Vec<blst_scalar> = Vec::with_capacity(num_sigs);
                for i in 0..num_sigs {
                    // Create public keys
                    let sks_i: Vec<_> = (0..num_pks_per_sig)
                        .map(|_| gen_random_key(&mut rng))
                        .collect();

                    let pks_i = sks_i
                        .iter()
                        .map(|sk| sk.sk_to_pk())
                        .collect::<Vec<_>>();
                    let pks_refs_i: Vec<&PublicKey> =
                        pks_i.iter().map(|pk| pk).collect();

                    // Create random message for pks to all sign
                    let msg_len = (rng.next_u64() & 0x3F) + 1;
                    msgs[i] = vec![0u8; msg_len as usize];
                    rng.fill_bytes(&mut msgs[i]);

                    // Generate signature for each key pair
                    let sigs_i = sks_i
                        .iter()
                        .map(|sk| sk.sign(&msgs[i], dst, &[]))
                        .collect::<Vec<Signature>>();

                    // Test each current single signature
                    let errs = sigs_i
                        .iter()
                        .zip(pks_i.iter())
                        .map(|(s, pk)| (s.verify(&msgs[i], dst, &[], pk)))
                        .collect::<Vec<BLST_ERROR>>();
                    assert_eq!(
                        errs,
                        vec![BLST_ERROR::BLST_SUCCESS; num_pks_per_sig]
                    );

                    let sig_refs_i =
                        sigs_i.iter().map(|s| s).collect::<Vec<&Signature>>();
                    let agg_i = AggregateSignature::aggregate(&sig_refs_i);

                    // Test current aggregate signature
                    sigs.push(agg_i.to_signature());
                    let mut result = sigs[i].fast_aggregate_verify(
                        &msgs[i],
                        dst,
                        &pks_refs_i,
                    );
                    assert_eq!(result, BLST_ERROR::BLST_SUCCESS);

                    // negative test
                    if i != 0 {
                        result = sigs[i - 1].fast_aggregate_verify(
                            &msgs[i],
                            dst,
                            &pks_refs_i,
                        );
                        assert_ne!(result, BLST_ERROR::BLST_SUCCESS);
                    }

                    // aggregate public keys and push into vec
                    let agg_pk_i = AggregatePublicKey::aggregate(&pks_refs_i);
                    pks.push(agg_pk_i.to_public_key());

                    // Test current aggregate signature with aggregated pks
                    result = sigs[i].fast_aggregate_verify_pre_aggregated(
                        &msgs[i], dst, &pks[i],
                    );
                    assert_eq!(result, BLST_ERROR::BLST_SUCCESS);

                    // negative test
                    if i != 0 {
                        result = sigs[i - 1]
                            .fast_aggregate_verify_pre_aggregated(
                                &msgs[i], dst, &pks[i],
                            );
                        assert_ne!(result, BLST_ERROR::BLST_SUCCESS);
                    }

                    // create random values
                    let mut vals = [0u64; 4];
                    vals[0] = rng.next_u64();
                    let mut rand_i = MaybeUninit::<blst_scalar>::uninit();
                    unsafe {
                        blst_scalar_from_uint64(
                            rand_i.as_mut_ptr(),
                            vals.as_ptr(),
                        );
                        rands.push(rand_i.assume_init());
                    }
                }

                let msgs_refs: Vec<&[u8]> =
                    msgs.iter().map(|m| m.as_slice()).collect();
                let sig_refs =
                    sigs.iter().map(|s| s).collect::<Vec<&Signature>>();
                let pks_refs: Vec<&PublicKey> =
                    pks.iter().map(|pk| pk).collect();

                let msgs_rev: Vec<&[u8]> =
                    msgs.iter().rev().map(|m| m.as_slice()).collect();
                let sig_rev =
                    sigs.iter().rev().map(|s| s).collect::<Vec<&Signature>>();
                let pks_rev: Vec<&PublicKey> =
                    pks.iter().rev().map(|pk| pk).collect();

                let mut result =
                    Signature::verify_multiple_aggregate_signatures(
                        &msgs_refs, dst, &pks_refs, &sig_refs, &rands, 64,
                    );
                assert_eq!(result, BLST_ERROR::BLST_SUCCESS);

                // negative tests (use reverse msgs, pks, and sigs)
                result = Signature::verify_multiple_aggregate_signatures(
                    &msgs_rev, dst, &pks_refs, &sig_refs, &rands, 64,
                );
                assert_ne!(result, BLST_ERROR::BLST_SUCCESS);

                result = Signature::verify_multiple_aggregate_signatures(
                    &msgs_refs, dst, &pks_rev, &sig_refs, &rands, 64,
                );
                assert_ne!(result, BLST_ERROR::BLST_SUCCESS);

                result = Signature::verify_multiple_aggregate_signatures(
                    &msgs_refs, dst, &pks_refs, &sig_rev, &rands, 64,
                );
                assert_ne!(result, BLST_ERROR::BLST_SUCCESS);
            }

            #[test]
            fn test_serialization() {
                let seed = [0u8; 32];
                let mut rng = ChaCha20Rng::from_seed(seed);

                let sk = gen_random_key(&mut rng);
                let sk2 = gen_random_key(&mut rng);

                let pk = sk.sk_to_pk();
                let pk_comp = pk.compress();
                let pk_ser = pk.serialize();

                let pk_uncomp = PublicKey::uncompress(&pk_comp);
                assert_eq!(pk_uncomp.is_ok(), true);
                assert_eq!(pk_uncomp.unwrap(), pk);

                let pk_deser = PublicKey::deserialize(&pk_ser);
                assert_eq!(pk_deser.is_ok(), true);
                assert_eq!(pk_deser.unwrap(), pk);

                let pk2 = sk2.sk_to_pk();
                let pk_comp2 = pk2.compress();
                let pk_ser2 = pk2.serialize();

                let pk_uncomp2 = PublicKey::uncompress(&pk_comp2);
                assert_eq!(pk_uncomp2.is_ok(), true);
                assert_eq!(pk_uncomp2.unwrap(), pk2);

                let pk_deser2 = PublicKey::deserialize(&pk_ser2);
                assert_eq!(pk_deser2.is_ok(), true);
                assert_eq!(pk_deser2.unwrap(), pk2);

                assert_ne!(pk, pk2);
                assert_ne!(pk_uncomp.unwrap(), pk2);
                assert_ne!(pk_deser.unwrap(), pk2);
                assert_ne!(pk_uncomp2.unwrap(), pk);
                assert_ne!(pk_deser2.unwrap(), pk);
            }
        }
    };
}

pub mod min_pk {
    use super::*;

    sig_variant_impl!(
        "MinPk",
        blst_p1,
        blst_p1_affine,
        blst_p2,
        blst_p2_affine,
        blst_sk_to_pk2_in_g1,
        true,
        blst_hash_to_g2,
        blst_sign_pk2_in_g1,
        blst_p1_affine_is_equal,
        blst_p2_affine_is_equal,
        blst_core_verify_pk_in_g1,
        blst_p1_affine_in_g1,
        blst_p1_to_affine,
        blst_p1_from_affine,
        blst_p1_serialize,
        blst_p1_compress,
        blst_p1_deserialize,
        blst_p1_uncompress,
        48,
        96,
        blst_p2_affine_in_g2,
        blst_p2_to_affine,
        blst_p2_from_affine,
        blst_p2_serialize,
        blst_p2_compress,
        blst_p2_deserialize,
        blst_p2_uncompress,
        96,
        192,
        blst_p1_add_or_double,
        blst_p1_add_or_double_affine,
        blst_p2_add_or_double,
        blst_p2_add_or_double_affine,
    );
}

pub mod min_sig {
    use super::*;

    sig_variant_impl!(
        "MinSig",
        blst_p2,
        blst_p2_affine,
        blst_p1,
        blst_p1_affine,
        blst_sk_to_pk2_in_g2,
        true,
        blst_hash_to_g1,
        blst_sign_pk2_in_g2,
        blst_p2_affine_is_equal,
        blst_p1_affine_is_equal,
        blst_core_verify_pk_in_g2,
        blst_p2_affine_in_g2,
        blst_p2_to_affine,
        blst_p2_from_affine,
        blst_p2_serialize,
        blst_p2_compress,
        blst_p2_deserialize,
        blst_p2_uncompress,
        96,
        192,
        blst_p1_affine_in_g1,
        blst_p1_to_affine,
        blst_p1_from_affine,
        blst_p1_serialize,
        blst_p1_compress,
        blst_p1_deserialize,
        blst_p1_uncompress,
        48,
        96,
        blst_p2_add_or_double,
        blst_p2_add_or_double_affine,
        blst_p1_add_or_double,
        blst_p1_add_or_double_affine,
    );
}
