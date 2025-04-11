package itests

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/lf3"
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

	mockF3 := newMockF3Backend()
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
		tipSetAtHeight = func(height abi.ChainEpoch) func(t *testing.T) *types.TipSet {
			return func(t *testing.T) *types.TipSet {
				ts, err := subject.ChainGetTipSetByHeight(ctx, height, types.EmptyTSK)
				require.NoError(t, err)
				return ts
			}
		}
		internalF3Error = errors.New("lost hearing in left eye")
		plausibleCert   = func(t *testing.T) *certs.FinalityCertificate {
			f3FinalisedTipSet := tipSetAtHeight(f3FinalizedEpoch)(t)
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
					mockF3.running = false
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         ecFinalized,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag is ok",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCertErr = nil
					mockF3.latestCert = plausibleCert(t)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         tipSetAtHeight(f3FinalizedEpoch),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 not ready falls back to ec",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = nil
					mockF3.latestCertErr = api.ErrF3NotReady
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantTipSet:         ecFinalized,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 fails is error",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = nil
					mockF3.latestCertErr = internalF3Error
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantErr:            internalF3Error.Error(),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "latest tag when f3 fails is ok",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = nil
					mockF3.latestCertErr = internalF3Error
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"latest"}],"id":1}`,
				wantTipSet:         heaviest,
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "finalized tag when f3 is broken",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = implausibleCert
					mockF3.latestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"tag":"finalized"}],"id":1}`,
				wantErr:            "decoding latest f3 cert tipset key",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor without f3 falling back to ec is ok",
				when: func(t *testing.T) {
					mockF3.running = false
					mockF3.latestCert = nil
					mockF3.latestCertErr = nil
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
					mockF3.running = true
					mockF3.latestCert = plausibleCert(t)
					mockF3.latestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":111}}],"id":1}`,
				wantTipSet:         tipSetAtHeight(111),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor after finalized epoch is error",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = plausibleCert(t)
					mockF3.latestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":145}}],"id":1}`,
				wantErr:            "looking for tipset with height greater than start point",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor when f3 fails is error",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = nil
					mockF3.latestCertErr = internalF3Error
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":456}}],"id":1}`,
				wantErr:            internalF3Error.Error(),
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with no anchor and nil f3 cert falling back to ec fails",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = nil
					mockF3.latestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":[{"height":{"at":111}}],"id":1}`,
				wantErr:            "looking for tipset with height greater than start point",
				wantResponseStatus: http.StatusOK,
			},
			{
				name: "height with anchor to latest",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = plausibleCert(t)
					mockF3.latestCertErr = nil
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
				gotResponseCode, gotResponseBody := subject.DoRawRPCRequest(t, 2, test.request)
				require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))
				var resultOrError struct {
					Result *types.TipSet `json:"result,omitempty"`
					Error  *struct {
						Code    int    `json:"code,omitempty"`
						Message string `json:"message,omitempty"`
					} `json:"error,omitempty"`
				}
				require.NoError(t, json.Unmarshal(gotResponseBody, &resultOrError))
				if test.wantErr != "" {
					require.Nil(t, resultOrError.Result)
					require.Contains(t, resultOrError.Error.Message, test.wantErr)
				} else {
					require.Nil(t, resultOrError.Error)
					require.Equal(t, test.wantTipSet(t), resultOrError.Result)
				}
			})
		}
	})
	t.Run("StateGetActor", func(t *testing.T) {
		v1StateGetActor := func(t *testing.T, ts func(*testing.T) *types.TipSet) func(*testing.T) *types.Actor {
			return func(t *testing.T) *types.Actor {
				wantActor, err := subject.StateGetActor(ctx, miner.ActorAddr, ts(t).Key())
				require.NoError(t, err)
				return wantActor
			}
		}

		for _, test := range []struct {
			name               string
			when               func(t *testing.T)
			request            string
			wantResponseStatus int
			wantActor          func(t *testing.T) *types.Actor
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
				wantActor:          v1StateGetActor(t, heaviest),
			},
			{
				name: "finalized tag when f3 disabled falls back to ec",
				when: func(t *testing.T) {
					mockF3.running = false
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"tag":"finalized"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantActor:          v1StateGetActor(t, ecFinalized),
			},
			{
				name: "finalized tag is ok",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCertErr = nil
					mockF3.latestCert = plausibleCert(t)
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"tag":"finalized"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantActor:          v1StateGetActor(t, tipSetAtHeight(f3FinalizedEpoch)),
			},
			{
				name: "height with anchor to latest",
				when: func(t *testing.T) {
					mockF3.running = true
					mockF3.latestCert = plausibleCert(t)
					mockF3.latestCertErr = nil
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetActor","params":["f01000",{"height":{"at":15,"anchor":{"tag":"latest"}}}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantActor:          v1StateGetActor(t, tipSetAtHeight(15)),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.when != nil {
					test.when(t)
				}
				gotResponseCode, gotResponseBody := subject.DoRawRPCRequest(t, 2, test.request)
				require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

				var resultOrError struct {
					Result *types.Actor `json:"result,omitempty"`
					Error  *struct {
						Code    int    `json:"code,omitempty"`
						Message string `json:"message,omitempty"`
					} `json:"error,omitempty"`
				}
				require.NoError(t, json.Unmarshal(gotResponseBody, &resultOrError))

				if test.wantErr != "" {
					require.Nil(t, resultOrError.Result)
					require.Contains(t, resultOrError.Error.Message, test.wantErr)
				} else {
					wantActor := test.wantActor(t)
					require.Equal(t, wantActor, resultOrError.Result)
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
			wantID             func(*testing.T) *address.Address
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
				wantID: func(t *testing.T) *address.Address {
					tsk := heaviest(t).Key()
					wantID, err := subject.StateLookupID(ctx, miner.ActorAddr, tsk)
					require.NoError(t, err)
					return &wantID
				},
			},
			{
				name: "finalized tag when f3 disabled falls back to ec",
				when: func(t *testing.T) {
					mockF3.running = false
				},
				request:            `{"jsonrpc":"2.0","method":"Filecoin.StateGetID","params":["f01000",{"tag":"finalized"}],"id":1}`,
				wantResponseStatus: http.StatusOK,
				wantID: func(t *testing.T) *address.Address {
					tsk := tipSetAtHeight(f3FinalizedEpoch)(t).Key()
					wantID, err := subject.StateLookupID(ctx, miner.ActorAddr, tsk)
					require.NoError(t, err)
					return &wantID
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.when != nil {
					test.when(t)
				}
				gotResponseCode, gotResponseBody := subject.DoRawRPCRequest(t, 2, test.request)
				require.Equal(t, test.wantResponseStatus, gotResponseCode, string(gotResponseBody))

				var resultOrError struct {
					Result *address.Address `json:"result,omitempty"`
					Error  *struct {
						Code    int    `json:"code,omitempty"`
						Message string `json:"message,omitempty"`
					} `json:"error,omitempty"`
				}
				require.NoError(t, json.Unmarshal(gotResponseBody, &resultOrError))

				if test.wantErr != "" {
					require.Nil(t, resultOrError.Result)
					require.Contains(t, resultOrError.Error.Message, test.wantErr)
				}
			})
		}
	})
}

var _ lf3.F3Backend = (*mockF3Backend)(nil)

type mockF3Backend struct {
	progress      gpbft.InstanceProgress
	latestCert    *certs.FinalityCertificate
	latestCertErr error
	certs         map[uint64]*certs.FinalityCertificate
	participants  map[uint64]struct{}
	manifest      *manifest.Manifest
	running       bool
}

func newMockF3Backend() *mockF3Backend {
	return &mockF3Backend{
		certs:        make(map[uint64]*certs.FinalityCertificate),
		participants: make(map[uint64]struct{}),
	}
}

func (t *mockF3Backend) GetOrRenewParticipationTicket(_ context.Context, minerID uint64, _ api.F3ParticipationTicket, _ uint64) (api.F3ParticipationTicket, error) {
	if !t.running {
		return nil, f3.ErrF3NotRunning
	}
	return binary.BigEndian.AppendUint64(nil, minerID), nil
}

func (t *mockF3Backend) Participate(_ context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	if !t.running {
		return api.F3ParticipationLease{}, f3.ErrF3NotRunning
	}
	mid := binary.BigEndian.Uint64(ticket)
	if _, ok := t.participants[mid]; !ok {
		return api.F3ParticipationLease{}, api.ErrF3ParticipationTicketInvalid
	}
	return api.F3ParticipationLease{
		Network:      "fish",
		Issuer:       "fishmonger",
		MinerID:      mid,
		FromInstance: t.progress.ID,
		ValidityTerm: 5,
	}, nil
}

func (t *mockF3Backend) GetManifest(context.Context) (*manifest.Manifest, error) {
	if !t.running {
		return nil, f3.ErrF3NotRunning
	}
	return t.manifest, nil
}

func (t *mockF3Backend) GetCert(_ context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if !t.running {
		return nil, f3.ErrF3NotRunning
	}
	return t.certs[instance], nil
}

func (t *mockF3Backend) GetLatestCert(context.Context) (*certs.FinalityCertificate, error) {
	if !t.running {
		return nil, f3.ErrF3NotRunning
	}
	return t.latestCert, t.latestCertErr
}

func (t *mockF3Backend) GetPowerTable(context.Context, types.TipSetKey) (gpbft.PowerEntries, error) {
	return nil, nil
}

func (t *mockF3Backend) GetF3PowerTable(context.Context, types.TipSetKey) (gpbft.PowerEntries, error) {
	return nil, nil
}

func (t *mockF3Backend) ListParticipants() []api.F3Participant { return nil }
func (t *mockF3Backend) IsRunning() bool                       { return t.running }
func (t *mockF3Backend) Progress() gpbft.InstanceProgress      { return t.progress }
