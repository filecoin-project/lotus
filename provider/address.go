package provider

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

func AddressSelector(addrConf []config.LotusProviderAddresses) func() (*ctladdr.MultiAddressSelector, error) {
	return func() (*ctladdr.MultiAddressSelector, error) {
		as := &ctladdr.MultiAddressSelector{
			MinerMap: make(map[address.Address]api.AddressConfig),
		}
		if addrConf == nil {
			return as, nil
		}

		for _, addrConf := range addrConf {
			for _, minerID := range addrConf.MinerAddresses {
				tmp := api.AddressConfig{
					DisableOwnerFallback:  addrConf.DisableOwnerFallback,
					DisableWorkerFallback: addrConf.DisableWorkerFallback,
				}

				for _, s := range addrConf.PreCommitControl {
					addr, err := address.NewFromString(s)
					if err != nil {
						return nil, xerrors.Errorf("parsing precommit control address: %w", err)
					}

					tmp.PreCommitControl = append(tmp.PreCommitControl, addr)
				}

				for _, s := range addrConf.CommitControl {
					addr, err := address.NewFromString(s)
					if err != nil {
						return nil, xerrors.Errorf("parsing commit control address: %w", err)
					}

					tmp.CommitControl = append(tmp.CommitControl, addr)
				}

				for _, s := range addrConf.TerminateControl {
					addr, err := address.NewFromString(s)
					if err != nil {
						return nil, xerrors.Errorf("parsing terminate control address: %w", err)
					}

					tmp.TerminateControl = append(tmp.TerminateControl, addr)
				}
				a, err := address.NewFromString(minerID)
				if err != nil {
					return nil, xerrors.Errorf("parsing miner address %s: %w", minerID, err)
				}
				as.MinerMap[a] = tmp
			}
		}
		return as, nil
	}
}
