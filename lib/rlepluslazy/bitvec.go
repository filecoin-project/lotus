package rlepluslazy

type rbitvec struct {
	index int

	bits   uint16
	bitCap byte

	vec []byte
}

func readBitvec(vec []byte) *rbitvec {
	bv := &rbitvec{vec: vec}
	if len(vec) > 0 {
		bv.bits = uint16(bv.vec[0])
	}
	return bv
}

const (
	minCap = 8
	maxCap = 16
)

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
	res := byte(bv.bits) & bitMasks[count]
	bv.bits = bv.bits >> count
	bv.bitCap = bv.bitCap - count

	if bv.index < len(bv.vec) {
		bv.bits = bv.bits | uint16(bv.vec[bv.index])<<bv.bitCap
	}
	// Here be dragons
	inc := (bv.bitCap - 8) >> 7 // inc == 1 iff bitcap<8 (+10% perf)
	bv.index = bv.index + int(inc)
	bv.bitCap = bv.bitCap + inc<<3

	return res
}

func writeBitvec(buf []byte) *wbitvec {
	return &wbitvec{buf: buf[:0]}
}

type wbitvec struct {
	buf   []byte
	index int

	bits   uint16
	bitCap byte
}

func (bv *wbitvec) Out() []byte {
	if bv.bitCap != 0 {
		bv.buf = append(bv.buf, 0)[:bv.index+1]
		bv.buf[bv.index] = byte(bv.bits)
	}
	if bv.bitCap > 8 {
		bv.buf = append(bv.buf, byte(bv.bitCap>>8))
	}
	return bv.buf
}

func (bv *wbitvec) Put(val byte, count byte) {
	bv.bits = bv.bits | uint16(val)<<bv.bitCap
	bv.bitCap = bv.bitCap + count

	bv.buf = append(bv.buf, 0)[:bv.index+1] // increase cap, keep len
	bv.buf[bv.index] = byte(bv.bits)

	// Warning, dragons again
	inc := (^(bv.bitCap - 8)) >> 7 // inc == 1 iff bitcap>=8
	bv.index = bv.index + int(inc)
	bv.bitCap = bv.bitCap - inc<<3
	bv.bits = bv.bits >> (inc << 3)
}
