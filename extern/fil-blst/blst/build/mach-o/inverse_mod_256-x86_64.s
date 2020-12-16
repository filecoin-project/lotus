.text	

.p2align	5
L$one_256:
.quad	1,0,0,0

.globl	_eucl_inverse_mod_256
.private_extern	_eucl_inverse_mod_256

.p2align	5
_eucl_inverse_mod_256:
.cfi_startproc
	.byte	0xf3,0x0f,0x1e,0xfa


	pushq	%rbp
.cfi_adjust_cfa_offset	8
.cfi_offset	%rbp,-16
	pushq	%rbx
.cfi_adjust_cfa_offset	8
.cfi_offset	%rbx,-24
	subq	$152,%rsp
.cfi_adjust_cfa_offset	152


	movq	%rdi,0(%rsp)
	leaq	L$one_256(%rip),%rbp
	cmpq	$0,%rcx
	cmoveq	%rbp,%rcx

	movq	0(%rsi),%rax
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11

	movq	%rax,%r8
	orq	%r9,%rax
	orq	%r10,%rax
	orq	%r11,%rax
	jz	L$abort_256

	leaq	16(%rsp),%rsi
	movq	0(%rcx),%rax
	movq	8(%rcx),%rbx
	movq	16(%rcx),%rbp
	movq	24(%rcx),%rdi

	movq	%r8,0(%rsi)
	movq	%r9,8(%rsi)
	movq	%r10,16(%rsi)
	movq	%r11,24(%rsi)

	leaq	80(%rsp),%rcx
	movq	0(%rdx),%r8
	movq	8(%rdx),%r9
	movq	16(%rdx),%r10
	movq	24(%rdx),%r11

	movq	%rax,32(%rsi)
	movq	%rbx,40(%rsi)
	movq	%rbp,48(%rsi)
	movq	%rdi,56(%rsi)

	movq	%r8,0(%rcx)
	movq	%r9,8(%rcx)
	movq	%r10,16(%rcx)
	movq	%r11,24(%rcx)

	xorl	%eax,%eax
	movq	%rax,32(%rcx)
	movq	%rax,40(%rcx)
	movq	%rax,48(%rcx)
	movq	%rax,56(%rcx)
	jmp	L$oop_inv_256

.p2align	5
L$oop_inv_256:
	leaq	80(%rsp),%rsi
	call	__remove_powers_of_2_256

	leaq	16(%rsp),%rsi
	call	__remove_powers_of_2_256

	leaq	80(%rsp),%rcx
	subq	80+0(%rsp),%r8
	sbbq	8(%rcx),%r9
	sbbq	16(%rcx),%r10
	sbbq	24(%rcx),%r11
	jae	L$u_greater_than_v_256


	xchgq	%rcx,%rsi

	notq	%r8
	notq	%r9
	notq	%r10
	notq	%r11

	addq	$1,%r8
	adcq	$0,%r9
	adcq	$0,%r10
	adcq	$0,%r11

L$u_greater_than_v_256:
	movq	32(%rsi),%rax
	movq	40(%rsi),%rbx
	movq	48(%rsi),%rbp
	movq	56(%rsi),%rdi

	subq	32(%rcx),%rax
	sbbq	40(%rcx),%rbx
	sbbq	48(%rcx),%rbp
	sbbq	56(%rcx),%rdi

	movq	%r8,0(%rsi)
	sbbq	%r8,%r8
	movq	%r9,8(%rsi)
	movq	%r8,%r9
	movq	%r10,16(%rsi)
	movq	%r8,%r10
	movq	%r11,24(%rsi)
	movq	%r8,%r11

	andq	0(%rdx),%r8
	andq	8(%rdx),%r9
	andq	16(%rdx),%r10
	andq	24(%rdx),%r11

	addq	%r8,%rax
	adcq	%r9,%rbx
	adcq	%r10,%rbp
	adcq	%r11,%rdi

	movq	%rax,32(%rsi)
	movq	%rbx,40(%rsi)
	movq	%rbp,48(%rsi)
	movq	%rdi,56(%rsi)

	movq	16+0(%rsp),%r8
	movq	16+8(%rsp),%r9
	movq	16+16(%rsp),%r10
	movq	16+24(%rsp),%r11
	orq	%r9,%r8
	orq	%r10,%r8
	orq	%r11,%r8
	jnz	L$oop_inv_256

	leaq	80(%rsp),%rsi
	movq	0(%rsp),%rdi
	movl	$1,%eax

	movq	32(%rsi),%r8
	movq	40(%rsi),%r9
	movq	48(%rsi),%r10
	movq	56(%rsi),%r11

L$abort_256:
	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)

	leaq	152(%rsp),%r8
	movq	0(%r8),%rbx
.cfi_restore	%rbx
	movq	8(%r8),%rbp
.cfi_restore	%rbp
	leaq	16(%r8),%rsp
.cfi_adjust_cfa_offset	-152-8*2

	.byte	0xf3,0xc3
.cfi_endproc	



.p2align	5
__remove_powers_of_2_256:
.cfi_startproc
	.byte	0xf3,0x0f,0x1e,0xfa

	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11

L$oop_of_2_256:
	bsfq	%r8,%rcx
	movl	$63,%eax
	cmovzl	%eax,%ecx

	cmpl	$0,%ecx
	je	L$oop_of_2_done_256

	shrq	%cl,%r8
	movq	%r9,%rax
	shrq	%cl,%r9
	movq	%r10,%rbx
	shrq	%cl,%r10
	movq	%r11,%rbp
	shrq	%cl,%r11
	negb	%cl
	shlq	%cl,%rax
	shlq	%cl,%rbx
	orq	%rax,%r8
	movq	32(%rsi),%rax
	shlq	%cl,%rbp
	orq	%rbx,%r9
	movq	40(%rsi),%rbx
	orq	%rbp,%r10
	movq	48(%rsi),%rbp
	negb	%cl
	movq	56(%rsi),%rdi

	movq	%r8,0(%rsi)
	movq	%r9,8(%rsi)
	movq	%r10,16(%rsi)
	movq	%r11,24(%rsi)
	jmp	L$oop_div_by_2_256

.p2align	5
L$oop_div_by_2_256:
	movq	$1,%r11
	movq	0(%rdx),%r8
	andq	%rax,%r11
	movq	8(%rdx),%r9
	negq	%r11
	movq	16(%rdx),%r10
	andq	%r11,%r8
	andq	%r11,%r9
	andq	%r11,%r10
	andq	24(%rdx),%r11

	addq	%r8,%rax
	adcq	%r9,%rbx
	adcq	%r10,%rbp
	adcq	%r11,%rdi
	sbbq	%r11,%r11

	shrq	$1,%rax
	movq	%rbx,%r8
	shrq	$1,%rbx
	movq	%rbp,%r9
	shrq	$1,%rbp
	movq	%rdi,%r10
	shrq	$1,%rdi
	shlq	$63,%r8
	shlq	$63,%r9
	orq	%r8,%rax
	shlq	$63,%r10
	orq	%r9,%rbx
	shlq	$63,%r11
	orq	%r10,%rbp
	orq	%r11,%rdi

	decl	%ecx
	jnz	L$oop_div_by_2_256

	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11

	movq	%rax,32(%rsi)
	movq	%rbx,40(%rsi)
	movq	%rbp,48(%rsi)
	movq	%rdi,56(%rsi)

	testq	$1,%r8
.byte	0x2e
	jz	L$oop_of_2_256

L$oop_of_2_done_256:
	.byte	0xf3,0xc3
.cfi_endproc

