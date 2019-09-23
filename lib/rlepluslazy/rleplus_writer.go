package rlepluslazy

import (
	"encoding/binary"

	bitvector "github.com/filecoin-project/go-lotus/lib/rlepluslazy/internal"
)

func EncodeRuns(rit RunIterator, buf []byte) ([]byte, error) {
	v := bitvector.NewBitVector(buf[:0], bitvector.LSB0)
	v.Extend(0, 2, bitvector.LSB0) // Version

	first := true
	varBuf := make([]byte, binary.MaxVarintLen64)

	for rit.HasNext() {
		run, err := rit.NextRun()
		if err != nil {
			return nil, err
		}

		if first {
			if run.Val {
				v.Push(1)
			} else {
				v.Push(0)
			}
			first = false
		}

		switch {
		case run.Len == 1:
			v.Push(1)
		case run.Len < 16:
			v.Push(0)
			v.Push(1)
			v.Extend(byte(run.Len), 4, bitvector.LSB0)
		case run.Len >= 16:
			v.Push(0)
			v.Push(0)
			numBytes := binary.PutUvarint(varBuf, run.Len)
			for i := 0; i < numBytes; i++ {
				v.Extend(varBuf[i], 8, bitvector.LSB0)
			}
		}

	}

	if first {
		v.Push(0)
	}

	return v.Buf, nil

}
