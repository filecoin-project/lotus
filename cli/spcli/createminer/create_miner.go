package createminer

import (
	"bytes"
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	power6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/power"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
)

func CreateStorageMiner(ctx context.Context, fullNode v1api.FullNode, owner, worker, sender address.Address, ssize abi.SectorSize, confidence uint64, depositMarginFactor float64) (address.Address, error) {
	// make sure the sender account exists on chain
	_, err := fullNode.StateLookupID(ctx, owner, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("sender must exist on chain: %w", err)
	}

	// make sure the worker account exists on chain
	_, err = fullNode.StateLookupID(ctx, worker, types.EmptyTSK)
	if err != nil {
		signed, err := fullNode.MpoolPushMessage(ctx, &types.Message{
			From:  sender,
			To:    worker,
			Value: types.NewInt(0),
		}, nil)
		if err != nil {
			return address.Undef, xerrors.Errorf("push worker init: %w", err)
		}

		fmt.Printf("Initializing worker account %s, message: %s\n", worker, signed.Cid())
		fmt.Println("Waiting for confirmation")

		mw, err := fullNode.StateWaitMsg(ctx, signed.Cid(), confidence, 2000, true)
		if err != nil {
			return address.Undef, xerrors.Errorf("waiting for worker init: %w", err)
		}
		if mw.Receipt.ExitCode != 0 {
			return address.Undef, xerrors.Errorf("initializing worker account failed: exit code %d", mw.Receipt.ExitCode)
		}
	}

	// make sure the owner account exists on chain
	_, err = fullNode.StateLookupID(ctx, owner, types.EmptyTSK)
	if err != nil {
		signed, err := fullNode.MpoolPushMessage(ctx, &types.Message{
			From:  sender,
			To:    owner,
			Value: types.NewInt(0),
		}, nil)
		if err != nil {
			return address.Undef, xerrors.Errorf("push owner init: %w", err)
		}

		fmt.Printf("Initializing owner account %s, message: %s\n", worker, signed.Cid())
		fmt.Println("Waiting for confirmation")

		mw, err := fullNode.StateWaitMsg(ctx, signed.Cid(), confidence, 2000, true)
		if err != nil {
			return address.Undef, xerrors.Errorf("waiting for owner init: %w", err)
		}
		if mw.Receipt.ExitCode != 0 {
			return address.Undef, xerrors.Errorf("initializing owner account failed: exit code %d", mw.Receipt.ExitCode)
		}
	}

	// Note: the correct thing to do would be to call SealProofTypeFromSectorSize if actors version is v3 or later, but this still works
	nv, err := fullNode.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get network version: %w", err)
	}
	spt, err := lminer.WindowPoStProofTypeFromSectorSize(ssize, nv)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting post proof type: %w", err)
	}

	params, err := actors.SerializeParams(&power6.CreateMinerParams{
		Owner:               owner,
		Worker:              worker,
		WindowPoStProofType: spt,
	})
	if err != nil {
		return address.Undef, err
	}

	if depositMarginFactor < 1 {
		return address.Undef, xerrors.Errorf("deposit margin factor must be greater than 1")
	}

	deposit, err := fullNode.StateMinerCreationDeposit(ctx, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting miner creation deposit: %w", err)
	}

	scaledDeposit := types.BigDiv(types.BigMul(deposit, types.NewInt(uint64(depositMarginFactor*100))), types.NewInt(100))

	createStorageMinerMsg := &types.Message{
		To:    power.Address,
		From:  sender,
		Value: scaledDeposit,

		Method: power.Methods.CreateMiner,
		Params: params,
	}

	signed, err := fullNode.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
	if err != nil {
		return address.Undef, xerrors.Errorf("pushing createMiner message: %w", err)
	}

	fmt.Printf("Pushed CreateMiner message: %s\n", signed.Cid())
	fmt.Println("Waiting for confirmation")

	mw, err := fullNode.StateWaitMsg(ctx, signed.Cid(), confidence, 2000, true)
	if err != nil {
		return address.Undef, xerrors.Errorf("waiting for createMiner message: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("create miner failed: exit code %d", mw.Receipt.ExitCode)
	}

	var retval power2.CreateMinerReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)); err != nil {
		return address.Undef, err
	}

	fmt.Printf("New miners address is: %s (%s)\n", retval.IDAddress, retval.RobustAddress)
	return retval.IDAddress, nil
}
