package stmgr

import (
	"context"

	amt "github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

func (sm *StateManager) handleStateForks(ctx context.Context, pstate cid.Cid, height, parentH uint64) (_ cid.Cid, err error) {
	for i := parentH; i < height; i++ {
		switch i {
		case build.ForkBlizzardHeight:
			log.Warnw("Executing blizzard fork logic", "height", i)
			pstate, err = fixBlizzardAMTBug(ctx, sm, pstate)
			if err != nil {
				return cid.Undef, xerrors.Errorf("blizzard bug fix failed: %w", err)
			}
		case build.ForkFrigidHeight:
			log.Warnw("Executing frigid fork logic", "height", i)
			pstate, err = fixBlizzardAMTBug(ctx, sm, pstate)
			if err != nil {
				return cid.Undef, xerrors.Errorf("frigid bug fix failed: %w", err)
			}
		case build.ForkBootyBayHeight:
			log.Warnw("Executing booty bay fork logic", "height", i)
			pstate, err = fixBlizzardAMTBug(ctx, sm, pstate)
			if err != nil {
				return cid.Undef, xerrors.Errorf("booty bay bug fix failed: %w", err)
			}
		case build.ForkMissingSnowballs:
			log.Warnw("Adding more snow to the world", "height", i)
			pstate, err = fixTooFewSnowballs(ctx, sm, pstate)
			if err != nil {
				return cid.Undef, xerrors.Errorf("missing snowballs bug fix failed: %w", err)
			}
		}
	}

	return pstate, nil
}

func fixTooFewSnowballs(ctx context.Context, sm *StateManager, pstate cid.Cid) (cid.Cid, error) {
	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	st, err := state.LoadStateTree(cst, pstate)
	if err != nil {
		return cid.Undef, err
	}

	spa, err := st.GetActor(actors.StoragePowerAddress)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get storage power actor: %w", err)
	}

	var spast actors.StoragePowerState
	if err := cst.Get(ctx, spa.Head, &spast); err != nil {
		return cid.Undef, err
	}

	miners, err := actors.MinerSetList(ctx, cst, spast.Miners)
	if err != nil {
		return cid.Undef, err
	}

	sum := types.NewInt(0)
	for _, m := range miners {
		mact, err := st.GetActor(m)
		if err != nil {
			return cid.Undef, xerrors.Errorf("getting miner actor to fix: %w", err)
		}

		var mstate actors.StorageMinerActorState
		if err := cst.Get(ctx, mact.Head, &mstate); err != nil {
			return cid.Undef, xerrors.Errorf("failed to load miner actor state: %w", err)
		}

		if mstate.SlashedAt != 0 {
			continue
		}
		sum = types.BigAdd(sum, mstate.Power)
	}

	spast.TotalStorage = sum
	nspahead, err := cst.Put(ctx, &spast)
	if err != nil {
		return cid.Undef, err
	}

	spa.Head = nspahead

	return st.Flush(ctx)
}

/*
1) Iterate through each miner in the chain:
	1.1) Fixup their sector set and proving set
	1.2) Change their code cid to point to the new miner actor code
*/
func fixBlizzardAMTBug(ctx context.Context, sm *StateManager, pstate cid.Cid) (cid.Cid, error) {
	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	st, err := state.LoadStateTree(cst, pstate)
	if err != nil {
		return cid.Undef, err
	}

	spa, err := st.GetActor(actors.StoragePowerAddress)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get storage power actor: %w", err)
	}

	var spast actors.StoragePowerState
	if err := cst.Get(ctx, spa.Head, &spast); err != nil {
		return cid.Undef, err
	}

	miners, err := actors.MinerSetList(ctx, cst, spast.Miners)
	if err != nil {
		return cid.Undef, err
	}

	for _, m := range miners {
		mact, err := st.GetActor(m)
		if err != nil {
			return cid.Undef, xerrors.Errorf("getting miner actor to fix: %w", err)
		}

		nhead, err := fixMiner(ctx, cst, sm.cs.Blockstore(), mact.Head)
		if err != nil {
			return cid.Undef, xerrors.Errorf("fixing miner: %w", err)
		}

		if nhead != mact.Head {
			log.Warnf("Miner %s had changes", m)
		}

		mact.Head = nhead
		mact.Code = actors.StorageMiner2CodeCid

		if err := st.SetActor(m, mact); err != nil {
			return cid.Undef, err
		}
	}

	return st.Flush(ctx)
}

func fixMiner(ctx context.Context, cst *hamt.CborIpldStore, bs blockstore.Blockstore, mscid cid.Cid) (cid.Cid, error) {
	var mstate actors.StorageMinerActorState
	if err := cst.Get(ctx, mscid, &mstate); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	amts := amt.WrapBlockstore(bs)

	nsectors, err := amtFsck(amts, mstate.Sectors)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error fsck'ing sector set: %w", err)
	}
	mstate.Sectors = nsectors

	nproving, err := amtFsck(amts, mstate.ProvingSet)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error fsck'ing proving set: %w", err)
	}
	mstate.ProvingSet = nproving

	nmcid, err := cst.Put(ctx, &mstate)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to put modified miner state: %w", err)
	}

	return nmcid, nil
}

func amtFsck(s amt.Blocks, ss cid.Cid) (cid.Cid, error) {
	a, err := amt.LoadAMT(s, ss)
	if err != nil {
		return cid.Undef, xerrors.Errorf("could not load AMT: %w", a)
	}

	b := amt.NewAMT(s)

	err = a.ForEach(func(id uint64, data *cbg.Deferred) error {
		err := b.Set(id, data)
		if err != nil {
			return xerrors.Errorf("could not copy at idx (%d): %w", id, err)
		}
		return nil
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("could not copy: %w", err)
	}

	nss, err := b.Flush()
	if err != nil {
		return cid.Undef, xerrors.Errorf("could not flush: %w", err)
	}

	return nss, nil
}
