package storage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
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

func AddressFor(ctx context.Context, a addrSelectApi, mi miner.MinerInfo, use AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error) {
	switch use {
	case PreCommitAddr, CommitAddr:
		// always use worker, at least for now
		return mi.Worker, big.Zero(), nil
	}

	leastBad := address.Undef
	bestAvail := minFunds

	for _, addr := range mi.ControlAddresses {
		b, err := a.WalletBalance(ctx, addr)
		if err != nil {
			return address.Undef, big.Zero(), xerrors.Errorf("checking control address balance: %w", err)
		}

		if b.GreaterThanEqual(goodFunds) {
			k, err := a.StateAccountKey(ctx, addr, types.EmptyTSK)
			if err != nil {
				log.Errorw("getting account key", "error", err)
				continue
			}

			have, err := a.WalletHas(ctx, k)
			if err != nil {
				return address.Undef, big.Zero(), xerrors.Errorf("failed to check control address: %w", err)
			}

			if !have {
				log.Errorw("don't have key", "key", k)
				continue
			}

			return addr, b, nil
		}

		if b.GreaterThan(bestAvail) {
			leastBad = addr
			bestAvail = b
		}

		log.Warnw("control address didn't have enough funds for window post message", "address", addr, "required", types.FIL(goodFunds), "balance", types.FIL(b))
	}

	// Try to use the owner account if we can, fallback to worker if we can't

	k, err := a.StateAccountKey(ctx, mi.Owner, types.EmptyTSK)
	if err != nil {
		log.Errorw("getting owner account key", "error", err)
		return mi.Worker, big.Zero(), nil
	}

	haveOwner, err := a.WalletHas(ctx, k)
	if err != nil {
		return address.Undef, big.Zero(), xerrors.Errorf("failed to check owner address: %w", err)
	}

	if haveOwner {
		ownerBalance, err := a.WalletBalance(ctx, mi.Owner)
		if err != nil {
			return address.Undef, big.Zero(), xerrors.Errorf("checking owner balance: %w", err)
		}

		if ownerBalance.GreaterThanEqual(goodFunds) {
			return mi.Owner, goodFunds, nil
		}

		if ownerBalance.GreaterThan(bestAvail) {
			leastBad = mi.Owner
			bestAvail = ownerBalance
		}
	}

	workerBalance, err := a.WalletBalance(ctx, mi.Worker)
	if err != nil {
		return address.Undef, big.Zero(), xerrors.Errorf("checking owner balance: %w", err)
	}

	if workerBalance.GreaterThanEqual(goodFunds) {
		return mi.Worker, goodFunds, nil
	}

	if workerBalance.GreaterThan(bestAvail) {
		leastBad = mi.Worker
		bestAvail = workerBalance
	}

	if bestAvail.GreaterThan(minFunds) {
		log.Warnw("No address had enough funds to for full PoSt message Fee, selecting least bad address", "address", leastBad, "balance", types.FIL(bestAvail), "optimalFunds", types.FIL(goodFunds), "minFunds", types.FIL(minFunds))

		return leastBad, bestAvail, nil
	}

	// This most likely won't work, but can't hurt to try

	log.Warnw("No address had enough funds to for minimum PoSt message Fee, selecting worker address as a fallback", "address", mi.Worker, "balance", types.FIL(workerBalance), "optimalFunds", types.FIL(goodFunds), "minFunds", types.FIL(minFunds))
	return mi.Worker, workerBalance, nil
}
