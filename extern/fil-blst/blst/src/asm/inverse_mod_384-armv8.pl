#!/usr/bin/env perl
#
# Copyright Supranational LLC
# Licensed under the Apache License, Version 2.0, see LICENSE for details.
# SPDX-License-Identifier: Apache-2.0
#
# int eucl_inverse_mod_384(vec384 ret, const vec384 inp,
#                          const vec384 mod, const vec384 one = NULL);
#
# If |one| is 1, then it works as plain inverse procedure.
# If |one| is (1<<768)%|mod|, then it's inverse in Montgomery
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
@acc=map("x$_",(4..15));
@mod=map("x$_",(16,17,19..22));

$frame=4*384/8;
$U=0;
$X=$U+384/8;
$V=$X+384/8;
$Y=$V+384/8;

$code.=<<___;
.text

.align	5
.Lone:
	.quad	1,0,0,0,0,0,0,0

.globl	eucl_inverse_mod_384
.hidden	eucl_inverse_mod_384
.type	eucl_inverse_mod_384,%function
.align	5
eucl_inverse_mod_384:
	paciasp
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]
	sub	sp,sp,#$frame

	adr	@acc[0],.Lone
	cmp	$one,#0
	csel	$one,$one,@acc[0],ne		// default $one to 1

	ldp	@acc[0],@acc[1],[$a_ptr]
	ldp	@acc[2],@acc[3],[$a_ptr,#16]
	ldp	@acc[4],@acc[5],[$a_ptr,#32]

	orr	@acc[6],@acc[0],@acc[1]
	orr	@acc[7],@acc[2],@acc[3]
	orr	@acc[8],@acc[4],@acc[5]
	orr	@acc[6],@acc[6],@acc[7]
	orr	@acc[6],@acc[6],@acc[8]
	cbz	@acc[6],.Labort			// abort if |inp|==0

	ldp	@acc[6],@acc[7],[$one]
	ldp	@acc[8],@acc[9],[$one,#16]
	ldp	@acc[10],@acc[11],[$one,#32]

	ldp	@mod[0],@mod[1],[$n_ptr]
	ldp	@mod[2],@mod[3],[$n_ptr,#16]
	ldp	@mod[4],@mod[5],[$n_ptr,#32]

	stp	@acc[0],@acc[1],[sp,#$U]	// copy |inp| to U
	stp	@acc[2],@acc[3],[sp,#$U+16]
	stp	@acc[4],@acc[5],[sp,#$U+32]

	stp	@acc[6],@acc[7],[sp,#$U+48]	// copy |one| to X
	stp	@acc[8],@acc[9],[sp,#$U+64]
	stp	@acc[10],@acc[11],[sp,#$U+80]

	stp	@mod[0],@mod[1],[sp,#$V]	// copy |mod| to V
	stp	@mod[2],@mod[3],[sp,#$V+16]
	stp	@mod[4],@mod[5],[sp,#$V+32]

	stp	xzr,xzr,[sp,#$V+48]		// clear Y
	stp	xzr,xzr,[sp,#$V+64]
	stp	xzr,xzr,[sp,#$V+80]
	b	.Loop_inv

.align	5
.Loop_inv:
	add	$ux_ptr,sp,#$V
	bl	__remove_powers_of_2

	add	$ux_ptr,sp,#$U
	bl	__remove_powers_of_2

	ldp	@acc[6],@acc[7],[sp,#$V]
	add	$vy_ptr,sp,#$V
	ldp	@acc[8],@acc[9],[sp,#$V+16]
	subs	@acc[0],@acc[0],@acc[6]		// U-V
	ldp	@acc[10],@acc[11],[sp,#$V+32]
	sbcs	@acc[1],@acc[1],@acc[7]
	sbcs	@acc[2],@acc[2],@acc[8]
	sbcs	@acc[3],@acc[3],@acc[9]
	sbcs	@acc[4],@acc[4],@acc[10]
	sbcs	@acc[5],@acc[5],@acc[11]
	b.hs	.Lu_greater_than_v

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
	 mvn	@acc[4],@acc[4]
	adcs	@acc[3],@acc[3],xzr
	 mvn	@acc[5],@acc[5]
	adcs	@acc[4],@acc[4],xzr
	adc	@acc[5],@acc[5],xzr

.Lu_greater_than_v:
	 stp	@acc[0],@acc[1],[$ux_ptr]
	ldp	@acc[0],@acc[1],[$vy_ptr,#48]
	ldp	@acc[6],@acc[7],[$ux_ptr,#48]
	 stp	@acc[2],@acc[3],[$ux_ptr,#16]
	ldp	@acc[2],@acc[3],[$vy_ptr,#64]
	subs	@acc[6],@acc[6],@acc[0]		// X-Y		# [alt. Y-X]
	ldp	@acc[8],@acc[9],[$ux_ptr,#64]
	sbcs	@acc[7],@acc[7],@acc[1]
	 stp	@acc[4],@acc[5],[$ux_ptr,#32]
	ldp	@acc[4],@acc[5],[$vy_ptr,#80]
	sbcs	@acc[8],@acc[8],@acc[2]
	ldp	@acc[10],@acc[11],[$ux_ptr,#80]
	sbcs	@acc[9],@acc[9],@acc[3]
	sbcs	@acc[10],@acc[10],@acc[4]
	sbcs	@acc[11],@acc[11],@acc[5]
	sbc	@acc[5],xzr,xzr			// borrow -> mask

	 and	@acc[0],@mod[0],@acc[5]
	 and	@acc[1],@mod[1],@acc[5]
	adds	@acc[6],@acc[6],@acc[0]		// reduce if X<Y # [alt. Y<X]
	 and	@acc[2],@mod[2],@acc[5]
	adcs	@acc[7],@acc[7],@acc[1]
	 and	@acc[3],@mod[3],@acc[5]
	adcs	@acc[8],@acc[8],@acc[2]
	 and	@acc[4],@mod[4],@acc[5]
	adcs	@acc[9],@acc[9],@acc[3]
	 and	@acc[5],@mod[5],@acc[5]
	ldp	@acc[0],@acc[1],[sp,#$U]
	adcs	@acc[10],@acc[10],@acc[4]
	ldp	@acc[2],@acc[3],[sp,#$U+16]
	adc	@acc[11],@acc[11],@acc[5]
	ldp	@acc[4],@acc[5],[sp,#$U+32]

	orr	@acc[0],@acc[0],@acc[1]
	orr	@acc[2],@acc[2],@acc[3]
	orr	@acc[4],@acc[4],@acc[5]
	 stp	@acc[6],@acc[7],[$ux_ptr,#48]
	orr	@acc[0],@acc[0],@acc[2]
	 stp	@acc[8],@acc[9],[$ux_ptr,#64]
	orr	@acc[0],@acc[0],@acc[4]
	 stp	@acc[10],@acc[11],[$ux_ptr,#80]

	cbnz	@acc[0],.Loop_inv		// U!=0?

	ldr	x30,[x29,#8]
	ldp	@acc[0],@acc[1],[sp,#$V+48]	// return Y
	ldp	@acc[2],@acc[3],[sp,#$V+64]
	ldp	@acc[4],@acc[5],[sp,#$V+80]
	mov	@acc[6],#1

.Labort:
	stp	@acc[0],@acc[1],[$r_ptr]
	stp	@acc[2],@acc[3],[$r_ptr,#16]
	stp	@acc[4],@acc[5],[$r_ptr,#32]
	mov	$r_ptr,@acc[6]			// boolean return value

	add	sp,sp,#$frame
	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	autiasp
	ret
.size	eucl_inverse_mod_384,.-eucl_inverse_mod_384

.type	__remove_powers_of_2,%function
.align	4
__remove_powers_of_2:
	ldp	@acc[0],@acc[1],[$ux_ptr]
	ldp	@acc[2],@acc[3],[$ux_ptr,#16]
	ldp	@acc[4],@acc[5],[$ux_ptr,#32]
	nop

.Loop_of_2:
	rbit	$cnt,@acc[0]
	tbnz	$acc[0],#0,.Loop_of_2_done

	clz	$cnt,$cnt
	cmp	@acc[0],#0
	mov	@acc[6],#63
	csel	$cnt,$cnt,@acc[6],ne		// unlikely in real life

	neg	@acc[11],$cnt
	lsrv	@acc[0],@acc[0],$cnt		// acc[0:5] >>= cnt
	lslv	@acc[6],@acc[1],@acc[11]
	orr	@acc[0],@acc[0],@acc[6]
	lsrv	@acc[1],@acc[1],$cnt
	lslv	@acc[7],@acc[2],@acc[11]
	orr	@acc[1],@acc[1],@acc[7]
	 ldp	@acc[6],@acc[7],[$ux_ptr,#48]
	lsrv	@acc[2],@acc[2],$cnt
	lslv	@acc[8],@acc[3],@acc[11]
	orr	@acc[2],@acc[2],@acc[8]
	lsrv	@acc[3],@acc[3],$cnt
	lslv	@acc[9],@acc[4],@acc[11]
	orr	@acc[3],@acc[3],@acc[9]
	 ldp	@acc[8],@acc[9],[$ux_ptr,#64]
	lsrv	@acc[4],@acc[4],$cnt
	lslv	@acc[10],@acc[5],@acc[11]
	orr	@acc[4],@acc[4],@acc[10]
	 ldp	@acc[10],@acc[11],[$ux_ptr,#80]
	lsrv	@acc[5],@acc[5],$cnt

	stp	@acc[0], @acc[1],[$ux_ptr]
	stp	@acc[2], @acc[3],[$ux_ptr,#16]
	stp	@acc[4], @acc[5],[$ux_ptr,#32]
	b	.Loop_div_by_2

.align	4
.Loop_div_by_2:
	sbfx	@acc[5],@acc[6],#0,#1
	sub	$cnt,$cnt,#1

	 and	@acc[0],@mod[0],@acc[5]
	 and	@acc[1],@mod[1],@acc[5]
	adds	@acc[6],@acc[6],@acc[0]
	 and	@acc[2],@mod[2],@acc[5]
	adcs	@acc[7],@acc[7],@acc[1]
	 and	@acc[3],@mod[3],@acc[5]
	adcs	@acc[8],@acc[8],@acc[2]
	 and	@acc[4],@mod[4],@acc[5]
	adcs	@acc[9],@acc[9],@acc[3]
	 and	@acc[5],@mod[5],@acc[5]
	adcs	@acc[10],@acc[10],@acc[4]
	 extr	@acc[6],@acc[7],@acc[6],#1	// acc[6:11] >>= 1
	adcs	@acc[11],@acc[11],@acc[5]
	 extr	@acc[7],@acc[8],@acc[7],#1
	adc	@acc[5],xzr,xzr		// redundant if modulus is <384 bits...
	 extr	@acc[8],@acc[9],@acc[8],#1
	 extr	@acc[9],@acc[10],@acc[9],#1
	 extr	@acc[10],@acc[11],@acc[10],#1
	 extr	@acc[11],@acc[5],@acc[11],#1

	cbnz	$cnt,.Loop_div_by_2

	ldp	@acc[0],@acc[1],[$ux_ptr] // reload X [mostly for 2nd caller]
	ldp	@acc[2],@acc[3],[$ux_ptr,#16]
	ldp	@acc[4],@acc[5],[$ux_ptr,#32]

	stp	@acc[6],@acc[7],[$ux_ptr,#48]
	stp	@acc[8],@acc[9],[$ux_ptr,#64]
	stp	@acc[10],@acc[11],[$ux_ptr,#80]

	tbz	@acc[0],#0,.Loop_of_2	// unlikely in real life

.Loop_of_2_done:
	ret
.size	__remove_powers_of_2,.-__remove_powers_of_2
___

print $code;
close STDOUT;
