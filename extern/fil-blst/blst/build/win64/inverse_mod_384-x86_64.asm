OPTION	DOTNAME
.text$	SEGMENT ALIGN(256) 'CODE'

ALIGN	32
$L$one::
	DQ	1,0,0,0,0,0,0,0

PUBLIC	eucl_inverse_mod_384


ALIGN	32
eucl_inverse_mod_384	PROC PUBLIC
	DB	243,15,30,250
	mov	QWORD PTR[8+rsp],rdi	;WIN64 prologue
	mov	QWORD PTR[16+rsp],rsi
	mov	r11,rsp
$L$SEH_begin_eucl_inverse_mod_384::
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

	sub	rsp,216

$L$SEH_body_eucl_inverse_mod_384::


	mov	QWORD PTR[rsp],rdi
	lea	rbp,QWORD PTR[$L$one]
	cmp	rcx,0
	cmove	rcx,rbp

	mov	rax,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

	mov	r8,rax
	or	rax,r9
	or	rax,r10
	or	rax,r11
	or	rax,r12
	or	rax,r13
	jz	$L$abort

	lea	rsi,QWORD PTR[16+rsp]
	mov	r14,QWORD PTR[rcx]
	mov	r15,QWORD PTR[8+rcx]
	mov	rax,QWORD PTR[16+rcx]
	mov	rbx,QWORD PTR[24+rcx]
	mov	rbp,QWORD PTR[32+rcx]
	mov	rdi,QWORD PTR[40+rcx]

	mov	QWORD PTR[rsi],r8
	mov	QWORD PTR[8+rsi],r9
	mov	QWORD PTR[16+rsi],r10
	mov	QWORD PTR[24+rsi],r11
	mov	QWORD PTR[32+rsi],r12
	mov	QWORD PTR[40+rsi],r13

	lea	rcx,QWORD PTR[112+rsp]
	mov	r8,QWORD PTR[rdx]
	mov	r9,QWORD PTR[8+rdx]
	mov	r10,QWORD PTR[16+rdx]
	mov	r11,QWORD PTR[24+rdx]
	mov	r12,QWORD PTR[32+rdx]
	mov	r13,QWORD PTR[40+rdx]

	mov	QWORD PTR[48+rsi],r14
	mov	QWORD PTR[56+rsi],r15
	mov	QWORD PTR[64+rsi],rax
	mov	QWORD PTR[72+rsi],rbx
	mov	QWORD PTR[80+rsi],rbp
	mov	QWORD PTR[88+rsi],rdi

	mov	QWORD PTR[rcx],r8
	mov	QWORD PTR[8+rcx],r9
	mov	QWORD PTR[16+rcx],r10
	mov	QWORD PTR[24+rcx],r11
	mov	QWORD PTR[32+rcx],r12
	mov	QWORD PTR[40+rcx],r13

	xor	eax,eax
	mov	QWORD PTR[48+rcx],rax
	mov	QWORD PTR[56+rcx],rax
	mov	QWORD PTR[64+rcx],rax
	mov	QWORD PTR[72+rcx],rax
	mov	QWORD PTR[80+rcx],rax
	mov	QWORD PTR[88+rcx],rax
	jmp	$L$oop_inv

ALIGN	32
$L$oop_inv::
	lea	rsi,QWORD PTR[112+rsp]
	call	__remove_powers_of_2

	lea	rsi,QWORD PTR[16+rsp]
	call	__remove_powers_of_2

	lea	rcx,QWORD PTR[112+rsp]
	sub	r8,QWORD PTR[((112+0))+rsp]
	sbb	r9,QWORD PTR[8+rcx]
	sbb	r10,QWORD PTR[16+rcx]
	sbb	r11,QWORD PTR[24+rcx]
	sbb	r12,QWORD PTR[32+rcx]
	sbb	r13,QWORD PTR[40+rcx]
	jae	$L$u_greater_than_v


	xchg	rsi,rcx

	not	r8
	not	r9
	not	r10
	not	r11
	not	r12
	not	r13

	add	r8,1
	adc	r9,0
	adc	r10,0
	adc	r11,0
	adc	r12,0
	adc	r13,0

$L$u_greater_than_v::
	mov	r14,QWORD PTR[48+rsi]
	mov	r15,QWORD PTR[56+rsi]
	mov	rax,QWORD PTR[64+rsi]
	mov	rbx,QWORD PTR[72+rsi]
	mov	rbp,QWORD PTR[80+rsi]
	mov	rdi,QWORD PTR[88+rsi]

	sub	r14,QWORD PTR[48+rcx]
	sbb	r15,QWORD PTR[56+rcx]
	sbb	rax,QWORD PTR[64+rcx]
	sbb	rbx,QWORD PTR[72+rcx]
	sbb	rbp,QWORD PTR[80+rcx]
	sbb	rdi,QWORD PTR[88+rcx]

	mov	QWORD PTR[rsi],r8
	sbb	r8,r8
	mov	QWORD PTR[8+rsi],r9
	mov	r9,r8
	mov	QWORD PTR[16+rsi],r10
	mov	r10,r8
	mov	QWORD PTR[24+rsi],r11
	mov	r11,r8
	mov	QWORD PTR[32+rsi],r12
	mov	r12,r8
	mov	QWORD PTR[40+rsi],r13
	mov	r13,r8

	and	r8,QWORD PTR[rdx]
	and	r9,QWORD PTR[8+rdx]
	and	r10,QWORD PTR[16+rdx]
	and	r11,QWORD PTR[24+rdx]
	and	r12,QWORD PTR[32+rdx]
	and	r13,QWORD PTR[40+rdx]

	add	r14,r8
	adc	r15,r9
	adc	rax,r10
	adc	rbx,r11
	adc	rbp,r12
	adc	rdi,r13

	mov	QWORD PTR[48+rsi],r14
	mov	QWORD PTR[56+rsi],r15
	mov	QWORD PTR[64+rsi],rax
	mov	QWORD PTR[72+rsi],rbx
	mov	QWORD PTR[80+rsi],rbp
	mov	QWORD PTR[88+rsi],rdi

	mov	r8,QWORD PTR[((16+0))+rsp]
	mov	r9,QWORD PTR[((16+8))+rsp]
	mov	r10,QWORD PTR[((16+16))+rsp]
	mov	r11,QWORD PTR[((16+24))+rsp]
	or	r8,r9
	or	r10,QWORD PTR[((16+32))+rsp]
	or	r11,QWORD PTR[((16+40))+rsp]
DB	067h
	or	r8,r10
	or	r8,r11
	jnz	$L$oop_inv

	lea	rsi,QWORD PTR[112+rsp]
	mov	rdi,QWORD PTR[rsp]
	mov	eax,1

	mov	r8,QWORD PTR[48+rsi]
	mov	r9,QWORD PTR[56+rsi]
	mov	r10,QWORD PTR[64+rsi]
	mov	r11,QWORD PTR[72+rsi]
	mov	r12,QWORD PTR[80+rsi]
	mov	r13,QWORD PTR[88+rsi]

$L$abort::
	mov	QWORD PTR[rdi],r8
	mov	QWORD PTR[8+rdi],r9
	mov	QWORD PTR[16+rdi],r10
	mov	QWORD PTR[24+rdi],r11
	mov	QWORD PTR[32+rdi],r12
	mov	QWORD PTR[40+rdi],r13

	lea	r8,QWORD PTR[216+rsp]
	mov	r15,QWORD PTR[r8]

	mov	r14,QWORD PTR[8+r8]

	mov	r13,QWORD PTR[16+r8]

	mov	r12,QWORD PTR[24+r8]

	mov	rbx,QWORD PTR[32+r8]

	mov	rbp,QWORD PTR[40+r8]

	lea	rsp,QWORD PTR[48+r8]

$L$SEH_epilogue_eucl_inverse_mod_384::
	mov	rdi,QWORD PTR[8+rsp]	;WIN64 epilogue
	mov	rsi,QWORD PTR[16+rsp]

	DB	0F3h,0C3h		;repret

$L$SEH_end_eucl_inverse_mod_384::
eucl_inverse_mod_384	ENDP


ALIGN	32
__remove_powers_of_2	PROC PRIVATE
	DB	243,15,30,250
	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

$L$oop_of_2::
	bsf	rcx,r8
	mov	eax,63
	cmovz	ecx,eax

	cmp	ecx,0
	je	$L$oop_of_2_done

	shr	r8,cl
	mov	r14,r9
	shr	r9,cl
	mov	r15,r10
	shr	r10,cl
	mov	rax,r11
	shr	r11,cl
	mov	rbx,r12
	shr	r12,cl
	mov	rbp,r13
	shr	r13,cl
	neg	cl
	shl	r14,cl
	shl	r15,cl
	or	r8,r14
	mov	r14,QWORD PTR[48+rsi]
	shl	rax,cl
	or	r9,r15
	mov	r15,QWORD PTR[56+rsi]
	shl	rbx,cl
	or	r10,rax
	mov	rax,QWORD PTR[64+rsi]
	shl	rbp,cl
	or	r11,rbx
	mov	rbx,QWORD PTR[72+rsi]
	or	r12,rbp
	mov	rbp,QWORD PTR[80+rsi]
	neg	cl
	mov	rdi,QWORD PTR[88+rsi]

	mov	QWORD PTR[rsi],r8
	mov	QWORD PTR[8+rsi],r9
	mov	QWORD PTR[16+rsi],r10
	mov	QWORD PTR[24+rsi],r11
	mov	QWORD PTR[32+rsi],r12
	mov	QWORD PTR[40+rsi],r13
	jmp	$L$oop_div_by_2

ALIGN	32
$L$oop_div_by_2::
	mov	r13,1
	mov	r8,QWORD PTR[rdx]
	and	r13,r14
	mov	r9,QWORD PTR[8+rdx]
	neg	r13
	mov	r10,QWORD PTR[16+rdx]
	and	r8,r13
	mov	r11,QWORD PTR[24+rdx]
	and	r9,r13
	mov	r12,QWORD PTR[32+rdx]
	and	r10,r13
	and	r11,r13
	and	r12,r13
	and	r13,QWORD PTR[40+rdx]

	add	r14,r8
	adc	r15,r9
	adc	rax,r10
	adc	rbx,r11
	adc	rbp,r12
	adc	rdi,r13
	sbb	r13,r13

	shr	r14,1
	mov	r8,r15
	shr	r15,1
	mov	r9,rax
	shr	rax,1
	mov	r10,rbx
	shr	rbx,1
	mov	r11,rbp
	shr	rbp,1
	mov	r12,rdi
	shr	rdi,1
	shl	r8,63
	shl	r9,63
	or	r14,r8
	shl	r10,63
	or	r15,r9
	shl	r11,63
	or	rax,r10
	shl	r12,63
	or	rbx,r11
	shl	r13,63
	or	rbp,r12
	or	rdi,r13

	dec	ecx
	jnz	$L$oop_div_by_2

	mov	r8,QWORD PTR[rsi]
	mov	r9,QWORD PTR[8+rsi]
	mov	r10,QWORD PTR[16+rsi]
	mov	r11,QWORD PTR[24+rsi]
	mov	r12,QWORD PTR[32+rsi]
	mov	r13,QWORD PTR[40+rsi]

	mov	QWORD PTR[48+rsi],r14
	mov	QWORD PTR[56+rsi],r15
	mov	QWORD PTR[64+rsi],rax
	mov	QWORD PTR[72+rsi],rbx
	mov	QWORD PTR[80+rsi],rbp
	mov	QWORD PTR[88+rsi],rdi

	test	r8,1
DB	02eh
	jz	$L$oop_of_2

$L$oop_of_2_done::
	DB	0F3h,0C3h		;repret
__remove_powers_of_2	ENDP
.text$	ENDS
.pdata	SEGMENT READONLY ALIGN(4)
ALIGN	4
	DD	imagerel $L$SEH_begin_eucl_inverse_mod_384
	DD	imagerel $L$SEH_body_eucl_inverse_mod_384
	DD	imagerel $L$SEH_info_eucl_inverse_mod_384_prologue

	DD	imagerel $L$SEH_body_eucl_inverse_mod_384
	DD	imagerel $L$SEH_epilogue_eucl_inverse_mod_384
	DD	imagerel $L$SEH_info_eucl_inverse_mod_384_body

	DD	imagerel $L$SEH_epilogue_eucl_inverse_mod_384
	DD	imagerel $L$SEH_end_eucl_inverse_mod_384
	DD	imagerel $L$SEH_info_eucl_inverse_mod_384_epilogue

.pdata	ENDS
.xdata	SEGMENT READONLY ALIGN(8)
ALIGN	8
$L$SEH_info_eucl_inverse_mod_384_prologue::
DB	1,0,5,00bh
DB	0,074h,1,0
DB	0,064h,2,0
DB	0,003h
DB	0,0
$L$SEH_info_eucl_inverse_mod_384_body::
DB	1,0,18,0
DB	000h,0f4h,01bh,000h
DB	000h,0e4h,01ch,000h
DB	000h,0d4h,01dh,000h
DB	000h,0c4h,01eh,000h
DB	000h,034h,01fh,000h
DB	000h,054h,020h,000h
DB	000h,074h,022h,000h
DB	000h,064h,023h,000h
DB	000h,001h,021h,000h
$L$SEH_info_eucl_inverse_mod_384_epilogue::
DB	1,0,4,0
DB	000h,074h,001h,000h
DB	000h,064h,002h,000h
DB	000h,000h,000h,000h


.xdata	ENDS
END
