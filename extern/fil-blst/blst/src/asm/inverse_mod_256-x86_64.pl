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

$win64=0; $win64=1 if ($flavour =~ /[nm]asm|mingw64/ || $output =~ /\.asm$/);

$0 =~ m/(.*[\/\\])[^\/\\]+$/; $dir=$1;
( $xlate="${dir}x86_64-xlate.pl" and -f $xlate ) or
( $xlate="${dir}../../perlasm/x86_64-xlate.pl" and -f $xlate) or
die "can't locate x86_64-xlate.pl";

open STDOUT,"| \"$^X\" \"$xlate\" $flavour \"$output\""
    or die "can't call $xlate: $!";

($r_ptr, $a_ptr, $n_ptr, $one) = ("%rdi","%rsi","%rdx","%rcx");
@acc=(map("%r$_",(8..11)),"%rax","%rbx","%rbp",$r_ptr);
($ux_ptr, $vy_ptr) = ($a_ptr, $one);

$frame=8*3+4*256/8;
$U=16;
$X=$U+256/8;
$V=$X+256/8;
$Y=$V+256/8;

$code.=<<___;
.text

.align	32
.Lone_256:
	.quad	1,0,0,0

.globl	eucl_inverse_mod_256
.hidden	eucl_inverse_mod_256
.type	eucl_inverse_mod_256,\@function,4,"unwind"
.align	32
eucl_inverse_mod_256:
.cfi_startproc
	push	%rbp
.cfi_push	%rbp
	push	%rbx
.cfi_push	%rbx
	sub	\$$frame, %rsp
.cfi_adjust_cfa_offset	$frame
.cfi_end_prologue

	mov	$r_ptr, 8*0(%rsp)
	lea	.Lone_256(%rip), %rbp
	cmp	\$0, $one
	cmove	%rbp, $one		# default $one to 1

	mov	8*0($a_ptr), %rax
	mov	8*1($a_ptr), @acc[1]
	mov	8*2($a_ptr), @acc[2]
	mov	8*3($a_ptr), @acc[3]

	mov	%rax, @acc[0]
	or	@acc[1], %rax
	or	@acc[2], %rax
	or	@acc[3], %rax
	jz	.Labort_256		# abort if |inp|==0

	lea	$U(%rsp), $ux_ptr
	mov	8*0($one), @acc[4]
	mov	8*1($one), @acc[5]
	mov	8*2($one), @acc[6]
	mov	8*3($one), @acc[7]

	mov	@acc[0], 8*0($ux_ptr)	# copy |inp| to U
	mov	@acc[1], 8*1($ux_ptr)
	mov	@acc[2], 8*2($ux_ptr)
	mov	@acc[3], 8*3($ux_ptr)

	lea	$V(%rsp), $vy_ptr
	mov	8*0($n_ptr), @acc[0]
	mov	8*1($n_ptr), @acc[1]
	mov	8*2($n_ptr), @acc[2]
	mov	8*3($n_ptr), @acc[3]

	mov	@acc[4], 8*4($ux_ptr)	# copy |one| to X
	mov	@acc[5], 8*5($ux_ptr)
	mov	@acc[6], 8*6($ux_ptr)
	mov	@acc[7], 8*7($ux_ptr)

	mov	@acc[0], 8*0($vy_ptr)	# copy |mod| to V
	mov	@acc[1], 8*1($vy_ptr)
	mov	@acc[2], 8*2($vy_ptr)
	mov	@acc[3], 8*3($vy_ptr)

	xor	%eax, %eax
	mov	%rax, 8*4($vy_ptr)	# clear Y
	mov	%rax, 8*5($vy_ptr)
	mov	%rax, 8*6($vy_ptr)
	mov	%rax, 8*7($vy_ptr)
	jmp	.Loop_inv_256

.align	32
.Loop_inv_256:
	lea	$V(%rsp), $ux_ptr
	call	__remove_powers_of_2_256

	lea	$U(%rsp), $ux_ptr
	call	__remove_powers_of_2_256

	lea	$V(%rsp), $vy_ptr
	sub	$V+8*0(%rsp), @acc[0]	# U-V
	sbb	8*1($vy_ptr), @acc[1]
	sbb	8*2($vy_ptr), @acc[2]
	sbb	8*3($vy_ptr), @acc[3]
	jae	.Lu_greater_than_v_256	# conditional pointers' swap
					# doesn't help [performance
					# with random inputs]
	xchg	$vy_ptr, $ux_ptr

	not	@acc[0]			# U-V => V-U
	not	@acc[1]
	not	@acc[2]
	not	@acc[3]

	add	\$1, @acc[0]
	adc	\$0, @acc[1]
	adc	\$0, @acc[2]
	adc	\$0, @acc[3]

.Lu_greater_than_v_256:
	mov	8*4($ux_ptr), @acc[4]
	mov	8*5($ux_ptr), @acc[5]
	mov	8*6($ux_ptr), @acc[6]
	mov	8*7($ux_ptr), @acc[7]

	sub	8*4($vy_ptr), @acc[4]	# X-Y		# [alt. Y-X]
	sbb	8*5($vy_ptr), @acc[5]
	sbb	8*6($vy_ptr), @acc[6]
	sbb	8*7($vy_ptr), @acc[7]

	mov	@acc[0], 8*0($ux_ptr)
	sbb	@acc[0], @acc[0]	# borrow -> mask
	mov	@acc[1], 8*1($ux_ptr)
	mov	@acc[0], @acc[1]
	mov	@acc[2], 8*2($ux_ptr)
	mov	@acc[0], @acc[2]
	mov	@acc[3], 8*3($ux_ptr)
	mov	@acc[0], @acc[3]

	and	8*0($n_ptr), @acc[0]
	and	8*1($n_ptr), @acc[1]
	and	8*2($n_ptr), @acc[2]
	and	8*3($n_ptr), @acc[3]

	add	@acc[0], @acc[4]	# reduce if X<Y # [alt. Y<X]
	adc	@acc[1], @acc[5]
	adc	@acc[2], @acc[6]
	adc	@acc[3], @acc[7]

	mov	@acc[4], 8*4($ux_ptr)
	mov	@acc[5], 8*5($ux_ptr)
	mov	@acc[6], 8*6($ux_ptr)
	mov	@acc[7], 8*7($ux_ptr)

	mov	$U+8*0(%rsp), @acc[0]
	mov	$U+8*1(%rsp), @acc[1]
	mov	$U+8*2(%rsp), @acc[2]
	mov	$U+8*3(%rsp), @acc[3]
	or	@acc[1], @acc[0]
	or	@acc[2], @acc[0]
	or	@acc[3], @acc[0]
	jnz	.Loop_inv_256		# U!=0?

	lea	$V(%rsp), $ux_ptr	# return Y
	mov	8*0(%rsp), $r_ptr
	mov	\$1,%eax		# return value

	mov	8*4($ux_ptr), @acc[0]
	mov	8*5($ux_ptr), @acc[1]
	mov	8*6($ux_ptr), @acc[2]
	mov	8*7($ux_ptr), @acc[3]

.Labort_256:
	mov	@acc[0], 8*0($r_ptr)
	mov	@acc[1], 8*1($r_ptr)
	mov	@acc[2], 8*2($r_ptr)
	mov	@acc[3], 8*3($r_ptr)

	lea	$frame(%rsp), %r8	# size optimization
	mov	8*0(%r8),%rbx
.cfi_restore	%rbx
	mov	8*1(%r8),%rbp
.cfi_restore	%rbp
	lea	8*2(%r8),%rsp
.cfi_adjust_cfa_offset	-$frame-8*2
.cfi_epilogue
	ret
.cfi_endproc
.size	eucl_inverse_mod_256,.-eucl_inverse_mod_256

.type	__remove_powers_of_2_256,\@abi-omnipotent
.align	32
__remove_powers_of_2_256:
	mov	8*0($ux_ptr), @acc[0]
	mov	8*1($ux_ptr), @acc[1]
	mov	8*2($ux_ptr), @acc[2]
	mov	8*3($ux_ptr), @acc[3]

.Loop_of_2_256:
	bsf	@acc[0], %rcx
	mov	\$63, %eax
	cmovz	%eax, %ecx		# unlikely in real life

	cmp	\$0, %ecx
	je	.Loop_of_2_done_256

	shrq	%cl, @acc[0]		# why not shrdq? while it provides
	mov	@acc[1], @acc[4]	# 6-9% improvement on Intel Core iN
	shrq	%cl, @acc[1]		# processors, it's significantly
	mov	@acc[2], @acc[5]	# worse on all others...
	shrq	%cl, @acc[2]
	mov	@acc[3], @acc[6]
	shrq	%cl, @acc[3]
	neg	%cl
	shlq	%cl, @acc[4]
	shlq	%cl, @acc[5]
	or	@acc[4], @acc[0]
	 mov	8*4($ux_ptr), @acc[4]
	shlq	%cl, @acc[6]
	or	@acc[5], @acc[1]
	 mov	8*5($ux_ptr), @acc[5]
	or	@acc[6], @acc[2]
	 mov	8*6($ux_ptr), @acc[6]
	neg	%cl
	 mov	8*7($ux_ptr), @acc[7]

	mov	@acc[0], 8*0($ux_ptr)
	mov	@acc[1], 8*1($ux_ptr)
	mov	@acc[2], 8*2($ux_ptr)
	mov	@acc[3], 8*3($ux_ptr)
	jmp	.Loop_div_by_2_256

.align	32
.Loop_div_by_2_256:
	mov	\$1, @acc[3]
	mov	8*0($n_ptr), @acc[0]	# conditional addition gives ~10%
	and	@acc[4], @acc[3]	# improvement over branch for
	mov	8*1($n_ptr), @acc[1]	# random inputs...
	neg	@acc[3]
	mov	8*2($n_ptr), @acc[2]
	and	@acc[3], @acc[0]
	and	@acc[3], @acc[1]
	and	@acc[3], @acc[2]
	and	8*3($n_ptr), @acc[3]

	add	@acc[0], @acc[4]
	adc	@acc[1], @acc[5]
	adc	@acc[2], @acc[6]
	adc	@acc[3], @acc[7]
	sbb	@acc[3], @acc[3]	# redundant if modulus is <256 bits...

	shr	\$1, @acc[4]
	mov	@acc[5], @acc[0]
	shr	\$1, @acc[5]
	mov	@acc[6], @acc[1]
	shr	\$1, @acc[6]
	mov	@acc[7], @acc[2]
	shr	\$1, @acc[7]
	shl	\$63, @acc[0]
	shl	\$63, @acc[1]
	or	@acc[0], @acc[4]
	shl	\$63, @acc[2]
	or	@acc[1], @acc[5]
	shl	\$63, @acc[3]
	or	@acc[2], @acc[6]
	or	@acc[3], @acc[7]

	dec	%ecx
	jnz	.Loop_div_by_2_256

	mov	8*0($ux_ptr), @acc[0]	# reload X [mostly for 2nd caller]
	mov	8*1($ux_ptr), @acc[1]
	mov	8*2($ux_ptr), @acc[2]
	mov	8*3($ux_ptr), @acc[3]

	mov	@acc[4], 8*4($ux_ptr)
	mov	@acc[5], 8*5($ux_ptr)
	mov	@acc[6], 8*6($ux_ptr)
	mov	@acc[7], 8*7($ux_ptr)

	test	\$1, $acc[0]
	.byte	0x2e			# predict non-taken
	jz	.Loop_of_2_256

.Loop_of_2_done_256:
	ret
.size	__remove_powers_of_2_256,.-__remove_powers_of_2_256
___

print $code;
close STDOUT;
