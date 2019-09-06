package stmgr

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	cid "github.com/ipfs/go-cid"
	"golang.org/x/xerrors"
)

func GetMinerWorker(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	recp, err := CallRaw(ctx, sm, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetWorkerAddr,
	}, st, 0)
	if err != nil {
		return address.Undef, xerrors.Errorf("callRaw failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getting miner worker addr failed (exit code %d)", recp.ExitCode)
	}

	worker, err := address.NewFromBytes(recp.Return)
	if err != nil {
		return address.Undef, err
	}

	if worker.Protocol() == address.ID {
		return address.Undef, xerrors.Errorf("need to resolve worker address to a pubkeyaddr")
	}

	return worker, nil
}

func GetMinerOwner(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	recp, err := CallRaw(ctx, sm, &types.Message{
		To:     maddr,
		From:   maddr,
		Method: actors.MAMethods.GetOwner,
	}, st, 0)
	if err != nil {
		return address.Undef, xerrors.Errorf("callRaw failed: %w", err)
	}

	if recp.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getting miner owner addr failed (exit code %d)", recp.ExitCode)
	}

	owner, err := address.NewFromBytes(recp.Return)
	if err != nil {
		return address.Undef, err
	}

	if owner.Protocol() == address.ID {
		return address.Undef, xerrors.Errorf("need to resolve owner address to a pubkeyaddr")
	}

	return owner, nil
}
