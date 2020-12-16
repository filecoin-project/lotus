#!/bin/sh -e
#
# The script allows to override 'CC', 'CFLAGS' and 'flavour' at command
# line, as well as specify additional compiler flags. For example to
# compile for x32:
#
#	/some/where/build.sh flavour=elf32 -mx32
#
# To cross-compile for mingw/Windows:
#
#	/some/where/build.sh flavour=mingw64 CC=x86_64-w64-mingw32-gcc
#
# In addition script recognizes -shared flag and creates shared library
# alongside libblst.lib.

TOP=`dirname $0`

CC=${CC:-cc}
# if -Werror stands in the way, bypass with -Wno-error on command line
CFLAGS=${CFLAGS:--O -fPIC -Wall -Wextra -Werror}
PERL=${PERL:-perl}
unset cflags shared

case `uname -s` in
    Darwin)	flavour=macosx
                if (sysctl machdep.cpu.features) 2>/dev/null | grep -q ADX; then
                    cflags="-D__ADX__"
                fi
                ;;
    CYGWIN*)	flavour=mingw64;;
    MINGW*)	flavour=mingw64;;
    *)		flavour=elf;;
esac

while [ "x$1" != "x" ]; do
    case $1 in
        -shared)    shared=1;;
        -*)         cflags="$cflags $1";;
        *=*)        eval "$1";;
    esac
    shift
done

if (${CC} ${CFLAGS} -dM -E -x c /dev/null) 2>/dev/null | grep -q x86_64; then
    cflags="$cflags -mno-avx" # avoid costly transitions
    if (grep -q -e '^flags.*\badx\b' /proc/cpuinfo) 2>/dev/null; then
        cflags="-D__ADX__ $cflags"
    fi
fi

CFLAGS="$CFLAGS $cflags"

rm -f libblst.a
trap '[ $? -ne 0 ] && rm -f libblst.a; rm -f *.o' 0

(set -x; ${CC} ${CFLAGS} -c ${TOP}/src/server.c)
(set -x; ${CC} ${CFLAGS} -c ${TOP}/build/assembly.S)
(set -x; ${AR:-ar} rc libblst.a *.o)

if [ $shared ]; then
    case $flavour in
        macosx) (set -x; ${CC} -dynamiclib -o libblst.dylib \
                               -all_load libblst.a ${CFLAGS}); exit 0;;
        mingw*) sharedlib=blst.dll
                CFLAGS="${CFLAGS} --entry=DllMain ${TOP}/build/win64/dll.c"
                CFLAGS="${CFLAGS} -nostdlib -lgcc";;
        *)      sharedlib=libblst.so;;
    esac
    echo "{ global: blst_*; BLS12_381_*; local: *; };" |\
    (set -x; ${CC} -shared -o $sharedlib libblst.a ${CFLAGS} \
                   -Wl,-Bsymbolic,--require-defined=blst_keygen \
                   -Wl,--version-script=/dev/fd/0)
fi
