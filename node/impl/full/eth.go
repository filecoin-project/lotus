package full

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/impl/eth"
)

// gatewayWithTrusted exists just to do type assertions on api.Gateway, but we know it won't have
// certain trusted-only APIs
type gatewayWithTrusted interface {
	api.Gateway
	EthGetTransactionByHashLimited(ctx context.Context, txHash *ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTx, error)
	EthGetTransactionReceiptLimited(ctx context.Context, txHash ethtypes.EthHash, limit abi.ChainEpoch) (*ethtypes.EthTxReceipt, error)
	EthGetBlockReceiptsLimited(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash, limit abi.ChainEpoch) ([]*ethtypes.EthTxReceipt, error)
	EthSendRawTransactionUntrusted(ctx context.Context, rawTx ethtypes.EthBytes) (ethtypes.EthHash, error)
}

var (
	_ eth.EthModuleAPI = *new(FullEthAPIV1)
	_ eth.EthModuleAPI = *new(api.FullNode)
	_ eth.EthModuleAPI = *new(gatewayWithTrusted)
)

// The Eth*V{1,2} interfaces are distinct interface for DI purposes. By making them separate, we can
// construct separate modules for v1 and v2 APIs and provide different TipSetResolvers for them.

type EthTipSetResolverV1 interface{ eth.TipSetResolver }
type EthFilecoinAPIV1 interface{ eth.EthFilecoinAPI }
type EthTransactionAPIV1 interface{ eth.EthTransactionAPI }
type EthLookupAPIV1 interface{ eth.EthLookupAPI }
type EthTraceAPIV1 interface{ eth.EthTraceAPI }
type EthGasAPIV1 interface{ eth.EthGasAPI }

type EthTipSetResolverV2 interface{ eth.TipSetResolver }
type EthFilecoinAPIV2 interface{ eth.EthFilecoinAPI }
type EthTransactionAPIV2 interface{ eth.EthTransactionAPI }
type EthLookupAPIV2 interface{ eth.EthLookupAPI }
type EthTraceAPIV2 interface{ eth.EthTraceAPI }
type EthGasAPIV2 interface{ eth.EthGasAPI }

type FullEthAPIV1 struct {
	fx.In

	EthFilecoinAPIV1
	eth.EthBasicAPI
	eth.EthSendAPI
	EthTransactionAPIV1
	EthLookupAPIV1
	EthTraceAPIV1
	EthGasAPIV1
	eth.EthEventsAPI
}

type FullEthAPIV2 struct {
	fx.In

	EthFilecoinAPIV2
	eth.EthBasicAPI
	eth.EthSendAPI
	EthTransactionAPIV2
	EthLookupAPIV2
	EthTraceAPIV2
	EthGasAPIV2
	eth.EthEventsAPI
}
