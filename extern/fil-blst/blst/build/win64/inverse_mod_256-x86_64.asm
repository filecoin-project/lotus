OPTION	DOTNAME
.text$	SEGMENT ALIGN(256) 'CODE'

ALIGN	32
$L$one_256::
	DQ	1,0,0,0

PUBLIC	eucl_inverse_mod_256


ALIGN	32
eucl_inverse_mod_256	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_eucl_inverse_mod_256::
	mov	rdi,rcx
	mov	rsi,rdx
	mov	rdx,r8
	mov	rcx,r9



	push	rbp

	push	rbx

	sub	rsp,152

$L$SEH_body_eucl_inverse_mod_256::


	mov	QWORD PTR[rsp],rdi
	lea	rbp,QWORD PTR[$L$one_256]
	cmp	rcx,0
	cmove	rcx,rbp

	mov	rax,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]

	mov	r8,rax
	or	rax,r9
	or	rax,r10
	or	rax,r11
	jz	$L$abort_256

	lea	rsi,QWORD PTR[16+rsp]
	mov	rax,QWORD PTR[rcx]
	mov	rbx,QWORD PTR[8+rcx]
	mov	rbp,QWORD PTR[16+rcx]
	mov	rdi,QWORD PTR[24+rcx]

	mov	QWORD PTR[rsi],r8
	mov	QWORD PTR[8+rsi],r9
	mov	QWORD PTR[16+rsi],r10
	mov	QWORD PTR[24+rsi],r11

	lea	rcx,QWORD PTR[80+rsp]
	mov	r8,QWORD PTR[rdx]
	mov	r9,QWORD PTR[8+rdx]
	mov	r10,QWORD PTR[16+rdx]
	mov	r11,QWORD PTR[24+rdx]

	mov	QWORD PTR[32+rsi],rax
	mov	QWORD PTR[40+rsi],rbx
	mov	QWORD PTR[48+rsi],rbp
	mov	QWORD PTR[56+rsi],rdi

	mov	QWORD PTR[rcx],r8
	mov	QWORD PTR[8+rcx],r9
	mov	QWORD PTR[16+rcx],r10
	mov	QWORD PTR[24+rcx],r11

	xor	eax,eax
	mov	QWORD PTR[32+rcx],rax
	mov	QWORD PTR[40+rcx],rax
	mov	QWORD PTR[48+rcx],rax
	mov	QWORD PTR[56+rcx],rax
	jmp	$L$oop_inv_256

ALIGN	32
$L$oop_inv_256::
	lea	rsi,QWORD PTR[80+rsp]
	call	__remove_powers_of_2_256

	lea	rsi,QWORD PTR[16+rsp]
	call	__remove_powers_of_2_256

	lea	rcx,QWORD PTR[80+rsp]
	sub	r8,QWORD PTR[((80+0))+rsp]
	sbb	r9,QWORD PTR[8+rcx]
	sbb	r10,QWORD PTR[16+rcx]
	sbb	r11,QWORD PTR[24+rcx]
	jae	$L$u_greater_than_v_256


	xchg	rsi,rcx

	not	r8
	not	r9
	not	r10
	not	r11

	add	r8,1
	adc	r9,0
	adc	r10,0
	adc	r11,0

$L$u_greater_than_v_256::
	mov	rax,QWORD PTR[32+rsi]
	mov	rbx,QWORD PTR[40+rsi]
	mov	rbp,QWORD PTR[48+rsi]
	mov	rdi,QWORD PTR[56+rsi]

	sub	rax,QWORD PTR[32+rcx]
	sbb	rbx,QWORD PTR[40+rcx]
	sbb	rbp,QWORD PTR[48+rcx]
	sbb	rdi,QWORD PTR[56+rcx]

	mov	QWORD PTR[rsi],r8
	sbb	r8,r8
	mov	QWORD PTR[8+rsi],r9
	mov	r9,r8
	mov	QWORD PTR[16+rsi],r10
	mov	r10,r8
	mov	QWORD PTR[24+rsi],r11
	mov	r11,r8

	and	r8,QWORD PTR[rdx]
	and	r9,QWORD PTR[8+rdx]
	and	r10,QWORD PTR[16+rdx]
	and	r11,QWORD PTR[24+rdx]

	add	rax,r8
	adc	rbx,r9
	adc	rbp,r10
	adc	rdi,r11

	mov	QWORD PTR[32+rsi],rax
	mov	QWORD PTR[40+rsi],rbx
	mov	QWORD PTR[48+rsi],rbp
	mov	QWORD PTR[56+rsi],rdi

	mov	r8,QWORD PTR[((16+0))+rsp]
	mov	r9,QWORD PTR[((16+8))+rsp]
	mov	r10,QWORD PTR[((16+16))+rsp]
	mov	r11,QWORD PTR[((16+24))+rsp]
	or	r8,r9
	or	r8,r10
	or	r8,r11
	jnz	$L$oop_inv_256

	lea	rsi,QWORD PTR[80+rsp]
	mov	rdi,QWORD PTR[rsp]
	mov	eax,1

	mov	r8,QWORD PTR[32+rsi]
	mov	r9,QWORD PTR[40+rsi]
	mov	r10,QWORD PTR[48+rsi]
	mov	r11,QWORD PTR[56+rsi]

$L$abort_256::
	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11

	lea	r8,QWORD PTR[152+rsp]
	mov	rbx,QWORD PTR[r8]

	mov	rbp,QWORD PTR[8+r8]

	lea	rsp,QWORD PTR[16+r8]

$L$SEH_epilogue_eucl_inverse_mod_256::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_eucl_inverse_mod_256::
eucl_inverse_mod_256	ENDP


ALIGN	32
__remove_powers_of_2_256	PROC PRIVATE
	DB	243,15,30,250
	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]

$L$oop_of_2_256::
	bsf	rcx,r8
	mov	eax,63
	cmovz	ecx,eax

	cmp	ecx,0
	je	$L$oop_of_2_done_256

	shr	r8,cl
	mov	rax,r9
	shr	r9,cl
	mov	rbx,r10
	shr	r10,cl
	mov	rbp,r11
	shr	r11,cl
	neg	cl
	shl	rax,cl
	shl	rbx,cl
	or	r8,rax
	mov	rax,QWORD PTR[32+rsi]
	shl	rbp,cl
	or	r9,rbx
	mov	rbx,QWORD PTR[40+rsi]
	or	r10,rbp
	mov	rbp,QWORD PTR[48+rsi]
	neg	cl
	mov	rdi,QWORD PTR[56+rsi]

	mov	QWORD PTR[rsi],r8
	mov	QWORD PTR[8+rsi],r9
	mov	QWORD PTR[16+rsi],r10
	mov	QWORD PTR[24+rsi],r11
	jmp	$L$oop_div_by_2_256

ALIGN	32
$L$oop_div_by_2_256::
	mov	r11,1
	mov	r8,QWORD PTR[rdx]
	and	r11,rax
	mov	r9,QWORD PTR[8+rdx]
	neg	r11
	mov	r10,QWORD PTR[16+rdx]
	and	r8,r11
	and	r9,r11
	and	r10,r11
	and	r11,QWORD PTR[24+rdx]

	add	rax,r8
	adc	rbx,r9
	adc	rbp,r10
	adc	rdi,r11
	sbb	r11,r11

	shr	rax,1
	mov	r8,rbx
	shr	rbx,1
	mov	r9,rbp
	shr	rbp,1
	mov	r10,rdi
	shr	rdi,1
	shl	r8,63
	shl	r9,63
	or	rax,r8
	shl	r10,63
	or	rbx,r9
	shl	r11,63
	or	rbp,r10
	or	rdi,r11

	dec	ecx
	jnz	$L$oop_div_by_2_256

	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]

	mov	QWORD PTR[32+rsi],rax
	mov	QWORD PTR[40+rsi],rbx
	mov	QWORD PTR[48+rsi],rbp
	mov	QWORD PTR[56+rsi],rdi

	test	r8,1
DB	02eh
	jz	$L$oop_of_2_256

$L$oop_of_2_done_256::
	DB	0F3h,0C3h		;repret
__remove_powers_of_2_256	ENDP
.text$	ENDS
.pdata	SEGMENT READONLY ALIGN(4)
ALIGN	4
	DD	imagerel $L$SEH_begin_eucl_inverse_mod_256
	DD	imagerel $L$SEH_body_eucl_inverse_mod_256
	DD	imagerel $L$SEH_info_eucl_inverse_mod_256_prologue

	DD	imagerel $L$SEH_body_eucl_inverse_mod_256
	DD	imagerel $L$SEH_epilogue_eucl_inverse_mod_256
	DD	imagerel $L$SEH_info_eucl_inverse_mod_256_body

	DD	imagerel $L$SEH_epilogue_eucl_inverse_mod_256
	DD	imagerel $L$SEH_end_eucl_inverse_mod_256
	DD	imagerel $L$SEH_info_eucl_inverse_mod_256_epilogue

.pdata	ENDS
.xdata	SEGMENT READONLY ALIGN(8)
ALIGN	8
$L$SEH_info_eucl_inverse_mod_256_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_eucl_inverse_mod_256_body::
DB	1,0,10,0
DB	000h,034h,013h,000h
DB	000h,054h,014h,000h
DB	000h,074h,016h,000h
DB	000h,064h,017h,000h
DB	000h,001h,015h,000h
$L$SEH_info_eucl_inverse_mod_256_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h


.xdata	ENDS
END
