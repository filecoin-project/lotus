OPTION	DOTNAME
.text$	SEGMENT ALIGN(256) 'CODE'
EXTERN	BLS12_381_P:NEAR


PUBLIC	add_mod_384


ALIGN	32
add_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_add_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,8

$L$SEH_body_add_mod_384::


	call	__add_mod_384

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_add_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_add_mod_384::
add_mod_384	ENDP


ALIGN	32
__add_mod_384	PROC PRIVATE
	DB	243,15,30,250
	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

__add_mod_384_a_is_loaded::
	add	r8,QWORD PTR[rdx]
	adc	r9,QWORD PTR[8+rdx]
	adc	r10,QWORD PTR[16+rdx]
	mov	r14,r8
	adc	r11,QWORD PTR[24+rdx]
	mov	r15,r9
	adc	r12,QWORD PTR[32+rdx]
	mov	rax,r10
	adc	r13,QWORD PTR[40+rdx]
	mov	rbx,r11
	sbb	rdx,rdx

	sub	r8,QWORD PTR[rcx]
	sbb	r9,QWORD PTR[8+rcx]
	mov	rbp,r12
	sbb	r10,QWORD PTR[16+rcx]
	sbb	r11,QWORD PTR[24+rcx]
	sbb	r12,QWORD PTR[32+rcx]
	mov	rsi,r13
	sbb	r13,QWORD PTR[40+rcx]
	sbb	rdx,0

	cmovc	r8,r14
	cmovc	r9,r15
	cmovc	r10,rax
	mov	QWORD PTR[rdi],r8
	cmovc	r11,rbx
	mov	QWORD PTR[8+rdi],r9
	cmovc	r12,rbp
	mov	QWORD PTR[16+rdi],r10
	cmovc	r13,rsi
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	DB	0F3h,0C3h		;repret
__add_mod_384	ENDP

PUBLIC	add_mod_384x


ALIGN	32
add_mod_384x	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_add_mod_384x::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,24

$L$SEH_body_add_mod_384x::


	mov	QWORD PTR[rsp],rsi
	mov	QWORD PTR[8+rsp],rdx
	lea	rsi,QWORD PTR[48+rsi]
	lea	rdx,QWORD PTR[48+rdx]
	lea	rdi,QWORD PTR[48+rdi]
	call	__add_mod_384

	mov	rsi,QWORD PTR[rsp]
	mov	rdx,QWORD PTR[8+rsp]
	lea	rdi,QWORD PTR[((-48))+rdi]
	call	__add_mod_384

	mov	r15,QWORD PTR[((24+0))+rsp]

	mov	r14,QWORD PTR[((24+8))+rsp]

	mov	r13,QWORD PTR[((24+16))+rsp]

	mov	r12,QWORD PTR[((24+24))+rsp]

	mov	rbx,QWORD PTR[((24+32))+rsp]

	mov	rbp,QWORD PTR[((24+40))+rsp]

	lea	rsp,QWORD PTR[((24+48))+rsp]

$L$SEH_epilogue_add_mod_384x::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_add_mod_384x::
add_mod_384x	ENDP


PUBLIC	lshift_mod_384


ALIGN	32
lshift_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_lshift_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rdi

$L$SEH_body_lshift_mod_384::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

$L$oop_lshift_mod_384::
	add	r8,r8
	adc	r9,r9
	adc	r10,r10
	mov	r14,r8
	adc	r11,r11
	mov	r15,r9
	adc	r12,r12
	mov	rax,r10
	adc	r13,r13
	mov	rbx,r11
	sbb	rdi,rdi

	sub	r8,QWORD PTR[rcx]
	sbb	r9,QWORD PTR[8+rcx]
	mov	rbp,r12
	sbb	r10,QWORD PTR[16+rcx]
	sbb	r11,QWORD PTR[24+rcx]
	sbb	r12,QWORD PTR[32+rcx]
	mov	rsi,r13
	sbb	r13,QWORD PTR[40+rcx]
	sbb	rdi,0

	mov	rdi,QWORD PTR[rsp]
	cmovc	r8,r14
	cmovc	r9,r15
	cmovc	r10,rax
	cmovc	r11,rbx
	cmovc	r12,rbp
	cmovc	r13,rsi

	dec	edx
	jnz	$L$oop_lshift_mod_384

	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_lshift_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_lshift_mod_384::
lshift_mod_384	ENDP


ALIGN	32
__lshift_mod_384	PROC PRIVATE
	DB	243,15,30,250
	add	r8,r8
	adc	r9,r9
	adc	r10,r10
	mov	r14,r8
	adc	r11,r11
	mov	r15,r9
	adc	r12,r12
	mov	rax,r10
	adc	r13,r13
	mov	rbx,r11
	sbb	rdx,rdx

	sub	r8,QWORD PTR[rcx]
	sbb	r9,QWORD PTR[8+rcx]
	mov	rbp,r12
	sbb	r10,QWORD PTR[16+rcx]
	sbb	r11,QWORD PTR[24+rcx]
	sbb	r12,QWORD PTR[32+rcx]
	mov	rsi,r13
	sbb	r13,QWORD PTR[40+rcx]
	sbb	rdx,0

	cmovc	r8,r14
	cmovc	r9,r15
	cmovc	r10,rax
	cmovc	r11,rbx
	cmovc	r12,rbp
	cmovc	r13,rsi

	DB	0F3h,0C3h		;repret
__lshift_mod_384	ENDP


PUBLIC	mul_by_3_mod_384


ALIGN	32
mul_by_3_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_3_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rsi

$L$SEH_body_mul_by_3_mod_384::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]
	mov	rcx,rdx

	call	__lshift_mod_384

	mov	rdx,QWORD PTR[rsp]
	call	__add_mod_384_a_is_loaded

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_3_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_3_mod_384::
mul_by_3_mod_384	ENDP

PUBLIC	mul_by_8_mod_384


ALIGN	32
mul_by_8_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_8_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,8

$L$SEH_body_mul_by_8_mod_384::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]
	mov	rcx,rdx

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_8_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_8_mod_384::
mul_by_8_mod_384	ENDP

PUBLIC	mul_by_b_onE1


ALIGN	32
mul_by_b_onE1	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_b_onE1::
	mov	rdi,rcx
	mov	rsi,rdx



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,8

$L$SEH_body_mul_by_b_onE1::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]
	lea	rcx,QWORD PTR[BLS12_381_P]

	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_b_onE1::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_b_onE1::
mul_by_b_onE1	ENDP

PUBLIC	mul_by_4b_onE1


ALIGN	32
mul_by_4b_onE1	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_4b_onE1::
	mov	rdi,rcx
	mov	rsi,rdx



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,8

$L$SEH_body_mul_by_4b_onE1::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]
	lea	rcx,QWORD PTR[BLS12_381_P]

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_4b_onE1::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_4b_onE1::
mul_by_4b_onE1	ENDP


PUBLIC	mul_by_3_mod_384x


ALIGN	32
mul_by_3_mod_384x	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_3_mod_384x::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rsi

$L$SEH_body_mul_by_3_mod_384x::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]
	mov	rcx,rdx

	call	__lshift_mod_384

	mov	rdx,QWORD PTR[rsp]
	call	__add_mod_384_a_is_loaded

	mov	rsi,QWORD PTR[rsp]
	lea	rdi,QWORD PTR[48+rdi]

	mov	r8,QWORD PTR[48+rsi]
	mov	r9,QWORD PTR[56+rsi]
	mov	r10,QWORD PTR[64+rsi]
	mov	r11,QWORD PTR[72+rsi]
	mov	r12,QWORD PTR[80+rsi]
	mov	r13,QWORD PTR[88+rsi]

	call	__lshift_mod_384

	mov	rdx,8*6
	add	rdx,QWORD PTR[rsp]
	call	__add_mod_384_a_is_loaded

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_3_mod_384x::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_3_mod_384x::
mul_by_3_mod_384x	ENDP

PUBLIC	mul_by_8_mod_384x


ALIGN	32
mul_by_8_mod_384x	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_8_mod_384x::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rsi

$L$SEH_body_mul_by_8_mod_384x::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]
	mov	rcx,rdx

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	rsi,QWORD PTR[rsp]
	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r8,QWORD PTR[((48+0))+rsi]
	mov	r9,QWORD PTR[((48+8))+rsi]
	mov	r10,QWORD PTR[((48+16))+rsi]
	mov	r11,QWORD PTR[((48+24))+rsi]
	mov	r12,QWORD PTR[((48+32))+rsi]
	mov	r13,QWORD PTR[((48+40))+rsi]

	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	QWORD PTR[((48+0))+rdi],r8
	mov	QWORD PTR[((48+8))+rdi],r9
	mov	QWORD PTR[((48+16))+rdi],r10
	mov	QWORD PTR[((48+24))+rdi],r11
	mov	QWORD PTR[((48+32))+rdi],r12
	mov	QWORD PTR[((48+40))+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_8_mod_384x::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_8_mod_384x::
mul_by_8_mod_384x	ENDP

PUBLIC	mul_by_b_onE2


ALIGN	32
mul_by_b_onE2	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_b_onE2::
	mov	rdi,rcx
	mov	rsi,rdx



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rsi

$L$SEH_body_mul_by_b_onE2::


	lea	rcx,QWORD PTR[BLS12_381_P]
	lea	rdx,QWORD PTR[48+rsi]
	call	__sub_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	rsi,QWORD PTR[rsp]
	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	lea	rdx,QWORD PTR[48+rsi]
	lea	rdi,QWORD PTR[48+rdi]
	call	__add_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_b_onE2::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_b_onE2::
mul_by_b_onE2	ENDP

PUBLIC	mul_by_4b_onE2


ALIGN	32
mul_by_4b_onE2	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_4b_onE2::
	mov	rdi,rcx
	mov	rsi,rdx



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rsi

$L$SEH_body_mul_by_4b_onE2::


	lea	rcx,QWORD PTR[BLS12_381_P]
	lea	rdx,QWORD PTR[48+rsi]
	call	__sub_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	rsi,QWORD PTR[rsp]
	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	lea	rdx,QWORD PTR[48+rsi]
	lea	rdi,QWORD PTR[48+rdi]
	call	__add_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384
	call	__lshift_mod_384

	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_mul_by_4b_onE2::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_4b_onE2::
mul_by_4b_onE2	ENDP


PUBLIC	cneg_mod_384


ALIGN	32
cneg_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_cneg_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	push	rdx

$L$SEH_body_cneg_mod_384::


	mov	rdx,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r8,rdx
	mov	r11,QWORD PTR[24+rsi]
	or	rdx,r9
	mov	r12,QWORD PTR[32+rsi]
	or	rdx,r10
	mov	r13,QWORD PTR[40+rsi]
	or	rdx,r11
	mov	rsi,-1
	or	rdx,r12
	or	rdx,r13

	mov	r14,QWORD PTR[rcx]
	cmovnz	rdx,rsi
	mov	r15,QWORD PTR[8+rcx]
	mov	rax,QWORD PTR[16+rcx]
	and	r14,rdx
	mov	rbx,QWORD PTR[24+rcx]
	and	r15,rdx
	mov	rbp,QWORD PTR[32+rcx]
	and	rax,rdx
	mov	rsi,QWORD PTR[40+rcx]
	and	rbx,rdx
	mov	rcx,QWORD PTR[rsp]
	and	rbp,rdx
	and	rsi,rdx

	sub	r14,r8
	sbb	r15,r9
	sbb	rax,r10
	sbb	rbx,r11
	sbb	rbp,r12
	sbb	rsi,r13

	or	rcx,rcx

	cmovz	r14,r8
	cmovz	r15,r9
	cmovz	rax,r10
	mov	QWORD PTR[rdi],r14
	cmovz	rbx,r11
	mov	QWORD PTR[8+rdi],r15
	cmovz	rbp,r12
	mov	QWORD PTR[16+rdi],rax
	cmovz	rsi,r13
	mov	QWORD PTR[24+rdi],rbx
	mov	QWORD PTR[32+rdi],rbp
	mov	QWORD PTR[40+rdi],rsi

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_cneg_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_cneg_mod_384::
cneg_mod_384	ENDP


PUBLIC	sub_mod_384


ALIGN	32
sub_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_sub_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,8

$L$SEH_body_sub_mod_384::


	call	__sub_mod_384

	mov	r15,QWORD PTR[8+rsp]

	mov	r14,QWORD PTR[16+rsp]

	mov	r13,QWORD PTR[24+rsp]

	mov	r12,QWORD PTR[32+rsp]

	mov	rbx,QWORD PTR[40+rsp]

	mov	rbp,QWORD PTR[48+rsp]

	lea	rsp,QWORD PTR[56+rsp]

$L$SEH_epilogue_sub_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_sub_mod_384::
sub_mod_384	ENDP


ALIGN	32
__sub_mod_384	PROC PRIVATE
	DB	243,15,30,250
	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

	sub	r8,QWORD PTR[rdx]
	mov	r14,QWORD PTR[rcx]
	sbb	r9,QWORD PTR[8+rdx]
	mov	r15,QWORD PTR[8+rcx]
	sbb	r10,QWORD PTR[16+rdx]
	mov	rax,QWORD PTR[16+rcx]
	sbb	r11,QWORD PTR[24+rdx]
	mov	rbx,QWORD PTR[24+rcx]
	sbb	r12,QWORD PTR[32+rdx]
	mov	rbp,QWORD PTR[32+rcx]
	sbb	r13,QWORD PTR[40+rdx]
	mov	rsi,QWORD PTR[40+rcx]
	sbb	rdx,rdx

	and	r14,rdx
	and	r15,rdx
	and	rax,rdx
	and	rbx,rdx
	and	rbp,rdx
	and	rsi,rdx

	add	r8,r14
	adc	r9,r15
	mov	QWORD PTR[rdi],r8
	adc	r10,rax
	mov	QWORD PTR[8+rdi],r9
	adc	r11,rbx
	mov	QWORD PTR[16+rdi],r10
	adc	r12,rbp
	mov	QWORD PTR[24+rdi],r11
	adc	r13,rsi
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	DB	0F3h,0C3h		;repret
__sub_mod_384	ENDP

PUBLIC	sub_mod_384x


ALIGN	32
sub_mod_384x	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_sub_mod_384x::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,24

$L$SEH_body_sub_mod_384x::


	mov	QWORD PTR[rsp],rsi
	mov	QWORD PTR[8+rsp],rdx
	lea	rsi,QWORD PTR[48+rsi]
	lea	rdx,QWORD PTR[48+rdx]
	lea	rdi,QWORD PTR[48+rdi]
	call	__sub_mod_384

	mov	rsi,QWORD PTR[rsp]
	mov	rdx,QWORD PTR[8+rsp]
	lea	rdi,QWORD PTR[((-48))+rdi]
	call	__sub_mod_384

	mov	r15,QWORD PTR[((24+0))+rsp]

	mov	r14,QWORD PTR[((24+8))+rsp]

	mov	r13,QWORD PTR[((24+16))+rsp]

	mov	r12,QWORD PTR[((24+24))+rsp]

	mov	rbx,QWORD PTR[((24+32))+rsp]

	mov	rbp,QWORD PTR[((24+40))+rsp]

	lea	rsp,QWORD PTR[((24+48))+rsp]

$L$SEH_epilogue_sub_mod_384x::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_sub_mod_384x::
sub_mod_384x	ENDP
PUBLIC	mul_by_1_plus_i_mod_384x


ALIGN	32
mul_by_1_plus_i_mod_384x	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_mul_by_1_plus_i_mod_384x::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8



	push	rbp

	push	rbx

	push	r12

	push	r13

	push	r14

	push	r15

	sub	rsp,56

$L$SEH_body_mul_by_1_plus_i_mod_384x::


	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

	mov	r14,r8
	add	r8,QWORD PTR[48+rsi]
	mov	r15,r9
	adc	r9,QWORD PTR[56+rsi]
	mov	rax,r10
	adc	r10,QWORD PTR[64+rsi]
	mov	rbx,r11
	adc	r11,QWORD PTR[72+rsi]
	mov	rcx,r12
	adc	r12,QWORD PTR[80+rsi]
	mov	rbp,r13
	adc	r13,QWORD PTR[88+rsi]
	mov	QWORD PTR[48+rsp],rdi
	sbb	rdi,rdi

	sub	r14,QWORD PTR[48+rsi]
	sbb	r15,QWORD PTR[56+rsi]
	sbb	rax,QWORD PTR[64+rsi]
	sbb	rbx,QWORD PTR[72+rsi]
	sbb	rcx,QWORD PTR[80+rsi]
	sbb	rbp,QWORD PTR[88+rsi]
	sbb	rsi,rsi

	mov	QWORD PTR[rsp],r8
	mov	r8,QWORD PTR[rdx]
	mov	QWORD PTR[8+rsp],r9
	mov	r9,QWORD PTR[8+rdx]
	mov	QWORD PTR[16+rsp],r10
	mov	r10,QWORD PTR[16+rdx]
	mov	QWORD PTR[24+rsp],r11
	mov	r11,QWORD PTR[24+rdx]
	mov	QWORD PTR[32+rsp],r12
	and	r8,rsi
	mov	r12,QWORD PTR[32+rdx]
	mov	QWORD PTR[40+rsp],r13
	and	r9,rsi
	mov	r13,QWORD PTR[40+rdx]
	and	r10,rsi
	and	r11,rsi
	and	r12,rsi
	and	r13,rsi
	mov	rsi,QWORD PTR[48+rsp]

	add	r14,r8
	mov	r8,QWORD PTR[rsp]
	adc	r15,r9
	mov	r9,QWORD PTR[8+rsp]
	adc	rax,r10
	mov	r10,QWORD PTR[16+rsp]
	adc	rbx,r11
	mov	r11,QWORD PTR[24+rsp]
	adc	rcx,r12
	mov	r12,QWORD PTR[32+rsp]
	adc	rbp,r13
	mov	r13,QWORD PTR[40+rsp]

	mov	QWORD PTR[rsi],r14
	mov	r14,r8
	mov	QWORD PTR[8+rsi],r15
	mov	QWORD PTR[16+rsi],rax
	mov	r15,r9
	mov	QWORD PTR[24+rsi],rbx
	mov	QWORD PTR[32+rsi],rcx
	mov	rax,r10
	mov	QWORD PTR[40+rsi],rbp

	sub	r8,QWORD PTR[rdx]
	mov	rbx,r11
	sbb	r9,QWORD PTR[8+rdx]
	sbb	r10,QWORD PTR[16+rdx]
	mov	rcx,r12
	sbb	r11,QWORD PTR[24+rdx]
	sbb	r12,QWORD PTR[32+rdx]
	mov	rbp,r13
	sbb	r13,QWORD PTR[40+rdx]
	sbb	rdi,0

	cmovc	r8,r14
	cmovc	r9,r15
	cmovc	r10,rax
	mov	QWORD PTR[48+rsi],r8
	cmovc	r11,rbx
	mov	QWORD PTR[56+rsi],r9
	cmovc	r12,rcx
	mov	QWORD PTR[64+rsi],r10
	cmovc	r13,rbp
	mov	QWORD PTR[72+rsi],r11
	mov	QWORD PTR[80+rsi],r12
	mov	QWORD PTR[88+rsi],r13

	mov	r15,QWORD PTR[((56+0))+rsp]

	mov	r14,QWORD PTR[((56+8))+rsp]

	mov	r13,QWORD PTR[((56+16))+rsp]

	mov	r12,QWORD PTR[((56+24))+rsp]

	mov	rbx,QWORD PTR[((56+32))+rsp]

	mov	rbp,QWORD PTR[((56+40))+rsp]

	lea	rsp,QWORD PTR[((56+48))+rsp]

$L$SEH_epilogue_mul_by_1_plus_i_mod_384x::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_mul_by_1_plus_i_mod_384x::
mul_by_1_plus_i_mod_384x	ENDP
PUBLIC	sgn0_pty_mod_384


ALIGN	32
sgn0_pty_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_sgn0_pty_mod_384::
	mov	rdi,rcx
	mov	rsi,rdx



$L$SEH_body_sgn0_pty_mod_384::

	mov	r8,QWORD PTR[rdi]
	mov	r9,QWORD PTR[8+rdi]
	mov	r10,QWORD PTR[16+rdi]
	mov	r11,QWORD PTR[24+rdi]
	mov	rcx,QWORD PTR[32+rdi]
	mov	rdx,QWORD PTR[40+rdi]

	xor	rax,rax
	mov	rdi,r8
	add	r8,r8
	adc	r9,r9
	adc	r10,r10
	adc	r11,r11
	adc	rcx,rcx
	adc	rdx,rdx
	adc	rax,0

	sub	r8,QWORD PTR[rsi]
	sbb	r9,QWORD PTR[8+rsi]
	sbb	r10,QWORD PTR[16+rsi]
	sbb	r11,QWORD PTR[24+rsi]
	sbb	rcx,QWORD PTR[32+rsi]
	sbb	rdx,QWORD PTR[40+rsi]
	sbb	rax,0

	not	rax
	and	rdi,1
	and	rax,2
	or	rax,rdi

$L$SEH_epilogue_sgn0_pty_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_sgn0_pty_mod_384::
sgn0_pty_mod_384	ENDP

PUBLIC	sgn0_pty_mod_384x


ALIGN	32
sgn0_pty_mod_384x	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_sgn0_pty_mod_384x::
	mov	rdi,rcx
	mov	rsi,rdx



	push	rbp

	push	rbx

	sub	rsp,8

$L$SEH_body_sgn0_pty_mod_384x::


	mov	r8,QWORD PTR[rdi]
	mov	r9,QWORD PTR[8+rdi]
	mov	r10,QWORD PTR[16+rdi]
	mov	r11,QWORD PTR[24+rdi]
	mov	rcx,QWORD PTR[32+rdi]
	mov	rdx,QWORD PTR[40+rdi]

	mov	rbx,r8
	or	r8,r9
	or	r8,r10
	or	r8,r11
	or	r8,rcx
	or	r8,rdx

	xor	rax,rax
	mov	rbp,rbx
	add	rbx,rbx
	adc	r9,r9
	adc	r10,r10
	adc	r11,r11
	adc	rcx,rcx
	adc	rdx,rdx
	adc	rax,0

	sub	rbx,QWORD PTR[rsi]
	sbb	r9,QWORD PTR[8+rsi]
	sbb	r10,QWORD PTR[16+rsi]
	sbb	r11,QWORD PTR[24+rsi]
	sbb	rcx,QWORD PTR[32+rsi]
	sbb	rdx,QWORD PTR[40+rsi]
	sbb	rax,0

	mov	QWORD PTR[rsp],r8
	not	rax
	and	rbp,1
	and	rax,2
	or	rax,rbp

	mov	r8,QWORD PTR[48+rdi]
	mov	r9,QWORD PTR[56+rdi]
	mov	r10,QWORD PTR[64+rdi]
	mov	r11,QWORD PTR[72+rdi]
	mov	rcx,QWORD PTR[80+rdi]
	mov	rdx,QWORD PTR[88+rdi]

	mov	rbx,r8
	or	r8,r9
	or	r8,r10
	or	r8,r11
	or	r8,rcx
	or	r8,rdx

	xor	rdi,rdi
	mov	rbp,rbx
	add	rbx,rbx
	adc	r9,r9
	adc	r10,r10
	adc	r11,r11
	adc	rcx,rcx
	adc	rdx,rdx
	adc	rdi,0

	sub	rbx,QWORD PTR[rsi]
	sbb	r9,QWORD PTR[8+rsi]
	sbb	r10,QWORD PTR[16+rsi]
	sbb	r11,QWORD PTR[24+rsi]
	sbb	rcx,QWORD PTR[32+rsi]
	sbb	rdx,QWORD PTR[40+rsi]
	sbb	rdi,0

	mov	rbx,QWORD PTR[rsp]

	not	rdi

	test	r8,r8
	cmovnz	rax,rdi

	test	rbx,rbx
	cmovz	rbp,rdi

	and	rbp,1
	and	rax,2
	or	rax,rbp

	mov	rbx,QWORD PTR[8+rsp]

	mov	rbp,QWORD PTR[16+rsp]

	lea	rsp,QWORD PTR[24+rsp]

$L$SEH_epilogue_sgn0_pty_mod_384x::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_sgn0_pty_mod_384x::
sgn0_pty_mod_384x	ENDP
PUBLIC	vec_select_144


ALIGN	32
vec_select_144	PROC PUBLIC
	DB	243,15,30,250
	movd	xmm5,r9d
	pxor	xmm4,xmm4
	pshufd	xmm5,xmm5,0
	movdqu	xmm0,XMMWORD PTR[rdx]
	lea	rdx,QWORD PTR[72+rdx]
	pcmpeqd	xmm5,xmm4
	movdqu	xmm1,XMMWORD PTR[r8]
	lea	r8,QWORD PTR[72+r8]
	pcmpeqd	xmm4,xmm5
	lea	rcx,QWORD PTR[72+rcx]
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((0+16-72))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((0+16-72))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(0-72)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((16+16-72))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((16+16-72))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(16-72)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((32+16-72))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((32+16-72))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(32-72)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((48+16-72))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((48+16-72))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(48-72)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((64+16-72))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((64+16-72))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(64-72)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((80+16-72))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((80+16-72))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(80-72)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((96+16-72))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((96+16-72))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(96-72)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((112+16-72))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((112+16-72))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(112-72)+rcx],xmm2
	pand	xmm0,xmm4
	pand	xmm1,xmm5
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(128-72)+rcx],xmm0
	DB	0F3h,0C3h		;repret
vec_select_144	ENDP
PUBLIC	vec_select_288


ALIGN	32
vec_select_288	PROC PUBLIC
	DB	243,15,30,250
	movd	xmm5,r9d
	pxor	xmm4,xmm4
	pshufd	xmm5,xmm5,0
	movdqu	xmm0,XMMWORD PTR[rdx]
	lea	rdx,QWORD PTR[144+rdx]
	pcmpeqd	xmm5,xmm4
	movdqu	xmm1,XMMWORD PTR[r8]
	lea	r8,QWORD PTR[144+r8]
	pcmpeqd	xmm4,xmm5
	lea	rcx,QWORD PTR[144+rcx]
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((0+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((0+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(0-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((16+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((16+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(16-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((32+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((32+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(32-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((48+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((48+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(48-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((64+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((64+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(64-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((80+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((80+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(80-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((96+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((96+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(96-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((112+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((112+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(112-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((128+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((128+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(128-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((144+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((144+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(144-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((160+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((160+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(160-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((176+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((176+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(176-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((192+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((192+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(192-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((208+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((208+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(208-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((224+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((224+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(224-144)+rcx],xmm0
	pand	xmm2,xmm4
	movdqu	xmm0,XMMWORD PTR[((240+16-144))+rdx]
	pand	xmm3,xmm5
	movdqu	xmm1,XMMWORD PTR[((240+16-144))+r8]
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(240-144)+rcx],xmm2
	pand	xmm0,xmm4
	movdqu	xmm2,XMMWORD PTR[((256+16-144))+rdx]
	pand	xmm1,xmm5
	movdqu	xmm3,XMMWORD PTR[((256+16-144))+r8]
	por	xmm0,xmm1
	movdqu	XMMWORD PTR[(256-144)+rcx],xmm0
	pand	xmm2,xmm4
	pand	xmm3,xmm5
	por	xmm2,xmm3
	movdqu	XMMWORD PTR[(272-144)+rcx],xmm2
	DB	0F3h,0C3h		;repret
vec_select_288	ENDP
.text$	ENDS
.pdata	SEGMENT READONLY ALIGN(4)
ALIGN	4
	DD	imagerel $L$SEH_begin_add_mod_384
	DD	imagerel $L$SEH_body_add_mod_384
	DD	imagerel $L$SEH_info_add_mod_384_prologue

	DD	imagerel $L$SEH_body_add_mod_384
	DD	imagerel $L$SEH_epilogue_add_mod_384
	DD	imagerel $L$SEH_info_add_mod_384_body

	DD	imagerel $L$SEH_epilogue_add_mod_384
	DD	imagerel $L$SEH_end_add_mod_384
	DD	imagerel $L$SEH_info_add_mod_384_epilogue

	DD	imagerel $L$SEH_begin_add_mod_384x
	DD	imagerel $L$SEH_body_add_mod_384x
	DD	imagerel $L$SEH_info_add_mod_384x_prologue

	DD	imagerel $L$SEH_body_add_mod_384x
	DD	imagerel $L$SEH_epilogue_add_mod_384x
	DD	imagerel $L$SEH_info_add_mod_384x_body

	DD	imagerel $L$SEH_epilogue_add_mod_384x
	DD	imagerel $L$SEH_end_add_mod_384x
	DD	imagerel $L$SEH_info_add_mod_384x_epilogue

	DD	imagerel $L$SEH_begin_lshift_mod_384
	DD	imagerel $L$SEH_body_lshift_mod_384
	DD	imagerel $L$SEH_info_lshift_mod_384_prologue

	DD	imagerel $L$SEH_body_lshift_mod_384
	DD	imagerel $L$SEH_epilogue_lshift_mod_384
	DD	imagerel $L$SEH_info_lshift_mod_384_body

	DD	imagerel $L$SEH_epilogue_lshift_mod_384
	DD	imagerel $L$SEH_end_lshift_mod_384
	DD	imagerel $L$SEH_info_lshift_mod_384_epilogue

	DD	imagerel $L$SEH_begin_mul_by_3_mod_384
	DD	imagerel $L$SEH_body_mul_by_3_mod_384
	DD	imagerel $L$SEH_info_mul_by_3_mod_384_prologue

	DD	imagerel $L$SEH_body_mul_by_3_mod_384
	DD	imagerel $L$SEH_epilogue_mul_by_3_mod_384
	DD	imagerel $L$SEH_info_mul_by_3_mod_384_body

	DD	imagerel $L$SEH_epilogue_mul_by_3_mod_384
	DD	imagerel $L$SEH_end_mul_by_3_mod_384
	DD	imagerel $L$SEH_info_mul_by_3_mod_384_epilogue

	DD	imagerel $L$SEH_begin_mul_by_8_mod_384
	DD	imagerel $L$SEH_body_mul_by_8_mod_384
	DD	imagerel $L$SEH_info_mul_by_8_mod_384_prologue

	DD	imagerel $L$SEH_body_mul_by_8_mod_384
	DD	imagerel $L$SEH_epilogue_mul_by_8_mod_384
	DD	imagerel $L$SEH_info_mul_by_8_mod_384_body

	DD	imagerel $L$SEH_epilogue_mul_by_8_mod_384
	DD	imagerel $L$SEH_end_mul_by_8_mod_384
	DD	imagerel $L$SEH_info_mul_by_8_mod_384_epilogue

	DD	imagerel $L$SEH_begin_mul_by_b_onE1
	DD	imagerel $L$SEH_body_mul_by_b_onE1
	DD	imagerel $L$SEH_info_mul_by_b_onE1_prologue

	DD	imagerel $L$SEH_body_mul_by_b_onE1
	DD	imagerel $L$SEH_epilogue_mul_by_b_onE1
	DD	imagerel $L$SEH_info_mul_by_b_onE1_body

	DD	imagerel $L$SEH_epilogue_mul_by_b_onE1
	DD	imagerel $L$SEH_end_mul_by_b_onE1
	DD	imagerel $L$SEH_info_mul_by_b_onE1_epilogue

	DD	imagerel $L$SEH_begin_mul_by_4b_onE1
	DD	imagerel $L$SEH_body_mul_by_4b_onE1
	DD	imagerel $L$SEH_info_mul_by_4b_onE1_prologue

	DD	imagerel $L$SEH_body_mul_by_4b_onE1
	DD	imagerel $L$SEH_epilogue_mul_by_4b_onE1
	DD	imagerel $L$SEH_info_mul_by_4b_onE1_body

	DD	imagerel $L$SEH_epilogue_mul_by_4b_onE1
	DD	imagerel $L$SEH_end_mul_by_4b_onE1
	DD	imagerel $L$SEH_info_mul_by_4b_onE1_epilogue

	DD	imagerel $L$SEH_begin_mul_by_3_mod_384x
	DD	imagerel $L$SEH_body_mul_by_3_mod_384x
	DD	imagerel $L$SEH_info_mul_by_3_mod_384x_prologue

	DD	imagerel $L$SEH_body_mul_by_3_mod_384x
	DD	imagerel $L$SEH_epilogue_mul_by_3_mod_384x
	DD	imagerel $L$SEH_info_mul_by_3_mod_384x_body

	DD	imagerel $L$SEH_epilogue_mul_by_3_mod_384x
	DD	imagerel $L$SEH_end_mul_by_3_mod_384x
	DD	imagerel $L$SEH_info_mul_by_3_mod_384x_epilogue

	DD	imagerel $L$SEH_begin_mul_by_8_mod_384x
	DD	imagerel $L$SEH_body_mul_by_8_mod_384x
	DD	imagerel $L$SEH_info_mul_by_8_mod_384x_prologue

	DD	imagerel $L$SEH_body_mul_by_8_mod_384x
	DD	imagerel $L$SEH_epilogue_mul_by_8_mod_384x
	DD	imagerel $L$SEH_info_mul_by_8_mod_384x_body

	DD	imagerel $L$SEH_epilogue_mul_by_8_mod_384x
	DD	imagerel $L$SEH_end_mul_by_8_mod_384x
	DD	imagerel $L$SEH_info_mul_by_8_mod_384x_epilogue

	DD	imagerel $L$SEH_begin_mul_by_b_onE2
	DD	imagerel $L$SEH_body_mul_by_b_onE2
	DD	imagerel $L$SEH_info_mul_by_b_onE2_prologue

	DD	imagerel $L$SEH_body_mul_by_b_onE2
	DD	imagerel $L$SEH_epilogue_mul_by_b_onE2
	DD	imagerel $L$SEH_info_mul_by_b_onE2_body

	DD	imagerel $L$SEH_epilogue_mul_by_b_onE2
	DD	imagerel $L$SEH_end_mul_by_b_onE2
	DD	imagerel $L$SEH_info_mul_by_b_onE2_epilogue

	DD	imagerel $L$SEH_begin_mul_by_4b_onE2
	DD	imagerel $L$SEH_body_mul_by_4b_onE2
	DD	imagerel $L$SEH_info_mul_by_4b_onE2_prologue

	DD	imagerel $L$SEH_body_mul_by_4b_onE2
	DD	imagerel $L$SEH_epilogue_mul_by_4b_onE2
	DD	imagerel $L$SEH_info_mul_by_4b_onE2_body

	DD	imagerel $L$SEH_epilogue_mul_by_4b_onE2
	DD	imagerel $L$SEH_end_mul_by_4b_onE2
	DD	imagerel $L$SEH_info_mul_by_4b_onE2_epilogue

	DD	imagerel $L$SEH_begin_cneg_mod_384
	DD	imagerel $L$SEH_body_cneg_mod_384
	DD	imagerel $L$SEH_info_cneg_mod_384_prologue

	DD	imagerel $L$SEH_body_cneg_mod_384
	DD	imagerel $L$SEH_epilogue_cneg_mod_384
	DD	imagerel $L$SEH_info_cneg_mod_384_body

	DD	imagerel $L$SEH_epilogue_cneg_mod_384
	DD	imagerel $L$SEH_end_cneg_mod_384
	DD	imagerel $L$SEH_info_cneg_mod_384_epilogue

	DD	imagerel $L$SEH_begin_sub_mod_384
	DD	imagerel $L$SEH_body_sub_mod_384
	DD	imagerel $L$SEH_info_sub_mod_384_prologue

	DD	imagerel $L$SEH_body_sub_mod_384
	DD	imagerel $L$SEH_epilogue_sub_mod_384
	DD	imagerel $L$SEH_info_sub_mod_384_body

	DD	imagerel $L$SEH_epilogue_sub_mod_384
	DD	imagerel $L$SEH_end_sub_mod_384
	DD	imagerel $L$SEH_info_sub_mod_384_epilogue

	DD	imagerel $L$SEH_begin_sub_mod_384x
	DD	imagerel $L$SEH_body_sub_mod_384x
	DD	imagerel $L$SEH_info_sub_mod_384x_prologue

	DD	imagerel $L$SEH_body_sub_mod_384x
	DD	imagerel $L$SEH_epilogue_sub_mod_384x
	DD	imagerel $L$SEH_info_sub_mod_384x_body

	DD	imagerel $L$SEH_epilogue_sub_mod_384x
	DD	imagerel $L$SEH_end_sub_mod_384x
	DD	imagerel $L$SEH_info_sub_mod_384x_epilogue

	DD	imagerel $L$SEH_begin_mul_by_1_plus_i_mod_384x
	DD	imagerel $L$SEH_body_mul_by_1_plus_i_mod_384x
	DD	imagerel $L$SEH_info_mul_by_1_plus_i_mod_384x_prologue

	DD	imagerel $L$SEH_body_mul_by_1_plus_i_mod_384x
	DD	imagerel $L$SEH_epilogue_mul_by_1_plus_i_mod_384x
	DD	imagerel $L$SEH_info_mul_by_1_plus_i_mod_384x_body

	DD	imagerel $L$SEH_epilogue_mul_by_1_plus_i_mod_384x
	DD	imagerel $L$SEH_end_mul_by_1_plus_i_mod_384x
	DD	imagerel $L$SEH_info_mul_by_1_plus_i_mod_384x_epilogue

	DD	imagerel $L$SEH_begin_sgn0_pty_mod_384
	DD	imagerel $L$SEH_body_sgn0_pty_mod_384
	DD	imagerel $L$SEH_info_sgn0_pty_mod_384_prologue

	DD	imagerel $L$SEH_body_sgn0_pty_mod_384
	DD	imagerel $L$SEH_epilogue_sgn0_pty_mod_384
	DD	imagerel $L$SEH_info_sgn0_pty_mod_384_body

	DD	imagerel $L$SEH_epilogue_sgn0_pty_mod_384
	DD	imagerel $L$SEH_end_sgn0_pty_mod_384
	DD	imagerel $L$SEH_info_sgn0_pty_mod_384_epilogue

	DD	imagerel $L$SEH_begin_sgn0_pty_mod_384x
	DD	imagerel $L$SEH_body_sgn0_pty_mod_384x
	DD	imagerel $L$SEH_info_sgn0_pty_mod_384x_prologue

	DD	imagerel $L$SEH_body_sgn0_pty_mod_384x
	DD	imagerel $L$SEH_epilogue_sgn0_pty_mod_384x
	DD	imagerel $L$SEH_info_sgn0_pty_mod_384x_body

	DD	imagerel $L$SEH_epilogue_sgn0_pty_mod_384x
	DD	imagerel $L$SEH_end_sgn0_pty_mod_384x
	DD	imagerel $L$SEH_info_sgn0_pty_mod_384x_epilogue

.pdata	ENDS
.xdata	SEGMENT READONLY ALIGN(8)
ALIGN	8
$L$SEH_info_add_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_add_mod_384_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_add_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_add_mod_384x_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_add_mod_384x_body::
DB	1,0,17,0
DB	000h,0f4h,003h,000h
DB	000h,0e4h,004h,000h
DB	000h,0d4h,005h,000h
DB	000h,0c4h,006h,000h
DB	000h,034h,007h,000h
DB	000h,054h,008h,000h
DB	000h,074h,00ah,000h
DB	000h,064h,00bh,000h
DB	000h,082h
DB	000h,000h
$L$SEH_info_add_mod_384x_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_lshift_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_lshift_mod_384_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_lshift_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_3_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_3_mod_384_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_3_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_8_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_8_mod_384_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_8_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_b_onE1_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_b_onE1_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_b_onE1_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_4b_onE1_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_4b_onE1_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_4b_onE1_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_3_mod_384x_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_3_mod_384x_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_3_mod_384x_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_8_mod_384x_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_8_mod_384x_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_8_mod_384x_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_b_onE2_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_b_onE2_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_b_onE2_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_4b_onE2_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_4b_onE2_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_mul_by_4b_onE2_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_cneg_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_cneg_mod_384_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_cneg_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_sub_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_sub_mod_384_body::
DB	1,0,17,0
DB	000h,0f4h,001h,000h
DB	000h,0e4h,002h,000h
DB	000h,0d4h,003h,000h
DB	000h,0c4h,004h,000h
DB	000h,034h,005h,000h
DB	000h,054h,006h,000h
DB	000h,074h,008h,000h
DB	000h,064h,009h,000h
DB	000h,062h
DB	000h,000h
$L$SEH_info_sub_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_sub_mod_384x_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_sub_mod_384x_body::
DB	1,0,17,0
DB	000h,0f4h,003h,000h
DB	000h,0e4h,004h,000h
DB	000h,0d4h,005h,000h
DB	000h,0c4h,006h,000h
DB	000h,034h,007h,000h
DB	000h,054h,008h,000h
DB	000h,074h,00ah,000h
DB	000h,064h,00bh,000h
DB	000h,082h
DB	000h,000h
$L$SEH_info_sub_mod_384x_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_mul_by_1_plus_i_mod_384x_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_mul_by_1_plus_i_mod_384x_body::
DB	1,0,17,0
DB	000h,0f4h,007h,000h
DB	000h,0e4h,008h,000h
DB	000h,0d4h,009h,000h
DB	000h,0c4h,00ah,000h
DB	000h,034h,00bh,000h
DB	000h,054h,00ch,000h
DB	000h,074h,00eh,000h
DB	000h,064h,00fh,000h
DB	000h,0c2h
DB	000h,000h
$L$SEH_info_mul_by_1_plus_i_mod_384x_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_sgn0_pty_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_sgn0_pty_mod_384_body::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h
$L$SEH_info_sgn0_pty_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h

$L$SEH_info_sgn0_pty_mod_384x_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_sgn0_pty_mod_384x_body::
DB	1,0,9,0
DB	000h,034h,001h,000h
DB	000h,054h,002h,000h
DB	000h,074h,004h,000h
DB	000h,064h,005h,000h
DB	000h,022h
DB	000h,000h
$L$SEH_info_sgn0_pty_mod_384x_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h


.xdata	ENDS
END
