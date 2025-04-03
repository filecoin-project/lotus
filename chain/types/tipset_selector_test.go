package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestTipSetSelector_Marshalling(t *testing.T) {
	for _, test := range []struct {
		name     string
		subject  func(t *testing.T) types.TipSetSelector
		wantJson string
		wantErr  bool
	}{
		{
			name:    "nil",
			subject: func(t *testing.T) types.TipSetSelector { return nil },
		},
		{
			name:    "zero-valued",
			subject: func(t *testing.T) types.TipSetSelector { return types.TipSetSelector{} },
		},
		{
			name: "height",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(
					types.TipSetHeight{
						At:       123,
						Previous: true,
					})
				require.NoError(t, err)
				return selector
			},
			wantJson: `{"height":{"at":123,"previous":true}}`,
		},
		{
			name: "height anchored to finalized",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(
					types.TipSetHeight{
						At:       123,
						Previous: true,
						Anchor: &types.TipSetAnchor{
							Tag: &types.TipSetTags.Finalized,
						},
					})
				require.NoError(t, err)
				return selector
			},
			wantJson: `{"height":{"at":123,"previous":true,"anchor":{"tag":"finalized"}}}`,
		},
		{
			name: "invalid height anchor",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(
					types.TipSetHeight{
						At:       123,
						Previous: true,
						Anchor: &types.TipSetAnchor{
							Tag: &types.TipSetTags.Finalized,
							Key: &types.TipSetKey{},
						},
					})
				require.NoError(t, err)
				return selector
			},
			wantErr: true,
		},
		{
			name: "invalid height epoch",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(
					types.TipSetHeight{
						At: -7,
					})
				require.NoError(t, err)
				return selector
			},
			wantErr: true,
		},
		{
			name: "nil height",
			subject: func(t *testing.T) types.TipSetSelector {
				return jsonrpc.RawParams(`{"height":null}`)
			},
			wantJson: `{}`,
		},
		{
			name: "key",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(types.TipSetKey{})
				require.NoError(t, err)
				return selector
			},
			wantJson: `{"key":[]}`,
		},
		{
			name: "tag finalized",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(types.TipSetTags.Finalized)
				require.NoError(t, err)
				return selector
			},
			wantJson: `{"tag":"finalized"}`,
		},
		{
			name: "tag latest",
			subject: func(t *testing.T) types.TipSetSelector {
				selector, err := types.NewTipSetSelector(types.TipSetTags.Latest)
				require.NoError(t, err)
				return selector
			},
			wantJson: `{"tag":"latest"}`,
		},
		{
			name: "invalid json",
			subject: func(t *testing.T) types.TipSetSelector {
				return []byte(`🐠`)
			},
			wantErr: true,
		},
		{
			name: "valid json with unknown fields",
			subject: func(t *testing.T) types.TipSetSelector {
				return []byte(`{"colour":"fishy"}`)
			},
			wantJson: `{}`, // non-nil criterion with no fields set.
		},
		{
			name: "valid json but invalid criterion",
			subject: func(t *testing.T) types.TipSetSelector {
				return []byte(`{"tag":"latest","height":{"at":123,"previous":true}}`)
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			subject := test.subject(t)
			gotCriterion, err := types.DecodeTipSetCriterion(subject)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NoError(t, gotCriterion.Validate())
				wantCriterion, err := types.DecodeTipSetCriterion(types.TipSetSelector(test.wantJson))
				require.NoError(t, err)
				require.NoError(t, wantCriterion.Validate())
				require.Equal(t, wantCriterion, gotCriterion)
			}
		})
	}
}

func TestTipSetCriterion_Validate(t *testing.T) {
	tests := []struct {
		name    string
		subject *types.TipSetCriterion
		wantErr bool
	}{
		{
			name: "nil",
		},
		{
			name:    "zero",
			subject: &types.TipSetCriterion{},
		},
		{
			name: "only key",
			subject: &types.TipSetCriterion{
				Key: &types.TipSetKey{},
			},
		},
		{
			name: "only tag",
			subject: &types.TipSetCriterion{
				Tag: &types.TipSetTags.Latest,
			},
		},
		{
			name: "only height",
			subject: &types.TipSetCriterion{
				Height: &types.TipSetHeight{
					At: 1413,
				},
			},
		},
		{
			name: "height and tag",
			subject: &types.TipSetCriterion{
				Height: &types.TipSetHeight{
					At: 1413,
				},
				Tag: &types.TipSetTags.Finalized,
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotErr := test.subject.Validate()
			if test.wantErr {
				require.Error(t, gotErr)
			} else {
				require.NoError(t, gotErr)
			}
		})
	}
}
