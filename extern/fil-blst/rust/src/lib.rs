// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

#![allow(non_upper_case_globals)]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]

include!(concat!(env!("OUT_DIR"), "/bindings.rs"));

use fff::PrimeField;
use groupy::EncodedPoint;
use paired::bls12_381::{Bls12, Fq, Fr, G1Affine, G1Uncompressed, G2Affine, G2Uncompressed};
use std::path::Path;

extern "C" {
    //     //pub fn print_blst_fr(fr: *const blst_fr);
    pub fn verify_batch_proof_c(
        proof_bytes: *const u8,
        num_proofs: usize,
        public_inputs: *const blst_fr,
        num_inputs: usize,
        rand_z: *const blst_scalar,
        nbits: usize,
        vk_path: *const u8,
        vk_len: usize,
    ) -> bool;
}

impl From<Fr> for blst_fr {
    fn from(fr: Fr) -> blst_fr {
        let mut b_fr = std::mem::MaybeUninit::<blst_fr>::uninit();
        unsafe {
            blst_scalar_from_uint64(
                b_fr.as_mut_ptr() as *mut blst_scalar,
                fr.into_repr().as_ref().as_ptr(),
            );
            b_fr.assume_init()
        }
    }
}

impl From<Fq> for blst_fp {
    fn from(fq: Fq) -> blst_fp {
        let mut fp = std::mem::MaybeUninit::<blst_fp>::uninit();
        unsafe {
            blst_fp_from_uint64(fp.as_mut_ptr(), fq.into_repr().as_ref().as_ptr());
            fp.assume_init()
        }
    }
}

impl blst_p1_affine {
    pub fn transform(p: <Bls12 as paired::Engine>::G1Affine) -> Self {
        let mut p1 = std::mem::MaybeUninit::<blst_p1_affine>::uninit();
        unsafe {
            blst_p1_deserialize(
                p1.as_mut_ptr(),
                G1Uncompressed::from_affine(p).as_ref().as_ptr(),
            );
            p1.assume_init()
        }
    }
}

// TODO - Is there a better way than deserializing?
impl From<G1Affine> for blst_p1_affine {
    fn from(p: G1Affine) -> blst_p1_affine {
        let mut p1 = std::mem::MaybeUninit::<blst_p1_affine>::uninit();
        unsafe {
            blst_p1_deserialize(
                p1.as_mut_ptr(),
                G1Uncompressed::from_affine(p).as_ref().as_ptr(),
            );
            p1.assume_init()
        }
    }
}

impl From<G2Affine> for blst_p2_affine {
    fn from(p: G2Affine) -> blst_p2_affine {
        let mut p2 = std::mem::MaybeUninit::<blst_p2_affine>::uninit();
        unsafe {
            blst_p2_deserialize(
                p2.as_mut_ptr(),
                G2Uncompressed::from_affine(p).as_ref().as_ptr(),
            );
            p2.assume_init()
        }
    }
}

pub fn scalar_from_u64(limbs: &[u64; 4]) -> blst_scalar {
    let mut s = std::mem::MaybeUninit::<blst_scalar>::uninit();
    unsafe {
        blst_scalar_from_uint64(s.as_mut_ptr(), &(limbs[0]));
        s.assume_init()
    }
}

pub fn verify_batch_proof(
    proof_vec: &[u8],
    num_proofs: usize,
    public_inputs: &[blst_fr],
    num_inputs: usize,
    rand_z: &[blst_scalar],
    nbits: usize,
    vk_path: &Path,
) -> bool {
    let s = vk_path
        .to_str()
        .expect("Path is expected to be valid UTF-8")
        .to_string();
    let vk_bytes = s.into_bytes();
    unsafe {
        verify_batch_proof_c(
            &(proof_vec[0]),
            num_proofs,
            public_inputs.as_ptr(),
            num_inputs,
            &(rand_z[0]),
            nbits,
            &(vk_bytes[0]),
            vk_bytes.len(),
        )
    }
}

pub fn print_bytes(bytes: &[u8], name: &str) {
    print!("{} ", name);
    for b in bytes.iter() {
        print!("{:02x}", b);
    }
    println!();
}

#[cfg(test)]
mod tests {
    use super::*;
    //use groupy::{CurveAffine, CurveProjective, EncodedPoint};
    use fff::Field;
    use groupy::{CurveAffine, CurveProjective};
    use paired::bls12_381::{Fq, G1Affine, G1Compressed, G1Uncompressed, G1};

    #[test]
    fn it_works() {
        unsafe {
            let mut bp1 = std::mem::MaybeUninit::<blst_p1>::zeroed().assume_init();
            //let mut bp1_aff = std::mem::MaybeUninit::<blst_p1_affine>::zeroed().assume_init();

            blst_p1_add_or_double_affine(&mut bp1, &bp1, &BLS12_381_G1);
            //blst_p1_to_affine(&mut bp1_aff, &bp1);

            let mut bp1_bytes = [0u8; 48];
            blst_p1_compress(bp1_bytes.as_mut_ptr(), &bp1);

            print_bytes(&bp1_bytes, "bp1");
            //assert_eq!(bp1_aff, BLS12_381_NEG_G1);

            let zp1 = G1::zero();
            let zp1_aff = G1Affine::zero();

            println!("zp1 {:?}", zp1);
            //println!("zp1.x {:?}", zp1.x);
            println!("zp1_aff {:?}", zp1_aff);

            let frX = Fr::one();
            println!("frX {:?}", frX);
            let frB = blst_fr::from(frX);
            println!("frB {:?}", frB);
            //print_blst_fr(&frB);
            // let mut frB_bytes = [0u8; 32];
            // blst_bendian_from_fr(frX_bytes.as_mut_ptr(), &frB);
            // print_bytes(&frB_bytes, "frB");

            let fqX = Fq::one();
            println!("fqX {:?}", fqX);

            let fpX = blst_fp::from(fqX);

            let mut fpX_bytes = [0u8; 48];
            blst_bendian_from_fp(fpX_bytes.as_mut_ptr(), &fpX);

            print_bytes(&fpX_bytes, "fpX");

            // Use compressed

            let mut g1_gen_bytes = [0u8; 48];
            blst_p1_affine_compress(g1_gen_bytes.as_mut_ptr(), &BLS12_381_G1);
            print_bytes(&g1_gen_bytes, "G1_Generator");

            let mut g1_gen_z_bytes = G1Compressed::empty();
            g1_gen_z_bytes.as_mut().copy_from_slice(&g1_gen_bytes);
            println!("g1_gen_z_bytes {:?}", g1_gen_z_bytes);

            let g1_gen_z = g1_gen_z_bytes.into_affine_unchecked().unwrap();

            let g1_gen_b = blst_p1_affine::from(g1_gen_z);
            let mut g1_gen_bytes2 = [0u8; 48];
            blst_p1_affine_compress(g1_gen_bytes2.as_mut_ptr(), &g1_gen_b);
            print_bytes(&g1_gen_bytes2, "G1_Generator");

            assert_eq!(&g1_gen_bytes[0..48], &g1_gen_bytes2[0..48]);

            // Use uncompressed

            let mut g1_gen_bytes_uncomp = [0u8; 96];
            blst_p1_affine_serialize(g1_gen_bytes_uncomp.as_mut_ptr(), &BLS12_381_G1);
            print_bytes(&g1_gen_bytes_uncomp, "G1_Generator");

            let mut g1_gen_z_bytes_uncomp = G1Uncompressed::empty();
            g1_gen_z_bytes_uncomp
                .as_mut()
                .copy_from_slice(&g1_gen_bytes_uncomp);
            println!("g1_gen_z_bytes_uncomp {:?}", g1_gen_z_bytes_uncomp);

            let g1_gen_z_uncomp = g1_gen_z_bytes_uncomp.into_affine_unchecked().unwrap();

            let g1_gen_b_uncomp = blst_p1_affine::from(g1_gen_z_uncomp);
            let mut g1_gen_bytes_uncomp2 = [0u8; 96];
            blst_p1_affine_serialize(g1_gen_bytes_uncomp2.as_mut_ptr(), &g1_gen_b_uncomp);
            print_bytes(&g1_gen_bytes_uncomp2, "G1_Generator");

            assert_eq!(&g1_gen_bytes_uncomp[0..96], &g1_gen_bytes_uncomp2[0..96]);
        }
    }
}
