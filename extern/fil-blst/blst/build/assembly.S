#if defined(__x86_64) || defined(__x86_64__)
# if defined(__ELF__)
#  include "elf/sha256-x86_64.s"
#  include "elf/inverse_mod_384-x86_64.s"
#  include "elf/add_mod_384-x86_64.s"
#  include "elf/add_mod_384x384-x86_64.s"
#  define __add_mod_384     __add_mont_384
#  define __sub_mod_384     __sub_mont_384
#  define __sub_mod_384x384 __sub_mont_384x384
#  if defined(__ADX__) && !defined(__BLST_PORTABLE__)
#   include "elf/mulx_mont_384-x86_64.s"
#   include "elf/mulx_mont_256-x86_64.s"
#  else
#   include "elf/mulq_mont_384-x86_64.s"
#   include "elf/mulq_mont_256-x86_64.s"
#  endif
#  include "elf/add_mod_256-x86_64.s"
#  include "elf/inverse_mod_256-x86_64.s"
# elif defined(_WIN64) || defined(__CYGWIN__)
#  include "coff/sha256-x86_64.s"
#  include "coff/inverse_mod_384-x86_64.s"
#  include "coff/add_mod_384-x86_64.s"
#  include "coff/add_mod_384x384-x86_64.s"
#  define __add_mod_384     __add_mont_384
#  define __sub_mod_384     __sub_mont_384
#  define __sub_mod_384x384 __sub_mont_384x384
#  if defined(__ADX__) && !defined(__BLST_PORTABLE__)
#   include "coff/mulx_mont_384-x86_64.s"
#   include "coff/mulx_mont_256-x86_64.s"
#  else
#   include "coff/mulq_mont_384-x86_64.s"
#   include "coff/mulq_mont_256-x86_64.s"
#  endif
#  include "coff/add_mod_256-x86_64.s"
#  include "coff/inverse_mod_256-x86_64.s"
# elif defined(__APPLE__)
#  include "mach-o/sha256-x86_64.s"
#  include "mach-o/inverse_mod_384-x86_64.s"
#  include "mach-o/add_mod_384-x86_64.s"
#  include "mach-o/add_mod_384x384-x86_64.s"
#  define __add_mod_384     __add_mont_384
#  define __sub_mod_384     __sub_mont_384
#  define __sub_mod_384x384 __sub_mont_384x384
#  if defined(__ADX__) && !defined(__BLST_PORTABLE__)
#   include "mach-o/mulx_mont_384-x86_64.s"
#   include "mach-o/mulx_mont_256-x86_64.s"
#  else
#   include "mach-o/mulq_mont_384-x86_64.s"
#   include "mach-o/mulq_mont_256-x86_64.s"
#  endif
#  include "mach-o/add_mod_256-x86_64.s"
#  include "mach-o/inverse_mod_256-x86_64.s"
# endif
#elif defined(__aarch64__)
# if defined(__ELF__)
#  include "elf/sha256-armv8.S"
#  include "elf/inverse_mod_384-armv8.S"
#  include "elf/add_mod_384-armv8.S"
#  define __add_mod_384     __add_mont_384
#  define __sub_mod_384     __sub_mont_384
#  include "elf/mul_mont_384-armv8.S"
#  include "elf/mul_mont_256-armv8.S"
#  include "elf/add_mod_256-armv8.S"
#  include "elf/inverse_mod_256-armv8.S"
# elif defined(_WIN64)
#  include "coff/sha256-armv8.S"
#  include "coff/inverse_mod_384-armv8.S"
#  include "coff/add_mod_384-armv8.S"
#  define __add_mod_384     __add_mont_384
#  define __sub_mod_384     __sub_mont_384
#  include "coff/mul_mont_384-armv8.S"
#  include "coff/mul_mont_256-armv8.S"
#  include "coff/add_mod_256-armv8.S"
#  include "coff/inverse_mod_256-armv8.S"
# elif defined(__APPLE__)
#  include "mach-o/sha256-armv8.S"
#  include "mach-o/inverse_mod_384-armv8.S"
#  include "mach-o/add_mod_384-armv8.S"
#  define __add_mod_384     __add_mont_384
#  define __sub_mod_384     __sub_mont_384
#  include "mach-o/mul_mont_384-armv8.S"
#  include "mach-o/mul_mont_256-armv8.S"
#  include "mach-o/add_mod_256-armv8.S"
#  include "mach-o/inverse_mod_256-armv8.S"
# endif
#else
# error "unsupported platform"
#endif
