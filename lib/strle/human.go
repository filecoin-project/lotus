package strle

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
)

func HumanRangesToBitField(h string) (bitfield.BitField, error) {
	var runs []rlepluslazy.Run
	var last uint64

	strRanges := strings.Split(h, ",")
	for i, strRange := range strRanges {
		lr := strings.Split(strRange, "-")

		var start, end uint64
		var err error

		switch len(lr) {
		case 1: // one number
			start, err = strconv.ParseUint(lr[0], 10, 64)
			if err != nil {
				return bitfield.BitField{}, xerrors.Errorf("parsing left side of run %d: %w", i, err)
			}

			end = start
		case 2: // x-y
			start, err = strconv.ParseUint(lr[0], 10, 64)
			if err != nil {
				return bitfield.BitField{}, xerrors.Errorf("parsing left side of run %d: %w", i, err)
			}
			end, err = strconv.ParseUint(lr[1], 10, 64)
			if err != nil {
				return bitfield.BitField{}, xerrors.Errorf("parsing right side of run %d: %w", i, err)
			}
		}

		if start <= last && last > 0 {
			return bitfield.BitField{}, xerrors.Errorf("run %d start(%d) was equal to last run end(%d)", i, start, last)
		}

		if start > end {
			return bitfield.BitField{}, xerrors.Errorf("run start(%d) can't be greater than run end(%d) (run %d)", start, end, i)
		}

		if start > last {
			runs = append(runs, rlepluslazy.Run{Val: false, Len: start - last})
		}

		runs = append(runs, rlepluslazy.Run{Val: true, Len: end - start + 1})
		last = end + 1
	}

	return bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: runs})
}

func BitfieldToHumanRanges(bf bitfield.BitField) (string, error) {
	bj, err := bf.MarshalJSON()
	if err != nil {
		return "", err
	}

	var bints []int64
	if err := json.Unmarshal(bj, &bints); err != nil {
		return "", err
	}

	var at int64
	var out string

	for i, bi := range bints {
		at += bi

		if i%2 == 0 {
			if i > 0 {
				out += ","
			}
			out += fmt.Sprint(at)
			continue
		}

		if bi > 1 {
			out += "-"
			out += fmt.Sprint(at - 1)
		}
	}

	return out, err
}
