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

func TestAPIV2_GetTipSetThroughRPC(t *testing.T) {
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
	subject, _, network := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.F3Backend(mockF3))
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
	// Layer 7 of ISO model.
	for _, test := range []struct {
		name       string
		when       func(t *testing.T)
		request    string
		wantTipSet func(t *testing.T) *types.TipSet
		wantErr    string
	}{
		{
			name:       "no selector",
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","id":1}`,
			wantTipSet: heaviest,
		},
		{
			name:       "latest tag",
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"latest"},"id":1}`,
			wantTipSet: heaviest,
		},
		{
			name: "finalized tag when f3 disabled",
			when: func(t *testing.T) {
				mockF3.running = false
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"finalized"},"id":1}`,
			wantTipSet: ecFinalized,
		},
		{
			name: "finalized tag",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCertErr = nil
				mockF3.latestCert = plausibleCert(t)
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"finalized"},"id":1}`,
			wantTipSet: tipSetAtHeight(f3FinalizedEpoch),
		},
		{
			name: "finalized tag when f3 not ready",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCert = nil
				mockF3.latestCertErr = api.ErrF3NotReady
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"finalized"},"id":1}`,
			wantTipSet: ecFinalized,
		},
		{
			name: "finalized tag when f3 fails",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCert = nil
				mockF3.latestCertErr = internalF3Error
			},
			request: `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"finalized"},"id":1}`,
			wantErr: internalF3Error.Error(),
		},
		{
			name: "latest tag when f3 fails",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCert = nil
				mockF3.latestCertErr = internalF3Error
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"latest"},"id":1}`,
			wantTipSet: heaviest,
		},
		{
			name: "finalized tag when f3 is broken",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCert = implausibleCert
				mockF3.latestCertErr = nil
			},
			request: `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"tag":"finalized"},"id":1}`,
			wantErr: "decoding latest f3 cert tipset key",
		},
		{
			name: "height without f3",
			when: func(t *testing.T) {
				mockF3.running = false
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"height":{"at":321}},"id":1}`,
			wantTipSet: tipSetAtHeight(321),
		},
		{
			name: "height when f3 fails",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCert = nil
				mockF3.latestCertErr = internalF3Error
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"height":{"at":456}},"id":1}`,
			wantTipSet: tipSetAtHeight(456),
		},
		{
			name: "height with anchor to finalized",
			when: func(t *testing.T) {
				mockF3.running = true
				mockF3.latestCert = plausibleCert(t)
				mockF3.latestCertErr = nil
			},
			request:    `{"jsonrpc":"2.0","method":"Filecoin.ChainGetTipSet","params":{"height":{"at":111,"anchor":{"tag":"finalized"}}},"id":1}`,
			wantTipSet: tipSetAtHeight(111),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.when != nil {
				test.when(t)
			}
			gotResponseCode, gotResponseBody := subject.DoRawRPCRequest(t, 2, test.request)
			require.Equal(t, http.StatusOK, gotResponseCode, string(gotResponseBody))
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
