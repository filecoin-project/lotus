package lpseal

import (
	"context"
	"net/url"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

func DropSectorPieceRefs(ctx context.Context, db *harmonydb.DB, sid abi.SectorID) error {
	//_, err := db.Exec(ctx, `SELECT FROM sectors_sdr_initial_pieces WHERE sp_id = $1 AND sector_number = $2`, sid.Miner, sid.Number)

	var PieceURL []struct {
		URL string `db:"data_url"`
	}

	err := db.Select(ctx, &PieceURL, `SELECT data_url FROM sectors_sdr_initial_pieces WHERE sp_id = $1 AND sector_number = $2`, sid.Miner, sid.Number)
	if err != nil {
		return xerrors.Errorf("getting piece url: %w", err)
	}

	for _, pu := range PieceURL {
		gourl, err := url.Parse(pu.URL)
		if err != nil {
			log.Errorw("failed to parse piece url", "url", pu.URL, "error", err, "miner", sid.Miner, "sector", sid.Number)
			continue
		}

		if gourl.Scheme == "pieceref" {
			refID, err := strconv.ParseInt(gourl.Opaque, 10, 64)
			if err != nil {
				log.Errorw("failed to parse piece ref id", "url", pu.URL, "error", err, "miner", sid.Miner, "sector", sid.Number)
				continue
			}

			n, err := db.Exec(ctx, `DELETE FROM parked_piece_refs WHERE ref_id = $1`, refID)
			if err != nil {
				log.Errorw("failed to delete piece ref", "url", pu.URL, "error", err, "miner", sid.Miner, "sector", sid.Number)
			}

			log.Debugw("deleted piece ref", "url", pu.URL, "miner", sid.Miner, "sector", sid.Number, "rows", n)
		}
	}

	return err
}
