#!/bin/bash

# g++ -Wall -g -I../blst/bindings \
#     zkblst.cpp main.cpp thread_pool.cpp ../blst/libblst.a -lpthread

if [ ! -f blst/libblst.a ]; then
    cd blst
    ./build.sh
    cd ..
fi

g++ -Wall -g -O3 -march=native -Icpp -Iblst/bindings -Iblst/src -Isrc \
    src/fil_blst.cpp src/util.cpp src/thread_pool.cpp test/main.cpp \
    blst/libblst.a -lpthread 
