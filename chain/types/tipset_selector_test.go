package types_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

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

func TestTipSetLimit_Marshalling(t *testing.T) {
	for _, test := range []struct {
		name          string
		jsonInput     string
		expectedLimit types.TipSetLimit
		expectError   bool
	}{
		{
			name:          "height only",
			jsonInput:     `{"height": 123}`,
			expectedLimit: types.TipSetLimits.Height(123),
		},
		{
			name:          "distance only",
			jsonInput:     `{"distance": 456}`,
			expectedLimit: types.TipSetLimits.Distance(456),
		},
		{
			name:        "both height and distance",
			jsonInput:   `{"height": 123, "distance": 456}`,
			expectError: true,
		},
		{
			name:          "empty object",
			jsonInput:     `{}`,
			expectedLimit: types.TipSetLimits.Unlimited,
		},
		{
			name:          "height with zero value",
			jsonInput:     `{"height": 0}`,
			expectedLimit: types.TipSetLimits.Height(0),
		},
		{
			name:        "negative height",
			jsonInput:   `{"height": -1}`,
			expectError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var tsl types.TipSetLimit
			err := json.Unmarshal([]byte(test.jsonInput), &tsl)
			require.NoError(t, err)
			err = tsl.Validate()
			if test.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if test.expectedLimit.Height != nil {
				require.NotNil(t, tsl.Height)
				require.Equal(t, *test.expectedLimit.Height, *tsl.Height)
			} else {
				require.Nil(t, tsl.Height)
			}

			if test.expectedLimit.Distance != nil {
				require.NotNil(t, tsl.Distance)
				require.Equal(t, *test.expectedLimit.Distance, *tsl.Distance)
			} else {
				require.Nil(t, tsl.Distance)
			}
		})
	}
}

func TestTipSetLimit_HeightRelativeTo(t *testing.T) {
	relative := abi.ChainEpoch(1000)
	for _, test := range []struct {
		name           string
		jsonInput      string
		expectedHeight abi.ChainEpoch
	}{
		{
			name:           "absolute height",
			jsonInput:      `{"height": 500}`,
			expectedHeight: 500,
		},
		{
			name:           "relative distance",
			jsonInput:      `{"distance": 50}`,
			expectedHeight: 1050,
		},
		{
			name:           "zero height",
			jsonInput:      `{"height": 0}`,
			expectedHeight: 0,
		},
		{
			name:           "zero distance",
			jsonInput:      `{"distance": 0}`,
			expectedHeight: 1000,
		},
		{
			name:           "unlimited",
			jsonInput:      `{}`,
			expectedHeight: -1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var tsl types.TipSetLimit
			err := json.Unmarshal([]byte(test.jsonInput), &tsl)
			require.NoError(t, err, "JSON unmarshaling should succeed")

			height := tsl.HeightRelativeTo(relative)
			require.Equal(t, test.expectedHeight, height)
		})
	}
}

func TestTipSetLimits(t *testing.T) {
	tests := []struct {
		name          string
		limit         types.TipSetLimit
		jsonExpected  string
		relativeEpoch abi.ChainEpoch
		heightResult  abi.ChainEpoch
	}{
		{
			name:          "Unlimited",
			limit:         types.TipSetLimits.Unlimited,
			jsonExpected:  `{}`,
			relativeEpoch: 1000,
			heightResult:  -1,
		},
		{
			name:          "Height",
			limit:         types.TipSetLimits.Height(500),
			jsonExpected:  `{"height":500}`,
			relativeEpoch: 1000,
			heightResult:  500,
		},
		{
			name:          "Distance",
			limit:         types.TipSetLimits.Distance(50),
			jsonExpected:  `{"distance":50}`,
			relativeEpoch: 1000,
			heightResult:  1050,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tc.limit)
			require.NoError(t, err)
			require.JSONEq(t, tc.jsonExpected, string(jsonData))
			height := tc.limit.HeightRelativeTo(tc.relativeEpoch)
			require.Equal(t, tc.heightResult, height)
		})
	}
}
