package itests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestAPIV2_ThroughRPC(t *testing.T) {
	const (
		timeout          = 2 * time.Minute
		blockTime        = 10 * time.Millisecond
		f3FinalizedEpoch = 123
		targetHeight     = 20 + policy.ChainFinality
	)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	kit.QuietMiningLogs()

	mockF3 := kit.NewMockF3Backend()
	subject, miner, network := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.F3Backend(mockF3))
	network.BeginMining(blockTime)
	subject.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))

	var (
		heaviest = func(t *testing.T) *types.TipSet {
			head, err := subject.ChainHead(ctx)
			require.NoError(t, err)
			return head
		}
		ecFinalized = func(t *testing.T) *types.TipSet {
			head, err := subject.ChainHead(ctx)
			require.NoError(t, err)
			ecFinalized, err := subject.ChainGetTipSetByHeight(ctx, head.Height()-policy.ChainFinality, head.Key())
			require.NoError(t, err)
			return ecFinalized
		}
		safe = func(t *testing.T) *types.TipSet {
			head, err := subject.ChainHead(ctx)
			require.NoError(t, err)
			safe, err := subject.ChainGetTipSetByHeight(ctx, head.Height()-buildconstants.SafeHeightDistance, head.Key())
			require.NoError(t, err)
			return safe
		}
		tipSetAtHeight = func(height abi.ChainEpoch) func(t *testing.T) *types.TipSet {
			return func(t *testing.T) *types.TipSet {
				ts, err := subject.ChainGetTipSetByHeight(ctx, height, types.EmptyTSK)
				require.NoError(t, err)
				return ts
			}
		}
		internalF3Error = errors.New("lost hearing in left eye")
		plausibleCertAt = func(t *testing.T, epoch abi.ChainEpoch) *certs.FinalityCertificate {
			f3FinalisedTipSet := tipSetAtHeight(epoch)(t)
			return &certs.FinalityCertificate{
				ECChain: &gpbft.ECChain{
					TipSets: []*gpbft.TipSet{{
						Epoch: int64(f3FinalisedTipSet.Height()),
						Key:   f3FinalisedTipSet.Key().Bytes(),
					}},
				},
			}
		}
		implausibleCert = &certs.FinalityCertificate{
			ECChain: &gpbft.ECChain{
				TipSets: []*gpbft.TipSet{{
					Epoch: int64(1413),
					Key:   []byte(`🐠`),
				}},
			},
		}
	)

	// The tests here use the raw JSON request form for testing to both test the API
	// through RPC, and showcase what the raw request on the wire would look like at
	// Layer 7 of the ISO model.

	t.Run("ChainGetTipSet", func(t *testing.T) {
		for _, test := range []struct {
			name               string
			when               func(t *testing.T)
			request            string
			wantTipSet         func(t *testing.T) *types.TipSet
			wantErr            string
			wantResponseStatus int
		}{
			{
				name:               "no selector is error",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[],"id":1}`,
				wantErr:            "wrong param count",
				wantResponseStatus: http.StatusInternalServerError,
			},
			{
				name:               "latest tag is ok",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"latest"}],"id":1}`,
				wantTipSet:         heaviest,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 disabled falls back to ec",
				when: func(t *testing.T) {
					mockF3.Running = false
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         ecFinalized,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag is ok",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCertErr = nil
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         tipSetAtHeight(f3FinalizedEpoch),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "old f3 finalized falls back to ec",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCertErr = nil
					mockF3.LatestCert = plausibleCertAt(t, targetHeight-policy.ChainFinality-5)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         tipSetAtHeight(targetHeight - policy.ChainFinality),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "safe tag is ec safe distance when more recent than f3 finalized",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCertErr = nil
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"safe"}],"id":1}`,
				wantTipSet:         safe,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "safe tag is f3 finalized when ec minus safe distance is too old",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCertErr = nil
					mockF3.LatestCert = plausibleCertAt(t, 890)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"safe"}],"id":1}`,
				wantTipSet:         tipSetAtHeight(890),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 not ready falls back to ec",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = nil
					mockF3.LatestCertErr = api.ErrF3NotReady
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         ecFinalized,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 fails is error",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = nil
					mockF3.LatestCertErr = internalF3Error
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantErr:            internalF3Error.Error(),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "latest tag when f3 fails is ok",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = nil
					mockF3.LatestCertErr = internalF3Error
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"latest"}],"id":1}`,
				wantTipSet:         heaviest,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 is broken",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = implausibleCert
					mockF3.LatestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantErr:            "decoding latest f3 cert tipset key",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor without f3 falling back to ec is ok",
				when: func(t *testing.T) {
					mockF3.Running = false
					mockF3.LatestCert = nil
					mockF3.LatestCertErr = nil
				},
				// Lookup a height that is sufficiently behind the epoch at WaitTillChain + a
				// little bit further back to avoid race conditions of WaitTillChain
				// implementation which can cause flaky tests.
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":15}}],"id":1}`,
				wantTipSet:         tipSetAtHeight(15),
				wantResponseStatus: http.StatusOK,
			},
			{
				name:               "height with no epoch",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{}}],"id":1}`,
				wantErr:            "epoch must be specified",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor before finalized epoch is ok",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
					mockF3.LatestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":111}}],"id":1}`,
				wantTipSet:         tipSetAtHeight(111),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor after finalized epoch is error",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
					mockF3.LatestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":145}}],"id":1}`,
				wantErr:            "looking for tipset with height greater than start point",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor when f3 fails is error",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = nil
					mockF3.LatestCertErr = internalF3Error
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":456}}],"id":1}`,
				wantErr:            internalF3Error.Error(),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor and nil f3 cert falling back to ec fails",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = nil
					mockF3.LatestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":111}}],"id":1}`,
				wantErr:            "looking for tipset with height greater than start point",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with anchor to latest",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
					mockF3.LatestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":890,"anchor":{"tag":"latest"}}}],"id":1}`,
				wantTipSet:         tipSetAtHeight(890),
				wantResponseStatus: http.StatusOK,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.when != nil {
					test.when(t)
				}

				// Use stable execute to ensure the test doesn't straddle tipsets
				stableExecute := kit.MakeStableExecute(ctx, t, test.wantTipSet)
				if test.wantTipSet == nil {
					stableExecute = kit.MakeStableExecute(ctx, t, heaviest)
				}

				var gotResponseCode int
				var gotResponseBody []byte
				var resultOrError struct {
					Result *types.TipSet `json:"result,omitempty"`
					Error  *struct {
						Code    int    `json:"code,omitempty"`
						Message string `json:"message,omitempty"`
					} `json:"error,omitempty"`
				}

				stableTipSet := stableExecute(func() {
					gotResponseCode, gotResponseBody = subject.DoRawRPCRequest(t, 2, test.request)
					if gotResponseCode == test.wantResponseStatus {
						require.NoError(t, json.Unmarshal(gotResponseBody, &resultOrError))
					}
				})
				require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

				if test.wantErr != "" {
					require.Nil(t, resultOrError.Result)
					if resultOrError.Error != nil {
						require.Contains(t, resultOrError.Error.Message, test.wantErr)
					}
				} else {
					require.Nil(t, resultOrError.Error)
					if test.wantTipSet != nil {
						require.Equal(t, stableTipSet, resultOrError.Result)
					}
				}
			})
		}
	})
	t.Run("StateGetActor", func(t *testing.T) {
		v1StateGetActor := func(ts *types.TipSet) func() (*types.Actor, error) {
			return func() (*types.Actor, error) {
				return subject.StateGetActor(ctx, miner.ActorAddr, ts.Key())
			}
		}

		for _, test := range []struct {
			name               string
			when               func(t *testing.T)
			request            string
			wantResponseStatus int
			wantTipSet         func(t *testing.T) *types.TipSet
			wantErr            string
		}{
			{
				name:               "no selector is error",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000"],"id":1}`,
				wantErr:            "wrong param count",
				wantResponseStatus: http.StatusInternalServerError,
			},
			{
				name:               "latest tag is ok",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"tag":"latest"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantTipSet:         heaviest,
			},
			{
				name: "finalized tag when f3 disabled falls back to ec",
				when: func(t *testing.T) {
					mockF3.Running = false
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"tag":"finalized"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantTipSet:         ecFinalized,
			},
			{
				name: "finalized tag is ok",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCertErr = nil
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"tag":"finalized"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantTipSet:         tipSetAtHeight(f3FinalizedEpoch),
			},
			{
				name: "height with anchor to latest",
				when: func(t *testing.T) {
					mockF3.Running = true
					mockF3.Finalizing = true
					mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
					mockF3.LatestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"height":{"at":15,"anchor":{"tag":"latest"}}}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantTipSet:         tipSetAtHeight(15),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.when != nil {
					test.when(t)
				}

				// Use stable execute to ensure the test doesn't straddle tipsets
				stableExecute := kit.MakeStableExecute(ctx, t, test.wantTipSet)
				if test.wantTipSet == nil {
					stableExecute = kit.MakeStableExecute(ctx, t, heaviest)
				}

				var gotResponseCode int
				var gotResponseBody []byte
				var resultOrError struct {
					Result *types.Actor `json:"result,omitempty"`
					Error  *struct {
						Code    int    `json:"code,omitempty"`
						Message string `json:"message,omitempty"`
					} `json:"error,omitempty"`
				}

				stableTipSet := stableExecute(func() {
					gotResponseCode, gotResponseBody = subject.DoRawRPCRequest(t, 2, test.request)
					if gotResponseCode == test.wantResponseStatus {
						require.NoError(t, json.Unmarshal(gotResponseBody, &resultOrError))
					}
				})
				require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

				if test.wantErr != "" {
					require.Nil(t, resultOrError.Result)
					if resultOrError.Error != nil {
						require.Contains(t, resultOrError.Error.Message, test.wantErr)
					}
				} else {
					if test.wantTipSet != nil && stableTipSet != nil {
						wantActor, err := v1StateGetActor(stableTipSet)()
						require.NoError(t, err)
						require.Equal(t, wantActor, resultOrError.Result)
					}
				}
			})
		}
	})
	t.Run("StateGetID", func(t *testing.T) {
		for _, test := range []struct {
			name               string
			when               func(t *testing.T)
			request            string
			wantResponseStatus int
			wantTipSet         func(t *testing.T) *types.TipSet
			wantErr            string
		}{
			{
				name:               "no selector is error",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetID","params":["f01000"],"id":1}`,
				wantErr:            "wrong param count",
				wantResponseStatus: http.StatusInternalServerError,
			},
			{
				name:               "latest tag is ok",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetID","params":["f01000",{"tag":"latest"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantTipSet:         heaviest,
			},
			{
				name: "finalized tag when f3 disabled falls back to ec",
				when: func(t *testing.T) {
					mockF3.Running = false
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetID","params":["f01000",{"tag":"finalized"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantTipSet:         tipSetAtHeight(f3FinalizedEpoch),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.when != nil {
					test.when(t)
				}

				// Use stable execute to ensure the test doesn't straddle tipsets
				stableExecute := kit.MakeStableExecute(ctx, t, test.wantTipSet)
				if test.wantTipSet == nil {
					stableExecute = kit.MakeStableExecute(ctx, t, heaviest)
				}

				var gotResponseCode int
				var gotResponseBody []byte
				var resultOrError struct {
					Result *address.Address `json:"result,omitempty"`
					Error  *struct {
						Code    int    `json:"code,omitempty"`
						Message string `json:"message,omitempty"`
					} `json:"error,omitempty"`
				}

				_ = stableExecute(func() {
					gotResponseCode, gotResponseBody = subject.DoRawRPCRequest(t, 2, test.request)
					if gotResponseCode == test.wantResponseStatus {
						require.NoError(t, json.Unmarshal(gotResponseBody, &resultOrError))
					}
				})
				require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

				if test.wantErr != "" {
					require.Nil(t, resultOrError.Result)
					if resultOrError.Error != nil {
						require.Contains(t, resultOrError.Error.Message, test.wantErr)
					}
				}
			})
		}
	})
}
