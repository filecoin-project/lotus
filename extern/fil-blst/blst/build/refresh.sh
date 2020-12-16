#!/bin/sh

HERE=`dirname $0`
cd "${HERE}"

PERL=${PERL:-perl}

for pl in ../src/asm/*-x86_64.pl; do
    s=`basename $pl .pl`.asm
    (set -x; ${PERL} $pl masm > win64/$s)
    s=`basename $pl .pl`.s
    (set -x; ${PERL} $pl elf > elf/$s)
    (set -x; ${PERL} $pl mingw64 > coff/$s)
    (set -x; ${PERL} $pl macosx > mach-o/$s)
done

for pl in ../src/asm/*-armv8.pl; do
    s=`basename $pl .pl`.asm
    (set -x; ${PERL} $pl win64 > win64/$s)
    s=`basename $pl .pl`.S
    (set -x; ${PERL} $pl linux64 > elf/$s)
    (set -x; ${PERL} $pl coff64 > coff/$s)
    (set -x; ${PERL} $pl ios64 > mach-o/$s)
done

( cd ../bindings;
  echo "LIBRARY blst\n\nEXPORTS"
  cc -E blst.h | \
  ${PERL} -ne '{ (/(blst_[\w]+)\s*\(/ || /(BLS12_[\w]+);/) &&  print "\t$1\n" }'
  echo
) > win64/blst.def

if which bindgen > /dev/null 2>&1; then
  ( cd ../bindings; set -x;
    bindgen --opaque-type blst_pairing \
            --with-derive-default \
            --with-derive-eq \
            --size_t-is-usize \
            --rustified-enum BLST.\* \
        blst.h | ${PERL} ../build/bindings_trim.pl > rust/src/bindings.rs
  )
else
    echo "Install Rust bindgen with 'cargo install bindgen'" 1>&2
    exit 1
fi
