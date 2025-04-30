package eth

import (
	"context"

	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var (
	_ EthSendAPI = (*ethSend)(nil)
	_ EthSendAPI = (*EthSendDisabled)(nil)
)

type ethSend struct {
	mpoolApi     MpoolAPI
	chainIndexer index.Indexer
}

func NewEthSendAPI(mpoolApi MpoolAPI, chainIndexer index.Indexer) EthSendAPI {
	return &ethSend{
		mpoolApi:     mpoolApi,
		chainIndexer: chainIndexer,
	}
}

func (e *ethSend) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return e.ethSendRawTransaction(ctx, rawTx, false)
}

func (e *ethSend) EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return e.ethSendRawTransaction(ctx, rawTx, true)
}

func (e *ethSend) ethSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes, untrusted bool) (ethtypes.EthHash, error) {
	txArgs, err := ethtypes.ParseEthTransaction(rawTx)
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	txHash, err := txArgs.TxHash()
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	smsg, err := ethtypes.ToSignedFilecoinMessage(txArgs)
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	if untrusted {
		if _, err = e.mpoolApi.MpoolPushUntrusted(ctx, smsg); err != nil {
			return ethtypes.EmptyEthHash, err
		}
	} else {
		if _, err = e.mpoolApi.MpoolPush(ctx, smsg); err != nil {
			return ethtypes.EmptyEthHash, err
		}
	}

	// make it immediately available in the transaction hash lookup db, even though it will also
	// eventually get there via the mpool
	if e.chainIndexer != nil {
		if err := e.chainIndexer.IndexEthTxHash(ctx, txHash, smsg.Cid()); err != nil {
			log.Errorf("error indexing tx: %s", err)
		}
	}

	return ethtypes.EthHashFromTxBytes(rawTx), nil
}

type EthSendDisabled struct{}

func (EthSendDisabled) EthSendRawTransaction(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return ethtypes.EmptyEthHash, ErrModuleDisabled
}
func (EthSendDisabled) EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error) {
	return ethtypes.EmptyEthHash, ErrModuleDisabled
}
