package ctladdr

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type MultiAddressSelector struct {
	MinerMap map[address.Address]api.AddressConfig
}

func (as *MultiAddressSelector) AddressFor(ctx context.Context, a NodeApi, minerID address.Address, mi api.MinerInfo, use api.AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error) {
	if as == nil {
		// should only happen in some tests
		log.Warnw("smart address selection disabled, using worker address")
		return mi.Worker, big.Zero(), nil
	}

	tmp := as.MinerMap[minerID]

	var addrs []address.Address
	switch use {
	case api.PreCommitAddr:
		addrs = append(addrs, tmp.PreCommitControl...)
	case api.CommitAddr:
		addrs = append(addrs, tmp.CommitControl...)
	case api.TerminateSectorsAddr:
		addrs = append(addrs, tmp.TerminateControl...)
	case api.DealPublishAddr:
		addrs = append(addrs, tmp.DealPublishControl...)
	default:
		defaultCtl := map[address.Address]struct{}{}
		for _, a := range mi.ControlAddresses {
			defaultCtl[a] = struct{}{}
		}
		delete(defaultCtl, mi.Owner)
		delete(defaultCtl, mi.Worker)

		configCtl := append([]address.Address{}, tmp.PreCommitControl...)
		configCtl = append(configCtl, tmp.CommitControl...)
		configCtl = append(configCtl, tmp.TerminateControl...)
		configCtl = append(configCtl, tmp.DealPublishControl...)

		for _, addr := range configCtl {
			if addr.Protocol() != address.ID {
				var err error
				addr, err = a.StateLookupID(ctx, addr, types.EmptyTSK)
				if err != nil {
					log.Warnw("looking up control address", "address", addr, "error", err)
					continue
				}
			}

			delete(defaultCtl, addr)
		}

		for a := range defaultCtl {
			addrs = append(addrs, a)
		}
	}

	if len(addrs) == 0 || !tmp.DisableWorkerFallback {
		addrs = append(addrs, mi.Worker)
	}
	if !tmp.DisableOwnerFallback {
		addrs = append(addrs, mi.Owner)
	}

	return pickAddress(ctx, a, mi, goodFunds, minFunds, addrs)
}
