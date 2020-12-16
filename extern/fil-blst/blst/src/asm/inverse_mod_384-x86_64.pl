#!/usr/bin/env perl
#
# Copyright Supranational LLC
# Licensed under the Apache License, Version 2.0, see LICENSE for details.
# SPDX-License-Identifier: Apache-2.0
#
# ~3x(*) faster than dedicated modulus-specific constant-time addition
# chain, but as implied, at the cost of constant-time-ness.
# Caveat lector! Benchmarking results can be deceptive, as timings vary
# wildly from input to input. And that is depending both on set bits'
# relative positions in input and modulus, and ability of branch
# prediction logic to adapt for a specific workflow. Right thing to do
# is to benchmark with series of random inputs.
#
# (*)	"up to ~3x" actually, as on adcx/adox-capable processor the
#	ratio is lower, less than 2x, because 384-bit multiplications
#	are relatively faster;
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

$win64=0; $win64=1 if ($flavour =~ /[nm]asm|mingw64/ || $output =~ /\.asm$/);

$0 =~ m/(.*[\/\\])[^\/\\]+$/; $dir=$1;
( $xlate="${dir}x86_64-xlate.pl" and -f $xlate ) or
( $xlate="${dir}../../perlasm/x86_64-xlate.pl" and -f $xlate) or
die "can't locate x86_64-xlate.pl";

open STDOUT,"| \"$^X\" \"$xlate\" $flavour \"$output\""
    or die "can't call $xlate: $!";

($r_ptr, $a_ptr, $n_ptr, $one) = ("%rdi","%rsi","%rdx","%rcx");
@acc=(map("%r$_",(8..15)),"%rax","%rbx","%rbp",$r_ptr);
($ux_ptr, $vy_ptr) = ($a_ptr, $one);

$frame=8*3+4*384/8;
$U=16;
$X=$U+384/8;
$V=$X+384/8;
$Y=$V+384/8;

$code.=<<___;
.text

.align	32
.Lone:
	.quad	1,0,0,0,0,0,0,0

.globl	eucl_inverse_mod_384
.hidden	eucl_inverse_mod_384
.type	eucl_inverse_mod_384,\@function,4,"unwind"
.align	32
eucl_inverse_mod_384:
.cfi_startproc
	push	%rbp
.cfi_push	%rbp
	push	%rbx
.cfi_push	%rbx
	push	%r12
.cfi_push	%r12
	push	%r13
.cfi_push	%r13
	push	%r14
.cfi_push	%r14
	push	%r15
.cfi_push	%r15
	sub	\$$frame, %rsp
.cfi_adjust_cfa_offset	$frame
.cfi_end_prologue

	mov	$r_ptr, 8*0(%rsp)
	lea	.Lone(%rip), %rbp
	cmp	\$0, $one
	cmove	%rbp, $one		# default $one to 1

	mov	8*0($a_ptr), %rax
	mov	8*1($a_ptr), @acc[1]
	mov	8*2($a_ptr), @acc[2]
	mov	8*3($a_ptr), @acc[3]
	mov	8*4($a_ptr), @acc[4]
	mov	8*5($a_ptr), @acc[5]

	mov	%rax, @acc[0]
	or	@acc[1], %rax
	or	@acc[2], %rax
	or	@acc[3], %rax
	or	@acc[4], %rax
	or	@acc[5], %rax
	jz	.Labort			# abort if |inp|==0

	lea	$U(%rsp), $ux_ptr
	mov	8*0($one), @acc[6]
	mov	8*1($one), @acc[7]
	mov	8*2($one), @acc[8]
	mov	8*3($one), @acc[9]
	mov	8*4($one), @acc[10]
	mov	8*5($one), @acc[11]

	mov	@acc[0], 8*0($ux_ptr)	# copy |inp| to U
	mov	@acc[1], 8*1($ux_ptr)
	mov	@acc[2], 8*2($ux_ptr)
	mov	@acc[3], 8*3($ux_ptr)
	mov	@acc[4], 8*4($ux_ptr)
	mov	@acc[5], 8*5($ux_ptr)

	lea	$V(%rsp), $vy_ptr
	mov	8*0($n_ptr), @acc[0]
	mov	8*1($n_ptr), @acc[1]
	mov	8*2($n_ptr), @acc[2]
	mov	8*3($n_ptr), @acc[3]
	mov	8*4($n_ptr), @acc[4]
	mov	8*5($n_ptr), @acc[5]

	mov	@acc[6], 8*6($ux_ptr)	# copy |one| to X
	mov	@acc[7], 8*7($ux_ptr)
	mov	@acc[8], 8*8($ux_ptr)
	mov	@acc[9], 8*9($ux_ptr)
	mov	@acc[10], 8*10($ux_ptr)
	mov	@acc[11], 8*11($ux_ptr)

	mov	@acc[0], 8*0($vy_ptr)	# copy |mod| to V
	mov	@acc[1], 8*1($vy_ptr)
	mov	@acc[2], 8*2($vy_ptr)
	mov	@acc[3], 8*3($vy_ptr)
	mov	@acc[4], 8*4($vy_ptr)
	mov	@acc[5], 8*5($vy_ptr)

	xor	%eax, %eax
	mov	%rax, 8*6($vy_ptr)	# clear Y
	mov	%rax, 8*7($vy_ptr)
	mov	%rax, 8*8($vy_ptr)
	mov	%rax, 8*9($vy_ptr)
	mov	%rax, 8*10($vy_ptr)
	mov	%rax, 8*11($vy_ptr)
	jmp	.Loop_inv

.align	32
.Loop_inv:
	lea	$V(%rsp), $ux_ptr
	call	__remove_powers_of_2

	lea	$U(%rsp), $ux_ptr
	call	__remove_powers_of_2

	lea	$V(%rsp), $vy_ptr
	sub	$V+8*0(%rsp), @acc[0]	# U-V
	sbb	8*1($vy_ptr), @acc[1]
	sbb	8*2($vy_ptr), @acc[2]
	sbb	8*3($vy_ptr), @acc[3]
	sbb	8*4($vy_ptr), @acc[4]
	sbb	8*5($vy_ptr), @acc[5]
	jae	.Lu_greater_than_v	# conditional pointers' swap
					# doesn't help [performance
					# with random inputs]
	xchg	$vy_ptr, $ux_ptr

	not	@acc[0]			# U-V => V-U
	not	@acc[1]
	not	@acc[2]
	not	@acc[3]
	not	@acc[4]
	not	@acc[5]

	add	\$1, @acc[0]
	adc	\$0, @acc[1]
	adc	\$0, @acc[2]
	adc	\$0, @acc[3]
	adc	\$0, @acc[4]
	adc	\$0, @acc[5]

.Lu_greater_than_v:
	mov	8*6($ux_ptr), @acc[6]
	mov	8*7($ux_ptr), @acc[7]
	mov	8*8($ux_ptr), @acc[8]
	mov	8*9($ux_ptr), @acc[9]
	mov	8*10($ux_ptr), @acc[10]
	mov	8*11($ux_ptr), @acc[11]

	sub	8*6($vy_ptr), @acc[6]	# X-Y		# [alt. Y-X]
	sbb	8*7($vy_ptr), @acc[7]
	sbb	8*8($vy_ptr), @acc[8]
	sbb	8*9($vy_ptr), @acc[9]
	sbb	8*10($vy_ptr), @acc[10]
	sbb	8*11($vy_ptr), @acc[11]

	mov	@acc[0], 8*0($ux_ptr)
	sbb	@acc[0], @acc[0]	# borrow -> mask
	mov	@acc[1], 8*1($ux_ptr)
	mov	@acc[0], @acc[1]
	mov	@acc[2], 8*2($ux_ptr)
	mov	@acc[0], @acc[2]
	mov	@acc[3], 8*3($ux_ptr)
	mov	@acc[0], @acc[3]
	mov	@acc[4], 8*4($ux_ptr)
	mov	@acc[0], @acc[4]
	mov	@acc[5], 8*5($ux_ptr)
	mov	@acc[0], @acc[5]

	and	8*0($n_ptr), @acc[0]
	and	8*1($n_ptr), @acc[1]
	and	8*2($n_ptr), @acc[2]
	and	8*3($n_ptr), @acc[3]
	and	8*4($n_ptr), @acc[4]
	and	8*5($n_ptr), @acc[5]

	add	@acc[0], @acc[6]	# reduce if X<Y # [alt. Y<X]
	adc	@acc[1], @acc[7]
	adc	@acc[2], @acc[8]
	adc	@acc[3], @acc[9]
	adc	@acc[4], @acc[10]
	adc	@acc[5], @acc[11]

	mov	@acc[6], 8*6($ux_ptr)
	mov	@acc[7], 8*7($ux_ptr)
	mov	@acc[8], 8*8($ux_ptr)
	mov	@acc[9], 8*9($ux_ptr)
	mov	@acc[10], 8*10($ux_ptr)
	mov	@acc[11], 8*11($ux_ptr)

	mov	$U+8*0(%rsp), @acc[0]
	mov	$U+8*1(%rsp), @acc[1]
	mov	$U+8*2(%rsp), @acc[2]
	mov	$U+8*3(%rsp), @acc[3]
	or	@acc[1], @acc[0]
	or	$U+8*4(%rsp), @acc[2]
	or	$U+8*5(%rsp), @acc[3]
	.byte	0x67
	or	@acc[2], @acc[0]
	or	@acc[3], @acc[0]
	jnz	.Loop_inv		# U!=0?

	lea	$V(%rsp), $ux_ptr	# return Y
	mov	8*0(%rsp), $r_ptr
	mov	\$1,%eax		# return value

	mov	8*6($ux_ptr), @acc[0]
	mov	8*7($ux_ptr), @acc[1]
	mov	8*8($ux_ptr), @acc[2]
	mov	8*9($ux_ptr), @acc[3]
	mov	8*10($ux_ptr), @acc[4]
	mov	8*11($ux_ptr), @acc[5]

.Labort:
	mov	@acc[0], 8*0($r_ptr)
	mov	@acc[1], 8*1($r_ptr)
	mov	@acc[2], 8*2($r_ptr)
	mov	@acc[3], 8*3($r_ptr)
	mov	@acc[4], 8*4($r_ptr)
	mov	@acc[5], 8*5($r_ptr)

	lea	$frame(%rsp), %r8	# size optimization
	mov	8*0(%r8),%r15
.cfi_restore	%r15
	mov	8*1(%r8),%r14
.cfi_restore	%r14
	mov	8*2(%r8),%r13
.cfi_restore	%r13
	mov	8*3(%r8),%r12
.cfi_restore	%r12
	mov	8*4(%r8),%rbx
.cfi_restore	%rbx
	mov	8*5(%r8),%rbp
.cfi_restore	%rbp
	lea	8*6(%r8),%rsp
.cfi_adjust_cfa_offset	-$frame-8*6
.cfi_epilogue
	ret
.cfi_endproc
.size	eucl_inverse_mod_384,.-eucl_inverse_mod_384

.type	__remove_powers_of_2,\@abi-omnipotent
.align	32
__remove_powers_of_2:
	mov	8*0($ux_ptr), @acc[0]
	mov	8*1($ux_ptr), @acc[1]
	mov	8*2($ux_ptr), @acc[2]
	mov	8*3($ux_ptr), @acc[3]
	mov	8*4($ux_ptr), @acc[4]
	mov	8*5($ux_ptr), @acc[5]

.Loop_of_2:
	bsf	@acc[0], %rcx
	mov	\$63, %eax
	cmovz	%eax, %ecx		# unlikely in real life

	cmp	\$0, %ecx
	je	.Loop_of_2_done

	shrq	%cl, @acc[0]		# why not shrdq? while it provides
	mov	@acc[1], @acc[6]	# 6-9% improvement on Intel Core iN
	shrq	%cl, @acc[1]		# processors, it's significantly
	mov	@acc[2], @acc[7]	# worse on all others...
	shrq	%cl, @acc[2]
	mov	@acc[3], @acc[8]
	shrq	%cl, @acc[3]
	mov	@acc[4], @acc[9]
	shrq	%cl, @acc[4]
	mov	@acc[5], @acc[10]
	shrq	%cl, @acc[5]
	neg	%cl
	shlq	%cl, @acc[6]
	shlq	%cl, @acc[7]
	or	@acc[6], @acc[0]
	 mov	8*6($ux_ptr), @acc[6]
	shlq	%cl, @acc[8]
	or	@acc[7], @acc[1]
	 mov	8*7($ux_ptr), @acc[7]
	shlq	%cl, @acc[9]
	or	@acc[8], @acc[2]
	 mov	8*8($ux_ptr), @acc[8]
	shlq	%cl, @acc[10]
	or	@acc[9], @acc[3]
	 mov	8*9($ux_ptr), @acc[9]
	or	@acc[10], @acc[4]
	 mov	8*10($ux_ptr), @acc[10]
	neg	%cl
	 mov	8*11($ux_ptr), @acc[11]

	mov	@acc[0], 8*0($ux_ptr)
	mov	@acc[1], 8*1($ux_ptr)
	mov	@acc[2], 8*2($ux_ptr)
	mov	@acc[3], 8*3($ux_ptr)
	mov	@acc[4], 8*4($ux_ptr)
	mov	@acc[5], 8*5($ux_ptr)
	jmp	.Loop_div_by_2

.align	32
.Loop_div_by_2:
	mov	\$1, @acc[5]
	mov	8*0($n_ptr), @acc[0]	# conditional addition gives ~10%
	and	@acc[6], @acc[5]	# improvement over branch for
	mov	8*1($n_ptr), @acc[1]	# random inputs...
	neg	@acc[5]
	mov	8*2($n_ptr), @acc[2]
	and	@acc[5], @acc[0]
	mov	8*3($n_ptr), @acc[3]
	and	@acc[5], @acc[1]
	mov	8*4($n_ptr), @acc[4]
	and	@acc[5], @acc[2]
	and	@acc[5], @acc[3]
	and	@acc[5], @acc[4]
	and	8*5($n_ptr), @acc[5]

	add	@acc[0], @acc[6]
	adc	@acc[1], @acc[7]
	adc	@acc[2], @acc[8]
	adc	@acc[3], @acc[9]
	adc	@acc[4], @acc[10]
	adc	@acc[5], @acc[11]
	sbb	@acc[5], @acc[5]	# redundant if modulus is <384 bits...

	shr	\$1, @acc[6]
	mov	@acc[7], @acc[0]
	shr	\$1, @acc[7]
	mov	@acc[8], @acc[1]
	shr	\$1, @acc[8]
	mov	@acc[9], @acc[2]
	shr	\$1, @acc[9]
	mov	@acc[10], @acc[3]
	shr	\$1, @acc[10]
	mov	@acc[11], @acc[4]
	shr	\$1, @acc[11]
	shl	\$63, @acc[0]
	shl	\$63, @acc[1]
	or	@acc[0], @acc[6]
	shl	\$63, @acc[2]
	or	@acc[1], @acc[7]
	shl	\$63, @acc[3]
	or	@acc[2], @acc[8]
	shl	\$63, @acc[4]
	or	@acc[3], @acc[9]
	shl	\$63, @acc[5]
	or	@acc[4], @acc[10]
	or	@acc[5], @acc[11]

	dec	%ecx
	jnz	.Loop_div_by_2

	mov	8*0($ux_ptr), @acc[0]	# reload X [mostly for 2nd caller]
	mov	8*1($ux_ptr), @acc[1]
	mov	8*2($ux_ptr), @acc[2]
	mov	8*3($ux_ptr), @acc[3]
	mov	8*4($ux_ptr), @acc[4]
	mov	8*5($ux_ptr), @acc[5]

	mov	@acc[6], 8*6($ux_ptr)
	mov	@acc[7], 8*7($ux_ptr)
	mov	@acc[8], 8*8($ux_ptr)
	mov	@acc[9], 8*9($ux_ptr)
	mov	@acc[10], 8*10($ux_ptr)
	mov	@acc[11], 8*11($ux_ptr)

	test	\$1, $acc[0]
	.byte	0x2e			# predict non-taken
	jz	.Loop_of_2

.Loop_of_2_done:
	ret
.size	__remove_powers_of_2,.-__remove_powers_of_2
___

print $code;
close STDOUT;
