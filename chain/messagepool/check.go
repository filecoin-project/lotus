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
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

var baseFeeUpperBoundFactor = types.NewInt(10)

type CheckStatus = api.MessageCheckStatus

// CheckMessages performs a set of logic checks for a list of messages, prior to submitting it to the mpool
func (mp *MessagePool) CheckMessages(msgs []*types.Message) (result []CheckStatus, err error) {
	return mp.checkMessages(msgs, false)
}

// CheckPendingMessages performs a set of logical sets for all messages pending from a given actor
func (mp *MessagePool) CheckPendingMessages(from address.Address) (result []CheckStatus, err error) {
	var msgs []*types.Message
	mp.lk.Lock()
	mset, ok := mp.pending[from]
	if ok {
		for _, sm := range mset.msgs {
			msgs = append(msgs, &sm.Message)
		}
	}
	mp.lk.Unlock()

	if len(msgs) == 0 {
		return nil, nil
	}

	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Nonce < msgs[j].Nonce
	})

	return mp.checkMessages(msgs, true)
}

func (mp *MessagePool) checkMessages(msgs []*types.Message, interned bool) (result []CheckStatus, err error) {
	mp.curTsLk.Lock()
	curTs := mp.curTs
	mp.curTsLk.Unlock()

	epoch := curTs.Height()

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

	for _, m := range msgs {
		// basic syntactic checks
		bytes, err := m.Serialize()
		if err != nil {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrSerialize,
				ErrorMsg:  err.Error(),
			})
		}

		if len(bytes) > 32*1024-128 { // 128 bytes to account for signature size
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrTooBig,
				ErrorMsg:  "message too big",
			})
		}

		if err := m.ValidForBlockInclusion(0, build.NewestNetworkVersion); err != nil {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrInvalid,
				ErrorMsg:  fmt.Sprintf("syntactically invalid message: %s", err.Error()),
			})
			// skip the remaining checks if it's syntactically invalid
			continue
		}

		// gas checks
		minGas := vm.PricelistByEpoch(epoch).OnChainMessage(m.ChainLength())
		if m.GasLimit < minGas.Total() {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrMinGas,
				ErrorMsg:  "GasLimit less than epoch minimum gas",
			})
		}

		if m.GasFeeCap.LessThan(minimumBaseFee) {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrMinBaseFee,
				ErrorMsg:  "GasFeeCap less than minimum base fee",
			})
			goto checkState
		}

		if m.GasFeeCap.LessThan(baseFee) {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrBaseFee,
				ErrorMsg:  "GasFeeCap less than current base fee",
			})
		}

		if m.GasFeeCap.LessThan(baseFeeLowerBound) {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrBaseFeeLowerBound,
				ErrorMsg:  "GasFeeCap less than base fee lower bound for inclusion in next 20 epochs",
			})
			goto checkState
		}

		if m.GasFeeCap.LessThan(baseFeeUpperBound) {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrBaseFeeUpperBound,
				ErrorMsg:  "GasFeeCap less than base fee upper bound for inclusion in next 20 epochs",
			})
		}

		// stateful checks
	checkState:
		st, ok := state[m.From]
		if !ok {
			mp.lk.Lock()
			mset, ok := mp.pending[m.From]
			if ok && !interned {
				st = &actorState{nextNonce: mset.nextNonce, requiredFunds: mset.requiredFunds}
				for _, m := range mset.msgs {
					st.requiredFunds = new(stdbig.Int).Add(st.requiredFunds, m.Message.Value.Int)
				}
				state[m.From] = st
				mp.lk.Unlock()
			} else {
				mp.lk.Unlock()

				stateNonce, err := mp.getStateNonce(m.From, curTs)
				if err != nil {
					result = append(result, CheckStatus{
						Cid:       m.Cid(),
						ErrorCode: api.CheckStatusErrGetStateNonce,
						ErrorMsg:  fmt.Sprintf("error retrieving state nonce: %s", err.Error()),
					})

					continue
				}

				st = &actorState{nextNonce: stateNonce, requiredFunds: new(stdbig.Int)}
				state[m.From] = st
			}
		}

		if st.nextNonce != m.Nonce {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrBadNonce,
				ErrorMsg:  fmt.Sprintf("message nonce doesn't match next nonce (%d)", st.nextNonce),
			})
		} else {
			st.nextNonce++
		}

		st.requiredFunds = new(stdbig.Int).Add(st.requiredFunds, m.RequiredFunds().Int)
		st.requiredFunds.Add(st.requiredFunds, m.Value.Int)

		balance, ok := balances[m.From]
		if !ok {
			balance, err = mp.getStateBalance(m.From, curTs)
			if err != nil {
				result = append(result, CheckStatus{
					Cid:       m.Cid(),
					ErrorCode: api.CheckStatusErrGetStateBalance,
					ErrorMsg:  fmt.Sprintf("error retrieving state balance: %s", err),
				})
				continue
			}
			balances[m.From] = balance
		}

		if balance.Int.Cmp(st.requiredFunds) < 0 {
			result = append(result, CheckStatus{
				Cid:       m.Cid(),
				ErrorCode: api.CheckStatusErrInsufficientBalance,
				ErrorMsg:  "insufficient balance",
			})
		}
	}

	return result, nil
}
