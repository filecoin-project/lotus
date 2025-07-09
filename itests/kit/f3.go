package kit

import (
	"context"
	"encoding/binary"

	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
)

var _ lf3.F3Backend = (*MockF3Backend)(nil)

type MockF3Backend struct {
	Running       bool
	LatestCert    *certs.FinalityCertificate
	LatestCertErr error
	Finalizing    bool

	progress     gpbft.InstanceProgress
	certs        map[uint64]*certs.FinalityCertificate
	participants map[uint64]struct{}
}

func NewMockF3Backend() *MockF3Backend {
	return &MockF3Backend{
		certs:        make(map[uint64]*certs.FinalityCertificate),
		participants: make(map[uint64]struct{}),
	}
}

func (t *MockF3Backend) GetOrRenewParticipationTicket(_ context.Context, minerID uint64, _ api.F3ParticipationTicket, _ uint64) (api.F3ParticipationTicket, error) {
	if !t.Running {
		return nil, f3.ErrF3NotRunning
	}
	return binary.BigEndian.AppendUint64(nil, minerID), nil
}

func (t *MockF3Backend) Participate(_ context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	if !t.Running {
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

func (t *MockF3Backend) GetManifest(context.Context) (*manifest.Manifest, error) {
	if !t.Running {
		return nil, f3.ErrF3NotRunning
	}
	return &manifest.Manifest{
		EC: manifest.EcConfig{
			Finalize: t.Finalizing,
		},
	}, nil
}

func (t *MockF3Backend) GetCert(_ context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if !t.Running {
		return nil, f3.ErrF3NotRunning
	}
	return t.certs[instance], nil
}

func (t *MockF3Backend) GetLatestCert(context.Context) (*certs.FinalityCertificate, error) {
	if !t.Running {
		return nil, f3.ErrF3NotRunning
	}
	return t.LatestCert, t.LatestCertErr
}

func (t *MockF3Backend) GetPowerTable(context.Context, types.TipSetKey) (gpbft.PowerEntries, error) {
	return nil, nil
}

func (t *MockF3Backend) GetF3PowerTable(context.Context, types.TipSetKey) (gpbft.PowerEntries, error) {
	return nil, nil
}

func (t *MockF3Backend) GetPowerTableByInstance(context.Context, uint64) (gpbft.PowerEntries, error) {
	return nil, nil
}

func (t *MockF3Backend) ListParticipants() []api.F3Participant { return nil }
func (t *MockF3Backend) IsRunning() bool                       { return t.Running }
func (t *MockF3Backend) Progress() gpbft.InstanceProgress      { return t.progress }
