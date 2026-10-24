package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPercent(t *testing.T) {
	for _, tc := range []struct {
		p Percent
		s string
	}{
		{100, "1.00"},
		{111, "1.11"},
		{12, "0.12"},
		{-12, "-0.12"},
		{1012, "10.12"},
		{-1012, "-10.12"},
		{0, "0.00"},
		{5, "0.05"},
		{1, "0.01"},
		{105, "1.05"},
		{-5, "-0.05"},
	} {
		t.Run(fmt.Sprintf("%d <> %s", tc.p, tc.s), func(t *testing.T) {
			m, err := tc.p.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tc.s, string(m))
			var p Percent
			require.NoError(t, p.UnmarshalJSON([]byte(tc.s)))
			require.Equal(t, tc.p, p)
		})
	}

}
