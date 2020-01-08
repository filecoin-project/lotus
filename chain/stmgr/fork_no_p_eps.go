package stmgr

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

func (sm *StateManager) forkNoPowerEPS(ctx context.Context, pstate cid.Cid) (cid.Cid, error) {
	cst := hamt.CSTFromBstore(sm.cs.Blockstore())
	st, err := state.LoadStateTree(cst, pstate)
	if err != nil {
		return cid.Undef, xerrors.Errorf("loading parent state tree: %w", err)
	}

	if err := st.MutateActor(actors.StoragePowerAddress, func(spa *types.Actor) error {
		var head actors.StoragePowerState
		if err := cst.Get(ctx, spa.Head, &head); err != nil {
			return xerrors.Errorf("reading StoragePower state: %w", err)
		}

		buckets, err := amt.LoadAMT(amt.WrapBlockstore(sm.cs.Blockstore()), head.ProvingBuckets)
		if err != nil {
			return xerrors.Errorf("opening proving buckets AMT: %w", err)
		}

		fixedBuckets := map[uint64]map[address.Address]struct{}{}

		if err := buckets.ForEach(func(bucketId uint64, ent *typegen.Deferred) error {
			var bcid cid.Cid
			if err := cbor.DecodeInto(ent.Raw, &bcid); err != nil {
				return xerrors.Errorf("decoding bucket cid: %w", err)
			}

			bucket, err := hamt.LoadNode(ctx, cst, bcid)
			if err != nil {
				return xerrors.Errorf("loading bucket hamt: %w", err)
			}

			return bucket.ForEach(ctx, func(abytes string, _ interface{}) error {
				addr, err := address.NewFromBytes([]byte(abytes))
				if err != nil {
					return xerrors.Errorf("parsing address in proving bucket: %w", err)
				}

				// now find the correct bucket
				miner, err := st.GetActor(addr)
				if err != nil {
					return xerrors.Errorf("getting miner %s: %w", addr, err)
				}

				var minerHead actors.StorageMinerActorState
				if err := cst.Get(ctx, miner.Head, &minerHead); err != nil {
					return xerrors.Errorf("reading miner %s state: %w", addr, err)
				}

				correctBucket := minerHead.ElectionPeriodStart % build.SlashablePowerDelay
				if correctBucket != bucketId {
					log.Warnf("miner %s was in wrong proving bucket %d, putting in %d (eps: %d)", addr, bucketId, correctBucket, minerHead.ElectionPeriodStart)
				}

				if _, ok := fixedBuckets[correctBucket]; !ok {
					fixedBuckets[correctBucket] = map[address.Address]struct{}{}
				}
				fixedBuckets[correctBucket][addr] = struct{}{}

				return nil
			})
		}); err != nil {
			return err
		}

		// /////
		// Write fixed buckets

		fixed := amt.NewAMT(amt.WrapBlockstore(sm.cs.Blockstore()))

		for bucketId, addrss := range fixedBuckets {
			bucket := hamt.NewNode(cst)
			for addr := range addrss {
				if err := bucket.Set(ctx, string(addr.Bytes()), actors.CborNull); err != nil {
					return xerrors.Errorf("setting address in bucket: %w", err)
				}
			}

			if err := bucket.Flush(ctx); err != nil {
				return xerrors.Errorf("flushing bucket amt: %w", err)
			}

			bcid, err := cst.Put(context.TODO(), bucket)
			if err != nil {
				return xerrors.Errorf("put bucket: %w", err)
			}

			if err := fixed.Set(bucketId, bcid); err != nil {
				return xerrors.Errorf("set bucket: %w", err)
			}
		}

		head.ProvingBuckets, err = fixed.Flush()
		if err != nil {
			return xerrors.Errorf("flushing bucket amt: %w", err)
		}

		spa.Head, err = cst.Put(ctx, &head)
		if err != nil {
			return xerrors.Errorf("putting actor head: %w", err)
		}

		return nil
	}); err != nil {
		return cid.Undef, err
	}

	return st.Flush()
}
