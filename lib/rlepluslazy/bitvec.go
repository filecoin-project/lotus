package rlepluslazy

type rbitvec struct {
	index int

	bits   uint16
	bitCap byte

	vec []byte
}

func readBitvec(vec []byte) *rbitvec {
	bv := &rbitvec{
		vec:    vec,
		index:  1,
		bitCap: 8,
	}
	if len(vec) > 0 {
		bv.bits = uint16(bv.vec[0])
	}
	return bv
}

// bitMasks is a mask for selecting N first bits out of a byte
var bitMasks = [9]byte{
	0x0,
	0x1,
	0x3,
	0x7,
	0xF,
	0x1F,
	0x3F,
	0x7F,
	0xFF,
}

func (bv *rbitvec) Get(count byte) byte {
	res := byte(bv.bits) & bitMasks[count] // select count bits
	bv.bits = bv.bits >> count             // remove those bits from storage
	bv.bitCap = bv.bitCap - count          // decrease nuber of stored bits

	if bv.index < len(bv.vec) { // if vector allows
		// add bits onto the end of temporary storage
		bv.bits = bv.bits | uint16(bv.vec[bv.index])<<bv.bitCap
	}

	// Here be dragons
	// This is equivalent to
	// if bv.bitCap < 8 {
	//     bv.index++
	//     bv.bitCap = bv.bitCap + 8
	// }
	// but implemented without branches because the branch here is unpredictable
	// Why this is without branches and reading has branch?
	//  Because branch above is predictable, in 99.99% of cases it will be true

	// if bitCap < 8 it underflows, then high bits get set to 1s
	// we shift by 7 so the highest bit is in place of the lowest
	inc := (bv.bitCap - 8) >> 7    // inc == 1 iff bitcap<8 (+10% perf)
	bv.index = bv.index + int(inc) // increase index if we need more bits
	bv.bitCap = bv.bitCap + inc*8  // increase bitCap by 8

	return res
}

func writeBitvec(buf []byte) *wbitvec {
	// reslice to 0 length for consistent input but to keep capacity
	return &wbitvec{buf: buf[:0]}
}

type wbitvec struct {
	buf   []byte // buffer we will be saving to
	index int    // index of at which the next byte will be saved

	bits   uint16 // temporary storage for bits
	bitCap byte   // number of bits stored in temporary storage
}

func (bv *wbitvec) Out() []byte {
	if bv.bitCap != 0 {
		// if there are some bits in temporary storage we need to save them
		bv.buf = append(bv.buf, 0)[:bv.index+1]
		bv.buf[bv.index] = byte(bv.bits)
	}
	if bv.bitCap > 8 {
		// if we store some needed bits in second byte, save them also
		bv.buf = append(bv.buf, byte(bv.bitCap>>8))
		bv.index++
		bv.bits = bv.bits - 8
	}
	return bv.buf
}

func (bv *wbitvec) Put(val byte, count byte) {
	// put val into its place in bv.bits
	bv.bits = bv.bits | uint16(val)<<bv.bitCap
	// increase bitCap by the number of bits
	bv.bitCap = bv.bitCap + count

	// increase len of the buffer if it is needed
	if bv.index+1 > cap(bv.buf) {
		bv.buf = append(bv.buf, 0)
	}
	bv.buf = bv.buf[:bv.index+1]
	// save the bits
	bv.buf[bv.index] = byte(bv.bits)

	// Warning, dragons again
	// if bitCap is greater than 7 it underflows, same thing as in Put
	inc := (7 - bv.bitCap) >> 7    // inc == 1 iff bitcap>=8
	bv.index = bv.index + int(inc) // increase index for the next save
	bv.bitCap = bv.bitCap - inc*8  // we store less bits now in temporary buffer
	bv.bits = bv.bits >> (inc * 8) // we can discard those bits as they were saved
}
