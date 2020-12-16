	AREA	|.text|,CODE,ALIGN=8,ARM64

	ALIGN	32
|$Lone|
	DCQU	1,0,0,0,0,0,0,0



	EXPORT	|eucl_inverse_mod_384|[FUNC]
	ALIGN	32
|eucl_inverse_mod_384| PROC
	DCDU	3573752639
	stp	x29,x30,[sp,#-48]!
	add	x29,sp,#0
	stp	x19,x20,[sp,#16]
	stp	x21,x22,[sp,#32]
	sub	sp,sp,#192

	adr	x4,|$Lone|
	cmp	x3,#0
	cselne	x3,x3,x4

	ldp	x4,x5,[x1]
	ldp	x6,x7,[x1,#16]
	ldp	x8,x9,[x1,#32]

	orr	x10,x4,x5
	orr	x11,x6,x7
	orr	x12,x8,x9
	orr	x10,x10,x11
	orr	x10,x10,x12
	cbz	x10,|$Labort|			// abort if |inp|==0

	ldp	x10,x11,[x3]
	ldp	x12,x13,[x3,#16]
	ldp	x14,x15,[x3,#32]

	ldp	x16,x17,[x2]
	ldp	x19,x20,[x2,#16]
	ldp	x21,x22,[x2,#32]

	stp	x4,x5,[sp,#0]	// copy |inp| to U
	stp	x6,x7,[sp,#0+16]
	stp	x8,x9,[sp,#0+32]

	stp	x10,x11,[sp,#0+48]	// copy |one| to X
	stp	x12,x13,[sp,#0+64]
	stp	x14,x15,[sp,#0+80]

	stp	x16,x17,[sp,#96]	// copy |mod| to V
	stp	x19,x20,[sp,#96+16]
	stp	x21,x22,[sp,#96+32]

	stp	xzr,xzr,[sp,#96+48]		// clear Y
	stp	xzr,xzr,[sp,#96+64]
	stp	xzr,xzr,[sp,#96+80]
	b	|$Loop_inv|

	ALIGN	32
|$Loop_inv|
	add	x1,sp,#96
	bl	__remove_powers_of_2

	add	x1,sp,#0
	bl	__remove_powers_of_2

	ldp	x10,x11,[sp,#96]
	add	x2,sp,#96
	ldp	x12,x13,[sp,#96+16]
	subs	x4,x4,x10		// U-V
	ldp	x14,x15,[sp,#96+32]
	sbcs	x5,x5,x11
	sbcs	x6,x6,x12
	sbcs	x7,x7,x13
	sbcs	x8,x8,x14
	sbcs	x9,x9,x15
	bhs	|$Lu_greater_than_v|

	eor	x2,x2,x1		// xchg	x2,x1
	mvn	x4,x4			// U-V => V-U
	eor	x1,x1,x2
	mvn	x5,x5
	eor	x2,x2,x1
	adds	x4,x4,#1
	mvn	x6,x6
	adcs	x5,x5,xzr
	mvn	x7,x7
	adcs	x6,x6,xzr
	mvn	x8,x8
	adcs	x7,x7,xzr
	mvn	x9,x9
	adcs	x8,x8,xzr
	adc	x9,x9,xzr

|$Lu_greater_than_v|
	stp	x4,x5,[x1]
	ldp	x4,x5,[x2,#48]
	ldp	x10,x11,[x1,#48]
	stp	x6,x7,[x1,#16]
	ldp	x6,x7,[x2,#64]
	subs	x10,x10,x4		// X-Y		# [alt. Y-X]
	ldp	x12,x13,[x1,#64]
	sbcs	x11,x11,x5
	stp	x8,x9,[x1,#32]
	ldp	x8,x9,[x2,#80]
	sbcs	x12,x12,x6
	ldp	x14,x15,[x1,#80]
	sbcs	x13,x13,x7
	sbcs	x14,x14,x8
	sbcs	x15,x15,x9
	sbc	x9,xzr,xzr			// borrow -> mask

	and	x4,x16,x9
	and	x5,x17,x9
	adds	x10,x10,x4		// reduce if X<Y # [alt. Y<X]
	and	x6,x19,x9
	adcs	x11,x11,x5
	and	x7,x20,x9
	adcs	x12,x12,x6
	and	x8,x21,x9
	adcs	x13,x13,x7
	and	x9,x22,x9
	ldp	x4,x5,[sp,#0]
	adcs	x14,x14,x8
	ldp	x6,x7,[sp,#0+16]
	adc	x15,x15,x9
	ldp	x8,x9,[sp,#0+32]

	orr	x4,x4,x5
	orr	x6,x6,x7
	orr	x8,x8,x9
	stp	x10,x11,[x1,#48]
	orr	x4,x4,x6
	stp	x12,x13,[x1,#64]
	orr	x4,x4,x8
	stp	x14,x15,[x1,#80]

	cbnz	x4,|$Loop_inv|		// U!=0?

	ldr	x30,[x29,#8]
	ldp	x4,x5,[sp,#96+48]	// return Y
	ldp	x6,x7,[sp,#96+64]
	ldp	x8,x9,[sp,#96+80]
	mov	x10,#1

|$Labort|
	stp	x4,x5,[x0]
	stp	x6,x7,[x0,#16]
	stp	x8,x9,[x0,#32]
	mov	x0,x10			// boolean return value

	add	sp,sp,#192
	ldp	x19,x20,[x29,#16]
	ldp	x21,x22,[x29,#32]
	ldr	x29,[sp],#48
	DCDU	3573752767
	ret
	ENDP


	ALIGN	16
|__remove_powers_of_2| PROC
	ldp	x4,x5,[x1]
	ldp	x6,x7,[x1,#16]
	ldp	x8,x9,[x1,#32]
	nop

|$Loop_of_2|
	rbit	x3,x4
	tbnz	x4,#0,|$Loop_of_2_done|

	clz	x3,x3
	cmp	x4,#0
	mov	x10,#63
	cselne	x3,x3,x10

	neg	x15,x3
	lsrv	x4,x4,x3		// acc[0:5] >>= cnt
	lslv	x10,x5,x15
	orr	x4,x4,x10
	lsrv	x5,x5,x3
	lslv	x11,x6,x15
	orr	x5,x5,x11
	ldp	x10,x11,[x1,#48]
	lsrv	x6,x6,x3
	lslv	x12,x7,x15
	orr	x6,x6,x12
	lsrv	x7,x7,x3
	lslv	x13,x8,x15
	orr	x7,x7,x13
	ldp	x12,x13,[x1,#64]
	lsrv	x8,x8,x3
	lslv	x14,x9,x15
	orr	x8,x8,x14
	ldp	x14,x15,[x1,#80]
	lsrv	x9,x9,x3

	stp	x4, x5,[x1]
	stp	x6, x7,[x1,#16]
	stp	x8, x9,[x1,#32]
	b	|$Loop_div_by_2|

	ALIGN	16
|$Loop_div_by_2|
	sbfx	x9,x10,#0,#1
	sub	x3,x3,#1

	and	x4,x16,x9
	and	x5,x17,x9
	adds	x10,x10,x4
	and	x6,x19,x9
	adcs	x11,x11,x5
	and	x7,x20,x9
	adcs	x12,x12,x6
	and	x8,x21,x9
	adcs	x13,x13,x7
	and	x9,x22,x9
	adcs	x14,x14,x8
	extr	x10,x11,x10,#1	// acc[6:11] >>= 1
	adcs	x15,x15,x9
	extr	x11,x12,x11,#1
	adc	x9,xzr,xzr		// redundant if modulus is <384 bits...
	extr	x12,x13,x12,#1
	extr	x13,x14,x13,#1
	extr	x14,x15,x14,#1
	extr	x15,x9,x15,#1

	cbnz	x3,|$Loop_div_by_2|

	ldp	x4,x5,[x1] // reload X [mostly for 2nd caller]
	ldp	x6,x7,[x1,#16]
	ldp	x8,x9,[x1,#32]

	stp	x10,x11,[x1,#48]
	stp	x12,x13,[x1,#64]
	stp	x14,x15,[x1,#80]

	tbz	x4,#0,|$Loop_of_2|	// unlikely in real life

|$Loop_of_2_done|
	ret
	ENDP
	END
