package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEscapeControl(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain", "plain"},
		{"café ✓", "café ✓"},
		{"a\x1b[31mb", `a\u001b[31mb`},
		{"a\u009b31mb", `a\u009b31mb`},
		{"a\x7fb", `a\u007fb`},
	} {
		require.Equal(t, tc.want, escapeControl(tc.in))
	}

	// JSON leaves C1 controls raw; escaping keeps the output valid JSON with the same value.
	value := "Hello\x1b[2K\u009b2K\u0085world"
	b, err := json.Marshal(map[string]string{"NetworkName": value})
	require.NoError(t, err)
	require.Contains(t, string(b), "\u009b")

	escaped := escapeControl(string(b))
	for _, r := range escaped {
		require.False(t, r < 0x20 || (r >= 0x7f && r <= 0x9f), "control character %U survived", r)
	}
	var out map[string]string
	require.NoError(t, json.Unmarshal([]byte(escaped), &out))
	require.Equal(t, value, out["NetworkName"])
}
