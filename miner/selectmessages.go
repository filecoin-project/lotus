package miner

import (
	"bytes"
	"context"
	big2 "math/big"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

func SelectMessages(ctx context.Context, al ActorLookup, ts *types.TipSet, msgs []*types.SignedMessage) ([]*types.SignedMessage, error) {
	al = (&cachedActorLookup{
		tsk:      ts.Key(),
		cache:    map[address.Address]actCacheEntry{},
		fallback: al,
	}).StateGetActor

	type senderMeta struct {
		lastReward   abi.TokenAmount
		lastGasLimit int64

		gasReward []abi.TokenAmount
		gasLimit  []int64

		msgs []*types.SignedMessage
	}

	inclNonces := make(map[address.Address]uint64)
	inclBalances := make(map[address.Address]big.Int)
	outBySender := make(map[address.Address]*senderMeta)

	tooLowFundMsgs := 0
	tooHighNonceMsgs := 0

	start := build.Clock.Now()
	vmValid := time.Duration(0)
	getbal := time.Duration(0)
	guessGasDur := time.Duration(0)

	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Message.Nonce < msgs[j].Message.Nonce
	})

	for _, msg := range msgs {
		vmstart := build.Clock.Now()

		minGas := vm.PricelistByEpoch(ts.Height()).OnChainMessage(msg.ChainLength()) // TODO: really should be doing just msg.ChainLength() but the sync side of this code doesnt seem to have access to that
		if err := msg.VMMessage().ValidForBlockInclusion(minGas.Total()); err != nil {
			log.Warnf("invalid message in message pool: %s", err)
			continue
		}

		vmValid += build.Clock.Since(vmstart)

		// TODO: this should be in some more general 'validate message' call
		if msg.Message.GasLimit > build.BlockGasLimit {
			log.Warnf("message in mempool had too high of a gas limit (%d)", msg.Message.GasLimit)
			continue
		}

		if msg.Message.To == address.Undef {
			log.Warnf("message in mempool had bad 'To' address")
			continue
		}

		from := msg.Message.From

		getBalStart := build.Clock.Now()
		if _, ok := inclNonces[from]; !ok {
			act, err := al(ctx, from, ts.Key())
			if err != nil {
				log.Warnf("failed to check message sender balance, skipping message: %+v", err)
				continue
			}

			inclNonces[from] = act.Nonce
			inclBalances[from] = act.Balance
		}
		getbal += build.Clock.Since(getBalStart)

		if inclBalances[from].LessThan(msg.Message.RequiredFunds()) {
			tooLowFundMsgs++
			// todo: drop from mpool
			continue
		}

		if msg.Message.Nonce > inclNonces[from] {
			tooHighNonceMsgs++
			continue
		}

		if msg.Message.Nonce < inclNonces[from] {
			log.Warnf("message in mempool has already used nonce (%d < %d), from %s, to %s, %s (%d pending for)", msg.Message.Nonce, inclNonces[from], msg.Message.From, msg.Message.To, msg.Cid(), countFrom(msgs, from))
			continue
		}

		inclNonces[from] = msg.Message.Nonce + 1
		inclBalances[from] = types.BigSub(inclBalances[from], msg.Message.RequiredFunds())
		sm := outBySender[from]
		if sm == nil {
			sm = &senderMeta{
				lastReward: big.Zero(),
			}
		}

		sm.gasLimit = append(sm.gasLimit, sm.lastGasLimit+msg.Message.GasLimit)
		sm.lastGasLimit = sm.gasLimit[len(sm.gasLimit)-1]

		guessGasStart := build.Clock.Now()
		guessedGas, err := GuessGasUsed(ctx, ts.Key(), msg, al)
		guessGasDur += build.Clock.Since(guessGasStart)
		if err != nil {
			log.Infow("failed to guess gas", "to", msg.Message.To, "method", msg.Message.Method, "err", err)
		}

		estimatedReward := big.Mul(types.NewInt(uint64(guessedGas)), msg.Message.GasPrice)

		sm.gasReward = append(sm.gasReward, big.Add(sm.lastReward, estimatedReward))
		sm.lastReward = sm.gasReward[len(sm.gasReward)-1]

		sm.msgs = append(sm.msgs, msg)

		outBySender[from] = sm
	}

	gasLimitLeft := int64(build.BlockGasLimit)

	orderedSenders := make([]address.Address, 0, len(outBySender))
	for k := range outBySender {
		orderedSenders = append(orderedSenders, k)
	}
	sort.Slice(orderedSenders, func(i, j int) bool {
		return bytes.Compare(orderedSenders[i].Bytes(), orderedSenders[j].Bytes()) == -1
	})

	out := make([]*types.SignedMessage, 0, build.BlockMessageLimit)
	{
		for {
			var bestSender address.Address
			var nBest int
			var bestGasToReward float64

			// TODO: This is O(n^2)-ish, could use something like container/heap to cache this math
			for _, sender := range orderedSenders {
				meta, ok := outBySender[sender]
				if !ok {
					continue
				}
				for n := range meta.msgs {
					if meta.gasLimit[n] > gasLimitLeft {
						break
					}

					if n+len(out) > build.BlockMessageLimit {
						break
					}

					gasToReward, _ := new(big2.Float).SetInt(meta.gasReward[n].Int).Float64()
					gasToReward /= float64(meta.gasLimit[n])

					if gasToReward >= bestGasToReward {
						bestSender = sender
						nBest = n + 1
						bestGasToReward = gasToReward
					}
				}
			}

			if nBest == 0 {
				break // block gas limit reached
			}

			{
				out = append(out, outBySender[bestSender].msgs[:nBest]...)
				gasLimitLeft -= outBySender[bestSender].gasLimit[nBest-1]

				outBySender[bestSender].msgs = outBySender[bestSender].msgs[nBest:]
				outBySender[bestSender].gasLimit = outBySender[bestSender].gasLimit[nBest:]
				outBySender[bestSender].gasReward = outBySender[bestSender].gasReward[nBest:]

				if len(outBySender[bestSender].msgs) == 0 {
					delete(outBySender, bestSender)
				}
			}

			if len(out) >= build.BlockMessageLimit {
				break
			}
		}
	}

	if tooLowFundMsgs > 0 {
		log.Warnf("%d messages in mempool does not have enough funds", tooLowFundMsgs)
	}

	if tooHighNonceMsgs > 0 {
		log.Warnf("%d messages in mempool had too high nonce", tooHighNonceMsgs)
	}

	sm := build.Clock.Now()
	if sm.Sub(start) > time.Second {
		log.Warnw("SelectMessages took a long time",
			"duration", sm.Sub(start),
			"vmvalidate", vmValid,
			"getbalance", getbal,
			"guessgas", guessGasDur,
			"msgs", len(msgs))
	}

	return out, nil
}
