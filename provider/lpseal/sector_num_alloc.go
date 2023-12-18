package lpseal

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"golang.org/x/xerrors"
)

type AllocAPI interface {
	StateMinerAllocated(context.Context, address.Address, types.TipSetKey) (*bitfield.BitField, error)
}

func AllocateSectorNumbers(ctx context.Context, a AllocAPI, db *harmonydb.DB, maddr address.Address, count int, txcb ...func(*harmonydb.Tx, []abi.SectorNumber) (bool, error)) ([]abi.SectorNumber, error) {
	chainAlloc, err := a.StateMinerAllocated(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, xerrors.Errorf("getting on-chain allocated sector numbers: %w", err)
	}

	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, xerrors.Errorf("getting miner id: %w", err)
	}

	var res []abi.SectorNumber

	comm, err := db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
		// query from db, if exists unmarsal to bitfield
		var dbAllocated bitfield.BitField
		var rawJson []byte

		err = tx.QueryRow("select COALESCE(allocated, '[0]') from sectors_allocated_numbers sa FULL OUTER JOIN (SELECT 1) AS d ON true where sp_id = $1 or sp_id is null", mid).Scan(&rawJson)
		if err != nil {
			return false, xerrors.Errorf("querying allocated sector numbers: %w", err)
		}

		if rawJson != nil {
			err = dbAllocated.UnmarshalJSON(rawJson)
			if err != nil {
				return false, xerrors.Errorf("unmarshaling allocated sector numbers: %w", err)
			}
		}

		if err := dbAllocated.UnmarshalJSON(rawJson); err != nil {
			return false, xerrors.Errorf("unmarshaling allocated sector numbers: %w", err)
		}

		merged, err := bitfield.MergeBitFields(*chainAlloc, dbAllocated)
		if err != nil {
			return false, xerrors.Errorf("merging allocated sector numbers: %w", err)
		}

		allAssignable, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: []rlepluslazy.Run{
			{
				Val: true,
				Len: abi.MaxSectorNumber,
			},
		}})

		inverted, err := bitfield.SubtractBitField(allAssignable, merged)
		if err != nil {
			return false, xerrors.Errorf("subtracting allocated sector numbers: %w", err)
		}

		toAlloc, err := inverted.Slice(0, uint64(count))
		if err != nil {
			return false, xerrors.Errorf("getting slice of allocated sector numbers: %w", err)
		}

		err = toAlloc.ForEach(func(u uint64) error {
			res = append(res, abi.SectorNumber(u))
			return nil
		})
		if err != nil {
			return false, xerrors.Errorf("iterating allocated sector numbers: %w", err)
		}

		toPersist, err := bitfield.MergeBitFields(merged, toAlloc)
		if err != nil {
			return false, xerrors.Errorf("merging allocated sector numbers: %w", err)
		}

		rawJson, err = toPersist.MarshalJSON()
		if err != nil {
			return false, xerrors.Errorf("marshaling allocated sector numbers: %w", err)
		}

		_, err = tx.Exec("insert into sectors_allocated_numbers(sp_id, allocated) values($1, $2) on conflict(sp_id) do update set allocated = $2", mid, rawJson)
		if err != nil {
			return false, xerrors.Errorf("persisting allocated sector numbers: %w", err)
		}

		for i, f := range txcb {
			commit, err = f(tx, res)
			if err != nil {
				return false, xerrors.Errorf("executing tx callback %d: %w", i, err)
			}

			if !commit {
				return false, nil
			}
		}

		return true, nil
	})

	if err != nil {
		return nil, xerrors.Errorf("allocating sector numbers: %w", err)
	}
	if !comm {
		return nil, xerrors.Errorf("allocating sector numbers: commit failed")
	}

	return res, nil
}
