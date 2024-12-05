package lf3

import (
	"context"

	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type DisabledF3 struct{}

var _ F3API = DisabledF3{}

func (DisabledF3) GetOrRenewParticipationTicket(_ context.Context, _ uint64, _ api.F3ParticipationTicket, _ uint64) (api.F3ParticipationTicket, error) {
	return api.F3ParticipationTicket{}, api.ErrF3Disabled
}
func (DisabledF3) Participate(_ context.Context, _ api.F3ParticipationTicket) (api.F3ParticipationLease, error) {
	return api.F3ParticipationLease{}, api.ErrF3Disabled
}
func (DisabledF3) GetCert(_ context.Context, _ uint64) (*certs.FinalityCertificate, error) {
	return nil, api.ErrF3Disabled
}
func (DisabledF3) GetLatestCert(_ context.Context) (*certs.FinalityCertificate, error) {
	return nil, api.ErrF3Disabled
}
func (DisabledF3) GetManifest(_ context.Context) (*manifest.Manifest, error) {
	return nil, api.ErrF3Disabled
}
func (DisabledF3) GetPowerTable(_ context.Context, _ types.TipSetKey) (gpbft.PowerEntries, error) {
	return nil, api.ErrF3Disabled
}
func (DisabledF3) GetF3PowerTable(_ context.Context, _ types.TipSetKey) (gpbft.PowerEntries, error) {
	return nil, api.ErrF3Disabled
}
func (DisabledF3) IsEnabled() bool                                { return false }
func (DisabledF3) IsRunning() (bool, error)                       { return false, api.ErrF3Disabled }
func (DisabledF3) Progress() (gpbft.Instant, error)               { return gpbft.Instant{}, api.ErrF3Disabled }
func (DisabledF3) ListParticipants() ([]api.F3Participant, error) { return nil, api.ErrF3Disabled }
