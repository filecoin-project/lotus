package mock

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
)

type MockF3API struct {
	lk sync.Mutex

	latestCert *certs.FinalityCertificate
	manifest   *manifest.Manifest
	enabled    bool
	running    bool
}

func (m *MockF3API) GetOrRenewParticipationTicket(ctx context.Context, minerID uint64, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) {
	if !m.IsEnabled() {
		return api.F3ParticipationTicket{}, api.ErrF3Disabled
	}
	return api.F3ParticipationTicket{}, nil
}

func (m *MockF3API) Participate(ctx context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	if !m.IsEnabled() {
		return api.F3ParticipationLease{}, api.ErrF3Disabled
	}
	return api.F3ParticipationLease{}, nil
}

func (m *MockF3API) GetCert(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if !m.IsEnabled() {
		return nil, api.ErrF3Disabled
	}
	return nil, nil
}

// SetLatestCert sets the latest certificate to be returned by GetLatestCert. If GetLatestCert is
// called before this method, it will return an error.
func (m *MockF3API) SetLatestCert(cert *certs.FinalityCertificate) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.latestCert = cert
}

func (m *MockF3API) GetLatestCert(ctx context.Context) (*certs.FinalityCertificate, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	if !m.enabled {
		return nil, api.ErrF3Disabled
	}

	if m.latestCert == nil {
		return nil, xerrors.Errorf("no latest cert set in test, did you mean to?")
	}

	return m.latestCert, nil
}

// SetManifest sets the manifest to be returned by GetManifest. If GetManifest is called before this
// method, it will return an error.
//
// Use manifest.LocalDevnetManifest() for a convenient manifest to use in tests.
func (m *MockF3API) SetManifest(manifest *manifest.Manifest) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.manifest = manifest
}

func (m *MockF3API) GetManifest(ctx context.Context) (*manifest.Manifest, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	if !m.enabled {
		return nil, api.ErrF3Disabled
	}

	if m.manifest == nil {
		return nil, xerrors.Errorf("no manifest set in test, did you mean to?")
	}

	return m.manifest, nil
}

func (m *MockF3API) GetPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	if !m.IsEnabled() {
		return nil, api.ErrF3Disabled
	}

	return nil, nil
}

func (m *MockF3API) GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	if !m.IsEnabled() {
		return nil, api.ErrF3Disabled
	}

	return nil, nil
}

func (m *MockF3API) SetEnabled(enabled bool) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.enabled = enabled
}

func (m *MockF3API) IsEnabled() bool {
	m.lk.Lock()
	defer m.lk.Unlock()

	return m.enabled
}

func (m *MockF3API) SetRunning(running bool) {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.running = running
}

func (m *MockF3API) IsRunning() (bool, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	if !m.enabled {
		return false, api.ErrF3Disabled
	}

	return m.running, nil
}

func (m *MockF3API) Progress() (gpbft.Instant, error) {
	if !m.IsEnabled() {
		return gpbft.Instant{}, api.ErrF3Disabled
	}

	return gpbft.Instant{}, nil
}

func (m *MockF3API) ListParticipants() ([]api.F3Participant, error) {
	if !m.IsEnabled() {
		return nil, api.ErrF3Disabled
	}

	return nil, nil
}

var _ lf3.F3API = (*MockF3API)(nil)
