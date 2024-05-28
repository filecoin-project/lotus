package full

import (
	"context"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type EthTxHashManager struct {
	StateAPI              StateAPI
	TransactionHashLookup *ethhashlookup.EthTxHashLookup
}

func (m *EthTxHashManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (m *EthTxHashManager) PopulateExistingMappings(ctx context.Context, minHeight abi.ChainEpoch) error {
	if minHeight < build.UpgradeHyggeHeight {
		minHeight = build.UpgradeHyggeHeight
	}

	ts := m.StateAPI.Chain.GetHeaviestTipSet()
	for ts.Height() > minHeight {
		for _, block := range ts.Blocks() {
			msgs, err := m.StateAPI.Chain.SecpkMessagesForBlock(ctx, block)
			if err != nil {
				// If we can't find the messages, we've either imported from snapshot or pruned the store
				log.Debug("exiting message mapping population at epoch ", ts.Height())
				return nil
			}

			for _, msg := range msgs {
				m.ProcessSignedMessage(ctx, msg)
			}
		}

		var err error
		ts, err = m.StateAPI.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *EthTxHashManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	for _, blk := range to.Blocks() {
		_, smsgs, err := m.StateAPI.Chain.MessagesForBlock(ctx, blk)
		if err != nil {
			return err
		}

		for _, smsg := range smsgs {
			if smsg.Signature.Type != crypto.SigTypeDelegated {
				continue
			}

			hash, err := ethTxHashFromSignedMessage(smsg)
			if err != nil {
				return err
			}

			err = m.TransactionHashLookup.UpsertHash(hash, smsg.Cid())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *EthTxHashManager) ProcessSignedMessage(ctx context.Context, msg *types.SignedMessage) {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return
	}

	ethTx, err := ethtypes.EthTxFromSignedEthMessage(msg)
	if err != nil {
		log.Errorf("error converting filecoin message to eth tx: %s", err)
		return
	}
	txHash, err := ethTx.TxHash()
	if err != nil {
		log.Errorf("error hashing transaction: %s", err)
		return
	}

	err = m.TransactionHashLookup.UpsertHash(txHash, msg.Cid())
	if err != nil {
		log.Errorf("error inserting tx mapping to db: %s", err)
		return
	}
}

func WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate, manager *EthTxHashManager) {
	for {
		select {
		case <-ctx.Done():
			return
		case u := <-ch:
			if u.Type != api.MpoolAdd {
				continue
			}

			manager.ProcessSignedMessage(ctx, u.Message)
		}
	}
}

func EthTxHashGC(ctx context.Context, retentionDays int, manager *EthTxHashManager) {
	if retentionDays == 0 {
		return
	}

	gcPeriod := 1 * time.Hour
	for {
		entriesDeleted, err := manager.TransactionHashLookup.DeleteEntriesOlderThan(retentionDays)
		if err != nil {
			log.Errorf("error garbage collecting eth transaction hash database: %s", err)
		}
		log.Info("garbage collection run on eth transaction hash lookup database. %d entries deleted", entriesDeleted)
		time.Sleep(gcPeriod)
	}
}
