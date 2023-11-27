#!/bin/sh
set -eu
#set -o xtrace

# By default compile for 512MiB and 32GiB sectors only, use `-r` to compile for
# other sector test sector sizes as well.
SECTOR_SIZE=""
while getopts r flag
do
    case "${flag}" in
        r) SECTOR_SIZE="-DRUNTIME_SECTOR_SIZE";;
        *) ;;
    esac
done

if [ ! -d "supra_seal" ]; then
    git clone https://github.com/supranational/supra_seal.git
fi

cd supra_seal

rm -fr obj
mkdir -p obj

rm -fr bin
mkdir -p bin

mkdir -p deps
if [ ! -d "deps/sppark" ]; then
    git clone https://github.com/supranational/sppark.git deps/sppark
fi
if [ ! -d "deps/blst" ]; then
    git clone https://github.com/supranational/blst.git deps/blst
    (cd deps/blst
     ./build.sh -march=native)
fi

# Generate .h files for the Poseidon constants
xxd -i poseidon/constants/constants_2  > obj/constants_2.h
xxd -i poseidon/constants/constants_4  > obj/constants_4.h
xxd -i poseidon/constants/constants_8  > obj/constants_8.h
xxd -i poseidon/constants/constants_11 > obj/constants_11.h
xxd -i poseidon/constants/constants_16 > obj/constants_16.h
xxd -i poseidon/constants/constants_24 > obj/constants_24.h
xxd -i poseidon/constants/constants_36 > obj/constants_36.h

nvcc ${SECTOR_SIZE} -DNO_SPDK -DSTREAMING_NODE_READER_FILES \
     -arch=sm_80 -gencode arch=compute_70,code=sm_70 -t0 \
     -std=c++17 -g -O3 -Xcompiler -march=native \
     -Xcompiler -Wall,-Wextra,-Werror \
     -Xcompiler -Wno-subobject-linkage,-Wno-unused-parameter \
     -x cu tools/pc2.cu -o bin/pc2 \
     -Iposeidon -Ideps/sppark -Ideps/sppark/util -Ideps/blst/src -L deps/blst -lblst -lconfig++