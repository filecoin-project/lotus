package messagepool

import (
	"context"
	"fmt"
	stdbig "math/big"
	"sort"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

var baseFeeUpperBoundFactor = types.NewInt(10)

// CheckMessages performs a set of logic checks for a list of messages, prior to submitting it to the mpool
func (mp *MessagePool) CheckMessages(ctx context.Context, protos []*api.MessagePrototype) ([][]api.MessageCheckStatus, error) {
	flex := make([]bool, len(protos))
	msgs := make([]*types.Message, len(protos))
	for i, p := range protos {
		flex[i] = !p.ValidNonce
		msgs[i] = &p.Message
	}
	return mp.checkMessages(ctx, msgs, false, flex)
}

// CheckPendingMessages performs a set of logical sets for all messages pending from a given actor
func (mp *MessagePool) CheckPendingMessages(ctx context.Context, from address.Address) ([][]api.MessageCheckStatus, error) {
	var msgs []*types.Message
	mp.lk.RLock()
	mset, ok, err := mp.getPendingMset(ctx, from)
	if err != nil {
		mp.lk.RUnlock()
		return nil, xerrors.Errorf("errored while getting pending mset: %w", err)
	}
	if ok {
		msgs = make([]*types.Message, 0, len(mset.msgs))
		for _, sm := range mset.msgs {
			msgs = append(msgs, &sm.Message)
		}
	}
	mp.lk.RUnlock()

	if len(msgs) == 0 {
		return nil, nil
	}

	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Nonce < msgs[j].Nonce
	})

	return mp.checkMessages(ctx, msgs, true, nil)
}

// CheckReplaceMessages performs a set of logical checks for related messages while performing a
// replacement.
func (mp *MessagePool) CheckReplaceMessages(ctx context.Context, replace []*types.Message) ([][]api.MessageCheckStatus, error) {
	msgMap := make(map[address.Address]map[uint64]*types.Message)
	count := 0

	mp.lk.RLock()
	for _, m := range replace {
		mmap, ok := msgMap[m.From]
		if !ok {
			mmap = make(map[uint64]*types.Message)
			msgMap[m.From] = mmap
			mset, ok, err := mp.getPendingMset(ctx, m.From)
			if err != nil {
				mp.lk.RUnlock()
				return nil, xerrors.Errorf("errored while getting pending mset: %w", err)
			}
			if ok {
				count += len(mset.msgs)
				for _, sm := range mset.msgs {
					mmap[sm.Message.Nonce] = &sm.Message
				}
			} else {
				count++
			}
		}
		mmap[m.Nonce] = m
	}
	mp.lk.RUnlock()

	msgs := make([]*types.Message, 0, count)
	start := 0
	for _, mmap := range msgMap {
		end := start + len(mmap)

		for _, m := range mmap {
			msgs = append(msgs, m)
		}

		sort.Slice(msgs[start:end], func(i, j int) bool {
			return msgs[start+i].Nonce < msgs[start+j].Nonce
		})

		start = end
	}

	return mp.checkMessages(ctx, msgs, true, nil)
}

// checkMessages should be either nil or of len(msgs), it signifies that message at given index
// has non-determied nonce at this point
func (mp *MessagePool) checkMessages(ctx context.Context, msgs []*types.Message, interned bool, flexibleNonces []bool) (result [][]api.MessageCheckStatus, err error) {
	if mp.api.IsLite() {
		return nil, nil
	}
	mp.curTsLk.RLock()
	curTs := mp.curTs
	mp.curTsLk.RUnlock()

	epoch := curTs.Height() + 1

	var baseFee big.Int
	if len(curTs.Blocks()) > 0 {
		baseFee = curTs.Blocks()[0].ParentBaseFee
	} else {
		baseFee, err = mp.api.ChainComputeBaseFee(context.Background(), curTs)
		if err != nil {
			return nil, xerrors.Errorf("error computing basefee: %w", err)
		}
	}

	baseFeeLowerBound := getBaseFeeLowerBound(baseFee, baseFeeLowerBoundFactor)
	baseFeeUpperBound := types.BigMul(baseFee, baseFeeUpperBoundFactor)

	type actorState struct {
		nextNonce     uint64
		requiredFunds *stdbig.Int
	}

	state := make(map[address.Address]*actorState)
	balances := make(map[address.Address]big.Int)

	result = make([][]api.MessageCheckStatus, len(msgs))

	for i, m := range msgs {
		// pre-check: actor nonce
		check := api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageGetStateNonce,
			},
		}

		st, ok := state[m.From]
		if !ok {
			mp.lk.RLock()
			mset, ok, err := mp.getPendingMset(ctx, m.From)
			if err != nil {
				mp.lk.RUnlock()
				return nil, xerrors.Errorf("errored while getting pending mset: %w", err)
			}
			if ok && !interned {
				st = &actorState{nextNonce: mset.nextNonce, requiredFunds: mset.requiredFunds}
				for _, m := range mset.msgs {
					st.requiredFunds = new(stdbig.Int).Add(st.requiredFunds, m.Message.Value.Int)
				}
				state[m.From] = st
				mp.lk.RUnlock()

				check.OK = true
				check.Hint = map[string]interface{}{
					"nonce": st.nextNonce,
				}
			} else {
				mp.lk.RUnlock()

				stateNonce, err := mp.getStateNonce(ctx, m.From, curTs)
				if err != nil {
					check.OK = false
					check.Err = fmt.Sprintf("error retrieving state nonce: %s", err.Error())
				} else {
					check.OK = true
					check.Hint = map[string]interface{}{
						"nonce": stateNonce,
					}
				}

				st = &actorState{nextNonce: stateNonce, requiredFunds: new(stdbig.Int)}
				state[m.From] = st
			}
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)
		if !check.OK {
			continue
		}

		// pre-check: actor balance
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageGetStateBalance,
			},
		}

		balance, ok := balances[m.From]
		if !ok {
			balance, err = mp.getStateBalance(ctx, m.From, curTs)
			if err != nil {
				check.OK = false
				check.Err = fmt.Sprintf("error retrieving state balance: %s", err)
			} else {
				check.OK = true
				check.Hint = map[string]interface{}{
					"balance": balance,
				}
			}

			balances[m.From] = balance
		} else {
			check.OK = true
			check.Hint = map[string]interface{}{
				"balance": balance,
			}
		}

		result[i] = append(result[i], check)
		if !check.OK {
			continue
		}

		// 1. Serialization
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageSerialize,
			},
		}

		bytes, err := m.Serialize()
		if err != nil {
			check.OK = false
			check.Err = err.Error()
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)

		// 2. Message size
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageSize,
			},
		}

		if len(bytes) > MaxMessageSize-128 { // 128 bytes to account for signature size
			check.OK = false
			check.Err = "message too big"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)

		// 3. Syntactic validation
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageValidity,
			},
		}
		nv, err := mp.getNtwkVersion(epoch)
		if err != nil {
			check.OK = false
			check.Err = fmt.Sprintf("error retrieving network version: %s", err.Error())
		} else {
			check.OK = true
		}
		if err := m.ValidForBlockInclusion(0, nv); err != nil {
			check.OK = false
			check.Err = fmt.Sprintf("syntactically invalid message: %s", err.Error())
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)
		if !check.OK {
			// skip remaining checks if it is a syntactically invalid message
			continue
		}

		// gas checks

		// 4. Min Gas
		minGas := vm.PricelistByEpoch(epoch).OnChainMessage(m.ChainLength())

		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageMinGas,
				Hint: map[string]interface{}{
					"minGas": minGas,
				},
			},
		}

		if m.GasLimit < minGas.Total() {
			check.OK = false
			check.Err = "GasLimit less than epoch minimum gas"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)

		// 5. Min Base Fee
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageMinBaseFee,
			},
		}

		if m.GasFeeCap.LessThan(minimumBaseFee) {
			check.OK = false
			check.Err = "GasFeeCap less than minimum base fee"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)
		if !check.OK {
			goto checkState
		}

		// 6. Base Fee
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageBaseFee,
				Hint: map[string]interface{}{
					"baseFee": baseFee,
				},
			},
		}

		if m.GasFeeCap.LessThan(baseFee) {
			check.OK = false
			check.Err = "GasFeeCap less than current base fee"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)

		// 7. Base Fee lower bound
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageBaseFeeLowerBound,
				Hint: map[string]interface{}{
					"baseFeeLowerBound": baseFeeLowerBound,
					"baseFee":           baseFee,
				},
			},
		}

		if m.GasFeeCap.LessThan(baseFeeLowerBound) {
			check.OK = false
			check.Err = "GasFeeCap less than base fee lower bound for inclusion in next 20 epochs"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)

		// 8. Base Fee upper bound
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageBaseFeeUpperBound,
				Hint: map[string]interface{}{
					"baseFeeUpperBound": baseFeeUpperBound,
					"baseFee":           baseFee,
				},
			},
		}

		if m.GasFeeCap.LessThan(baseFeeUpperBound) {
			check.OK = true // on purpose, the checks is more of a warning
			check.Err = "GasFeeCap less than base fee upper bound for inclusion in next 20 epochs"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)

		// stateful checks
	checkState:
		// 9. Message Nonce
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageNonce,
				Hint: map[string]interface{}{
					"nextNonce": st.nextNonce,
				},
			},
		}

		if (flexibleNonces == nil || !flexibleNonces[i]) && st.nextNonce != m.Nonce {
			check.OK = false
			check.Err = fmt.Sprintf("message nonce doesn't match next nonce (%d)", st.nextNonce)
		} else {
			check.OK = true
			st.nextNonce++
		}

		result[i] = append(result[i], check)

		// check required funds -vs- balance
		st.requiredFunds = new(stdbig.Int).Add(st.requiredFunds, m.RequiredFunds().Int)
		st.requiredFunds.Add(st.requiredFunds, m.Value.Int)

		// 10. Balance
		check = api.MessageCheckStatus{
			Cid: m.Cid(),
			CheckStatus: api.CheckStatus{
				Code: api.CheckStatusMessageBalance,
				Hint: map[string]interface{}{
					"requiredFunds": big.Int{Int: stdbig.NewInt(0).Set(st.requiredFunds)},
				},
			},
		}

		if balance.Int.Cmp(st.requiredFunds) < 0 {
			check.OK = false
			check.Err = "insufficient balance"
		} else {
			check.OK = true
		}

		result[i] = append(result[i], check)
	}

	return result, nil
}
