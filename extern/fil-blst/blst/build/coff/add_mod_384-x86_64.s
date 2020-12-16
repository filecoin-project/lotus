.text	



.globl	add_mod_384

.def	add_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
add_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_add_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx
	movq	%r9,%rcx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$8,%rsp

.LSEH_body_add_mod_384:


	call	__add_mod_384

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_add_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_add_mod_384:

.def	__add_mod_384;	.scl 3;	.type 32;	.endef
.p2align	5
__add_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa

	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13

__add_mod_384_a_is_loaded:
	addq	0(%rdx),%r8
	adcq	8(%rdx),%r9
	adcq	16(%rdx),%r10
	movq	%r8,%r14
	adcq	24(%rdx),%r11
	movq	%r9,%r15
	adcq	32(%rdx),%r12
	movq	%r10,%rax
	adcq	40(%rdx),%r13
	movq	%r11,%rbx
	sbbq	%rdx,%rdx

	subq	0(%rcx),%r8
	sbbq	8(%rcx),%r9
	movq	%r12,%rbp
	sbbq	16(%rcx),%r10
	sbbq	24(%rcx),%r11
	sbbq	32(%rcx),%r12
	movq	%r13,%rsi
	sbbq	40(%rcx),%r13
	sbbq	$0,%rdx

	cmovcq	%r14,%r8
	cmovcq	%r15,%r9
	cmovcq	%rax,%r10
	movq	%r8,0(%rdi)
	cmovcq	%rbx,%r11
	movq	%r9,8(%rdi)
	cmovcq	%rbp,%r12
	movq	%r10,16(%rdi)
	cmovcq	%rsi,%r13
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	.byte	0xf3,0xc3


.globl	add_mod_384x

.def	add_mod_384x;	.scl 2;	.type 32;	.endef
.p2align	5
add_mod_384x:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_add_mod_384x:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx
	movq	%r9,%rcx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$24,%rsp

.LSEH_body_add_mod_384x:


	movq	%rsi,0(%rsp)
	movq	%rdx,8(%rsp)
	leaq	48(%rsi),%rsi
	leaq	48(%rdx),%rdx
	leaq	48(%rdi),%rdi
	call	__add_mod_384

	movq	0(%rsp),%rsi
	movq	8(%rsp),%rdx
	leaq	-48(%rdi),%rdi
	call	__add_mod_384

	movq	24+0(%rsp),%r15

	movq	24+8(%rsp),%r14

	movq	24+16(%rsp),%r13

	movq	24+24(%rsp),%r12

	movq	24+32(%rsp),%rbx

	movq	24+40(%rsp),%rbp

	leaq	24+48(%rsp),%rsp

.LSEH_epilogue_add_mod_384x:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_add_mod_384x:


.globl	lshift_mod_384

.def	lshift_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
lshift_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_lshift_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx
	movq	%r9,%rcx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rdi

.LSEH_body_lshift_mod_384:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13

.Loop_lshift_mod_384:
	addq	%r8,%r8
	adcq	%r9,%r9
	adcq	%r10,%r10
	movq	%r8,%r14
	adcq	%r11,%r11
	movq	%r9,%r15
	adcq	%r12,%r12
	movq	%r10,%rax
	adcq	%r13,%r13
	movq	%r11,%rbx
	sbbq	%rdi,%rdi

	subq	0(%rcx),%r8
	sbbq	8(%rcx),%r9
	movq	%r12,%rbp
	sbbq	16(%rcx),%r10
	sbbq	24(%rcx),%r11
	sbbq	32(%rcx),%r12
	movq	%r13,%rsi
	sbbq	40(%rcx),%r13
	sbbq	$0,%rdi

	movq	(%rsp),%rdi
	cmovcq	%r14,%r8
	cmovcq	%r15,%r9
	cmovcq	%rax,%r10
	cmovcq	%rbx,%r11
	cmovcq	%rbp,%r12
	cmovcq	%rsi,%r13

	decl	%edx
	jnz	.Loop_lshift_mod_384

	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_lshift_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_lshift_mod_384:

.def	__lshift_mod_384;	.scl 3;	.type 32;	.endef
.p2align	5
__lshift_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa

	addq	%r8,%r8
	adcq	%r9,%r9
	adcq	%r10,%r10
	movq	%r8,%r14
	adcq	%r11,%r11
	movq	%r9,%r15
	adcq	%r12,%r12
	movq	%r10,%rax
	adcq	%r13,%r13
	movq	%r11,%rbx
	sbbq	%rdx,%rdx

	subq	0(%rcx),%r8
	sbbq	8(%rcx),%r9
	movq	%r12,%rbp
	sbbq	16(%rcx),%r10
	sbbq	24(%rcx),%r11
	sbbq	32(%rcx),%r12
	movq	%r13,%rsi
	sbbq	40(%rcx),%r13
	sbbq	$0,%rdx

	cmovcq	%r14,%r8
	cmovcq	%r15,%r9
	cmovcq	%rax,%r10
	cmovcq	%rbx,%r11
	cmovcq	%rbp,%r12
	cmovcq	%rsi,%r13

	.byte	0xf3,0xc3



.globl	mul_by_3_mod_384

.def	mul_by_3_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_3_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_3_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rsi

.LSEH_body_mul_by_3_mod_384:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13
	movq	%rdx,%rcx

	call	__lshift_mod_384

	movq	(%rsp),%rdx
	call	__add_mod_384_a_is_loaded

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_3_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_3_mod_384:

.globl	mul_by_8_mod_384

.def	mul_by_8_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_8_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_8_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$8,%rsp

.LSEH_body_mul_by_8_mod_384:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13
	movq	%rdx,%rcx

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_8_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_8_mod_384:

.globl	mul_by_b_onE1

.def	mul_by_b_onE1;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_b_onE1:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_b_onE1:
	movq	%rcx,%rdi
	movq	%rdx,%rsi


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$8,%rsp

.LSEH_body_mul_by_b_onE1:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13
	leaq	BLS12_381_P(%rip),%rcx

	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_b_onE1:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_b_onE1:

.globl	mul_by_4b_onE1

.def	mul_by_4b_onE1;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_4b_onE1:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_4b_onE1:
	movq	%rcx,%rdi
	movq	%rdx,%rsi


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$8,%rsp

.LSEH_body_mul_by_4b_onE1:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13
	leaq	BLS12_381_P(%rip),%rcx

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_4b_onE1:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_4b_onE1:


.globl	mul_by_3_mod_384x

.def	mul_by_3_mod_384x;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_3_mod_384x:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_3_mod_384x:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rsi

.LSEH_body_mul_by_3_mod_384x:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13
	movq	%rdx,%rcx

	call	__lshift_mod_384

	movq	(%rsp),%rdx
	call	__add_mod_384_a_is_loaded

	movq	(%rsp),%rsi
	leaq	48(%rdi),%rdi

	movq	48(%rsi),%r8
	movq	56(%rsi),%r9
	movq	64(%rsi),%r10
	movq	72(%rsi),%r11
	movq	80(%rsi),%r12
	movq	88(%rsi),%r13

	call	__lshift_mod_384

	movq	$48,%rdx
	addq	(%rsp),%rdx
	call	__add_mod_384_a_is_loaded

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_3_mod_384x:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_3_mod_384x:

.globl	mul_by_8_mod_384x

.def	mul_by_8_mod_384x;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_8_mod_384x:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_8_mod_384x:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rsi

.LSEH_body_mul_by_8_mod_384x:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13
	movq	%rdx,%rcx

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	(%rsp),%rsi
	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	48+0(%rsi),%r8
	movq	48+8(%rsi),%r9
	movq	48+16(%rsi),%r10
	movq	48+24(%rsi),%r11
	movq	48+32(%rsi),%r12
	movq	48+40(%rsi),%r13

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	%r8,48+0(%rdi)
	movq	%r9,48+8(%rdi)
	movq	%r10,48+16(%rdi)
	movq	%r11,48+24(%rdi)
	movq	%r12,48+32(%rdi)
	movq	%r13,48+40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_8_mod_384x:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_8_mod_384x:

.globl	mul_by_b_onE2

.def	mul_by_b_onE2;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_b_onE2:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_b_onE2:
	movq	%rcx,%rdi
	movq	%rdx,%rsi


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rsi

.LSEH_body_mul_by_b_onE2:


	leaq	BLS12_381_P(%rip),%rcx
	leaq	48(%rsi),%rdx
	call	__sub_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	0(%rsp),%rsi
	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	leaq	48(%rsi),%rdx
	leaq	48(%rdi),%rdi
	call	__add_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_b_onE2:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_b_onE2:

.globl	mul_by_4b_onE2

.def	mul_by_4b_onE2;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_4b_onE2:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_4b_onE2:
	movq	%rcx,%rdi
	movq	%rdx,%rsi


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rsi

.LSEH_body_mul_by_4b_onE2:


	leaq	BLS12_381_P(%rip),%rcx
	leaq	48(%rsi),%rdx
	call	__sub_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	0(%rsp),%rsi
	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	leaq	48(%rsi),%rdx
	leaq	48(%rdi),%rdi
	call	__add_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	movq	%r8,0(%rdi)
	movq	%r9,8(%rdi)
	movq	%r10,16(%rdi)
	movq	%r11,24(%rdi)
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_mul_by_4b_onE2:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_4b_onE2:


.globl	cneg_mod_384

.def	cneg_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
cneg_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_cneg_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx
	movq	%r9,%rcx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	pushq	%rdx

.LSEH_body_cneg_mod_384:


	movq	0(%rsi),%rdx
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	%rdx,%r8
	movq	24(%rsi),%r11
	orq	%r9,%rdx
	movq	32(%rsi),%r12
	orq	%r10,%rdx
	movq	40(%rsi),%r13
	orq	%r11,%rdx
	movq	$-1,%rsi
	orq	%r12,%rdx
	orq	%r13,%rdx

	movq	0(%rcx),%r14
	cmovnzq	%rsi,%rdx
	movq	8(%rcx),%r15
	movq	16(%rcx),%rax
	andq	%rdx,%r14
	movq	24(%rcx),%rbx
	andq	%rdx,%r15
	movq	32(%rcx),%rbp
	andq	%rdx,%rax
	movq	40(%rcx),%rsi
	andq	%rdx,%rbx
	movq	0(%rsp),%rcx
	andq	%rdx,%rbp
	andq	%rdx,%rsi

	subq	%r8,%r14
	sbbq	%r9,%r15
	sbbq	%r10,%rax
	sbbq	%r11,%rbx
	sbbq	%r12,%rbp
	sbbq	%r13,%rsi

	orq	%rcx,%rcx

	cmovzq	%r8,%r14
	cmovzq	%r9,%r15
	cmovzq	%r10,%rax
	movq	%r14,0(%rdi)
	cmovzq	%r11,%rbx
	movq	%r15,8(%rdi)
	cmovzq	%r12,%rbp
	movq	%rax,16(%rdi)
	cmovzq	%r13,%rsi
	movq	%rbx,24(%rdi)
	movq	%rbp,32(%rdi)
	movq	%rsi,40(%rdi)

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_cneg_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_cneg_mod_384:


.globl	sub_mod_384

.def	sub_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
sub_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_sub_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx
	movq	%r9,%rcx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$8,%rsp

.LSEH_body_sub_mod_384:


	call	__sub_mod_384

	movq	8(%rsp),%r15

	movq	16(%rsp),%r14

	movq	24(%rsp),%r13

	movq	32(%rsp),%r12

	movq	40(%rsp),%rbx

	movq	48(%rsp),%rbp

	leaq	56(%rsp),%rsp

.LSEH_epilogue_sub_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_sub_mod_384:

.def	__sub_mod_384;	.scl 3;	.type 32;	.endef
.p2align	5
__sub_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa

	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13

	subq	0(%rdx),%r8
	movq	0(%rcx),%r14
	sbbq	8(%rdx),%r9
	movq	8(%rcx),%r15
	sbbq	16(%rdx),%r10
	movq	16(%rcx),%rax
	sbbq	24(%rdx),%r11
	movq	24(%rcx),%rbx
	sbbq	32(%rdx),%r12
	movq	32(%rcx),%rbp
	sbbq	40(%rdx),%r13
	movq	40(%rcx),%rsi
	sbbq	%rdx,%rdx

	andq	%rdx,%r14
	andq	%rdx,%r15
	andq	%rdx,%rax
	andq	%rdx,%rbx
	andq	%rdx,%rbp
	andq	%rdx,%rsi

	addq	%r14,%r8
	adcq	%r15,%r9
	movq	%r8,0(%rdi)
	adcq	%rax,%r10
	movq	%r9,8(%rdi)
	adcq	%rbx,%r11
	movq	%r10,16(%rdi)
	adcq	%rbp,%r12
	movq	%r11,24(%rdi)
	adcq	%rsi,%r13
	movq	%r12,32(%rdi)
	movq	%r13,40(%rdi)

	.byte	0xf3,0xc3


.globl	sub_mod_384x

.def	sub_mod_384x;	.scl 2;	.type 32;	.endef
.p2align	5
sub_mod_384x:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_sub_mod_384x:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx
	movq	%r9,%rcx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$24,%rsp

.LSEH_body_sub_mod_384x:


	movq	%rsi,0(%rsp)
	movq	%rdx,8(%rsp)
	leaq	48(%rsi),%rsi
	leaq	48(%rdx),%rdx
	leaq	48(%rdi),%rdi
	call	__sub_mod_384

	movq	0(%rsp),%rsi
	movq	8(%rsp),%rdx
	leaq	-48(%rdi),%rdi
	call	__sub_mod_384

	movq	24+0(%rsp),%r15

	movq	24+8(%rsp),%r14

	movq	24+16(%rsp),%r13

	movq	24+24(%rsp),%r12

	movq	24+32(%rsp),%rbx

	movq	24+40(%rsp),%rbp

	leaq	24+48(%rsp),%rsp

.LSEH_epilogue_sub_mod_384x:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_sub_mod_384x:
.globl	mul_by_1_plus_i_mod_384x

.def	mul_by_1_plus_i_mod_384x;	.scl 2;	.type 32;	.endef
.p2align	5
mul_by_1_plus_i_mod_384x:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_mul_by_1_plus_i_mod_384x:
	movq	%rcx,%rdi
	movq	%rdx,%rsi
	movq	%r8,%rdx


	pushq	%rbp

	pushq	%rbx

	pushq	%r12

	pushq	%r13

	pushq	%r14

	pushq	%r15

	subq	$56,%rsp

.LSEH_body_mul_by_1_plus_i_mod_384x:


	movq	0(%rsi),%r8
	movq	8(%rsi),%r9
	movq	16(%rsi),%r10
	movq	24(%rsi),%r11
	movq	32(%rsi),%r12
	movq	40(%rsi),%r13

	movq	%r8,%r14
	addq	48(%rsi),%r8
	movq	%r9,%r15
	adcq	56(%rsi),%r9
	movq	%r10,%rax
	adcq	64(%rsi),%r10
	movq	%r11,%rbx
	adcq	72(%rsi),%r11
	movq	%r12,%rcx
	adcq	80(%rsi),%r12
	movq	%r13,%rbp
	adcq	88(%rsi),%r13
	movq	%rdi,48(%rsp)
	sbbq	%rdi,%rdi

	subq	48(%rsi),%r14
	sbbq	56(%rsi),%r15
	sbbq	64(%rsi),%rax
	sbbq	72(%rsi),%rbx
	sbbq	80(%rsi),%rcx
	sbbq	88(%rsi),%rbp
	sbbq	%rsi,%rsi

	movq	%r8,0(%rsp)
	movq	0(%rdx),%r8
	movq	%r9,8(%rsp)
	movq	8(%rdx),%r9
	movq	%r10,16(%rsp)
	movq	16(%rdx),%r10
	movq	%r11,24(%rsp)
	movq	24(%rdx),%r11
	movq	%r12,32(%rsp)
	andq	%rsi,%r8
	movq	32(%rdx),%r12
	movq	%r13,40(%rsp)
	andq	%rsi,%r9
	movq	40(%rdx),%r13
	andq	%rsi,%r10
	andq	%rsi,%r11
	andq	%rsi,%r12
	andq	%rsi,%r13
	movq	48(%rsp),%rsi

	addq	%r8,%r14
	movq	0(%rsp),%r8
	adcq	%r9,%r15
	movq	8(%rsp),%r9
	adcq	%r10,%rax
	movq	16(%rsp),%r10
	adcq	%r11,%rbx
	movq	24(%rsp),%r11
	adcq	%r12,%rcx
	movq	32(%rsp),%r12
	adcq	%r13,%rbp
	movq	40(%rsp),%r13

	movq	%r14,0(%rsi)
	movq	%r8,%r14
	movq	%r15,8(%rsi)
	movq	%rax,16(%rsi)
	movq	%r9,%r15
	movq	%rbx,24(%rsi)
	movq	%rcx,32(%rsi)
	movq	%r10,%rax
	movq	%rbp,40(%rsi)

	subq	0(%rdx),%r8
	movq	%r11,%rbx
	sbbq	8(%rdx),%r9
	sbbq	16(%rdx),%r10
	movq	%r12,%rcx
	sbbq	24(%rdx),%r11
	sbbq	32(%rdx),%r12
	movq	%r13,%rbp
	sbbq	40(%rdx),%r13
	sbbq	$0,%rdi

	cmovcq	%r14,%r8
	cmovcq	%r15,%r9
	cmovcq	%rax,%r10
	movq	%r8,48(%rsi)
	cmovcq	%rbx,%r11
	movq	%r9,56(%rsi)
	cmovcq	%rcx,%r12
	movq	%r10,64(%rsi)
	cmovcq	%rbp,%r13
	movq	%r11,72(%rsi)
	movq	%r12,80(%rsi)
	movq	%r13,88(%rsi)

	movq	56+0(%rsp),%r15

	movq	56+8(%rsp),%r14

	movq	56+16(%rsp),%r13

	movq	56+24(%rsp),%r12

	movq	56+32(%rsp),%rbx

	movq	56+40(%rsp),%rbp

	leaq	56+48(%rsp),%rsp

.LSEH_epilogue_mul_by_1_plus_i_mod_384x:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_mul_by_1_plus_i_mod_384x:
.globl	sgn0_pty_mod_384

.def	sgn0_pty_mod_384;	.scl 2;	.type 32;	.endef
.p2align	5
sgn0_pty_mod_384:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_sgn0_pty_mod_384:
	movq	%rcx,%rdi
	movq	%rdx,%rsi


.LSEH_body_sgn0_pty_mod_384:

	movq	0(%rdi),%r8
	movq	8(%rdi),%r9
	movq	16(%rdi),%r10
	movq	24(%rdi),%r11
	movq	32(%rdi),%rcx
	movq	40(%rdi),%rdx

	xorq	%rax,%rax
	movq	%r8,%rdi
	addq	%r8,%r8
	adcq	%r9,%r9
	adcq	%r10,%r10
	adcq	%r11,%r11
	adcq	%rcx,%rcx
	adcq	%rdx,%rdx
	adcq	$0,%rax

	subq	0(%rsi),%r8
	sbbq	8(%rsi),%r9
	sbbq	16(%rsi),%r10
	sbbq	24(%rsi),%r11
	sbbq	32(%rsi),%rcx
	sbbq	40(%rsi),%rdx
	sbbq	$0,%rax

	notq	%rax
	andq	$1,%rdi
	andq	$2,%rax
	orq	%rdi,%rax

.LSEH_epilogue_sgn0_pty_mod_384:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_sgn0_pty_mod_384:

.globl	sgn0_pty_mod_384x

.def	sgn0_pty_mod_384x;	.scl 2;	.type 32;	.endef
.p2align	5
sgn0_pty_mod_384x:
	.byte	0xf3,0x0f,0x1e,0xfa
	movq	%rdi,8(%rsp)
	movq	%rsi,16(%rsp)
	movq	%rsp,%r11
.LSEH_begin_sgn0_pty_mod_384x:
	movq	%rcx,%rdi
	movq	%rdx,%rsi


	pushq	%rbp

	pushq	%rbx

	subq	$8,%rsp

.LSEH_body_sgn0_pty_mod_384x:


	movq	0(%rdi),%r8
	movq	8(%rdi),%r9
	movq	16(%rdi),%r10
	movq	24(%rdi),%r11
	movq	32(%rdi),%rcx
	movq	40(%rdi),%rdx

	movq	%r8,%rbx
	orq	%r9,%r8
	orq	%r10,%r8
	orq	%r11,%r8
	orq	%rcx,%r8
	orq	%rdx,%r8

	xorq	%rax,%rax
	movq	%rbx,%rbp
	addq	%rbx,%rbx
	adcq	%r9,%r9
	adcq	%r10,%r10
	adcq	%r11,%r11
	adcq	%rcx,%rcx
	adcq	%rdx,%rdx
	adcq	$0,%rax

	subq	0(%rsi),%rbx
	sbbq	8(%rsi),%r9
	sbbq	16(%rsi),%r10
	sbbq	24(%rsi),%r11
	sbbq	32(%rsi),%rcx
	sbbq	40(%rsi),%rdx
	sbbq	$0,%rax

	movq	%r8,0(%rsp)
	notq	%rax
	andq	$1,%rbp
	andq	$2,%rax
	orq	%rbp,%rax

	movq	48(%rdi),%r8
	movq	56(%rdi),%r9
	movq	64(%rdi),%r10
	movq	72(%rdi),%r11
	movq	80(%rdi),%rcx
	movq	88(%rdi),%rdx

	movq	%r8,%rbx
	orq	%r9,%r8
	orq	%r10,%r8
	orq	%r11,%r8
	orq	%rcx,%r8
	orq	%rdx,%r8

	xorq	%rdi,%rdi
	movq	%rbx,%rbp
	addq	%rbx,%rbx
	adcq	%r9,%r9
	adcq	%r10,%r10
	adcq	%r11,%r11
	adcq	%rcx,%rcx
	adcq	%rdx,%rdx
	adcq	$0,%rdi

	subq	0(%rsi),%rbx
	sbbq	8(%rsi),%r9
	sbbq	16(%rsi),%r10
	sbbq	24(%rsi),%r11
	sbbq	32(%rsi),%rcx
	sbbq	40(%rsi),%rdx
	sbbq	$0,%rdi

	movq	0(%rsp),%rbx

	notq	%rdi

	testq	%r8,%r8
	cmovnzq	%rdi,%rax

	testq	%rbx,%rbx
	cmovzq	%rdi,%rbp

	andq	$1,%rbp
	andq	$2,%rax
	orq	%rbp,%rax

	movq	8(%rsp),%rbx

	movq	16(%rsp),%rbp

	leaq	24(%rsp),%rsp

.LSEH_epilogue_sgn0_pty_mod_384x:
	mov	8(%rsp),%rdi
	mov	16(%rsp),%rsi

	.byte	0xf3,0xc3

.LSEH_end_sgn0_pty_mod_384x:
.globl	vec_select_144

.def	vec_select_144;	.scl 2;	.type 32;	.endef
.p2align	5
vec_select_144:
	.byte	0xf3,0x0f,0x1e,0xfa

	movd	%r9d,%xmm5
	pxor	%xmm4,%xmm4
	pshufd	$0,%xmm5,%xmm5
	movdqu	(%rdx),%xmm0
	leaq	72(%rdx),%rdx
	pcmpeqd	%xmm4,%xmm5
	movdqu	(%r8),%xmm1
	leaq	72(%r8),%r8
	pcmpeqd	%xmm5,%xmm4
	leaq	72(%rcx),%rcx
	pand	%xmm4,%xmm0
	movdqu	0+16-72(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	0+16-72(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,0-72(%rcx)
	pand	%xmm4,%xmm2
	movdqu	16+16-72(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	16+16-72(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,16-72(%rcx)
	pand	%xmm4,%xmm0
	movdqu	32+16-72(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	32+16-72(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,32-72(%rcx)
	pand	%xmm4,%xmm2
	movdqu	48+16-72(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	48+16-72(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,48-72(%rcx)
	pand	%xmm4,%xmm0
	movdqu	64+16-72(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	64+16-72(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,64-72(%rcx)
	pand	%xmm4,%xmm2
	movdqu	80+16-72(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	80+16-72(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,80-72(%rcx)
	pand	%xmm4,%xmm0
	movdqu	96+16-72(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	96+16-72(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,96-72(%rcx)
	pand	%xmm4,%xmm2
	movdqu	112+16-72(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	112+16-72(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,112-72(%rcx)
	pand	%xmm4,%xmm0
	pand	%xmm5,%xmm1
	por	%xmm1,%xmm0
	movdqu	%xmm0,128-72(%rcx)
	.byte	0xf3,0xc3

.globl	vec_select_288

.def	vec_select_288;	.scl 2;	.type 32;	.endef
.p2align	5
vec_select_288:
	.byte	0xf3,0x0f,0x1e,0xfa

	movd	%r9d,%xmm5
	pxor	%xmm4,%xmm4
	pshufd	$0,%xmm5,%xmm5
	movdqu	(%rdx),%xmm0
	leaq	144(%rdx),%rdx
	pcmpeqd	%xmm4,%xmm5
	movdqu	(%r8),%xmm1
	leaq	144(%r8),%r8
	pcmpeqd	%xmm5,%xmm4
	leaq	144(%rcx),%rcx
	pand	%xmm4,%xmm0
	movdqu	0+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	0+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,0-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	16+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	16+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,16-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	32+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	32+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,32-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	48+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	48+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,48-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	64+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	64+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,64-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	80+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	80+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,80-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	96+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	96+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,96-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	112+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	112+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,112-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	128+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	128+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,128-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	144+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	144+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,144-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	160+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	160+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,160-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	176+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	176+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,176-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	192+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	192+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,192-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	208+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	208+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,208-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	224+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	224+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,224-144(%rcx)
	pand	%xmm4,%xmm2
	movdqu	240+16-144(%rdx),%xmm0
	pand	%xmm5,%xmm3
	movdqu	240+16-144(%r8),%xmm1
	por	%xmm3,%xmm2
	movdqu	%xmm2,240-144(%rcx)
	pand	%xmm4,%xmm0
	movdqu	256+16-144(%rdx),%xmm2
	pand	%xmm5,%xmm1
	movdqu	256+16-144(%r8),%xmm3
	por	%xmm1,%xmm0
	movdqu	%xmm0,256-144(%rcx)
	pand	%xmm4,%xmm2
	pand	%xmm5,%xmm3
	por	%xmm3,%xmm2
	movdqu	%xmm2,272-144(%rcx)
	.byte	0xf3,0xc3

.section	.pdata
.p2align	2
.rva	.LSEH_begin_add_mod_384
.rva	.LSEH_body_add_mod_384
.rva	.LSEH_info_add_mod_384_prologue

.rva	.LSEH_body_add_mod_384
.rva	.LSEH_epilogue_add_mod_384
.rva	.LSEH_info_add_mod_384_body

.rva	.LSEH_epilogue_add_mod_384
.rva	.LSEH_end_add_mod_384
.rva	.LSEH_info_add_mod_384_epilogue

.rva	.LSEH_begin_add_mod_384x
.rva	.LSEH_body_add_mod_384x
.rva	.LSEH_info_add_mod_384x_prologue

.rva	.LSEH_body_add_mod_384x
.rva	.LSEH_epilogue_add_mod_384x
.rva	.LSEH_info_add_mod_384x_body

.rva	.LSEH_epilogue_add_mod_384x
.rva	.LSEH_end_add_mod_384x
.rva	.LSEH_info_add_mod_384x_epilogue

.rva	.LSEH_begin_lshift_mod_384
.rva	.LSEH_body_lshift_mod_384
.rva	.LSEH_info_lshift_mod_384_prologue

.rva	.LSEH_body_lshift_mod_384
.rva	.LSEH_epilogue_lshift_mod_384
.rva	.LSEH_info_lshift_mod_384_body

.rva	.LSEH_epilogue_lshift_mod_384
.rva	.LSEH_end_lshift_mod_384
.rva	.LSEH_info_lshift_mod_384_epilogue

.rva	.LSEH_begin_mul_by_3_mod_384
.rva	.LSEH_body_mul_by_3_mod_384
.rva	.LSEH_info_mul_by_3_mod_384_prologue

.rva	.LSEH_body_mul_by_3_mod_384
.rva	.LSEH_epilogue_mul_by_3_mod_384
.rva	.LSEH_info_mul_by_3_mod_384_body

.rva	.LSEH_epilogue_mul_by_3_mod_384
.rva	.LSEH_end_mul_by_3_mod_384
.rva	.LSEH_info_mul_by_3_mod_384_epilogue

.rva	.LSEH_begin_mul_by_8_mod_384
.rva	.LSEH_body_mul_by_8_mod_384
.rva	.LSEH_info_mul_by_8_mod_384_prologue

.rva	.LSEH_body_mul_by_8_mod_384
.rva	.LSEH_epilogue_mul_by_8_mod_384
.rva	.LSEH_info_mul_by_8_mod_384_body

.rva	.LSEH_epilogue_mul_by_8_mod_384
.rva	.LSEH_end_mul_by_8_mod_384
.rva	.LSEH_info_mul_by_8_mod_384_epilogue

.rva	.LSEH_begin_mul_by_b_onE1
.rva	.LSEH_body_mul_by_b_onE1
.rva	.LSEH_info_mul_by_b_onE1_prologue

.rva	.LSEH_body_mul_by_b_onE1
.rva	.LSEH_epilogue_mul_by_b_onE1
.rva	.LSEH_info_mul_by_b_onE1_body

.rva	.LSEH_epilogue_mul_by_b_onE1
.rva	.LSEH_end_mul_by_b_onE1
.rva	.LSEH_info_mul_by_b_onE1_epilogue

.rva	.LSEH_begin_mul_by_4b_onE1
.rva	.LSEH_body_mul_by_4b_onE1
.rva	.LSEH_info_mul_by_4b_onE1_prologue

.rva	.LSEH_body_mul_by_4b_onE1
.rva	.LSEH_epilogue_mul_by_4b_onE1
.rva	.LSEH_info_mul_by_4b_onE1_body

.rva	.LSEH_epilogue_mul_by_4b_onE1
.rva	.LSEH_end_mul_by_4b_onE1
.rva	.LSEH_info_mul_by_4b_onE1_epilogue

.rva	.LSEH_begin_mul_by_3_mod_384x
.rva	.LSEH_body_mul_by_3_mod_384x
.rva	.LSEH_info_mul_by_3_mod_384x_prologue

.rva	.LSEH_body_mul_by_3_mod_384x
.rva	.LSEH_epilogue_mul_by_3_mod_384x
.rva	.LSEH_info_mul_by_3_mod_384x_body

.rva	.LSEH_epilogue_mul_by_3_mod_384x
.rva	.LSEH_end_mul_by_3_mod_384x
.rva	.LSEH_info_mul_by_3_mod_384x_epilogue

.rva	.LSEH_begin_mul_by_8_mod_384x
.rva	.LSEH_body_mul_by_8_mod_384x
.rva	.LSEH_info_mul_by_8_mod_384x_prologue

.rva	.LSEH_body_mul_by_8_mod_384x
.rva	.LSEH_epilogue_mul_by_8_mod_384x
.rva	.LSEH_info_mul_by_8_mod_384x_body

.rva	.LSEH_epilogue_mul_by_8_mod_384x
.rva	.LSEH_end_mul_by_8_mod_384x
.rva	.LSEH_info_mul_by_8_mod_384x_epilogue

.rva	.LSEH_begin_mul_by_b_onE2
.rva	.LSEH_body_mul_by_b_onE2
.rva	.LSEH_info_mul_by_b_onE2_prologue

.rva	.LSEH_body_mul_by_b_onE2
.rva	.LSEH_epilogue_mul_by_b_onE2
.rva	.LSEH_info_mul_by_b_onE2_body

.rva	.LSEH_epilogue_mul_by_b_onE2
.rva	.LSEH_end_mul_by_b_onE2
.rva	.LSEH_info_mul_by_b_onE2_epilogue

.rva	.LSEH_begin_mul_by_4b_onE2
.rva	.LSEH_body_mul_by_4b_onE2
.rva	.LSEH_info_mul_by_4b_onE2_prologue

.rva	.LSEH_body_mul_by_4b_onE2
.rva	.LSEH_epilogue_mul_by_4b_onE2
.rva	.LSEH_info_mul_by_4b_onE2_body

.rva	.LSEH_epilogue_mul_by_4b_onE2
.rva	.LSEH_end_mul_by_4b_onE2
.rva	.LSEH_info_mul_by_4b_onE2_epilogue

.rva	.LSEH_begin_cneg_mod_384
.rva	.LSEH_body_cneg_mod_384
.rva	.LSEH_info_cneg_mod_384_prologue

.rva	.LSEH_body_cneg_mod_384
.rva	.LSEH_epilogue_cneg_mod_384
.rva	.LSEH_info_cneg_mod_384_body

.rva	.LSEH_epilogue_cneg_mod_384
.rva	.LSEH_end_cneg_mod_384
.rva	.LSEH_info_cneg_mod_384_epilogue

.rva	.LSEH_begin_sub_mod_384
.rva	.LSEH_body_sub_mod_384
.rva	.LSEH_info_sub_mod_384_prologue

.rva	.LSEH_body_sub_mod_384
.rva	.LSEH_epilogue_sub_mod_384
.rva	.LSEH_info_sub_mod_384_body

.rva	.LSEH_epilogue_sub_mod_384
.rva	.LSEH_end_sub_mod_384
.rva	.LSEH_info_sub_mod_384_epilogue

.rva	.LSEH_begin_sub_mod_384x
.rva	.LSEH_body_sub_mod_384x
.rva	.LSEH_info_sub_mod_384x_prologue

.rva	.LSEH_body_sub_mod_384x
.rva	.LSEH_epilogue_sub_mod_384x
.rva	.LSEH_info_sub_mod_384x_body

.rva	.LSEH_epilogue_sub_mod_384x
.rva	.LSEH_end_sub_mod_384x
.rva	.LSEH_info_sub_mod_384x_epilogue

.rva	.LSEH_begin_mul_by_1_plus_i_mod_384x
.rva	.LSEH_body_mul_by_1_plus_i_mod_384x
.rva	.LSEH_info_mul_by_1_plus_i_mod_384x_prologue

.rva	.LSEH_body_mul_by_1_plus_i_mod_384x
.rva	.LSEH_epilogue_mul_by_1_plus_i_mod_384x
.rva	.LSEH_info_mul_by_1_plus_i_mod_384x_body

.rva	.LSEH_epilogue_mul_by_1_plus_i_mod_384x
.rva	.LSEH_end_mul_by_1_plus_i_mod_384x
.rva	.LSEH_info_mul_by_1_plus_i_mod_384x_epilogue

.rva	.LSEH_begin_sgn0_pty_mod_384
.rva	.LSEH_body_sgn0_pty_mod_384
.rva	.LSEH_info_sgn0_pty_mod_384_prologue

.rva	.LSEH_body_sgn0_pty_mod_384
.rva	.LSEH_epilogue_sgn0_pty_mod_384
.rva	.LSEH_info_sgn0_pty_mod_384_body

.rva	.LSEH_epilogue_sgn0_pty_mod_384
.rva	.LSEH_end_sgn0_pty_mod_384
.rva	.LSEH_info_sgn0_pty_mod_384_epilogue

.rva	.LSEH_begin_sgn0_pty_mod_384x
.rva	.LSEH_body_sgn0_pty_mod_384x
.rva	.LSEH_info_sgn0_pty_mod_384x_prologue

.rva	.LSEH_body_sgn0_pty_mod_384x
.rva	.LSEH_epilogue_sgn0_pty_mod_384x
.rva	.LSEH_info_sgn0_pty_mod_384x_body

.rva	.LSEH_epilogue_sgn0_pty_mod_384x
.rva	.LSEH_end_sgn0_pty_mod_384x
.rva	.LSEH_info_sgn0_pty_mod_384x_epilogue

.section	.xdata
.p2align	3
.LSEH_info_add_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_add_mod_384_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_add_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_add_mod_384x_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_add_mod_384x_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x03,0x00
.byte	0x00,0xe4,0x04,0x00
.byte	0x00,0xd4,0x05,0x00
.byte	0x00,0xc4,0x06,0x00
.byte	0x00,0x34,0x07,0x00
.byte	0x00,0x54,0x08,0x00
.byte	0x00,0x74,0x0a,0x00
.byte	0x00,0x64,0x0b,0x00
.byte	0x00,0x82
.byte	0x00,0x00
.LSEH_info_add_mod_384x_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_lshift_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_lshift_mod_384_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_lshift_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_3_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_3_mod_384_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_3_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_8_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_8_mod_384_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_8_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_b_onE1_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_b_onE1_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_b_onE1_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_4b_onE1_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_4b_onE1_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_4b_onE1_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_3_mod_384x_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_3_mod_384x_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_3_mod_384x_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_8_mod_384x_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_8_mod_384x_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_8_mod_384x_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_b_onE2_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_b_onE2_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_b_onE2_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_4b_onE2_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_4b_onE2_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_mul_by_4b_onE2_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_cneg_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_cneg_mod_384_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_cneg_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_sub_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_sub_mod_384_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x01,0x00
.byte	0x00,0xe4,0x02,0x00
.byte	0x00,0xd4,0x03,0x00
.byte	0x00,0xc4,0x04,0x00
.byte	0x00,0x34,0x05,0x00
.byte	0x00,0x54,0x06,0x00
.byte	0x00,0x74,0x08,0x00
.byte	0x00,0x64,0x09,0x00
.byte	0x00,0x62
.byte	0x00,0x00
.LSEH_info_sub_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_sub_mod_384x_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_sub_mod_384x_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x03,0x00
.byte	0x00,0xe4,0x04,0x00
.byte	0x00,0xd4,0x05,0x00
.byte	0x00,0xc4,0x06,0x00
.byte	0x00,0x34,0x07,0x00
.byte	0x00,0x54,0x08,0x00
.byte	0x00,0x74,0x0a,0x00
.byte	0x00,0x64,0x0b,0x00
.byte	0x00,0x82
.byte	0x00,0x00
.LSEH_info_sub_mod_384x_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_mul_by_1_plus_i_mod_384x_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_mul_by_1_plus_i_mod_384x_body:
.byte	1,0,17,0
.byte	0x00,0xf4,0x07,0x00
.byte	0x00,0xe4,0x08,0x00
.byte	0x00,0xd4,0x09,0x00
.byte	0x00,0xc4,0x0a,0x00
.byte	0x00,0x34,0x0b,0x00
.byte	0x00,0x54,0x0c,0x00
.byte	0x00,0x74,0x0e,0x00
.byte	0x00,0x64,0x0f,0x00
.byte	0x00,0xc2
.byte	0x00,0x00
.LSEH_info_mul_by_1_plus_i_mod_384x_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_sgn0_pty_mod_384_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_sgn0_pty_mod_384_body:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00
.LSEH_info_sgn0_pty_mod_384_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

.LSEH_info_sgn0_pty_mod_384x_prologue:
.byte	1,0,5,0x0b
.byte	0,0x74,1,0
.byte	0,0x64,2,0
.byte	0,0x03
.byte	0,0
.LSEH_info_sgn0_pty_mod_384x_body:
.byte	1,0,9,0
.byte	0x00,0x34,0x01,0x00
.byte	0x00,0x54,0x02,0x00
.byte	0x00,0x74,0x04,0x00
.byte	0x00,0x64,0x05,0x00
.byte	0x00,0x22
.byte	0x00,0x00
.LSEH_info_sgn0_pty_mod_384x_epilogue:
.byte	1,0,4,0
.byte	0x00,0x74,0x01,0x00
.byte	0x00,0x64,0x02,0x00
.byte	0x00,0x00,0x00,0x00

