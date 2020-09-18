package storage

import (
	"context"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

type AddrUse int

const (
	PreCommitAddr AddrUse = iota
	CommitAddr
	PoStAddr
)

type addrSelectApi interface {
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)

	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

func AddressFor(ctx context.Context, a addrSelectApi, mi miner.MinerInfo, use AddrUse, minFunds abi.TokenAmount) (address.Address, error) {
	switch use {
	case PreCommitAddr, CommitAddr:
		// always use worker, at least for now
		return mi.Worker, nil
	}

	for _, addr := range mi.ControlAddresses {
		b, err := a.WalletBalance(ctx, addr)
		if err != nil {
			return address.Undef, xerrors.Errorf("checking control address balance: %w", err)
		}

		if b.GreaterThanEqual(minFunds) {
			k, err := a.StateAccountKey(ctx, addr, types.EmptyTSK)
			if err != nil {
				log.Errorw("getting account key", "error", err)
				continue
			}

			have, err := a.WalletHas(ctx, k)
			if err != nil {
				return address.Undef, xerrors.Errorf("failed to check control address: %w", err)
			}

			if !have {
				log.Errorw("don't have key", "key", k)
				continue
			}

			return addr, nil
		}

		log.Warnw("control address didn't have enough funds for window post message", "address", addr, "required", types.FIL(minFunds), "balance", types.FIL(b))
	}

	// Try to use the owner account if we can, fallback to worker if we can't

	b, err := a.WalletBalance(ctx, mi.Owner)
	if err != nil {
		return address.Undef, xerrors.Errorf("checking owner balance: %w", err)
	}

	if !b.GreaterThanEqual(minFunds) {
		return mi.Worker, nil
	}

	k, err := a.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
	if err != nil {
		log.Errorw("getting owner account key", "error", err)
		return mi.Worker, nil
	}

	have, err := a.WalletHas(ctx, k)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to check owner address: %w", err)
	}

	if !have {
		return mi.Worker, nil
	}

	return mi.Owner, nil
}
