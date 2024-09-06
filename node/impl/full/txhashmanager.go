package full

import (
	"context"
	"errors"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type EthTxHashManager struct {
	StateAPI              StateAPI
	ChainAPI              ChainAPI
	TransactionHashLookup *ethhashlookup.EthTxHashLookup
}

func (m *EthTxHashManager) Revert(_ context.Context, _, _ *types.TipSet) error {
	return nil
}

// FillIndexGap populates the Ethereum transaction hash lookup database with missing entries
// by processing blocks until we reach the maximum number of automatic back-fill epochs or the message is already indexed.
func (m *EthTxHashManager) FillIndexGap(ctx context.Context, currHead *types.TipSet, maxAutomaticBackFillBlocks abi.ChainEpoch) error {
	log.Info("Start back-filling transaction index from current head: %d", currHead.Height())

	var (
		fromTs          = currHead
		processedBlocks = uint64(0)
	)

	for i := abi.ChainEpoch(0); i < maxAutomaticBackFillBlocks && fromTs.Height() > 0; i++ {
		for _, block := range fromTs.Blocks() {
			msgs, err := m.StateAPI.Chain.SecpkMessagesForBlock(ctx, block)
			if err != nil {
				log.Debugf("Exiting back-filling at epoch %d: %s", currHead.Height(), err)
				return nil
			}

			for _, msg := range msgs {
				err = m.StoreMsg(ctx, msg)
				if err != nil {
					if errors.Is(err, ethhashlookup.ErrAlreadyIndexed) {
						log.Infof("Reached already indexed transaction at height %d. ", fromTs.Height())

						log.Info("Stop back-filling tx index, Total processed blocks: %d", processedBlocks)
						return nil
					}

					return err
				}
			}

			processedBlocks++
		}

		// Move to the previous tipset
		var err error
		fromTs, err = m.ChainAPI.ChainGetTipSet(ctx, fromTs.Parents())
		if err != nil {
			return err
		}
	}

	log.Info("Finished back-filling tx index, Total processed blocks: %d", processedBlocks)

	return nil
}

// PopulateExistingMappings walks back from the current head to the minimum height and populates the eth transaction hash lookup database
func (m *EthTxHashManager) PopulateExistingMappings(ctx context.Context, minHeight abi.ChainEpoch) error {
	if minHeight < buildconstants.UpgradeHyggeHeight {
		minHeight = buildconstants.UpgradeHyggeHeight
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

func (m *EthTxHashManager) Apply(ctx context.Context, _, to *types.TipSet) error {
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

// StoreMsg is similar to ProcessSignedMessage, but it returns an error if the message is already indexed
// and does not attempt to index the message again.
func (m *EthTxHashManager) StoreMsg(_ context.Context, msg *types.SignedMessage) error {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return nil
	}

	txHash, err := m.getEthTxHash(msg)
	if err != nil {
		return err
	}

	return m.TransactionHashLookup.UpsertUniqueHash(txHash, msg.Cid())
}

func (m *EthTxHashManager) getEthTxHash(msg *types.SignedMessage) (ethtypes.EthHash, error) {
	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(msg)
	if err != nil {
		return ethtypes.EthHash{}, err
	}

	return ethTx.TxHash()
}

func (m *EthTxHashManager) ProcessSignedMessage(_ context.Context, msg *types.SignedMessage) {
	txHash, err := m.getEthTxHash(msg)
	if err != nil {
		log.Errorf("error converting filecoin message to eth tx: %s", err)
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

func EthTxHashGC(_ context.Context, retentionDays int, manager *EthTxHashManager) {
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
