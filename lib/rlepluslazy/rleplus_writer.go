package rlepluslazy

import (
	"encoding/binary"
)

func EncodeRuns(rit RunIterator, buf []byte) ([]byte, error) {
	bv := writeBitvec(buf)
	bv.Put(0, 2)

	first := true
	varBuf := make([]byte, binary.MaxVarintLen64)

	for rit.HasNext() {
		run, err := rit.NextRun()
		if err != nil {
			return nil, err
		}

		if first {
			if run.Val {
				bv.Put(1, 1)
			} else {
				bv.Put(0, 1)
			}
			first = false
		}

		switch {
		case run.Len == 1:
			bv.Put(1, 1)
		case run.Len < 16:
			bv.Put(2, 2)
			bv.Put(byte(run.Len), 4)
		case run.Len >= 16:
			bv.Put(0, 2)
			numBytes := binary.PutUvarint(varBuf, run.Len)
			for i := 0; i < numBytes; i++ {
				bv.Put(varBuf[i], 8)
			}
		}

	}

	if first {
		bv.Put(0, 1)
	}

	return bv.Out(), nil

}
