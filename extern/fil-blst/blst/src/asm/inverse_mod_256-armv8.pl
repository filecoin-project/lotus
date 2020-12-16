#!/usr/bin/env perl
#
# Copyright Supranational LLC
# Licensed under the Apache License, Version 2.0, see LICENSE for details.
# SPDX-License-Identifier: Apache-2.0
#
# int eucl_inverse_mod_256(vec256 ret, const vec256 inp, const vec256 mod,
#                          const vec256 one = NULL);
#
# If |one| is 1, then it works as plain inverse procedure.
# If |one| is (1<<512)%|mod|, then it's inverse in Montgomery
# representation domain.

$flavour = shift;
$output  = shift;
if ($flavour =~ /\./) { $output = $flavour; undef $flavour; }

if ($flavour && $flavour ne "void") {
    $0 =~ m/(.*[\/\\])[^\/\\]+$/; $dir=$1;
    ( $xlate="${dir}arm-xlate.pl" and -f $xlate ) or
    ( $xlate="${dir}../../perlasm/arm-xlate.pl" and -f $xlate) or
    die "can't locate arm-xlate.pl";

    open STDOUT,"| \"$^X\" $xlate $flavour $output";
} else {
    open STDOUT,">$output";
}

($r_ptr, $a_ptr, $n_ptr, $one) = map("x$_",(0..3));
($ux_ptr, $vy_ptr, $cnt) = ($a_ptr, $n_ptr, $one);
@acc=map("x$_",(4..11));
@mod=map("x$_",(12..15));

$frame=4*256/8;
$U=0;
$X=$U+256/8;
$V=$X+256/8;
$Y=$V+256/8;

$code.=<<___;
.text

.align	5
.Lone_256:
	.quad	1,0,0,0

.globl	eucl_inverse_mod_256
.hidden	eucl_inverse_mod_256
.type	eucl_inverse_mod_256,%function
.align	5
eucl_inverse_mod_256:
	paciasp
	stp	x29,x30,[sp,#-16]!
	add	x29,sp,#0
	sub	sp,sp,#$frame

	adr	@acc[0],.Lone_256
	cmp	$one,#0
	csel	$one,$one,@acc[0],ne		// default $one to 1

	ldp	@acc[0],@acc[1],[$a_ptr]
	ldp	@acc[2],@acc[3],[$a_ptr,#16]

	orr	@acc[4],@acc[0],@acc[1]
	orr	@acc[5],@acc[2],@acc[3]
	orr	@acc[6],@acc[4],@acc[5]
	cbz	@acc[6],.Labort_256		// abort if |inp|==0

	ldp	@acc[4],@acc[5],[$one]
	ldp	@acc[6],@acc[7],[$one,#16]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]

	stp	@acc[0],@acc[1],[sp,#$U]	// copy |inp| to U
	stp	@acc[2],@acc[3],[sp,#$U+16]

	stp	@acc[4],@acc[5],[sp,#$U+32]	// copy |one| to X
	stp	@acc[6],@acc[7],[sp,#$U+48]

	stp	@mod[0],@mod[1],[sp,#$V]	// copy |mod| to V
	stp	@mod[2],@mod[3],[sp,#$V+16]

	stp	xzr,xzr,[sp,#$V+32]		// clear Y
	stp	xzr,xzr,[sp,#$V+48]
	b	.Loop_inv_256

.align	4
.Loop_inv_256:
	add	$ux_ptr,sp,#$V
	bl	__remove_powers_of_2_256

	add	$ux_ptr,sp,#$U
	bl	__remove_powers_of_2_256

	ldp	@acc[4],@acc[5],[sp,#$V]
	add	$vy_ptr,sp,#$V
	ldp	@acc[6],@acc[7],[sp,#$V+16]
	subs	@acc[0],@acc[0],@acc[4]		// U-V
	sbcs	@acc[1],@acc[1],@acc[5]
	sbcs	@acc[2],@acc[2],@acc[6]
	sbcs	@acc[3],@acc[3],@acc[7]
	b.hs	.Lu_greater_than_v_256

	eor	$vy_ptr,$vy_ptr,$ux_ptr		// xchg	$vy_ptr,$ux_ptr
	 mvn	@acc[0],@acc[0]			// U-V => V-U
	eor	$ux_ptr,$ux_ptr,$vy_ptr
	 mvn	@acc[1],@acc[1]
	eor	$vy_ptr,$vy_ptr,$ux_ptr
	adds	@acc[0],@acc[0],#1
	 mvn	@acc[2],@acc[2]
	adcs	@acc[1],@acc[1],xzr
	 mvn	@acc[3],@acc[3]
	adcs	@acc[2],@acc[2],xzr
	adc	@acc[3],@acc[3],xzr

.Lu_greater_than_v_256:
	 stp	@acc[0],@acc[1],[$ux_ptr]
	ldp	@acc[0],@acc[1],[$vy_ptr,#32]
	ldp	@acc[4],@acc[5],[$ux_ptr,#32]
	 stp	@acc[2],@acc[3],[$ux_ptr,#16]
	ldp	@acc[2],@acc[3],[$vy_ptr,#48]
	subs	@acc[4],@acc[4],@acc[0]		// X-Y		# [alt. Y-X]
	ldp	@acc[6],@acc[7],[$ux_ptr,#48]
	sbcs	@acc[5],@acc[5],@acc[1]
	sbcs	@acc[6],@acc[6],@acc[2]
	sbcs	@acc[7],@acc[7],@acc[3]
	sbc	@acc[3],xzr,xzr			// borrow -> mask

	 and	@acc[0],@mod[0],@acc[3]
	 and	@acc[1],@mod[1],@acc[3]
	adds	@acc[4],@acc[4],@acc[0]		// reduce if X<Y # [alt. Y<X]
	 and	@acc[2],@mod[2],@acc[3]
	adcs	@acc[5],@acc[5],@acc[1]
	 and	@acc[3],@mod[3],@acc[3]
	adcs	@acc[6],@acc[6],@acc[2]
	ldp	@acc[0],@acc[1],[sp,#$U]
	adc	@acc[7],@acc[7],@acc[3]
	ldp	@acc[2],@acc[3],[sp,#$U+16]

	orr	@acc[0],@acc[0],@acc[1]
	orr	@acc[2],@acc[2],@acc[3]
	 stp	@acc[4],@acc[5],[$ux_ptr,#32]
	orr	@acc[0],@acc[0],@acc[2]
	 stp	@acc[6],@acc[7],[$ux_ptr,#48]

	cbnz	@acc[0],.Loop_inv_256		// U!=0?

	ldr	x30,[x29,#8]
	ldp	@acc[0],@acc[1],[sp,#$V+32]	// return Y
	ldp	@acc[2],@acc[3],[sp,#$V+48]
	mov	@acc[6],#1

.Labort_256:
	stp	@acc[0],@acc[1],[$r_ptr]
	stp	@acc[2],@acc[3],[$r_ptr,#16]
	mov	$r_ptr,@acc[6]			// boolean return value

	add	sp,sp,#$frame
	ldr	x29,[sp],#16
	autiasp
	ret
.size	eucl_inverse_mod_256,.-eucl_inverse_mod_256

.type	__remove_powers_of_2_256,%function
.align	4
__remove_powers_of_2_256:
	ldp	@acc[0],@acc[1],[$ux_ptr]
	ldp	@acc[2],@acc[3],[$ux_ptr,#16]

.Loop_of_2_256:
	rbit	$cnt,@acc[0]
	tbnz	$acc[0],#0,.Loop_of_2_done_256

	clz	$cnt,$cnt
	cmp	@acc[0],#0
	mov	@acc[4],#63
	csel	$cnt,$cnt,@acc[4],ne		// unlikely in real life

	neg	@acc[7],$cnt
	lsrv	@acc[0],@acc[0],$cnt		// acc[0:3] >>= cnt
	lslv	@acc[4],@acc[1],@acc[7]
	orr	@acc[0],@acc[0],@acc[4]
	lsrv	@acc[1],@acc[1],$cnt
	lslv	@acc[5],@acc[2],@acc[7]
	orr	@acc[1],@acc[1],@acc[5]
	 ldp	@acc[4],@acc[5],[$ux_ptr,#32]
	lsrv	@acc[2],@acc[2],$cnt
	lslv	@acc[6],@acc[3],@acc[7]
	orr	@acc[2],@acc[2],@acc[6]
	 ldp	@acc[6],@acc[7],[$ux_ptr,#48]
	lsrv	@acc[3],@acc[3],$cnt

	stp	@acc[0], @acc[1],[$ux_ptr]
	stp	@acc[2], @acc[3],[$ux_ptr,#16]
	b	.Loop_div_by_2_256

.align	4
.Loop_div_by_2_256:
	sbfx	@acc[3],@acc[4],#0,#1
	sub	$cnt,$cnt,#1

	 and	@acc[0],@mod[0],@acc[3]
	 and	@acc[1],@mod[1],@acc[3]
	adds	@acc[4],@acc[4],@acc[0]
	 and	@acc[2],@mod[2],@acc[3]
	adcs	@acc[5],@acc[5],@acc[1]
	 and	@acc[3],@mod[3],@acc[3]
	adcs	@acc[6],@acc[6],@acc[2]
	 extr	@acc[4],@acc[5],@acc[4],#1	// acc[4:7] >>= 1
	adcs	@acc[7],@acc[7],@acc[3]
	 extr	@acc[5],@acc[6],@acc[5],#1
	adc	@acc[3],xzr,xzr		// redundant if modulus is <256 bits...
	 extr	@acc[6],@acc[7],@acc[6],#1
	 extr	@acc[7],@acc[3],@acc[7],#1

	cbnz	$cnt,.Loop_div_by_2_256

	ldp	@acc[0],@acc[1],[$ux_ptr] // reload X [mostly for 2nd caller]
	ldp	@acc[2],@acc[3],[$ux_ptr,#16]

	stp	@acc[4],@acc[5],[$ux_ptr,#32]
	stp	@acc[6],@acc[7],[$ux_ptr,#48]

	tbz	@acc[0],#0,.Loop_of_2_256// unlikely in real life

.Loop_of_2_done_256:
	ret
.size	__remove_powers_of_2_256,.-__remove_powers_of_2_256
___

print $code;
close STDOUT;
