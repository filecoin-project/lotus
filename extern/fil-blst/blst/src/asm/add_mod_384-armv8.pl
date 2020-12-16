#!/usr/bin/env perl
#
# Copyright Supranational LLC
# Licensed under the Apache License, Version 2.0, see LICENSE for details.
# SPDX-License-Identifier: Apache-2.0

$flavour = shift;
$output  = shift;

if ($flavour && $flavour ne "void") {
    $0 =~ m/(.*[\/\\])[^\/\\]+$/; $dir=$1;
    ( $xlate="${dir}arm-xlate.pl" and -f $xlate ) or
    ( $xlate="${dir}../../perlasm/arm-xlate.pl" and -f $xlate) or
    die "can't locate arm-xlate.pl";

    open STDOUT,"| \"$^X\" $xlate $flavour $output";
} else {
    open STDOUT,">$output";
}

($r_ptr,$a_ptr,$b_ptr,$n_ptr) = map("x$_", 0..3);

@mod=map("x$_",(4..9));
@a=map("x$_",(10..15));
@b=map("x$_",(16,17,19..22));
$carry=$n_ptr;

$code.=<<___;
.text

.extern BLS12_381_P
.hidden BLS12_381_P

.globl	add_mod_384
.hidden	add_mod_384
.type	add_mod_384,%function
.align	5
add_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__add_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	add_mod_384,.-add_mod_384

.type	__add_mod_384,%function
.align	5
__add_mod_384:
	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@b[0],@b[1],[$b_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@b[2],@b[3],[$b_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]
	ldp	@b[4],@b[5],[$b_ptr,#32]

__add_mod_384_ab_are_loaded:
	adds	@a[0],@a[0],@b[0]
	adcs	@a[1],@a[1],@b[1]
	adcs	@a[2],@a[2],@b[2]
	adcs	@a[3],@a[3],@b[3]
	adcs	@a[4],@a[4],@b[4]
	adcs	@a[5],@a[5],@b[5]
	adc	$carry,xzr,xzr

	subs	@b[0],@a[0],@mod[0]
	sbcs	@b[1],@a[1],@mod[1]
	sbcs	@b[2],@a[2],@mod[2]
	sbcs	@b[3],@a[3],@mod[3]
	sbcs	@b[4],@a[4],@mod[4]
	sbcs	@b[5],@a[5],@mod[5]
	sbcs	xzr,$carry,xzr

	csel	@a[0],@a[0],@b[0],lo
	csel	@a[1],@a[1],@b[1],lo
	csel	@a[2],@a[2],@b[2],lo
	csel	@a[3],@a[3],@b[3],lo
	csel	@a[4],@a[4],@b[4],lo
	csel	@a[5],@a[5],@b[5],lo

	ret
.size	__add_mod_384,.-__add_mod_384

.globl	add_mod_384x
.hidden	add_mod_384x
.type	add_mod_384x,%function
.align	5
add_mod_384x:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__add_mod_384

	stp	@a[0],@a[1],[$r_ptr]
	add	$a_ptr,$a_ptr,#48
	stp	@a[2],@a[3],[$r_ptr,#16]
	add	$b_ptr,$b_ptr,#48
	stp	@a[4],@a[5],[$r_ptr,#32]

	bl	__add_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	add_mod_384x,.-add_mod_384x

.globl	lshift_mod_384
.hidden	lshift_mod_384
.type	lshift_mod_384,%function
.align	5
lshift_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

.Loop_lshift_mod_384:
	sub	$b_ptr,$b_ptr,#1
	bl	__lshift_mod_384
	cbnz	$b_ptr,.Loop_lshift_mod_384

	ldr	x30,[sp,#8]
	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	lshift_mod_384,.-lshift_mod_384

.type	__lshift_mod_384,%function
.align	5
__lshift_mod_384:
	adds	@a[0],@a[0],@a[0]
	adcs	@a[1],@a[1],@a[1]
	adcs	@a[2],@a[2],@a[2]
	adcs	@a[3],@a[3],@a[3]
	adcs	@a[4],@a[4],@a[4]
	adcs	@a[5],@a[5],@a[5]
	adc	$carry,xzr,xzr

	subs	@b[0],@a[0],@mod[0]
	sbcs	@b[1],@a[1],@mod[1]
	sbcs	@b[2],@a[2],@mod[2]
	sbcs	@b[3],@a[3],@mod[3]
	sbcs	@b[4],@a[4],@mod[4]
	sbcs	@b[5],@a[5],@mod[5]
	sbcs	xzr,$carry,xzr

	csel	@a[0],@a[0],@b[0],lo
	csel	@a[1],@a[1],@b[1],lo
	csel	@a[2],@a[2],@b[2],lo
	csel	@a[3],@a[3],@b[3],lo
	csel	@a[4],@a[4],@b[4],lo
	csel	@a[5],@a[5],@b[5],lo

	ret
.size	__lshift_mod_384,.-__lshift_mod_384

.globl	mul_by_3_mod_384
.hidden	mul_by_3_mod_384
.type	mul_by_3_mod_384,%function
.align	5
mul_by_3_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$b_ptr]
	ldp	@mod[2],@mod[3],[$b_ptr,#16]
	ldp	@mod[4],@mod[5],[$b_ptr,#32]

	bl	__lshift_mod_384

	ldp	@b[0],@b[1],[$a_ptr]
	ldp	@b[2],@b[3],[$a_ptr,#16]
	ldp	@b[4],@b[5],[$a_ptr,#32]

	bl	__add_mod_384_ab_are_loaded
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_3_mod_384,.-mul_by_3_mod_384

.globl	mul_by_8_mod_384
.hidden	mul_by_8_mod_384
.type	mul_by_8_mod_384,%function
.align	5
mul_by_8_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$b_ptr]
	ldp	@mod[2],@mod[3],[$b_ptr,#16]
	ldp	@mod[4],@mod[5],[$b_ptr,#32]

	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_8_mod_384,.-mul_by_8_mod_384

.globl	mul_by_b_onE1
.hidden	mul_by_b_onE1
.type	mul_by_b_onE1,%function
.align	5
mul_by_b_onE1:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	adrp	$n_ptr,BLS12_381_P
	ldp	@a[0],@a[1],[$a_ptr]
	add	$n_ptr,$n_ptr,#:lo12:BLS12_381_P
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__lshift_mod_384
	bl	__lshift_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_b_onE1,.-mul_by_b_onE1

.globl	mul_by_4b_onE1
.hidden	mul_by_4b_onE1
.type	mul_by_4b_onE1,%function
.align	5
mul_by_4b_onE1:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	adrp	$n_ptr,BLS12_381_P
	ldp	@a[0],@a[1],[$a_ptr]
	add	$n_ptr,$n_ptr,#:lo12:BLS12_381_P
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_4b_onE1,.-mul_by_4b_onE1

.globl	mul_by_3_mod_384x
.hidden	mul_by_3_mod_384x
.type	mul_by_3_mod_384x,%function
.align	5
mul_by_3_mod_384x:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$b_ptr]
	ldp	@mod[2],@mod[3],[$b_ptr,#16]
	ldp	@mod[4],@mod[5],[$b_ptr,#32]

	bl	__lshift_mod_384

	ldp	@b[0],@b[1],[$a_ptr]
	ldp	@b[2],@b[3],[$a_ptr,#16]
	ldp	@b[4],@b[5],[$a_ptr,#32]

	bl	__add_mod_384_ab_are_loaded

	stp	@a[0],@a[1],[$r_ptr]
	ldp	@a[0],@a[1],[$a_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#16]
	ldp	@a[2],@a[3],[$a_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#32]
	ldp	@a[4],@a[5],[$a_ptr,#80]

	bl	__lshift_mod_384

	ldp	@b[0],@b[1],[$a_ptr,#48]
	ldp	@b[2],@b[3],[$a_ptr,#64]
	ldp	@b[4],@b[5],[$a_ptr,#80]

	bl	__add_mod_384_ab_are_loaded
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_3_mod_384x,.-mul_by_3_mod_384x

.globl	mul_by_8_mod_384x
.hidden	mul_by_8_mod_384x
.type	mul_by_8_mod_384x,%function
.align	5
mul_by_8_mod_384x:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]

	ldp	@mod[0],@mod[1],[$b_ptr]
	ldp	@mod[2],@mod[3],[$b_ptr,#16]
	ldp	@mod[4],@mod[5],[$b_ptr,#32]

	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384

	stp	@a[0],@a[1],[$r_ptr]
	ldp	@a[0],@a[1],[$a_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#16]
	ldp	@a[2],@a[3],[$a_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#32]
	ldp	@a[4],@a[5],[$a_ptr,#80]

	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_8_mod_384x,.-mul_by_8_mod_384x

.globl	mul_by_b_onE2
.hidden	mul_by_b_onE2
.type	mul_by_b_onE2,%function
.align	5
mul_by_b_onE2:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	adrp	$n_ptr,BLS12_381_P
	add	$n_ptr,$n_ptr,#:lo12:BLS12_381_P
	add	$b_ptr,$a_ptr,#48

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__sub_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	bl	__add_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_b_onE2,.-mul_by_b_onE2

.globl	mul_by_4b_onE2
.hidden	mul_by_4b_onE2
.type	mul_by_4b_onE2,%function
.align	5
mul_by_4b_onE2:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	adrp	$n_ptr,BLS12_381_P
	add	$n_ptr,$n_ptr,#:lo12:BLS12_381_P
	add	$b_ptr,$a_ptr,#48

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__sub_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	bl	__add_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	bl	__lshift_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_4b_onE2,.-mul_by_4b_onE2

.globl	cneg_mod_384
.hidden	cneg_mod_384
.type	cneg_mod_384,%function
.align	5
cneg_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]

	subs	@b[0],@mod[0],@a[0]
	ldp	@a[4],@a[5],[$a_ptr,#32]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]
	 orr	$carry,@a[0],@a[1]
	sbcs	@b[1],@mod[1],@a[1]
	 orr	$carry,$carry,@a[2]
	sbcs	@b[2],@mod[2],@a[2]
	 orr	$carry,$carry,@a[3]
	sbcs	@b[3],@mod[3],@a[3]
	 orr	$carry,$carry,@a[4]
	sbcs	@b[4],@mod[4],@a[4]
	 orr	$carry,$carry,@a[5]
	sbc	@b[5],@mod[5],@a[5]

	cmp	$carry,#0
	csetm	$carry,ne
	ands	$b_ptr,$b_ptr,$carry

	csel	@a[0],@a[0],@b[0],eq
	csel	@a[1],@a[1],@b[1],eq
	csel	@a[2],@a[2],@b[2],eq
	csel	@a[3],@a[3],@b[3],eq
	stp	@a[0],@a[1],[$r_ptr]
	csel	@a[4],@a[4],@b[4],eq
	stp	@a[2],@a[3],[$r_ptr,#16]
	csel	@a[5],@a[5],@b[5],eq
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	cneg_mod_384,.-cneg_mod_384

.globl	sub_mod_384
.hidden	sub_mod_384
.type	sub_mod_384,%function
.align	5
sub_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__sub_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr]
	stp	@a[2],@a[3],[$r_ptr,#16]
	stp	@a[4],@a[5],[$r_ptr,#32]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	sub_mod_384,.-sub_mod_384

.type	__sub_mod_384,%function
.align	5
__sub_mod_384:
	ldp	@a[0],@a[1],[$a_ptr]
	ldp	@b[0],@b[1],[$b_ptr]
	ldp	@a[2],@a[3],[$a_ptr,#16]
	ldp	@b[2],@b[3],[$b_ptr,#16]
	ldp	@a[4],@a[5],[$a_ptr,#32]
	ldp	@b[4],@b[5],[$b_ptr,#32]

	subs	@a[0],@a[0],@b[0]
	sbcs	@a[1],@a[1],@b[1]
	sbcs	@a[2],@a[2],@b[2]
	sbcs	@a[3],@a[3],@b[3]
	sbcs	@a[4],@a[4],@b[4]
	sbcs	@a[5],@a[5],@b[5]
	sbc	$carry,xzr,xzr

	 and	@b[0],@mod[0],$carry
	 and	@b[1],@mod[1],$carry
	adds	@a[0],@a[0],@b[0]
	 and	@b[2],@mod[2],$carry
	adcs	@a[1],@a[1],@b[1]
	 and	@b[3],@mod[3],$carry
	adcs	@a[2],@a[2],@b[2]
	 and	@b[4],@mod[4],$carry
	adcs	@a[3],@a[3],@b[3]
	 and	@b[5],@mod[5],$carry
	adcs	@a[4],@a[4],@b[4]
	adc	@a[5],@a[5],@b[5]

	ret
.size	__sub_mod_384,.-__sub_mod_384

.globl	sub_mod_384x
.hidden	sub_mod_384x
.type	sub_mod_384x,%function
.align	5
sub_mod_384x:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	bl	__sub_mod_384

	stp	@a[0],@a[1],[$r_ptr]
	add	$a_ptr,$a_ptr,#48
	stp	@a[2],@a[3],[$r_ptr,#16]
	add	$b_ptr,$b_ptr,#48
	stp	@a[4],@a[5],[$r_ptr,#32]

	bl	__sub_mod_384
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	sub_mod_384x,.-sub_mod_384x

.globl	mul_by_1_plus_i_mod_384x
.hidden	mul_by_1_plus_i_mod_384x
.type	mul_by_1_plus_i_mod_384x,%function
.align	5
mul_by_1_plus_i_mod_384x:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]

	ldp	@mod[0],@mod[1],[$b_ptr]
	ldp	@mod[2],@mod[3],[$b_ptr,#16]
	ldp	@mod[4],@mod[5],[$b_ptr,#32]
	add	$b_ptr,$a_ptr,#48

	bl	__sub_mod_384			// a->re - a->im

	ldp	@b[0],@b[1],[$a_ptr]
	ldp	@b[2],@b[3],[$a_ptr,#16]
	ldp	@b[4],@b[5],[$a_ptr,#32]
	stp	@a[0],@a[1],[$r_ptr]
	ldp	@a[0],@a[1],[$a_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#16]
	ldp	@a[2],@a[3],[$a_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#32]
	ldp	@a[4],@a[5],[$a_ptr,#80]

	bl	__add_mod_384_ab_are_loaded	// a->re + a->im
	ldr	x30,[sp,#8]

	stp	@a[0],@a[1],[$r_ptr,#48]
	stp	@a[2],@a[3],[$r_ptr,#64]
	stp	@a[4],@a[5],[$r_ptr,#80]

	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	mul_by_1_plus_i_mod_384x,.-mul_by_1_plus_i_mod_384x

.globl	sgn0_pty_mod_384
.hidden	sgn0_pty_mod_384
.type	sgn0_pty_mod_384,%function
.align	5
sgn0_pty_mod_384:
	ldp	@a[0],@a[1],[$r_ptr]
	ldp	@a[2],@a[3],[$r_ptr,#16]
	ldp	@a[4],@a[5],[$r_ptr,#32]

	ldp	@mod[0],@mod[1],[$a_ptr]
	ldp	@mod[2],@mod[3],[$a_ptr,#16]
	ldp	@mod[4],@mod[5],[$a_ptr,#32]

	and	$r_ptr,@a[0],#1
	adds	@a[0],@a[0],@a[0]
	adcs	@a[1],@a[1],@a[1]
	adcs	@a[2],@a[2],@a[2]
	adcs	@a[3],@a[3],@a[3]
	adcs	@a[4],@a[4],@a[4]
	adcs	@a[5],@a[5],@a[5]
	adc	$carry,xzr,xzr

	subs	@a[0],@a[0],@mod[0]
	sbcs	@a[1],@a[1],@mod[1]
	sbcs	@a[2],@a[2],@mod[2]
	sbcs	@a[3],@a[3],@mod[3]
	sbcs	@a[4],@a[4],@mod[4]
	sbcs	@a[5],@a[5],@mod[5]
	sbc	$carry,$carry,xzr

	mvn	$carry,$carry
	and	$carry,$carry,#2
	orr	$r_ptr,$r_ptr,$carry

	ret
.size	sgn0_pty_mod_384,.-sgn0_pty_mod_384

.globl	sgn0_pty_mod_384x
.hidden	sgn0_pty_mod_384x
.type	sgn0_pty_mod_384x,%function
.align	5
sgn0_pty_mod_384x:
	ldp	@a[0],@a[1],[$r_ptr]
	ldp	@a[2],@a[3],[$r_ptr,#16]
	ldp	@a[4],@a[5],[$r_ptr,#32]

	ldp	@mod[0],@mod[1],[$a_ptr]
	ldp	@mod[2],@mod[3],[$a_ptr,#16]
	ldp	@mod[4],@mod[5],[$a_ptr,#32]

	and	$b_ptr,@a[0],#1
	 orr	$n_ptr,@a[0],@a[1]
	adds	@a[0],@a[0],@a[0]
	 orr	$n_ptr,$n_ptr,@a[2]
	adcs	@a[1],@a[1],@a[1]
	 orr	$n_ptr,$n_ptr,@a[3]
	adcs	@a[2],@a[2],@a[2]
	 orr	$n_ptr,$n_ptr,@a[4]
	adcs	@a[3],@a[3],@a[3]
	 orr	$n_ptr,$n_ptr,@a[5]
	adcs	@a[4],@a[4],@a[4]
	adcs	@a[5],@a[5],@a[5]
	adc	@b[0],xzr,xzr

	subs	@a[0],@a[0],@mod[0]
	sbcs	@a[1],@a[1],@mod[1]
	sbcs	@a[2],@a[2],@mod[2]
	sbcs	@a[3],@a[3],@mod[3]
	sbcs	@a[4],@a[4],@mod[4]
	sbcs	@a[5],@a[5],@mod[5]
	sbc	@b[0],@b[0],xzr

	ldp	@a[0],@a[1],[$r_ptr,#48]
	ldp	@a[2],@a[3],[$r_ptr,#64]
	ldp	@a[4],@a[5],[$r_ptr,#80]

	mvn	@b[0],@b[0]
	and	@b[0],@b[0],#2
	orr	$b_ptr,$b_ptr,@b[0]

	and	$r_ptr,@a[0],#1
	 orr	$a_ptr,@a[0],@a[1]
	adds	@a[0],@a[0],@a[0]
	 orr	$a_ptr,$a_ptr,@a[2]
	adcs	@a[1],@a[1],@a[1]
	 orr	$a_ptr,$a_ptr,@a[3]
	adcs	@a[2],@a[2],@a[2]
	 orr	$a_ptr,$a_ptr,@a[4]
	adcs	@a[3],@a[3],@a[3]
	 orr	$a_ptr,$a_ptr,@a[5]
	adcs	@a[4],@a[4],@a[4]
	adcs	@a[5],@a[5],@a[5]
	adc	@b[0],xzr,xzr

	subs	@a[0],@a[0],@mod[0]
	sbcs	@a[1],@a[1],@mod[1]
	sbcs	@a[2],@a[2],@mod[2]
	sbcs	@a[3],@a[3],@mod[3]
	sbcs	@a[4],@a[4],@mod[4]
	sbcs	@a[5],@a[5],@mod[5]
	sbc	@b[0],@b[0],xzr

	mvn	@b[0],@b[0]
	and	@b[0],@b[0],#2
	orr	$r_ptr,$r_ptr,@b[0]

	cmp	$n_ptr,#0
	csel	$n_ptr,$r_ptr,$b_ptr,eq	// a->re==0? prty(a->im) : prty(a->re)

	cmp	$a_ptr,#0
	csel	$a_ptr,$r_ptr,$b_ptr,ne	// a->im!=0? sgn0(a->im) : sgn0(a->re)

	and	$n_ptr,$n_ptr,#1
	and	$a_ptr,$a_ptr,#2
	orr	$r_ptr,$a_ptr,$n_ptr	// pack sign and parity

	ret
.size	sgn0_pty_mod_384x,.-sgn0_pty_mod_384x
___
if (1) {
sub vec_select {
my $sz = shift;
my @v=map("v$_",(0..5,16..21));

$code.=<<___;
.globl	vec_select_$sz
.hidden	vec_select_$sz
.type	vec_select_$sz,%function
.align	5
vec_select_$sz:
	dup	v6.2d, $n_ptr
	ld1	{@v[0].2d, @v[1].2d, @v[2].2d}, [$a_ptr],#48
	cmeq	v6.2d, v6.2d, #0
	ld1	{@v[3].2d, @v[4].2d, @v[5].2d}, [$b_ptr],#48
___
for($i=0; $i<$sz-48; $i+=48) {
$code.=<<___;
	bit	@v[0].16b, @v[3].16b, v6.16b
	ld1	{@v[6].2d, @v[7].2d, @v[8].2d}, [$a_ptr],#48
	bit	@v[1].16b, @v[4].16b, v6.16b
	ld1	{@v[9].2d, @v[10].2d, @v[11].2d}, [$b_ptr],#48
	bit	@v[2].16b, @v[5].16b, v6.16b
	st1	{@v[0].2d, @v[1].2d, @v[2].2d}, [$r_ptr],#48
___
	@v = @v[6..11,0..5];
}
$code.=<<___;
	bit	@v[0].16b, @v[3].16b, v6.16b
	bit	@v[1].16b, @v[4].16b, v6.16b
	bit	@v[2].16b, @v[5].16b, v6.16b
	st1	{@v[0].2d, @v[1].2d, @v[2].2d}, [$r_ptr]
	ret
.size	vec_select_$sz,.-vec_select_$sz
___
}
vec_select(144);
vec_select(288);
}
print $code;

close STDOUT;
