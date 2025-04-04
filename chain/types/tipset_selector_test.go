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
		wantErr  bool
	}{
		{
			name:    "zero-valued",
			wantErr: true,
		},
		{
			name: "height",
			subject: types.TipSetSelector{
				Height: &types.TipSetHeight{
					At:       123,
					Previous: true,
				},
			},
			wantJson: `{"height":{"at":123,"previous":true}}`,
		},
		{
			name: "height anchored to finalized",
			subject: types.TipSetSelector{
				Height: &types.TipSetHeight{
					At:       123,
					Previous: true,
					Anchor: &types.TipSetAnchor{
						Tag: &types.TipSetTags.Finalized,
					},
				},
			},
			wantJson: `{"height":{"at":123,"previous":true,"anchor":{"tag":"finalized"}}}`,
		},
		{
			name: "invalid height anchor",
			subject: types.TipSetSelector{
				Height: &types.TipSetHeight{
					At:       123,
					Previous: true,
					Anchor: &types.TipSetAnchor{
						Tag: &types.TipSetTags.Finalized,
						Key: &types.TipSetKey{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid height epoch",
			subject: types.TipSetSelector{
				Height: &types.TipSetHeight{
					At: -7,
				},
			},
			wantErr: true,
		},
		{
			name: "key",
			subject: types.TipSetSelector{
				Key: &types.TipSetKey{},
			},
			wantJson: `{"key":[]}`,
		},
		{
			name: "tag finalized",
			subject: types.TipSetSelector{
				Tag: &types.TipSetTags.Finalized,
			},
			wantJson: `{"tag":"finalized"}`,
		},
		{
			name: "tag latest",
			subject: types.TipSetSelector{
				Tag: &types.TipSetTags.Latest,
			},
			wantJson: `{"tag":"latest"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.subject.Validate()
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				gotMarshalled, err := json.Marshal(test.subject)
				require.NoError(t, err)
				require.JSONEq(t, test.wantJson, string(gotMarshalled))
			}
		})
	}
}
