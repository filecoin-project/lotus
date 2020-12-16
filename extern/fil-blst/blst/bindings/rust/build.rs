extern crate cc;

use std::env;
use std::path::Path;
use std::path::PathBuf;

#[cfg(all(target_env = "msvc", target_arch = "x86_64"))]
fn assembly(file_vec: &mut Vec<PathBuf>, base_dir: &str) {
    let files = glob::glob(&(base_dir.to_owned() + "win64/*-x86_64.asm"))
        .expect("disaster");
    for file in files {
        file_vec.push(file.unwrap());
    }
}

#[cfg(all(target_env = "msvc", target_arch = "aarch64"))]
fn assembly(file_vec: &mut Vec<PathBuf>, base_dir: &str) {
    let files = glob::glob(&(base_dir.to_owned() + "win64/*-armv8.asm"))
        .expect("disaster");
    for file in files {
        file_vec.push(file.unwrap());
    }
}

#[cfg(all(target_pointer_width = "64", not(target_env = "msvc")))]
fn assembly(file_vec: &mut Vec<PathBuf>, base_dir: &str) {
    file_vec.push(Path::new(base_dir).join("assembly.S"))
}

#[cfg(target_arch = "x86_64")]
fn is_adx() -> bool {
    use std::arch::x86_64::*;
    let mut id = unsafe { __cpuid(0) };
    if id.eax >= 7 {
        id = unsafe { __cpuid_count(7, 0) };
        return (id.ebx & 1 << 19) != 0;
    }
    false
}
#[cfg(not(target_arch = "x86_64"))]
fn is_adx() -> bool {
    false
}

fn main() {
    /*
     * Use pre-built libblst.a if there is one. This is primarily
     * for trouble-shooting purposes. Idea is that libblst.a can be
     * compiled with flags independent from cargo defaults, e.g.
     * '../../build.sh -O1 ...'.
     */
    if Path::new("libblst.a").exists() {
        println!("cargo:rustc-link-search=.");
        println!("cargo:rustc-link-lib=blst");
        return;
    }

    let mut file_vec = Vec::new();

    let _out_dir = env::var_os("OUT_DIR").unwrap();

    let blst_base_dir = match env::var("BLST_SRC_DIR") {
        Ok(val) => val,
        Err(_) => {
            if Path::new("blst").exists() {
                "blst".to_string()
            } else {
                "../..".to_string()
            }
        }
    };
    println!("Using blst source directory {:?}", blst_base_dir);

    let c_src_dir = blst_base_dir.clone() + "/src/";
    let build_dir = blst_base_dir + "/build/";

    file_vec.push(Path::new(&c_src_dir).join("server.c"));
    assembly(&mut file_vec, &build_dir);

    // Set CC environment variable to choose alternative C compiler.
    // Optimization level depends on whether or not --release is passed
    // or implied.
    let mut cc = cc::Build::new();
    if is_adx() {
        cc.define("__ADX__", None);
    }
    cc.flag_if_supported("-mno-avx") // avoid costly transitions
        .flag_if_supported("-Wno-unused-command-line-argument");
    if !cfg!(debug_assertions) {
        cc.opt_level(2);
    }
    cc.files(&file_vec).compile("libblst.a");

    /*
    let binding_src_dir = blst_base_dir + "/bindings/";
    let bindings = bindgen::Builder::default()
        .header(binding_src_dir + "blst.h")
        .opaque_type("blst_pairing")
        .size_t_is_usize(true)
        .rustified_enum("BLST_ERROR")
        .generate()
        .expect("Unable to generate bindings");

    // Write the bindings to the $OUT_DIR/bindings.rs file.
    let out_path = PathBuf::from(env::var("OUT_DIR").unwrap());
    bindings
        .write_to_file(out_path.join("bindings.rs"))
        .expect("Couldn't write bindings!");
    */
}
