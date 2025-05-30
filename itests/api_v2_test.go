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

	// mkStableExecute creates a stable execution wrapper that ensures the chain doesn't
	// advance during test execution to avoid flaky tests due to chain reorgs
	mkStableExecute := func(getTipSet func() (*types.TipSet, error)) func(fn func() (interface{}, error)) (interface{}, *types.TipSet, error) {
		return func(fn func() (interface{}, error)) (interface{}, *types.TipSet, error) {
			for {
				// Check if context is cancelled to avoid infinite loops
				select {
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				default:
				}

				beforeTs, err := getTipSet()
				if err != nil {
					return nil, nil, err
				}
				result, execErr := fn()
				afterTs, err := getTipSet()
				if err != nil {
					return nil, nil, err
				}
				if beforeTs != nil && afterTs != nil && beforeTs.Equals(afterTs) {
					// Chain hasn't changed during execution, safe to return
					return result, beforeTs, execErr
				}
				// Chain changed, retry
			}
		}
	}

	var (
		heaviest = func() (*types.TipSet, error) {
			return subject.ChainHead(ctx)
		}
		ecFinalized = func() (*types.TipSet, error) {
			head, err := subject.ChainHead(ctx)
			if err != nil {
				return nil, err
			}
			return subject.ChainGetTipSetByHeight(ctx, head.Height()-policy.ChainFinality, head.Key())
		}
		safe = func() (*types.TipSet, error) {
			head, err := subject.ChainHead(ctx)
			if err != nil {
				return nil, err
			}
			return subject.ChainGetTipSetByHeight(ctx, head.Height()-buildconstants.SafeHeightDistance, head.Key())
		}
		tipSetAtHeight = func(height abi.ChainEpoch) func() (*types.TipSet, error) {
			return func() (*types.TipSet, error) {
				return subject.ChainGetTipSetByHeight(ctx, height, types.EmptyTSK)
			}
		}
		internalF3Error = errors.New("lost hearing in left eye")
		plausibleCertAt = func(epoch abi.ChainEpoch) (*certs.FinalityCertificate, error) {
			f3FinalisedTipSet, err := tipSetAtHeight(epoch)()
			if err != nil {
				return nil, err
			}
			return &certs.FinalityCertificate{
				ECChain: &gpbft.ECChain{
					TipSets: []*gpbft.TipSet{{
						Epoch: int64(f3FinalisedTipSet.Height()),
						Key:   f3FinalisedTipSet.Key().Bytes(),
					}},
				},
			}, nil
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

	// executeRPCRequest is a helper that executes an RPC request, unmarshals the response,
	// and returns the parsed result in a structured format
	executeRPCRequest := func(t *testing.T, request string, wantResponseStatus int, resultType interface{}) (interface{}, error) {
		gotResponseCode, gotResponseBody := subject.DoRawRPCRequest(t, 2, request)
		if gotResponseCode != wantResponseStatus {
			return nil, nil
		}

		switch resultType.(type) {
		case *types.TipSet:
			var resultOrError struct {
				Result *types.TipSet `json:"result,omitempty"`
				Error  *struct {
					Code    int    `json:"code,omitempty"`
					Message string `json:"message,omitempty"`
				} `json:"error,omitempty"`
			}
			if err := json.Unmarshal(gotResponseBody, &resultOrError); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"responseCode": gotResponseCode,
				"responseBody": gotResponseBody,
				"result":       resultOrError.Result,
				"error":        resultOrError.Error,
			}, nil
		case *types.Actor:
			var resultOrError struct {
				Result *types.Actor `json:"result,omitempty"`
				Error  *struct {
					Code    int    `json:"code,omitempty"`
					Message string `json:"message,omitempty"`
				} `json:"error,omitempty"`
			}
			if err := json.Unmarshal(gotResponseBody, &resultOrError); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"responseCode": gotResponseCode,
				"responseBody": gotResponseBody,
				"result":       resultOrError.Result,
				"error":        resultOrError.Error,
			}, nil
		case *address.Address:
			var resultOrError struct {
				Result *address.Address `json:"result,omitempty"`
				Error  *struct {
					Code    int    `json:"code,omitempty"`
					Message string `json:"message,omitempty"`
				} `json:"error,omitempty"`
			}
			if err := json.Unmarshal(gotResponseBody, &resultOrError); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"responseCode": gotResponseCode,
				"responseBody": gotResponseBody,
				"result":       resultOrError.Result,
				"error":        resultOrError.Error,
			}, nil
		default:
			return nil, errors.New("unsupported result type")
		}
	}

	t.Run("ChainGetTipSet", func(t *testing.T) {
		for _, test := range []struct {
			name               string
			when               func(t *testing.T)
			request            string
			wantTipSet         func() (*types.TipSet, error)
			wantErr            string
			wantResponseStatus int
		}{
			{
				name:               "no selector is error",
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet"],"id":1}`,
				wantErr:            "Parse error",
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(targetHeight - policy.ChainFinality - 5)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(890)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
				stableExecute := mkStableExecute(test.wantTipSet)
				if test.wantTipSet == nil {
					stableExecute = mkStableExecute(heaviest)
				}

				result, stableTipSet, err := stableExecute(func() (interface{}, error) {
					return executeRPCRequest(t, test.request, test.wantResponseStatus, (*types.TipSet)(nil))
				})

				require.NoError(t, err)
				if result != nil {
					response := result.(map[string]interface{})
					gotResponseCode := response["responseCode"].(int)
					gotResponseBody := response["responseBody"].([]byte)
					resultValue := response["result"].(*types.TipSet)
					errorValue := response["error"]

					require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))
					if test.wantErr != "" {
						require.Nil(t, resultValue)
						if errorValue != nil {
							errorObj := errorValue.(*struct {
								Code    int    `json:"code,omitempty"`
								Message string `json:"message,omitempty"`
							})
							require.Contains(t, errorObj.Message, test.wantErr)
						}
					} else {
						require.Nil(t, errorValue)
						if test.wantTipSet != nil {
							require.Equal(t, stableTipSet, resultValue)
						}
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
			wantTipSet         func() (*types.TipSet, error)
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
					cert, err := plausibleCertAt(f3FinalizedEpoch)
					require.NoError(t, err)
					mockF3.LatestCert = cert
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
				stableExecute := mkStableExecute(test.wantTipSet)
				if test.wantTipSet == nil {
					stableExecute = mkStableExecute(heaviest)
				}

				result, stableTipSet, err := stableExecute(func() (interface{}, error) {
					return executeRPCRequest(t, test.request, test.wantResponseStatus, (*types.Actor)(nil))
				})

				require.NoError(t, err)
				if result != nil {
					response := result.(map[string]interface{})
					gotResponseCode := response["responseCode"].(int)
					gotResponseBody := response["responseBody"].([]byte)
					resultValue := response["result"].(*types.Actor)
					errorValue := response["error"]

					require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

					if test.wantErr != "" {
						require.Nil(t, resultValue)
						if errorValue != nil {
							errorObj := errorValue.(*struct {
								Code    int    `json:"code,omitempty"`
								Message string `json:"message,omitempty"`
							})
							require.Contains(t, errorObj.Message, test.wantErr)
						}
					} else {
						if test.wantTipSet != nil && stableTipSet != nil {
							wantActor, err := v1StateGetActor(stableTipSet)()
							require.NoError(t, err)
							require.Equal(t, wantActor, resultValue)
						}
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
			wantTipSet         func() (*types.TipSet, error)
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
				stableExecute := mkStableExecute(test.wantTipSet)
				if test.wantTipSet == nil {
					stableExecute = mkStableExecute(heaviest)
				}

				result, _, err := stableExecute(func() (interface{}, error) {
					return executeRPCRequest(t, test.request, test.wantResponseStatus, (*address.Address)(nil))
				})

				require.NoError(t, err)
				if result != nil {
					response := result.(map[string]interface{})
					gotResponseCode := response["responseCode"].(int)
					gotResponseBody := response["responseBody"].([]byte)
					resultValue := response["result"].(*address.Address)
					errorValue := response["error"]

					require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

					if test.wantErr != "" {
						require.Nil(t, resultValue)
						if errorValue != nil {
							errorObj := errorValue.(*struct {
								Code    int    `json:"code,omitempty"`
								Message string `json:"message,omitempty"`
							})
							require.Contains(t, errorObj.Message, test.wantErr)
						}
					}
				}
			})
		}
	})
}
