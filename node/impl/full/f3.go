package full

import (
	"context"

	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/lf3"
	"github.com/filecoin-project/lotus/chain/types"
)

type F3CertificateProvider interface {
	F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error)
	F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error)
}

type F3ModuleAPI interface {
	F3CertificateProvider

	F3GetOrRenewParticipationTicket(ctx context.Context, minerID address.Address, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error)
	F3Participate(ctx context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error)
	F3GetManifest(ctx context.Context) (*manifest.Manifest, error)
	F3GetECPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error)
	F3GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error)
	F3IsRunning(ctx context.Context) (bool, error)
	F3GetProgress(ctx context.Context) (gpbft.InstanceProgress, error)
	F3ListParticipants(ctx context.Context) ([]api.F3Participant, error)
}

type F3API struct {
	fx.In

	F3      lf3.F3Backend         `optional:"true"`
	F3Certs F3CertificateProvider `optional:"true"`
}

var _ F3ModuleAPI = (*F3API)(nil)

func (f3api *F3API) F3GetOrRenewParticipationTicket(ctx context.Context, miner address.Address, previous api.F3ParticipationTicket, instances uint64) (api.F3ParticipationTicket, error) {
	if f3api.F3 == nil {
		log.Infof("F3GetParticipationTicket called for %v, F3 is disabled", miner)
		return nil, api.ErrF3Disabled
	}
	minerID, err := address.IDFromAddress(miner)
	if err != nil {
		return nil, xerrors.Errorf("miner address is not of ID type: %v: %w", miner, err)
	}
	return f3api.F3.GetOrRenewParticipationTicket(ctx, minerID, previous, instances)
}

func (f3api *F3API) F3Participate(ctx context.Context, ticket api.F3ParticipationTicket) (api.F3ParticipationLease, error) {

	if f3api.F3 == nil {
		log.Infof("F3Participate called, F3 is disabled")
		return api.F3ParticipationLease{}, api.ErrF3Disabled
	}
	return f3api.F3.Participate(ctx, ticket)
}

func (f3api *F3API) F3GetCertificate(ctx context.Context, instance uint64) (*certs.FinalityCertificate, error) {
	if f3api.F3 != nil {
		return f3api.F3.GetCert(ctx, instance)
	}
	if f3api.F3Certs != nil {
		return f3api.F3Certs.F3GetCertificate(ctx, instance)
	}

	return nil, api.ErrF3Disabled
}

func (f3api *F3API) F3GetLatestCertificate(ctx context.Context) (*certs.FinalityCertificate, error) {
	if f3api.F3 != nil {
		return f3api.F3.GetLatestCert(ctx)
	}
	if f3api.F3Certs != nil {
		return f3api.F3Certs.F3GetLatestCertificate(ctx)
	}
	return nil, api.ErrF3Disabled
}

func (f3api *F3API) F3GetManifest(ctx context.Context) (*manifest.Manifest, error) {
	if f3api.F3 == nil {
		return nil, api.ErrF3Disabled
	}
	return f3api.F3.GetManifest(ctx)
}

func (f3api *F3API) F3IsRunning(context.Context) (bool, error) {
	if f3api.F3 == nil {
		return false, api.ErrF3Disabled
	}
	return f3api.F3.IsRunning(), nil
}

func (f3api *F3API) F3GetECPowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	if f3api.F3 == nil {
		return nil, api.ErrF3Disabled
	}
	return f3api.F3.GetPowerTable(ctx, tsk)
}

func (f3api *F3API) F3GetF3PowerTable(ctx context.Context, tsk types.TipSetKey) (gpbft.PowerEntries, error) {
	if f3api.F3 == nil {
		return nil, api.ErrF3Disabled
	}
	return f3api.F3.GetF3PowerTable(ctx, tsk)
}

func (f3api *F3API) F3GetPowerTableByInstance(ctx context.Context, instance uint64) (gpbft.PowerEntries, error) {
	if f3api.F3 == nil {
		return nil, api.ErrF3Disabled
	}
	return f3api.F3.GetPowerTableByInstance(ctx, instance)
}

func (f3api *F3API) F3GetProgress(context.Context) (gpbft.InstanceProgress, error) {
	if f3api.F3 == nil {
		return gpbft.InstanceProgress{}, api.ErrF3Disabled
	}
	return f3api.F3.Progress(), nil
}

func (f3api *F3API) F3ListParticipants(context.Context) ([]api.F3Participant, error) {
	if f3api.F3 == nil {
		return nil, api.ErrF3Disabled
	}
	return f3api.F3.ListParticipants(), nil
}
