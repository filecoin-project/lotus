package stages

import (
	"bytes"
	"context"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cmd/lotus-sim/simulation/blockbuilder"
)

var (
	TargetFunds  = abi.TokenAmount(types.MustParseFIL("1000FIL"))
	MinimumFunds = abi.TokenAmount(types.MustParseFIL("100FIL"))
)

type FundingStage struct {
	fundAccount        address.Address
	taxMin             abi.TokenAmount
	minFunds, maxFunds abi.TokenAmount
}

func NewFundingStage() (*FundingStage, error) {
	// TODO: make all this configurable.
	addr, err := address.NewIDAddress(100)
	if err != nil {
		return nil, err
	}
	return &FundingStage{
		fundAccount: addr,
		taxMin:      abi.TokenAmount(types.MustParseFIL("1000FIL")),
		minFunds:    abi.TokenAmount(types.MustParseFIL("1000000FIL")),
		maxFunds:    abi.TokenAmount(types.MustParseFIL("100000000FIL")),
	}, nil
}

func (*FundingStage) Name() string {
	return "funding"
}

func (fs *FundingStage) Fund(bb *blockbuilder.BlockBuilder, target address.Address) error {
	return fs.fund(bb, target, 0)
}

// SendAndFund "packs" the given message, funding the actor if necessary. It:
//
//  1. Tries to send the given message.
//  2. If that fails, it checks to see if the exit code was ErrInsufficientFunds.
//  3. If so, it sends 1K FIL from the "burnt funds actor" (because we need to send it from
//     somewhere) and re-tries the message.0
func (fs *FundingStage) SendAndFund(bb *blockbuilder.BlockBuilder, msg *types.Message) (res *types.MessageReceipt, err error) {
	for i := 0; i < 10; i++ {
		res, err = bb.PushMessage(msg)
		if err == nil {
			return res, nil
		}
		aerr, ok := err.(aerrors.ActorError)
		if !ok || aerr.RetCode() != exitcode.ErrInsufficientFunds {
			return nil, err
		}

		// Ok, insufficient funds. Let's fund this miner and try again.
		if err := fs.fund(bb, msg.To, i); err != nil {
			if !blockbuilder.IsOutOfGas(err) {
				err = xerrors.Errorf("failed to fund %s: %w", msg.To, err)
			}
			return nil, err
		}
	}
	return res, err
}

// fund funds the target actor with 'TargetFunds << shift' FIL. The "shift" parameter allows us to
// keep doubling the amount until the intended operation succeeds.
func (fs *FundingStage) fund(bb *blockbuilder.BlockBuilder, target address.Address, shift int) error {
	amt := TargetFunds
	if shift > 0 {
		if shift >= 8 {
			shift = 8 // cap
		}
		amt = big.Lsh(amt, uint(shift))
	}
	_, err := bb.PushMessage(&types.Message{
		From:   fs.fundAccount,
		To:     target,
		Value:  amt,
		Method: builtin.MethodSend,
	})
	return err
}

func (fs *FundingStage) PackMessages(ctx context.Context, bb *blockbuilder.BlockBuilder) (_err error) {
	st := bb.StateTree()
	fundAccActor, err := st.GetActor(fs.fundAccount)
	if err != nil {
		return err
	}
	if fs.minFunds.LessThan(fundAccActor.Balance) {
		return nil
	}

	// Ok, we're going to go fund this thing.
	start := time.Now()

	type actor struct {
		types.Actor
		Address address.Address
	}

	var targets []*actor
	err = st.ForEach(func(addr address.Address, act *types.Actor) error {
		// Don't steal from ourselves!
		if addr == fs.fundAccount {
			return nil
		}
		if act.Balance.LessThan(fs.taxMin) {
			return nil
		}
		if !(builtin.IsAccountActor(act.Code) || builtin.IsMultisigActor(act.Code)) {
			return nil
		}
		targets = append(targets, &actor{*act, addr})
		return nil
	})
	if err != nil {
		return err
	}

	balance := fundAccActor.Balance.Copy()

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Balance.GreaterThan(targets[j].Balance)
	})

	store := bb.ActorStore()
	epoch := bb.Height()
	actorsVersion, err := bb.ActorsVersion()
	if err != nil {
		return err
	}

	var accounts, multisigs int
	defer func() {
		if _err != nil {
			return
		}
		bb.L().Infow("finished funding the simulation",
			"duration", time.Since(start),
			"targets", len(targets),
			"epoch", epoch,
			"new-balance", types.FIL(balance),
			"old-balance", types.FIL(fundAccActor.Balance),
			"multisigs", multisigs,
			"accounts", accounts,
		)
	}()

	for _, actorTmp := range targets {
		actor := actorTmp
		switch {
		case builtin.IsAccountActor(actor.Code):
			if _, err := bb.PushMessage(&types.Message{
				From:  actor.Address,
				To:    fs.fundAccount,
				Value: actor.Balance,
			}); blockbuilder.IsOutOfGas(err) {
				return nil
			} else if err != nil {
				return err
			}
			accounts++
		case builtin.IsMultisigActor(actor.Code):
			msigState, err := multisig.Load(store, &actor.Actor)
			if err != nil {
				return err
			}

			threshold, err := msigState.Threshold()
			if err != nil {
				return err
			}

			if threshold > 16 {
				bb.L().Debugw("ignoring multisig with high threshold",
					"multisig", actor.Address,
					"threshold", threshold,
					"max", 16,
				)
				continue
			}

			locked, err := msigState.LockedBalance(epoch)
			if err != nil {
				return err
			}

			if locked.LessThan(fs.taxMin) {
				continue // not worth it.
			}

			allSigners, err := msigState.Signers()
			if err != nil {
				return err
			}
			signers := make([]address.Address, 0, threshold)
			for _, signer := range allSigners {
				actor, err := st.GetActor(signer)
				if err != nil {
					return err
				}
				if !builtin.IsAccountActor(actor.Code) {
					// I am so not dealing with this mess.
					continue
				}
				if uint64(len(signers)) >= threshold {
					break
				}
			}
			// Ok, we're not dealing with this one.
			if uint64(len(signers)) < threshold {
				continue
			}

			available := big.Sub(actor.Balance, locked)

			var txnId uint64
			{
				msg, err := multisig.Message(actorsVersion, signers[0]).Propose(
					actor.Address, fs.fundAccount, available,
					builtin.MethodSend, nil,
				)
				if err != nil {
					return err
				}
				res, err := bb.PushMessage(msg)
				if err != nil {
					if blockbuilder.IsOutOfGas(err) {
						err = nil
					}
					return err
				}
				var ret multisig.ProposeReturn
				err = ret.UnmarshalCBOR(bytes.NewReader(res.Return))
				if err != nil {
					return err
				}
				if ret.Applied {
					if !ret.Code.IsSuccess() {
						bb.L().Errorw("failed to tax multisig",
							"multisig", actor.Address,
							"exitcode", ret.Code,
						)
					}
					break
				}
				txnId = uint64(ret.TxnID)
			}
			var ret multisig.ProposeReturn
			for _, signer := range signers[1:] {
				msg, err := multisig.Message(actorsVersion, signer).Approve(actor.Address, txnId, nil)
				if err != nil {
					return err
				}
				res, err := bb.PushMessage(msg)
				if err != nil {
					if blockbuilder.IsOutOfGas(err) {
						err = nil
					}
					return err
				}
				var ret multisig.ProposeReturn
				err = ret.UnmarshalCBOR(bytes.NewReader(res.Return))
				if err != nil {
					return err
				}
				// A bit redundant, but nice.
				if ret.Applied {
					break
				}

			}
			if !ret.Applied {
				bb.L().Errorw("failed to apply multisig transaction",
					"multisig", actor.Address,
					"txnid", txnId,
					"signers", len(signers),
					"threshold", threshold,
				)
				continue
			}
			if !ret.Code.IsSuccess() {
				bb.L().Errorw("failed to tax multisig",
					"multisig", actor.Address,
					"txnid", txnId,
					"exitcode", ret.Code,
				)
			} else {
				multisigs++
			}
		default:
			panic("impossible case")
		}
		balance = big.Int{Int: balance.Add(balance.Int, actor.Balance.Int)}
		if balance.GreaterThanEqual(fs.maxFunds) {
			// There's no need to get greedy.
			// Well, really, we're trying to avoid messing with state _too_ much.
			return nil
		}
	}
	return nil
}
