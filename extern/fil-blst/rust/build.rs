extern crate cc;

use std::env;
use std::path::Path;
use std::path::PathBuf;

#[cfg(all(target_env = "msvc", target_arch = "x86_64"))]
fn assembly(file_vec: &mut Vec<PathBuf>, base_dir: &Path) {
    let glob_path = format!("{:?}/win64/*-x86_64.asm", base_dir);
    let files = glob::glob(&glob_path).expect("disaster");
    for file in files {
        file_vec.push(file.unwrap());
    }
}

#[cfg(all(target_env = "msvc", target_arch = "aarch64"))]
fn assembly(file_vec: &mut Vec<PathBuf>, base_dir: &Path) {
    let glob_path = format!("{:?}/win64/*-armv8.asm", base_dir);
    let files = glob::glob(&glob_path).expect("disaster");
    for file in files {
        file_vec.push(file.unwrap());
    }
}

#[cfg(all(target_pointer_width = "64", not(target_env = "msvc")))]
fn assembly(file_vec: &mut Vec<PathBuf>, base_dir: &Path) {
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

    // There are symlinks in the current directory to blst sources to make sure those sources are
    // also bundled in the crate when published
    let fil_blst_src_dir = PathBuf::from("fil-blst");
    let blst_base_dir = PathBuf::from("blst");
    println!(
        "Using fil-blst source directory {:?}",
        fil_blst_src_dir
    );
    println!("Using       blst source directory {:?}", blst_base_dir);

    let c_src_dir = blst_base_dir.join("src");
    let build_dir = blst_base_dir.join("build");
    let binding_src_dir = blst_base_dir.join("bindings");

    file_vec.push(c_src_dir.join("server.c"));
    assembly(&mut file_vec, &build_dir);

    let mut cpp_file_vec = Vec::new();
    cpp_file_vec.push(fil_blst_src_dir.join("fil_blst.cpp"));
    cpp_file_vec.push(fil_blst_src_dir.join("thread_pool.cpp"));

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
        cc.opt_level(3); // Must be consistent with Go build
    }
    cc.files(&file_vec).compile("libblst.a");

    // Build fil-blst
    let mut cpp = cc::Build::new();
    if is_adx() {
        cpp.define("__ADX__", None);
    }
    cpp.flag_if_supported("-mno-avx"); // avoid costly transitions
    if !cfg!(debug_assertions) {
        cpp.opt_level(3); // Must be consistent with Go build
    }
    // Needed for macOS
    cpp.flag_if_supported("-std=gnu++11");
    cpp.cpp(true)
        .include(&binding_src_dir)
        .files(&cpp_file_vec)
        .compile("libfilblst.a");

    let bindings = bindgen::Builder::default()
        .header(
            binding_src_dir
                .join("blst.h")
                .to_str()
                .expect("path is valid utf-8")
                .to_string(),
        )
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
}
