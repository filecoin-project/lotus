package lf3_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jpillora/backoff"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type manifestFailAPI struct {
	manifestRequested chan struct{}
}

func (m *manifestFailAPI) F3GetManifest(ctx context.Context) (*manifest.Manifest, error) {
	select {
	case m.manifestRequested <- struct{}{}:
	default:
	}
	return nil, errors.New("test error")
}

func (m *manifestFailAPI) F3GetOrRenewParticipationTicket(ctx context.Context, minerID address.Address, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) {
	switch string(previous) {
	case "good ticket":
		return api.F3ParticipationTicket("bad ticket"), nil
	case "":
		return api.F3ParticipationTicket("good ticket"), nil
	default:
		panic("unexpected ticket")
	}
}

func (m *manifestFailAPI) F3GetProgress(ctx context.Context) (gpbft.Instant, error) {
	return gpbft.Instant{}, nil
}

func (m *manifestFailAPI) F3Participate(ctx context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	return api.F3ParticipationLease{
		Network:      "test",
		Issuer:       "foobar",
		MinerID:      0,
		FromInstance: 0,
		ValidityTerm: 10,
	}, nil
}

// Test that we correctly handle failed requests for the manifest and keep trying to get it.
func TestParticipantManifestFailure(t *testing.T) {
	api := &manifestFailAPI{manifestRequested: make(chan struct{}, 5)}
	addr, err := address.NewIDAddress(1000)
	require.NoError(t, err)

	p := lf3.NewParticipant(context.Background(), api, dtypes.MinerAddress(addr),
		&backoff.Backoff{
			Min:    1 * time.Second,
			Max:    1 * time.Minute,
			Factor: 1.5,
		}, 13, 5)
	require.NoError(t, p.Start(context.Background()))
	<-api.manifestRequested
	<-api.manifestRequested
	<-api.manifestRequested
	require.NoError(t, p.Stop(context.Background()))
}
