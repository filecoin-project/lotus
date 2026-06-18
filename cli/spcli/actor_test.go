package spcli

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitDealBatchesByGas(t *testing.T) {
	expectedErr := errors.New("estimate failed")

	tests := []struct {
		name       string
		dealIDs    []uint64
		maxDeals   int
		gasCeiling int64
		estimate   func(deals []uint64) (int64, error)
		want       [][]uint64
		wantErr    error
		check      func(t *testing.T, dealIDs []uint64, batches [][]uint64, calls int)
	}{
		{
			name:       "happy path under ceiling returns single unchanged batch",
			dealIDs:    []uint64{1, 2, 3, 4},
			maxDeals:   20,
			gasCeiling: 100,
			estimate: func(deals []uint64) (int64, error) {
				return int64(len(deals) * 10), nil
			},
			want: [][]uint64{{1, 2, 3, 4}},
			check: func(t *testing.T, dealIDs []uint64, batches [][]uint64, calls int) {
				assert.Equal(t, 1, calls)
			},
		},
		{
			name:       "over ceiling batch recursively splits without dropping or duplicating deals",
			dealIDs:    []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			maxDeals:   10,
			gasCeiling: 30,
			estimate: func(deals []uint64) (int64, error) {
				return int64(len(deals) * 10), nil
			},
			check: func(t *testing.T, dealIDs []uint64, batches [][]uint64, calls int) {
				for _, batch := range batches {
					assert.LessOrEqual(t, int64(len(batch)*10), int64(30))
				}

				seen := map[uint64]struct{}{}
				var got []uint64
				for _, batch := range batches {
					for _, dealID := range batch {
						if _, ok := seen[dealID]; ok {
							t.Fatalf("duplicate deal ID %d in returned batches", dealID)
						}
						seen[dealID] = struct{}{}
						got = append(got, dealID)
					}
				}
				sort.Slice(got, func(i, j int) bool {
					return got[i] < got[j]
				})
				require.Equal(t, dealIDs, got)
			},
		},
		{
			name:       "single deal over ceiling is still emitted",
			dealIDs:    []uint64{42},
			maxDeals:   20,
			gasCeiling: 10,
			estimate: func(deals []uint64) (int64, error) {
				return 1000, nil
			},
			want: [][]uint64{{42}},
			check: func(t *testing.T, dealIDs []uint64, batches [][]uint64, calls int) {
				assert.Equal(t, 1, calls)
			},
		},
		{
			name:       "estimator error aborts without batches",
			dealIDs:    []uint64{1, 2, 3},
			maxDeals:   20,
			gasCeiling: 100,
			estimate: func(deals []uint64) (int64, error) {
				return 0, expectedErr
			},
			wantErr: expectedErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			estimate := func(deals []uint64) (int64, error) {
				calls++
				return tt.estimate(deals)
			}

			batches, err := splitDealBatchesByGas(tt.dealIDs, tt.maxDeals, estimate, tt.gasCeiling)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, batches)
				return
			}

			require.NoError(t, err)
			if tt.want != nil {
				require.Equal(t, tt.want, batches)
			}
			if tt.check != nil {
				tt.check(t, tt.dealIDs, batches, calls)
			}
		})
	}
}
