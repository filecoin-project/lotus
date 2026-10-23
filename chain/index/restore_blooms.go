package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// restoreBloomCandidatesQuery selects non-reverted indexed tipsets in a height
// range whose bloom is missing or an all-0xff placeholder, with what the index
// holds for them.
const restoreBloomCandidatesQuery = `
WITH c AS (
	SELECT tipset_key_cid, MIN(height) AS h, MAX(message_cid IS NOT NULL) AS has_messages
	FROM tipset_message
	WHERE height BETWEEN ? AND ?
	GROUP BY tipset_key_cid
	HAVING MAX(reverted) = 0
)
SELECT c.tipset_key_cid, c.h, c.has_messages,
	EXISTS (SELECT 1 FROM event e JOIN tipset_message tm ON e.message_id = tm.id
		WHERE tm.tipset_key_cid = c.tipset_key_cid AND e.reverted = 0) AS has_events,
	EXISTS (SELECT 1 FROM event e JOIN tipset_message tm ON e.message_id = tm.id
		WHERE tm.tipset_key_cid = c.tipset_key_cid AND e.reverted = 1) AS has_reverted_events
FROM c
LEFT JOIN tipset_bloom b ON b.tipset_key_cid = c.tipset_key_cid
WHERE b.tipset_key_cid IS NULL OR b.bloom = ?
ORDER BY c.h DESC`

type RestoreTipsetBloomsOptions struct {
	// From and To bound the heights to restore, inclusive, with From >= To.
	From, To abi.ChainEpoch
	// Batch is the number of epochs restored per transaction.
	Batch abi.ChainEpoch
	// QuietStretch is the span of restored tipsets without indexed events,
	// after FEVM, worth reporting for verification against the chain.
	QuietStretch abi.ChainEpoch
	// DryRun counts the tipsets that would be restored without writing.
	DryRun bool
	// Progress, if set, is called after each batch with the batch bounds and
	// the running counts.
	Progress func(lo, hi abi.ChainEpoch, counts *RestoreTipsetBloomsCounts)
}

type RestoreTipsetBloomsCounts struct {
	// Restored tipsets whose indexed events are known to be complete.
	NoMessages   int
	WithEvents   int
	BeforeEvents int
	// Unverified counts restored tipsets with messages but no indexed events
	// after FEVM: either they emitted none, or their events were never
	// indexed, and only the chain can tell.
	Unverified int
	// QuietStretches lists spans of at least QuietStretch epochs where every
	// restored tipset is unverified or has no messages, highest first.
	QuietStretches []HeightRange
	// RevertedEvents lists tipsets left alone because the index holds reverted
	// events for them, which only happens when their executing tipset was
	// reverted and not replaced.
	RevertedEvents []abi.ChainEpoch

	// quiet tracks the stretch being walked; quietFrom is 0 when none is open
	quietFrom, quietTo abi.ChainEpoch
}

const quietGap = 20

type HeightRange struct {
	From, To abi.ChainEpoch
}

func (c *RestoreTipsetBloomsCounts) Restored() int {
	return c.NoMessages + c.WithEvents + c.BeforeEvents + c.Unverified
}

type bloomClass int

const (
	bloomNoMessages bloomClass = iota
	bloomWithEvents
	bloomBeforeEvents
	bloomUnverified
	bloomRevertedEvents
)

// add records a classified tipset; tipsets arrive highest first.
func (c *RestoreTipsetBloomsCounts) add(h abi.ChainEpoch, class bloomClass, quietStretch abi.ChainEpoch) {
	switch class {
	case bloomNoMessages:
		c.NoMessages++
		return
	case bloomWithEvents:
		c.WithEvents++
		c.closeQuiet(quietStretch)
	case bloomBeforeEvents:
		c.BeforeEvents++
		c.closeQuiet(quietStretch)
	case bloomUnverified:
		c.Unverified++
		// Tipsets missing from the candidates already have blooms, and may
		// have events, so only a short gap (null rounds) continues a stretch
		if c.quietFrom != 0 && c.quietTo-h > quietGap {
			c.closeQuiet(quietStretch)
		}
		if c.quietFrom == 0 {
			c.quietFrom = h
		}
		c.quietTo = h
	case bloomRevertedEvents:
		c.RevertedEvents = append(c.RevertedEvents, h)
	}
}

func (c *RestoreTipsetBloomsCounts) closeQuiet(quietStretch abi.ChainEpoch) {
	if c.quietFrom != 0 && c.quietFrom-c.quietTo+1 >= quietStretch {
		c.QuietStretches = append(c.QuietStretches, HeightRange{From: c.quietFrom, To: c.quietTo})
	}
	c.quietFrom, c.quietTo = 0, 0
}

// IndexedRange returns the lowest indexed height and the highest non-reverted
// indexed height, and false if the index is empty.
func (si *SqliteIndexer) IndexedRange(ctx context.Context) (lowest, highest abi.ChainEpoch, ok bool, err error) {
	var lo, hi sql.NullInt64
	// Separate subqueries so each uses the height index rather than a scan
	err = si.db.QueryRowContext(ctx, `SELECT
		(SELECT MIN(height) FROM tipset_message),
		(SELECT MAX(height) FROM tipset_message WHERE reverted = 0)`).Scan(&lo, &hi)
	if err != nil {
		return 0, 0, false, xerrors.Errorf("failed to query indexed height range: %w", err)
	}
	if !lo.Valid || !hi.Valid {
		return 0, 0, false, nil
	}
	return abi.ChainEpoch(lo.Int64), abi.ChainEpoch(hi.Int64), true, nil
}

// RestoreTipsetBlooms writes tipset blooms that are missing or all-0xff
// placeholders, building each from the tipset's indexed events.
//
// The index can't always prove a tipset's events are complete. It can for
// tipsets with no messages, with indexed events (all of a tipset's events are
// indexed in one transaction), or from before FEVM. A tipset with messages but
// no indexed events may have emitted none, or its events may never have been
// indexed, for example if it was indexed from a snapshot import or with event
// storage off. These are restored with an empty bloom and counted as
// unverified, and long stretches of them are reported: a wrong empty bloom
// answers "no events" where the index would otherwise fail the query, and
// validate-backfill against the chain finds and repairs it. Tipsets with
// reverted events are left alone and reported.
//
// It only reads the index, so the indexer can be opened without a chain store.
// It walks down from opts.From in batches. The indexer must not be running:
// callers hold the repo lock and never Start it.
func (si *SqliteIndexer) RestoreTipsetBlooms(ctx context.Context, opts RestoreTipsetBloomsOptions) (*RestoreTipsetBloomsCounts, error) {
	counts := &RestoreTipsetBloomsCounts{}
	if opts.From < opts.To {
		return counts, fmt.Errorf("from (%d) must not be below to (%d)", opts.From, opts.To)
	}
	if opts.Batch < 1 || opts.QuietStretch < 1 {
		return counts, errors.New("batch and quiet stretch must be at least 1")
	}

	type candidate struct {
		key               []byte
		height            abi.ChainEpoch
		hasMessages       bool
		hasEvents         bool
		hasRevertedEvents bool
		class             bloomClass
	}

	placeholder := ethtypes.NewFullEthBloom()
	for hi := opts.From; hi >= opts.To; hi -= opts.Batch {
		lo := max(hi-opts.Batch+1, opts.To)

		var candidates []candidate
		err := withTx(ctx, si.db, func(tx *sql.Tx) error {
			candidates = candidates[:0]
			rows, err := tx.QueryContext(ctx, restoreBloomCandidatesQuery, lo, hi, placeholder)
			if err != nil {
				return xerrors.Errorf("failed to query tipsets missing blooms: %w", err)
			}
			for rows.Next() {
				var c candidate
				if err := rows.Scan(&c.key, &c.height, &c.hasMessages, &c.hasEvents, &c.hasRevertedEvents); err != nil {
					_ = rows.Close()
					return xerrors.Errorf("failed to read tipset missing a bloom: %w", err)
				}
				switch {
				case c.hasRevertedEvents:
					c.class = bloomRevertedEvents
				case !c.hasMessages:
					c.class = bloomNoMessages
				case c.hasEvents:
					c.class = bloomWithEvents
				case c.height < buildconstants.UpgradeHyggeHeight:
					c.class = bloomBeforeEvents
				default:
					c.class = bloomUnverified
				}
				candidates = append(candidates, c)
			}
			if err := rows.Close(); err != nil {
				return xerrors.Errorf("failed to read tipsets missing blooms: %w", err)
			}
			if err := rows.Err(); err != nil {
				return xerrors.Errorf("failed to read tipsets missing blooms: %w", err)
			}

			if opts.DryRun {
				return nil
			}
			for _, c := range candidates {
				if c.class == bloomRevertedEvents {
					continue
				}
				bloom := ethtypes.NewEmptyEthBloom()
				if err := si.buildTipsetBloomFromIndex(ctx, tx, c.key, bloom); err != nil {
					return xerrors.Errorf("failed to build bloom for tipset at height %d: %w", c.height, err)
				}
				if err := si.upsertTipsetBloom(ctx, tx, c.key, c.height, bloom); err != nil {
					return xerrors.Errorf("failed to store bloom for tipset at height %d: %w", c.height, err)
				}
			}
			return nil
		})
		if err != nil {
			return counts, err
		}

		for _, c := range candidates {
			counts.add(c.height, c.class, opts.QuietStretch)
		}
		if opts.Progress != nil {
			opts.Progress(lo, hi, counts)
		}
	}
	counts.closeQuiet(opts.QuietStretch)
	return counts, nil
}
