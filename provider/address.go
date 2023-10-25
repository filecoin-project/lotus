package provider

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

func AddressSelector(addrConf *config.LotusProviderAddresses) func() (*ctladdr.AddressSelector, error) {
	return func() (*ctladdr.AddressSelector, error) {
		as := &ctladdr.AddressSelector{}
		if addrConf == nil {
			return as, nil
		}

		as.DisableOwnerFallback = addrConf.DisableOwnerFallback
		as.DisableWorkerFallback = addrConf.DisableWorkerFallback

		for _, s := range addrConf.PreCommitControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing precommit control address: %w", err)
			}

			as.PreCommitControl = append(as.PreCommitControl, addr)
		}

		for _, s := range addrConf.CommitControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing commit control address: %w", err)
			}

			as.CommitControl = append(as.CommitControl, addr)
		}

		for _, s := range addrConf.TerminateControl {
			addr, err := address.NewFromString(s)
			if err != nil {
				return nil, xerrors.Errorf("parsing terminate control address: %w", err)
			}

			as.TerminateControl = append(as.TerminateControl, addr)
		}

		return as, nil
	}
}
