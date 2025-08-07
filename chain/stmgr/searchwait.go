package stmgr

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var ErrFailedToLoadMessage = errors.New("failed to load message")

// WaitForMessage blocks until a message appears on chain. It looks backwards in the chain to see if this has already
// happened, with an optional limit to how many epochs it will search. It guarantees that the message has been on
// chain for at least confidence epochs without being reverted before returning.
func (sm *StateManager) WaitForMessage(ctx context.Context, mcid cid.Cid, confidence uint64, lookbackLimit abi.ChainEpoch, allowReplaced bool) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {
	// TODO use the index to speed this up.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	msg, err := sm.cs.GetCMessage(ctx, mcid)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("%w: %w", ErrFailedToLoadMessage, err)
	}

	tsub := sm.cs.SubHeadChanges(ctx)

	head, ok := <-tsub
	if !ok {
		return nil, nil, cid.Undef, xerrors.Errorf("SubHeadChanges stream was invalid")
	}

	if len(head) != 1 {
		return nil, nil, cid.Undef, xerrors.Errorf("SubHeadChanges first entry should have been one item")
	}

	if head[0].Type != store.HCCurrent {
		return nil, nil, cid.Undef, xerrors.Errorf("expected current head on SHC stream (got %s)", head[0].Type)
	}

	r, foundMsg, err := sm.tipsetExecutedMessage(ctx, head[0].Val, mcid, msg.VMMessage(), allowReplaced)
	if err != nil {
		return nil, nil, cid.Undef, err
	}

	if r != nil {
		return head[0].Val, r, foundMsg, nil
	}

	var backTs *types.TipSet
	var backRcp *types.MessageReceipt
	var backFm cid.Cid
	backSearchWait := make(chan struct{})
	go func() {
		fts, r, foundMsg, err := sm.searchForIndexedMsg(ctx, mcid, msg)

		found := (err == nil && r != nil && foundMsg.Defined())
		if !found {
			fts, r, foundMsg, err = sm.searchBackForMsg(ctx, head[0].Val, msg, lookbackLimit, allowReplaced)
			if err != nil {
				log.Warnf("failed to look back through chain for message: %v", err)
				return
			}
		}

		backTs = fts
		backRcp = r
		backFm = foundMsg
		close(backSearchWait)
	}()

	var candidateTs *types.TipSet
	var candidateRcp *types.MessageReceipt
	var candidateFm cid.Cid
	heightOfHead := head[0].Val.Height()
	reverts := map[types.TipSetKey]bool{}

	for {
		select {
		case notif, ok := <-tsub:
			if !ok {
				return nil, nil, cid.Undef, ctx.Err()
			}
			for _, val := range notif {
				switch val.Type {
				case store.HCRevert:
					if val.Val.Equals(candidateTs) {
						candidateTs = nil
						candidateRcp = nil
						candidateFm = cid.Undef
					}
					if backSearchWait != nil {
						reverts[val.Val.Key()] = true
					}
				case store.HCApply:
					if candidateTs != nil && val.Val.Height() >= candidateTs.Height()+abi.ChainEpoch(confidence) {
						return candidateTs, candidateRcp, candidateFm, nil
					}
					r, foundMsg, err := sm.tipsetExecutedMessage(ctx, val.Val, mcid, msg.VMMessage(), allowReplaced)
					if err != nil {
						return nil, nil, cid.Undef, err
					}
					if r != nil {
						if confidence == 0 {
							return val.Val, r, foundMsg, err
						}
						candidateTs = val.Val
						candidateRcp = r
						candidateFm = foundMsg
					}
					heightOfHead = val.Val.Height()
				}
			}
		case <-backSearchWait:
			// check if we found the message in the chain and that is hasn't been reverted since we started searching
			if backTs != nil && !reverts[backTs.Key()] {
				// if head is at or past confidence interval, return immediately
				if heightOfHead >= backTs.Height()+abi.ChainEpoch(confidence) {
					return backTs, backRcp, backFm, nil
				}

				// wait for confidence interval
				candidateTs = backTs
				candidateRcp = backRcp
				candidateFm = backFm
			}
			reverts = nil
			backSearchWait = nil
		case <-ctx.Done():
			return nil, nil, cid.Undef, ctx.Err()
		}
	}
}

func (sm *StateManager) SearchForMessage(ctx context.Context, head *types.TipSet, mcid cid.Cid, lookbackLimit abi.ChainEpoch, allowReplaced bool) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {
	msg, err := sm.cs.GetCMessage(ctx, mcid)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("%w: %w", ErrFailedToLoadMessage, err)
	}

	r, foundMsg, err := sm.tipsetExecutedMessage(ctx, head, mcid, msg.VMMessage(), allowReplaced)
	if err != nil {
		return nil, nil, cid.Undef, err
	}

	if r != nil {
		return head, r, foundMsg, nil
	}

	fts, r, foundMsg, err := sm.searchForIndexedMsg(ctx, mcid, msg)

	switch {
	case err == nil:
		if r != nil && foundMsg.Defined() {
			return fts, r, foundMsg, nil
		}

		// debug log this, it's noteworthy
		if r == nil {
			log.Debugf("missing receipt for message in index for %s", mcid)
		}
		if !foundMsg.Defined() {
			log.Debugf("message %s not found", mcid)
		}

	case errors.Is(err, index.ErrNotFound):
		// ok for the index to have incomplete data

	default:
		log.Warnf("error searching message index: %s", err)
	}

	fts, r, foundMsg, err = sm.searchBackForMsg(ctx, head, msg, lookbackLimit, allowReplaced)

	if err != nil {
		log.Warnf("failed to look back through chain for message %s", mcid)
		return nil, nil, cid.Undef, err
	}

	if fts == nil {
		return nil, nil, cid.Undef, nil
	}

	return fts, r, foundMsg, nil
}

func (sm *StateManager) searchForIndexedMsg(ctx context.Context, mcid cid.Cid, m types.ChainMsg) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {
	if sm.chainIndexer == nil {
		return nil, nil, cid.Undef, index.ErrNotFound
	}
	minfo, err := sm.chainIndexer.GetMsgInfo(ctx, mcid)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("error looking up message in index: %w", err)
	}

	// check the height against the current tipset; minimum execution confidence requires that the
	// inclusion tipset height is lower than the current head + 1
	curTs := sm.cs.GetHeaviestTipSet()
	if curTs.Height() <= minfo.Epoch+1 {
		return nil, nil, cid.Undef, xerrors.Errorf("indexed message does not appear before the current tipset; index epoch: %d, current epoch: %d", minfo.Epoch, curTs.Height())
	}

	// now get the execution tipset
	// TODO optimization: the index should have it implicitly so we can return it in the msginfo.
	xts, err := sm.cs.GetTipsetByHeight(ctx, minfo.Epoch+1, curTs, false)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("error looking up execution tipset: %w", err)
	}

	// check that the parent of the execution index is indeed the inclusion tipset
	parent := xts.Parents()
	parentCid, err := parent.Cid()
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("error computing tipset cid: %w", err)
	}

	if !parentCid.Equals(minfo.TipSet) {
		return nil, nil, cid.Undef, xerrors.Errorf("inclusion tipset mismatch: have %s, expected %s", parentCid, minfo.TipSet)
	}

	r, foundMsg, err := sm.tipsetExecutedMessage(ctx, xts, mcid, m.VMMessage(), false)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("error in tipstExecutedMessage: %w", err)
	}
	return xts, r, foundMsg, nil
}

// searchBackForMsg searches up to limit tipsets backwards from the given
// tipset for a message receipt.
// If limit is
// - 0 then no tipsets are searched
// - 5 then five tipset are searched
// - LookbackNoLimit then there is no limit
func (sm *StateManager) searchBackForMsg(ctx context.Context, from *types.TipSet, m types.ChainMsg, limit abi.ChainEpoch, allowReplaced bool) (*types.TipSet, *types.MessageReceipt, cid.Cid, error) {
	limitHeight := from.Height() - limit
	noLimit := limit == LookbackNoLimit

	cur := from
	curActor, err := sm.LoadActor(ctx, m.VMMessage().From, cur)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("failed to load initial tipset")
	}

	mFromId, err := sm.LookupIDAddress(ctx, m.VMMessage().From, from)
	if err != nil {
		return nil, nil, cid.Undef, xerrors.Errorf("looking up From id address: %w", err)
	}

	mNonce := m.VMMessage().Nonce

	for {
		// If we've reached the genesis block, or we've reached the limit of
		// how far back to look
		if cur.Height() == 0 || !noLimit && cur.Height() <= limitHeight {
			// it ain't here!
			return nil, nil, cid.Undef, nil
		}

		select {
		case <-ctx.Done():
			return nil, nil, cid.Undef, nil
		default:
		}

		// we either have no messages from the sender, or the latest message we found has a lower nonce than the one being searched for,
		// either way, no reason to lookback, it ain't there
		if curActor == nil || curActor.Nonce == 0 || curActor.Nonce < mNonce {
			return nil, nil, cid.Undef, nil
		}

		pts, err := sm.cs.LoadTipSet(ctx, cur.Parents())
		if err != nil {
			return nil, nil, cid.Undef, xerrors.Errorf("failed to load tipset during msg wait searchback: %w", err)
		}

		act, err := sm.LoadActor(ctx, mFromId, pts)
		actorNoExist := errors.Is(err, types.ErrActorNotFound)
		if err != nil && !actorNoExist {
			return nil, nil, cid.Cid{}, xerrors.Errorf("failed to load the actor: %w", err)
		}

		// check that between cur and parent tipset the nonce fell into range of our message
		if actorNoExist || (curActor.Nonce > mNonce && act.Nonce <= mNonce) {
			r, foundMsg, err := sm.tipsetExecutedMessage(ctx, cur, m.Cid(), m.VMMessage(), allowReplaced)
			if err != nil {
				return nil, nil, cid.Undef, xerrors.Errorf("checking for message execution during lookback: %w", err)
			}

			if r != nil {
				return cur, r, foundMsg, nil
			}
		}

		cur = pts
		curActor = act
	}
}

func (sm *StateManager) tipsetExecutedMessage(ctx context.Context, ts *types.TipSet, msg cid.Cid, vmm *types.Message, allowReplaced bool) (*types.MessageReceipt, cid.Cid, error) {
	// The genesis block did not execute any messages
	if ts.Height() == 0 {
		return nil, cid.Undef, nil
	}

	pts, err := sm.cs.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return nil, cid.Undef, err
	}

	cm, err := sm.cs.MessagesForTipset(ctx, pts)
	if err != nil {
		return nil, cid.Undef, err
	}

	for ii := range cm {
		// iterate in reverse because we going backwards through the chain
		i := len(cm) - ii - 1
		m := cm[i]

		if m.VMMessage().From == vmm.From { // cheaper to just check origin first
			if m.VMMessage().Nonce == vmm.Nonce {
				if !m.VMMessage().EqualCall(vmm) {
					// this is an entirely different message, fail
					return nil, cid.Undef, xerrors.Errorf("found message with equal nonce as the one we are looking for that is NOT a valid replacement message (F:%s n %d, TS: %s n%d)",
						msg, vmm.Nonce, m.Cid(), m.VMMessage().Nonce)
				}

				if m.Cid() != msg {
					if !allowReplaced {
						log.Warnw("found message with equal nonce and call params but different CID",
							"wanted", msg, "found", m.Cid(), "nonce", vmm.Nonce, "from", vmm.From)
						return nil, cid.Undef, xerrors.Errorf("found message with equal nonce as the one we are looking for (F:%s n %d, TS: %s n%d)",
							msg, vmm.Nonce, m.Cid(), m.VMMessage().Nonce)
					}
				}

				pr, err := sm.cs.GetParentReceipt(ctx, ts.Blocks()[0], i)
				if err != nil {
					return nil, cid.Undef, err
				}

				return pr, m.Cid(), nil
			}
			if m.VMMessage().Nonce < vmm.Nonce {
				return nil, cid.Undef, nil // don't bother looking further
			}
		}
	}

	return nil, cid.Undef, nil
}
