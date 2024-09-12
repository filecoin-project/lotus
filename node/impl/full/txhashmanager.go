package full

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

type EthTxHashManager interface {
	events.TipSetObserver

	PopulateExistingMappings(ctx context.Context, minHeight abi.ChainEpoch) error
	ProcessSignedMessage(ctx context.Context, msg *types.SignedMessage)
	UpsertHash(txHash ethtypes.EthHash, c cid.Cid) error
	GetCidFromHash(txHash ethtypes.EthHash) (cid.Cid, error)
	DeleteEntriesOlderThan(days int) (int64, error)
}

var (
	_ EthTxHashManager = (*ethTxHashManager)(nil)
	_ EthTxHashManager = (*EthTxHashManagerDummy)(nil)
)

type ethTxHashManager struct {
	stateAPI              StateAPI
	transactionHashLookup *ethhashlookup.EthTxHashLookup
}

func NewEthTxHashManager(stateAPI StateAPI, transactionHashLookup *ethhashlookup.EthTxHashLookup) EthTxHashManager {
	return &ethTxHashManager{
		stateAPI:              stateAPI,
		transactionHashLookup: transactionHashLookup,
	}
}

func (m *ethTxHashManager) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (m *ethTxHashManager) PopulateExistingMappings(ctx context.Context, minHeight abi.ChainEpoch) error {
	if minHeight < buildconstants.UpgradeHyggeHeight {
		minHeight = buildconstants.UpgradeHyggeHeight
	}

	ts := m.stateAPI.Chain.GetHeaviestTipSet()
	for ts.Height() > minHeight {
		for _, block := range ts.Blocks() {
			msgs, err := m.stateAPI.Chain.SecpkMessagesForBlock(ctx, block)
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
		ts, err = m.stateAPI.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *ethTxHashManager) Apply(ctx context.Context, from, to *types.TipSet) error {
	for _, blk := range to.Blocks() {
		_, smsgs, err := m.stateAPI.Chain.MessagesForBlock(ctx, blk)
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

			err = m.transactionHashLookup.UpsertHash(hash, smsg.Cid())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *ethTxHashManager) UpsertHash(txHash ethtypes.EthHash, c cid.Cid) error {
	return m.transactionHashLookup.UpsertHash(txHash, c)
}

func (m *ethTxHashManager) GetCidFromHash(txHash ethtypes.EthHash) (cid.Cid, error) {
	return m.transactionHashLookup.GetCidFromHash(txHash)
}

func (m *ethTxHashManager) DeleteEntriesOlderThan(days int) (int64, error) {
	return m.transactionHashLookup.DeleteEntriesOlderThan(days)
}

func (m *ethTxHashManager) ProcessSignedMessage(ctx context.Context, msg *types.SignedMessage) {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return
	}

	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(msg)
	if err != nil {
		log.Errorf("error converting filecoin message to eth tx: %s", err)
		return
	}

	txHash, err := ethTx.TxHash()
	if err != nil {
		log.Errorf("error hashing transaction: %s", err)
		return
	}

	err = m.UpsertHash(txHash, msg.Cid())
	if err != nil {
		log.Errorf("error inserting tx mapping to db: %s", err)
		return
	}
}

func WaitForMpoolUpdates(ctx context.Context, ch <-chan api.MpoolUpdate, manager EthTxHashManager) {
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

func EthTxHashGC(ctx context.Context, retentionDays int, manager EthTxHashManager) {
	if retentionDays == 0 {
		return
	}

	gcPeriod := 1 * time.Hour
	for {
		entriesDeleted, err := manager.DeleteEntriesOlderThan(retentionDays)
		if err != nil {
			log.Errorf("error garbage collecting eth transaction hash database: %s", err)
		}
		log.Info("garbage collection run on eth transaction hash lookup database. %d entries deleted", entriesDeleted)
		time.Sleep(gcPeriod)
	}
}

type EthTxHashManagerDummy struct{}

func (d *EthTxHashManagerDummy) PopulateExistingMappings(ctx context.Context, minHeight abi.ChainEpoch) error {
	return nil
}

func (d *EthTxHashManagerDummy) Revert(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (d *EthTxHashManagerDummy) Apply(ctx context.Context, from, to *types.TipSet) error {
	return nil
}

func (d *EthTxHashManagerDummy) ProcessSignedMessage(ctx context.Context, msg *types.SignedMessage) {}

func (d *EthTxHashManagerDummy) UpsertHash(txHash ethtypes.EthHash, c cid.Cid) error {
	return nil
}

func (d *EthTxHashManagerDummy) GetCidFromHash(txHash ethtypes.EthHash) (cid.Cid, error) {
	return cid.Undef, nil
}

func (d *EthTxHashManagerDummy) DeleteEntriesOlderThan(days int) (int64, error) {
	return 0, nil
}
