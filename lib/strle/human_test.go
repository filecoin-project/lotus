package strle

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
)

func TestHumanBitfield(t *testing.T) {
	check := func(ints []uint64, out string) {
		bf := bitfield.NewFromSet(ints)
		h, err := BitfieldToHumanRanges(bf)
		require.NoError(t, err)
		require.Equal(t, out, h)
	}

	check([]uint64{2, 3, 4}, "2-4")
	check([]uint64{2, 3, 4, 8, 9, 10, 11}, "2-4,8-11")
	check([]uint64{2}, "2")
	check([]uint64{0}, "0")
	check([]uint64{0, 1, 2}, "0-2")
	check([]uint64{0, 1, 5, 9, 11, 13, 14, 19}, "0-1,5,9,11,13-14,19")
}

func TestHumanRangesRejectsMalformed(t *testing.T) {
	// A range token with more than one dash (e.g. "1-2-3") used to fall through
	// the len(lr) switch with no match, leaving start/end at their zero value and
	// returning a nil error - so a malformed input silently resolved to sector 0.
	// Every malformed token must now error instead.
	for _, in := range []string{"1-2-3", "1-2-3,5", "1-", "-5", "a", "5-x", "", "1--2"} {
		_, err := HumanRangesToBitField(in)
		require.Error(t, err, "expected error for malformed input %q", in)
	}
}

func TestHumanBitfieldRoundtrip(t *testing.T) {
	check := func(ints []uint64, out string) {
		parsed, err := HumanRangesToBitField(out)
		require.NoError(t, err)

		h, err := BitfieldToHumanRanges(parsed)
		require.NoError(t, err)
		require.Equal(t, out, h)

		bf := bitfield.NewFromSet(ints)
		ins, err := bitfield.IntersectBitField(bf, parsed)
		require.NoError(t, err)

		// if intersected bitfield has the same length as both bitfields they are the same
		ic, err := ins.Count()
		require.NoError(t, err)

		pc, err := parsed.Count()
		require.NoError(t, err)

		require.Equal(t, uint64(len(ints)), ic)
		require.Equal(t, ic, pc)
	}

	check([]uint64{2, 3, 4}, "2-4")
	check([]uint64{2, 3, 4, 8, 9, 10, 11}, "2-4,8-11")
	check([]uint64{2}, "2")
	check([]uint64{0}, "0")
	check([]uint64{0, 1, 2}, "0-2")
	check([]uint64{0, 1, 5, 9, 11, 13, 14, 19}, "0-1,5,9,11,13-14,19")
}
