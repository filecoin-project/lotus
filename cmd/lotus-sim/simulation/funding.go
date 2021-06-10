package simulation

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

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/types"
)

var (
	fundAccount = func() address.Address {
		addr, err := address.NewIDAddress(100)
		if err != nil {
			panic(err)
		}
		return addr
	}()
	minFundAcctFunds = abi.TokenAmount(types.MustParseFIL("1000000FIL"))
	maxFundAcctFunds = abi.TokenAmount(types.MustParseFIL("100000000FIL"))
	taxMin           = abi.TokenAmount(types.MustParseFIL("1000FIL"))
)

func fund(send packFunc, target address.Address) error {
	_, err := send(&types.Message{
		From:   fundAccount,
		To:     target,
		Value:  targetFunds,
		Method: builtin.MethodSend,
	})
	return err
}

// sendAndFund "packs" the given message, funding the actor if necessary. It:
//
// 1. Tries to send the given message.
// 2. If that fails, it checks to see if the exit code was ErrInsufficientFunds.
// 3. If so, it sends 1K FIL from the "burnt funds actor" (because we need to send it from
//    somewhere) and re-tries the message.0
//
// NOTE: If the message fails a second time, the funds won't be "unsent".
func sendAndFund(send packFunc, msg *types.Message) (*types.MessageReceipt, error) {
	res, err := send(msg)
	aerr, ok := err.(aerrors.ActorError)
	if !ok || aerr.RetCode() != exitcode.ErrInsufficientFunds {
		return res, err
	}
	// Ok, insufficient funds. Let's fund this miner and try again.
	err = fund(send, msg.To)
	if err != nil {
		if err != ErrOutOfGas {
			err = xerrors.Errorf("failed to fund %s: %w", msg.To, err)
		}
		return nil, err
	}
	return send(msg)
}

func (ss *simulationState) packFunding(ctx context.Context, cb packFunc) (_err error) {
	st, err := ss.stateTree(ctx)
	if err != nil {
		return err
	}
	fundAccActor, err := st.GetActor(fundAccount)
	if err != nil {
		return err
	}
	if minFundAcctFunds.LessThan(fundAccActor.Balance) {
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
		if addr == fundAccount {
			return nil
		}
		if act.Balance.LessThan(taxMin) {
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

	store := ss.Chainstore.ActorStore(ctx)

	epoch := ss.nextEpoch()

	nv := ss.StateManager.GetNtwkVersion(ctx, epoch)
	actorsVersion := actors.VersionForNetwork(nv)

	var accounts, multisigs int
	defer func() {
		if _err != nil {
			return
		}
		log.Infow("finished funding the simulation",
			"duration", time.Since(start),
			"targets", len(targets),
			"epoch", epoch,
			"new-balance", types.FIL(balance),
			"old-balance", types.FIL(fundAccActor.Balance),
			"multisigs", multisigs,
			"accounts", accounts,
		)
	}()

	for _, actor := range targets {
		switch {
		case builtin.IsAccountActor(actor.Code):
			if _, err := cb(&types.Message{
				From:  actor.Address,
				To:    fundAccount,
				Value: actor.Balance,
			}); err == ErrOutOfGas {
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
				log.Debugw("ignoring multisig with high threshold",
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

			if locked.LessThan(taxMin) {
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
					actor.Address, fundAccount, available,
					builtin.MethodSend, nil,
				)
				if err != nil {
					return err
				}
				res, err := cb(msg)
				if err != nil {
					if err == ErrOutOfGas {
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
						log.Errorw("failed to tax multisig",
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
				res, err := cb(msg)
				if err != nil {
					if err == ErrOutOfGas {
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
				log.Errorw("failed to apply multisig transaction",
					"multisig", actor.Address,
					"txnid", txnId,
					"signers", len(signers),
					"threshold", threshold,
				)
				continue
			}
			if !ret.Code.IsSuccess() {
				log.Errorw("failed to tax multisig",
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
		if balance.GreaterThanEqual(maxFundAcctFunds) {
			// There's no need to get greedy.
			// Well, really, we're trying to avoid messing with state _too_ much.
			return nil
		}
	}
	return nil
}
