package types

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/lib/ptr"
)

func TestOptionalEpochSelectorArgJson(t *testing.T) {
	tc := []struct {
		arg      string
		expected EpochSelector
		err      string
	}{
		{arg: `["latest"]`, expected: EpochLatest},
		{arg: `["finalized"]`, expected: EpochFinalized},
		{arg: `[""]`, expected: EpochLatest},
		{arg: `[]`, expected: EpochLatest},
		{arg: `["invalid"]`, err: `json: invalid epoch selector ("invalid")`},
		{arg: `["latest", []]`, err: "json: too many parameters for epoch selector"},
		{arg: `["latest", "latest"]`, err: "json: too many parameters for epoch selector"},
	}

	for _, c := range tc {
		t.Run(c.arg, func(t *testing.T) {
			req := require.New(t)
			var oes OptionalEpochSelectorArg
			err := oes.UnmarshalJSON([]byte(c.arg))
			if c.err != "" {
				req.Error(err)
				req.Contains(err.Error(), c.err)
				return
			}
			req.NoError(err)
			req.Equal(c.expected, EpochSelector(oes))
		})
	}
}

var tskcids = []cid.Cid{
	cid.MustParse("bafy2bzacecesrkxghscnq7vatble2hqdvwat6ed23vdu4vvo3uuggsoaya7ki"),
	cid.MustParse("bafy2bzacebxfyh2fzoxrt6kcgc5dkaodpcstgwxxdizrww225vrhsizsfcg4g"),
	cid.MustParse("bafy2bzacedwviarjtjraqakob5pslltmuo5n3xev3nt5zylezofkbbv5jclyu"),
}

func TestTipSetKeyOrEpochSelectorJson(t *testing.T) {
	tc := []struct {
		arg      string
		expected TipSetKeyOrEpochSelector
		err      string
	}{
		{arg: `"latest"`, expected: TipSetKeyOrEpochSelector{EpochSelector: ptr.PtrTo(EpochLatest)}},
		{arg: `"finalized"`, expected: TipSetKeyOrEpochSelector{EpochSelector: ptr.PtrTo(EpochFinalized)}},
		{arg: `""`, expected: TipSetKeyOrEpochSelector{EpochSelector: ptr.PtrTo(EpochLatest)}},
		{
			arg:      `[` + string(must.One(tskcids[0].MarshalJSON())) + `,` + string(must.One(tskcids[1].MarshalJSON())) + `,` + string(must.One(tskcids[2].MarshalJSON())) + `]`,
			expected: TipSetKeyOrEpochSelector{TipSetKey: ptr.PtrTo(NewTipSetKey(tskcids...))},
		},
		{arg: `[]`, expected: TipSetKeyOrEpochSelector{TipSetKey: ptr.PtrTo(EmptyTSK)}},
		{arg: `"invalid"`, err: `json: invalid epoch selector ("invalid")`},
	}

	for _, c := range tc {
		t.Run(c.arg, func(t *testing.T) {
			req := require.New(t)
			var oes TipSetKeyOrEpochSelector
			err := oes.UnmarshalJSON([]byte(c.arg))
			if c.err != "" {
				req.Error(err)
				req.Contains(err.Error(), c.err)
				return
			}
			req.NoError(err)
			req.Equal(c.expected, oes)
		})
	}
}

func TestHeightOrEpochSelector(t *testing.T) {
	tc := []struct {
		arg      string
		expected HeightOrEpochSelector
		err      string
	}{
		{arg: `"latest"`, expected: HeightOrEpochSelector{EpochSelector: ptr.PtrTo(EpochLatest)}},
		{arg: `"finalized"`, expected: HeightOrEpochSelector{EpochSelector: ptr.PtrTo(EpochFinalized)}},
		{arg: `""`, expected: HeightOrEpochSelector{EpochSelector: ptr.PtrTo(EpochLatest)}},
		{arg: "101", expected: HeightOrEpochSelector{ChainEpoch: ptr.PtrTo(abi.ChainEpoch(101))}},
		{arg: `"invalid"`, err: `json: invalid epoch selector ("invalid")`},
		{arg: `[]`, err: `json: cannot unmarshal array into Go value of type abi.ChainEpoch`},
	}

	for _, c := range tc {
		t.Run(c.arg, func(t *testing.T) {
			req := require.New(t)
			var oes HeightOrEpochSelector
			err := oes.UnmarshalJSON([]byte(c.arg))
			if c.err != "" {
				req.Error(err)
				req.Contains(err.Error(), c.err)
				return
			}
			req.NoError(err)
			req.Equal(c.expected, oes)
		})
	}
}
