package lpwinning

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	lrand "github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("lpwinning")

type WinPostTask struct {
	max int
	db  *harmonydb.DB

	prover   ProverWinningPoSt
	verifier storiface.Verifier

	api    WinPostAPI
	actors []dtypes.MinerAddress

	mineTF promise.Promise[harmonytask.AddTaskFunc]
}

type WinPostAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainTipSetWeight(context.Context, types.TipSetKey) (types.BigInt, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)

	StateGetBeaconEntry(context.Context, abi.ChainEpoch) (*types.BeaconEntry, error)
	SyncSubmitBlock(context.Context, *types.BlockMsg) error
	StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error)
	StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (api.MinerInfo, error)

	MinerGetBaseInfo(context.Context, address.Address, abi.ChainEpoch, types.TipSetKey) (*api.MiningBaseInfo, error)
	MinerCreateBlock(context.Context, *api.BlockTemplate) (*types.BlockMsg, error)
	MpoolSelect(context.Context, types.TipSetKey, float64) ([]*types.SignedMessage, error)

	WalletSign(context.Context, address.Address, []byte) (*crypto.Signature, error)
}

type ProverWinningPoSt interface {
	GenerateWinningPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, minerID abi.ActorID, sectorInfo []storiface.PostSectorChallenge, randomness abi.PoStRandomness) ([]prooftypes.PoStProof, error)
}

func NewWinPostTask(max int, db *harmonydb.DB, prover ProverWinningPoSt, verifier storiface.Verifier, api WinPostAPI, actors []dtypes.MinerAddress) *WinPostTask {
	t := &WinPostTask{
		max:      max,
		db:       db,
		prover:   prover,
		verifier: verifier,
		api:      api,
		actors:   actors,
	}
	// TODO: run warmup

	go t.mineBasic(context.TODO())

	return t
}

func (t *WinPostTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	log.Debugw("WinPostTask.Do()", "taskID", taskID)

	ctx := context.TODO()

	type BlockCID struct {
		CID string
	}

	type MiningTaskDetails struct {
		SpID      uint64
		Epoch     uint64
		BlockCIDs []BlockCID
		CompTime  time.Time
	}

	var details MiningTaskDetails

	// First query to fetch from mining_tasks
	err = t.db.QueryRow(ctx, `SELECT sp_id, epoch, base_compute_time FROM mining_tasks WHERE task_id = $1`, taskID).Scan(&details.SpID, &details.Epoch, &details.CompTime)
	if err != nil {
		return false, err
	}

	// Second query to fetch from mining_base_block
	rows, err := t.db.Query(ctx, `SELECT block_cid FROM mining_base_block WHERE task_id = $1`, taskID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid BlockCID
		if err := rows.Scan(&cid.CID); err != nil {
			return false, err
		}
		details.BlockCIDs = append(details.BlockCIDs, cid)
	}

	if err := rows.Err(); err != nil {
		return false, err
	}

	// construct base
	maddr, err := address.NewIDAddress(details.SpID)
	if err != nil {
		return false, err
	}

	var bcids []cid.Cid
	for _, c := range details.BlockCIDs {
		bcid, err := cid.Parse(c.CID)
		if err != nil {
			return false, err
		}
		bcids = append(bcids, bcid)
	}

	tsk := types.NewTipSetKey(bcids...)
	baseTs, err := t.api.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return false, xerrors.Errorf("loading base tipset: %w", err)
	}

	base := MiningBase{
		TipSet:      baseTs,
		AddRounds:   abi.ChainEpoch(details.Epoch) - baseTs.Height() - 1,
		ComputeTime: details.CompTime,
	}

	persistNoWin := func() error {
		_, err := t.db.Exec(ctx, `UPDATE mining_base_block SET no_win = true WHERE task_id = $1`, taskID)
		if err != nil {
			return xerrors.Errorf("marking base as not-won: %w", err)
		}

		return nil
	}

	// ensure we have a beacon entry for the epoch we're mining on
	round := base.epoch()

	_ = retry1(func() (*types.BeaconEntry, error) {
		return t.api.StateGetBeaconEntry(ctx, round)
	})

	// MAKE A MINING ATTEMPT!!
	log.Debugw("attempting to mine a block", "tipset", types.LogCids(base.TipSet.Cids()))

	mbi, err := t.api.MinerGetBaseInfo(ctx, maddr, round, base.TipSet.Key())
	if err != nil {
		return false, xerrors.Errorf("failed to get mining base info: %w", err)
	}
	if mbi == nil {
		// not eligible to mine on this base, we're done here
		log.Debugw("WinPoSt not eligible to mine on this base", "tipset", types.LogCids(base.TipSet.Cids()))
		return true, persistNoWin()
	}

	if !mbi.EligibleForMining {
		// slashed or just have no power yet, we're done here
		log.Debugw("WinPoSt not eligible for mining", "tipset", types.LogCids(base.TipSet.Cids()))
		return true, persistNoWin()
	}

	if len(mbi.Sectors) == 0 {
		log.Warnw("WinPoSt no sectors to mine", "tipset", types.LogCids(base.TipSet.Cids()))
		return false, xerrors.Errorf("no sectors selected for winning PoSt")
	}

	var rbase types.BeaconEntry
	var bvals []types.BeaconEntry
	var eproof *types.ElectionProof

	// winner check
	{
		bvals = mbi.BeaconEntries
		rbase = mbi.PrevBeaconEntry
		if len(bvals) > 0 {
			rbase = bvals[len(bvals)-1]
		}

		eproof, err = gen.IsRoundWinner(ctx, round, maddr, rbase, mbi, t.api)
		if err != nil {
			log.Warnw("WinPoSt failed to check if we win next round", "error", err)
			return false, xerrors.Errorf("failed to check if we win next round: %w", err)
		}

		if eproof == nil {
			// not a winner, we're done here
			log.Debugw("WinPoSt not a winner", "tipset", types.LogCids(base.TipSet.Cids()))
			return true, persistNoWin()
		}
	}

	// winning PoSt
	var wpostProof []prooftypes.PoStProof
	{
		buf := new(bytes.Buffer)
		if err := maddr.MarshalCBOR(buf); err != nil {
			err = xerrors.Errorf("failed to marshal miner address: %w", err)
			return false, err
		}

		brand, err := lrand.DrawRandomnessFromBase(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, round, buf.Bytes())
		if err != nil {
			err = xerrors.Errorf("failed to get randomness for winning post: %w", err)
			return false, err
		}

		prand := abi.PoStRandomness(brand)
		prand[31] &= 0x3f // make into fr

		sectorNums := make([]abi.SectorNumber, len(mbi.Sectors))
		for i, s := range mbi.Sectors {
			sectorNums[i] = s.SectorNumber
		}

		ppt, err := mbi.Sectors[0].SealProof.RegisteredWinningPoStProof()
		if err != nil {
			return false, xerrors.Errorf("mapping sector seal proof type to post proof type: %w", err)
		}

		postChallenges, err := ffi.GeneratePoStFallbackSectorChallenges(ppt, abi.ActorID(details.SpID), prand, sectorNums)
		if err != nil {
			return false, xerrors.Errorf("generating election challenges: %v", err)
		}

		sectorChallenges := make([]storiface.PostSectorChallenge, len(mbi.Sectors))
		for i, s := range mbi.Sectors {
			sectorChallenges[i] = storiface.PostSectorChallenge{
				SealProof:    s.SealProof,
				SectorNumber: s.SectorNumber,
				SealedCID:    s.SealedCID,
				Challenge:    postChallenges.Challenges[s.SectorNumber],
				Update:       s.SectorKey != nil,
			}
		}

		wpostProof, err = t.prover.GenerateWinningPoSt(ctx, ppt, abi.ActorID(details.SpID), sectorChallenges, prand)
		if err != nil {
			err = xerrors.Errorf("failed to compute winning post proof: %w", err)
			return false, err
		}
	}

	ticket, err := t.computeTicket(ctx, maddr, &rbase, round, base.TipSet.MinTicket(), mbi)
	if err != nil {
		return false, xerrors.Errorf("scratching ticket failed: %w", err)
	}

	// get pending messages early,
	msgs, err := t.api.MpoolSelect(ctx, base.TipSet.Key(), ticket.Quality())
	if err != nil {
		return false, xerrors.Errorf("failed to select messages for block: %w", err)
	}

	// equivocation handling
	{
		// This next block exists to "catch" equivocating miners,
		// who submit 2 blocks at the same height at different times in order to split the network.
		// To safeguard against this, we make sure it's been EquivocationDelaySecs since our base was calculated,
		// then re-calculate it.
		// If the daemon detected equivocated blocks, those blocks will no longer be in the new base.
		time.Sleep(time.Until(base.ComputeTime.Add(time.Duration(build.EquivocationDelaySecs) * time.Second)))

		bestTs, err := t.api.ChainHead(ctx)
		if err != nil {
			return false, xerrors.Errorf("failed to get chain head: %w", err)
		}

		headWeight, err := t.api.ChainTipSetWeight(ctx, bestTs.Key())
		if err != nil {
			return false, xerrors.Errorf("failed to get chain head weight: %w", err)
		}

		baseWeight, err := t.api.ChainTipSetWeight(ctx, base.TipSet.Key())
		if err != nil {
			return false, xerrors.Errorf("failed to get base weight: %w", err)
		}
		if types.BigCmp(headWeight, baseWeight) <= 0 {
			bestTs = base.TipSet
		}

		// If the base has changed, we take the _intersection_ of our old base and new base,
		// thus ejecting blocks from any equivocating miners, without taking any new blocks.
		if bestTs.Height() == base.TipSet.Height() && !bestTs.Equals(base.TipSet) {
			log.Warnf("base changed from %s to %s, taking intersection", base.TipSet.Key(), bestTs.Key())
			newBaseMap := map[cid.Cid]struct{}{}
			for _, newBaseBlk := range bestTs.Cids() {
				newBaseMap[newBaseBlk] = struct{}{}
			}

			refreshedBaseBlocks := make([]*types.BlockHeader, 0, len(base.TipSet.Cids()))
			for _, baseBlk := range base.TipSet.Blocks() {
				if _, ok := newBaseMap[baseBlk.Cid()]; ok {
					refreshedBaseBlocks = append(refreshedBaseBlocks, baseBlk)
				}
			}

			if len(refreshedBaseBlocks) != 0 && len(refreshedBaseBlocks) != len(base.TipSet.Blocks()) {
				refreshedBase, err := types.NewTipSet(refreshedBaseBlocks)
				if err != nil {
					return false, xerrors.Errorf("failed to create new tipset when refreshing: %w", err)
				}

				if !base.TipSet.MinTicket().Equals(refreshedBase.MinTicket()) {
					log.Warn("recomputing ticket due to base refresh")

					ticket, err = t.computeTicket(ctx, maddr, &rbase, round, refreshedBase.MinTicket(), mbi)
					if err != nil {
						return false, xerrors.Errorf("failed to refresh ticket: %w", err)
					}
				}

				log.Warn("re-selecting messages due to base refresh")
				// refresh messages, as the selected messages may no longer be valid
				msgs, err = t.api.MpoolSelect(ctx, refreshedBase.Key(), ticket.Quality())
				if err != nil {
					return false, xerrors.Errorf("failed to re-select messages for block: %w", err)
				}

				base.TipSet = refreshedBase
			}
		}
	}

	// block construction
	var blockMsg *types.BlockMsg
	{
		uts := base.TipSet.MinTimestamp() + build.BlockDelaySecs*(uint64(base.AddRounds)+1)

		blockMsg, err = t.api.MinerCreateBlock(context.TODO(), &api.BlockTemplate{
			Miner:            maddr,
			Parents:          base.TipSet.Key(),
			Ticket:           ticket,
			Eproof:           eproof,
			BeaconValues:     bvals,
			Messages:         msgs,
			Epoch:            round,
			Timestamp:        uts,
			WinningPoStProof: wpostProof,
		})
		if err != nil {
			return false, xerrors.Errorf("failed to create block: %w", err)
		}
	}

	// persist in db
	{
		bhjson, err := json.Marshal(blockMsg.Header)
		if err != nil {
			return false, xerrors.Errorf("failed to marshal block header: %w", err)
		}

		_, err = t.db.Exec(ctx, `UPDATE mining_tasks
            SET won = true, mined_cid = $2, mined_header = $3, mined_at = $4
            WHERE task_id = $1`, taskID, blockMsg.Header.Cid(), string(bhjson), time.Now().UTC())
		if err != nil {
			return false, xerrors.Errorf("failed to update mining task: %w", err)
		}
	}

	// wait until block timestamp
	{
		time.Sleep(time.Until(time.Unix(int64(blockMsg.Header.Timestamp), 0)))
	}

	// submit block!!
	{
		if err := t.api.SyncSubmitBlock(ctx, blockMsg); err != nil {
			return false, xerrors.Errorf("failed to submit block: %w", err)
		}
	}

	log.Infow("mined a block", "tipset", types.LogCids(blockMsg.Header.Parents), "height", blockMsg.Header.Height, "miner", maddr, "cid", blockMsg.Header.Cid())

	// persist that we've submitted the block
	{
		_, err = t.db.Exec(ctx, `UPDATE mining_tasks
		SET submitted_at = $2
		WHERE task_id = $1`, taskID, time.Now().UTC())
		if err != nil {
			return false, xerrors.Errorf("failed to update mining task: %w", err)
		}
	}

	return true, nil
}

func (t *WinPostTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	if len(ids) == 0 {
		// probably can't happen, but panicking is bad
		return nil, nil
	}

	// select lowest epoch
	var lowestEpoch abi.ChainEpoch
	var lowestEpochID = ids[0]
	for _, id := range ids {
		var epoch uint64
		err := t.db.QueryRow(context.Background(), `SELECT epoch FROM mining_tasks WHERE task_id = $1`, id).Scan(&epoch)
		if err != nil {
			return nil, err
		}

		if lowestEpoch == 0 || abi.ChainEpoch(epoch) < lowestEpoch {
			lowestEpoch = abi.ChainEpoch(epoch)
			lowestEpochID = id
		}
	}

	return &lowestEpochID, nil
}

func (t *WinPostTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Name:        "WinPost",
		Max:         t.max,
		MaxFailures: 3,
		Follows:     nil,
		Cost: resources.Resources{
			Cpu: 1,

			// todo set to something for 32/64G sector sizes? Technically windowPoSt is happy on a CPU
			//  but it will use a GPU if available
			Gpu: 0,

			Ram: 1 << 30, // todo arbitrary number
		},
	}
}

func (t *WinPostTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	t.mineTF.Set(taskFunc)
}

// MiningBase is the tipset on top of which we plan to construct our next block.
// Refer to godocs on GetBestMiningCandidate.
type MiningBase struct {
	TipSet      *types.TipSet
	ComputeTime time.Time
	AddRounds   abi.ChainEpoch
}

func (mb MiningBase) epoch() abi.ChainEpoch {
	// return the epoch that will result from mining on this base
	return mb.TipSet.Height() + mb.AddRounds + 1
}

func (mb MiningBase) baseTime() time.Time {
	tsTime := time.Unix(int64(mb.TipSet.MinTimestamp()), 0)
	roundDelay := build.BlockDelaySecs * uint64(mb.AddRounds+1)
	tsTime = tsTime.Add(time.Duration(roundDelay) * time.Second)
	return tsTime
}

func (mb MiningBase) afterPropDelay() time.Time {
	return mb.baseTime().Add(randTimeOffset(time.Second))
}

func (t *WinPostTask) mineBasic(ctx context.Context) {
	var workBase MiningBase

	taskFn := t.mineTF.Val(ctx)

	// initialize workbase
	{
		head := retry1(func() (*types.TipSet, error) {
			return t.api.ChainHead(ctx)
		})

		workBase = MiningBase{
			TipSet:      head,
			AddRounds:   0,
			ComputeTime: time.Now(),
		}
	}

	/*

		         /- T+0 == workBase.baseTime
		         |
		>--------*------*--------[wait until next round]----->
		                |
		                |- T+PD == workBase.afterPropDelay+(~1s)
		                |- Here we acquire the new workBase, and start a new round task
		                \- Then we loop around, and wait for the next head

		time -->
	*/

	for {
		// limit the rate at which we mine blocks to at least EquivocationDelaySecs
		// this is to prevent races on devnets in catch up mode. Acts as a minimum
		// delay for the sleep below.
		time.Sleep(time.Duration(build.EquivocationDelaySecs)*time.Second + time.Second)

		// wait for *NEXT* propagation delay
		time.Sleep(time.Until(workBase.afterPropDelay()))

		// check current best candidate
		maybeBase := retry1(func() (*types.TipSet, error) {
			return t.api.ChainHead(ctx)
		})

		if workBase.TipSet.Equals(maybeBase) {
			// workbase didn't change in the new round so we have a null round here
			workBase.AddRounds++
			log.Debugw("workbase update", "tipset", workBase.TipSet.Cids(), "nulls", workBase.AddRounds, "lastUpdate", time.Since(workBase.ComputeTime), "type", "same-tipset")
		} else {
			btsw := retry1(func() (types.BigInt, error) {
				return t.api.ChainTipSetWeight(ctx, maybeBase.Key())
			})

			ltsw := retry1(func() (types.BigInt, error) {
				return t.api.ChainTipSetWeight(ctx, workBase.TipSet.Key())
			})

			if types.BigCmp(btsw, ltsw) <= 0 {
				// new tipset for some reason has less weight than the old one, assume null round here
				// NOTE: the backing node may have reorged, or manually changed head
				workBase.AddRounds++
				log.Debugw("workbase update", "tipset", workBase.TipSet.Cids(), "nulls", workBase.AddRounds, "lastUpdate", time.Since(workBase.ComputeTime), "type", "prefer-local-weight")
			} else {
				// new tipset has more weight, so we should mine on it, no null round here
				log.Debugw("workbase update", "tipset", workBase.TipSet.Cids(), "nulls", workBase.AddRounds, "lastUpdate", time.Since(workBase.ComputeTime), "type", "prefer-new-tipset")

				workBase = MiningBase{
					TipSet:      maybeBase,
					AddRounds:   0,
					ComputeTime: time.Now(),
				}
			}
		}

		// dispatch mining task
		// (note equivocation prevention is handled by the mining code)

		baseEpoch := workBase.TipSet.Height()

		for _, act := range t.actors {
			spID, err := address.IDFromAddress(address.Address(act))
			if err != nil {
				log.Errorf("failed to get spID from address %s: %s", act, err)
				continue
			}

			taskFn(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
				// First we check if the mining base includes blocks we may have mined previously to avoid getting slashed
				// select mining_tasks where epoch==base_epoch if win=true to maybe get base block cid which has to be included in our tipset
				var baseBlockCids []string
				err := tx.Select(&baseBlockCids, `SELECT mined_cid FROM mining_tasks WHERE epoch = $1 AND sp_id = $2 AND won = true`, baseEpoch, spID)
				if err != nil {
					return false, xerrors.Errorf("querying mining_tasks: %w", err)
				}
				if len(baseBlockCids) >= 1 {
					baseBlockCid := baseBlockCids[0]
					c, err := cid.Parse(baseBlockCid)
					if err != nil {
						return false, xerrors.Errorf("parsing mined_cid: %w", err)
					}

					// we have mined in the previous round, make sure that our block is included in the tipset
					// if it's not we risk getting slashed

					var foundOurs bool
					for _, c2 := range workBase.TipSet.Cids() {
						if c == c2 {
							foundOurs = true
							break
						}
					}
					if !foundOurs {
						log.Errorw("our block was not included in the tipset, aborting", "tipset", workBase.TipSet.Cids(), "ourBlock", c)
						return false, xerrors.Errorf("our block was not included in the tipset, aborting")
					}
				}

				_, err = tx.Exec(`INSERT INTO mining_tasks (task_id, sp_id, epoch, base_compute_time) VALUES ($1, $2, $3, $4)`, id, spID, workBase.epoch(), workBase.ComputeTime.UTC())
				if err != nil {
					return false, xerrors.Errorf("inserting mining_tasks: %w", err)
				}

				for _, c := range workBase.TipSet.Cids() {
					_, err = tx.Exec(`INSERT INTO mining_base_block (task_id, sp_id, block_cid) VALUES ($1, $2, $3)`, id, spID, c)
					if err != nil {
						return false, xerrors.Errorf("inserting mining base blocks: %w", err)
					}
				}

				return true, nil // no errors, commit the transaction
			})
		}
	}
}

func (t *WinPostTask) computeTicket(ctx context.Context, maddr address.Address, brand *types.BeaconEntry, round abi.ChainEpoch, chainRand *types.Ticket, mbi *api.MiningBaseInfo) (*types.Ticket, error) {
	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal address to cbor: %w", err)
	}

	if round > build.UpgradeSmokeHeight {
		buf.Write(chainRand.VRFProof)
	}

	input, err := lrand.DrawRandomnessFromBase(brand.Data, crypto.DomainSeparationTag_TicketProduction, round-build.TicketRandomnessLookback, buf.Bytes())
	if err != nil {
		return nil, err
	}

	vrfOut, err := gen.ComputeVRF(ctx, t.api.WalletSign, mbi.WorkerKey, input)
	if err != nil {
		return nil, err
	}

	return &types.Ticket{
		VRFProof: vrfOut,
	}, nil
}

func randTimeOffset(width time.Duration) time.Duration {
	buf := make([]byte, 8)
	rand.Reader.Read(buf) //nolint:errcheck
	val := time.Duration(binary.BigEndian.Uint64(buf) % uint64(width))

	return val - (width / 2)
}

func retry1[R any](f func() (R, error)) R {
	for {
		r, err := f()
		if err == nil {
			return r
		}

		log.Errorw("error in mining loop, retrying", "error", err)
		time.Sleep(time.Second)
	}
}

var _ harmonytask.TaskInterface = &WinPostTask{}
