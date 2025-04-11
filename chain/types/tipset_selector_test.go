package types_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestTipSetSelector_Marshalling(t *testing.T) {
	for _, test := range []struct {
		name     string
		subject  types.TipSetSelector
		wantJson string
		wantErr  string
	}{
		{
			name:    "zero-valued",
			wantErr: "exactly one tipset selection criteria must be specified, found: 0",
		},
		{
			name:     "height",
			subject:  types.TipSetSelectors.Height(123, true, nil),
			wantJson: `{"height":{"at":123,"previous":true}}`,
		},
		{
			name:     "height anchored to finalized",
			subject:  types.TipSetSelectors.Height(123, true, types.TipSetAnchors.Finalized),
			wantJson: `{"height":{"at":123,"previous":true,"anchor":{"tag":"finalized"}}}`,
		},
		{
			name: "invalid height anchor",
			subject: types.TipSetSelectors.Height(123, true, &types.TipSetAnchor{
				Tag: &types.TipSetTags.Finalized,
				Key: &types.TipSetKey{},
			}),
			wantErr: "at most one of key or tag",
		},
		{
			name: "height with no epoch",
			subject: types.TipSetSelector{
				Height: &types.TipSetHeight{},
			},
			wantErr: "epoch must be specified",
		},
		{
			name:    "invalid height epoch",
			subject: types.TipSetSelectors.Height(-1, false, nil),
			wantErr: "epoch cannot be less than zero",
		},
		{
			name:     "key",
			subject:  types.TipSetSelectors.Key(types.TipSetKey{}),
			wantJson: `{"key":[]}`,
		},
		{
			name:     "tag finalized",
			subject:  types.TipSetSelectors.Finalized,
			wantJson: `{"tag":"finalized"}`,
		},
		{
			name:     "tag latest",
			subject:  types.TipSetSelectors.Latest,
			wantJson: `{"tag":"latest"}`,
		},
		{
			name:     "tag safe",
			subject:  types.TipSetSelectors.Safe,
			wantJson: `{"tag":"safe"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.subject.Validate()
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
				gotMarshalled, err := json.Marshal(test.subject)
				require.NoError(t, err)
				require.JSONEq(t, test.wantJson, string(gotMarshalled))
			}
		})
	}
}
