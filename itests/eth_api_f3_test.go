package itests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/result"
)

type ethApi interface {
	FilecoinAddressToEthAddress(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthAddress, error)
	EthGetCode(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetStorageAt(ctx context.Context, address ethtypes.EthAddress, position ethtypes.EthBytes, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetBalance(ctx context.Context, address ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBigInt, error)
	EthTraceBlock(ctx context.Context, blkNum string) ([]*ethtypes.EthTraceBlock, error)
	EthTraceReplayBlockTransactions(ctx context.Context, blkNum string, traceTypes []string) ([]*ethtypes.EthTraceReplayBlockTransaction, error)
	EthFeeHistory(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthFeeHistory, error)
	EthEstimateGas(ctx context.Context, p jsonrpc.RawParams) (ethtypes.EthUint64, error)
	EthCall(ctx context.Context, tx ethtypes.EthCall, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthBytes, error)
	EthGetBlockTransactionCountByNumber(ctx context.Context, blkNum string) (ethtypes.EthUint64, error)
	EthGetBlockByNumber(ctx context.Context, blkNum string, fullTxInfo bool) (ethtypes.EthBlock, error)
	EthGetTransactionByBlockNumberAndIndex(ctx context.Context, blkNum string, txIndex ethtypes.EthUint64) (*ethtypes.EthTx, error)
	EthGetTransactionCount(ctx context.Context, sender ethtypes.EthAddress, blkParam ethtypes.EthBlockNumberOrHash) (ethtypes.EthUint64, error)
	EthGetBlockReceipts(ctx context.Context, blkParam ethtypes.EthBlockNumberOrHash) ([]*ethtypes.EthTxReceipt, error)
}

func TestEthAPIWithF3(t *testing.T) {
	const (
		timeout             = 5 * time.Minute
		blockTime           = 50 * time.Millisecond
		targetHeight        = 20 + policy.ChainFinality
		preF3FinalizedEpoch = targetHeight - 400
		f3FinalizedEpoch    = targetHeight - 300
	)

	req := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	kit.QuietMiningLogs()

	mockF3 := kit.NewMockF3Backend()
	client, miner, network := kit.EnsembleMinimal(t, kit.F3Backend(mockF3), kit.MockProofs())
	network.BeginMining(blockTime)

	_, fundedEthAddr, fundedFilAddr := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, fundedFilAddr, types.FromFil(10))

	client.WaitTillChain(ctx, kit.HeightAtLeast(preF3FinalizedEpoch))

	var (
		filecoinSecpAddr    address.Address
		filecoinSecpEthAddr ethtypes.EthAddress
		contractEthAddr     ethtypes.EthAddress
		contractBytes       []byte
	)
	{
		// create an f1 before F3 finalized epoch
		var err error
		filecoinSecpAddr, err = client.WalletNew(ctx, types.KTSecp256k1)
		req.NoError(err)
		kit.SendFunds(ctx, t, client, filecoinSecpAddr, abi.NewTokenAmount(1))
		idAddr, err := client.StateLookupID(ctx, filecoinSecpAddr, types.EmptyTSK)
		req.NoError(err)
		filecoinSecpEthAddr, err = ethtypes.EthAddressFromFilecoinAddress(idAddr)
		req.NoError(err)

		// Deploy a contract that will give us both a contractAddress and a location to query for nonzero storage
		contractHex, err := os.ReadFile("./contracts/DelegatecallActor.hex")
		req.NoError(err)
		contractBytes, err = hex.DecodeString(string(contractHex))
		req.NoError(err)
		createReturn := client.EVM().DeployContract(ctx, client.DefaultKey.Address, contractBytes)
		actorAddr, err := address.NewIDAddress(createReturn.ActorID)
		req.NoError(err)
		contractHex, err = os.ReadFile("./contracts/DelegatecallStorage.hex")
		req.NoError(err)
		contract, err := hex.DecodeString(string(contractHex))
		req.NoError(err)
		createReturn = client.EVM().DeployContract(ctx, client.DefaultKey.Address, contract)
		storageAddr, err := address.NewIDAddress(createReturn.ActorID)
		req.NoError(err)
		contractEthAddr = createReturn.EthAddress
		// call Contract Storage which makes a delegatecall to contract Actor
		// this contract call sets the "counter" variable to 7, from default value 0
		fromId, err := client.StateLookupID(ctx, actorAddr, types.EmptyTSK)
		require.NoError(t, err)
		senderEthAddr, err := ethtypes.EthAddressFromFilecoinAddress(fromId)
		require.NoError(t, err)
		inputData := make([]byte, 64) // 2 arguments * 32 bytes each
		copy(inputData[32-len(senderEthAddr):32], senderEthAddr[:])
		inputData[63] = 7
		result, _, err := client.EVM().InvokeContractByFuncName(ctx, client.DefaultKey.Address, storageAddr, "setVars(address,uint256)", inputData)
		req.NoError(err)
		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
		req.NoError(err)
		req.Equal(result, expectedResult)
	}

	client.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))

	var (
		parentOf = func(tsfn func(t *testing.T) *types.TipSet) func(t *testing.T) *types.TipSet {
			return func(t *testing.T) *types.TipSet {
				parent, err := client.ChainGetTipSet(ctx, tsfn(t).Parents())
				req.NoError(err)
				return parent
			}
		}
		heaviest = func(t *testing.T) *types.TipSet {
			head, err := client.ChainHead(ctx)
			req.NoError(err)
			// -1 in here for Ethereum APIs which use inclusion tipset
			return parentOf(func(t *testing.T) *types.TipSet { return head })(t)
		}
		ecFixedLookback = func(lookback abi.ChainEpoch) func(t *testing.T) *types.TipSet {
			return func(t *testing.T) *types.TipSet {
				head, err := client.ChainHead(ctx)
				req.NoError(err)
				ecFixedLookback, err := client.ChainGetTipSetByHeight(ctx, head.Height()-lookback, head.Key())
				req.NoError(err)
				return ecFixedLookback
			}
		}
		ecFinalized    = ecFixedLookback(policy.ChainFinality)
		ecSafeV1       = ecFixedLookback(200)
		ecSafeV2       = ecFixedLookback(200)
		tipSetAtHeight = func(height abi.ChainEpoch) func(t *testing.T) *types.TipSet {
			return func(t *testing.T) *types.TipSet {
				ts, err := client.ChainGetTipSetByHeight(ctx, height, types.EmptyTSK)
				req.NoError(err)
				return ts
			}
		}
		f3Finalized = func(t *testing.T) *types.TipSet {
			return tipSetAtHeight(f3FinalizedEpoch)(t)
		}
		internalF3Error = errors.New("lost hearing in left eye")
		plausibleCertAt = func(t *testing.T, epoch abi.ChainEpoch) *certs.FinalityCertificate {
			f3FinalisedTipSet := tipSetAtHeight(epoch)(t)
			return &certs.FinalityCertificate{
				ECChain: &gpbft.ECChain{
					TipSets: []*gpbft.TipSet{{
						Epoch: int64(f3FinalisedTipSet.Height()),
						Key:   f3FinalisedTipSet.Key().Bytes(),
					}},
				},
			}
		}
		implausibleCert = &certs.FinalityCertificate{
			ECChain: &gpbft.ECChain{
				TipSets: []*gpbft.TipSet{{
					Epoch: int64(1413),
					Key:   []byte(`🐠`),
				}},
			},
		}
	)

	testStates := []struct {
		name         string
		blkParam     string
		setup        func(t *testing.T)
		wantTipSetV2 func(t *testing.T) *types.TipSet
		wantTipSetV1 func(t *testing.T) *types.TipSet
		wantErrV1    string
		wantErrV2    string
	}{
		{
			name:         "latest tag",
			blkParam:     "latest",
			wantTipSetV1: heaviest,
			wantTipSetV2: heaviest,
		},
		{
			name:     "finalized tag when f3 disabled",
			blkParam: "finalized",
			setup: func(t *testing.T) {
				mockF3.Running = false
			},
			wantTipSetV1: ecFinalized,
			wantTipSetV2: ecFinalized,
		},
		{
			name:     "safe tag when f3 disabled",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = false
			},
			wantTipSetV1: ecSafeV1,
			wantTipSetV2: ecSafeV2,
		},
		{
			name:     "finalized tag is ok",
			blkParam: "finalized",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCertErr = nil
				mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
			},
			wantTipSetV1: f3Finalized,
			wantTipSetV2: f3Finalized,
		},
		{
			name:     "safe tag is finalized when more recent than ec safe",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCertErr = nil
				mockF3.LatestCert = plausibleCertAt(t, targetHeight)
			},
			wantTipSetV1: tipSetAtHeight(targetHeight),
			wantTipSetV2: tipSetAtHeight(targetHeight),
		},
		{
			name:     "safe tag is ec safe distance when more recent than f3 finalized, but f3 is not activated",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = false
				mockF3.LatestCertErr = nil
				mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
			},
			wantTipSetV1: ecSafeV1,
			wantTipSetV2: ecSafeV2,
		},
		{
			name:     "safe tag is ec safe distance when more recent than f3 finalized",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCertErr = nil
				mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
				mockF3.Finalizing = true
			},
			wantTipSetV1: ecSafeV1,
			wantTipSetV2: ecSafeV2,
		},
		{
			name:     "finalized tag when f3 not ready falls back to ec",
			blkParam: "finalized",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = api.ErrF3NotReady
			},
			wantTipSetV1: ecFinalized,
			wantTipSetV2: ecFinalized,
		},
		{
			name:     "safe tag when f3 not ready falls back to ec safe, but f3 is not activated",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = false
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = api.ErrF3NotReady
			},
			wantTipSetV1: ecSafeV1,
			wantTipSetV2: ecSafeV2,
		},
		{
			name:     "safe tag when f3 not ready falls back to ec safe",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = api.ErrF3NotReady
			},
			wantTipSetV1: ecSafeV1,
			wantTipSetV2: ecSafeV2,
		},
		{
			name:     "finalized tag when f3 fails",
			blkParam: "finalized",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = internalF3Error
			},
			wantErrV1: internalF3Error.Error(),
			wantErrV2: internalF3Error.Error(),
		},
		{
			name:     "safe tag when f3 fails, but f3 is not activated",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = false
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = internalF3Error
			},
			wantErrV1: internalF3Error.Error(),
			wantErrV2: internalF3Error.Error(),
		},
		{
			name:     "safe tag when f3 fails",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = internalF3Error
			},
			wantErrV1: internalF3Error.Error(),
			wantErrV2: internalF3Error.Error(),
		},
		{
			name:     "finalize tag when f3 is too far behind falls back to ec",
			blkParam: "finalized",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = plausibleCertAt(t, targetHeight-policy.ChainFinality-5)
				mockF3.LatestCertErr = nil
			},
			wantTipSetV1: ecFinalized,
			wantTipSetV2: ecFinalized,
		},
		{
			name:     "latest tag when f3 fails is ok",
			blkParam: "latest",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = internalF3Error
			},
			wantTipSetV1: heaviest,
			wantTipSetV2: heaviest,
		},
		{
			name:     "finalized tag when f3 is broken",
			blkParam: "finalized",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = implausibleCert
				mockF3.LatestCertErr = nil
			},
			wantErrV1: "decoding latest f3 cert tipset key",
			wantErrV2: "decoding latest f3 cert tipset key",
		},
		{
			name:     "safe tag when f3 is broken, but f3 is not activated",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = false
				mockF3.LatestCert = implausibleCert
				mockF3.LatestCertErr = nil
			},
			wantErrV1: "decoding latest f3 cert tipset key",
			wantErrV2: "decoding latest f3 cert tipset key",
		},
		{
			name:     "safe tag when f3 is broken",
			blkParam: "safe",
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = implausibleCert
				mockF3.LatestCertErr = nil
			},
			wantErrV1: "decoding latest f3 cert tipset key",
			wantErrV2: "decoding latest f3 cert tipset key",
		},
		{
			name:     "height before ec finalized epoch is ok",
			blkParam: fmt.Sprintf("0x%x", int(tipSetAtHeight(10)(t).Height())),
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
				mockF3.LatestCertErr = nil
			},
			wantTipSetV1: tipSetAtHeight(10),
			wantTipSetV2: tipSetAtHeight(10),
		},
		{
			name:     "height after f3 finalized epoch is ok",
			blkParam: fmt.Sprintf("0x%x", int(tipSetAtHeight(f3FinalizedEpoch+2)(t).Height())),
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = plausibleCertAt(t, f3FinalizedEpoch)
				mockF3.LatestCertErr = nil
			},
			wantTipSetV1: tipSetAtHeight(f3FinalizedEpoch + 2),
			wantTipSetV2: tipSetAtHeight(f3FinalizedEpoch + 2),
		},
		{
			name:     "height before ec finalized epoch when f3 not ready is ok",
			blkParam: fmt.Sprintf("0x%x", int(tipSetAtHeight(10)(t).Height())),
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = api.ErrF3NotReady
			},
			wantTipSetV1: tipSetAtHeight(10),
			wantTipSetV2: tipSetAtHeight(10),
		},
		{
			name:     "height after f3 finalized epoch when f3 not ready is ok",
			blkParam: fmt.Sprintf("0x%x", int(f3FinalizedEpoch)+2),
			setup: func(t *testing.T) {
				mockF3.Running = true
				mockF3.Finalizing = true
				mockF3.LatestCert = nil
				mockF3.LatestCertErr = api.ErrF3NotReady
			},
			wantTipSetV1: tipSetAtHeight(f3FinalizedEpoch + 2),
			wantTipSetV2: tipSetAtHeight(f3FinalizedEpoch + 2),
		},
	}
	testCases := []struct {
		name    string
		execute func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string)
	}{
		{
			name: "FilecoinAddressToEthAddress",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var apiEthAddr ethtypes.EthAddress
				var err error
				expect := stableExecute(func() {
					apiEthAddr, err = subject.FilecoinAddressToEthAddress(ctx, result.Wrap[jsonrpc.RawParams](
						json.Marshal([]interface{}{filecoinSecpAddr, blkParam}),
					).Assert(req.NoError))
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else if expect.Height() < preF3FinalizedEpoch {
					req.ErrorContains(err, "failed to lookup ID address for given Filecoin address")
				} else {
					req.NoError(err)
					req.Equal(filecoinSecpEthAddr, apiEthAddr)
				}
			},
		},

		{
			name: "EthGetCode",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))

				var code ethtypes.EthBytes
				var err error
				expect := stableExecute(func() {
					code, err = subject.EthGetCode(ctx, contractEthAddr, param)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Empty(code)
				} else if expect.Height() < preF3FinalizedEpoch {
					// no error, but no code
					req.NoError(err)
					req.Empty(code)
				} else {
					req.NoError(err)
					req.NotEmpty(code)
				}
			},
		},

		{
			name: "EthGetStorageAt",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))

				var value ethtypes.EthBytes
				var err error
				expect := stableExecute(func() {
					value, err = subject.EthGetStorageAt(ctx, contractEthAddr, nil, param)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Empty(value)
				} else if expect.Height() < preF3FinalizedEpoch {
					// no error, but a zero value
					req.NoError(err)
					req.Equal(ethtypes.EthBytes(make([]byte, 32)), value)
				} else {
					req.NoError(err)
					expected, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
					require.NoError(t, err)
					req.Equal(ethtypes.EthBytes(expected), value)
				}
			},
		},

		{
			name: "EthGetBalance",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))

				var balance ethtypes.EthBigInt
				var err error
				expect := stableExecute(func() {
					balance, err = subject.EthGetBalance(ctx, filecoinSecpEthAddr, param)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Equal(balance.String(), "0x0")
				} else if expect.Height() < preF3FinalizedEpoch {
					// no error, but a zero balance
					req.NoError(err)
					req.Equal(balance.String(), "0x0")
				} else {
					req.NoError(err)
					req.Equal(int64(1), balance.Int64())
				}
			},
		},

		{
			name: "EthTraceBlock",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var trace []*ethtypes.EthTraceBlock
				var err error
				expect := stableExecute(func() {
					trace, err = subject.EthTraceBlock(ctx, blkParam)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Nil(trace)
				} else {
					req.NoError(err)
					msgs, err := client.ChainGetMessagesInTipset(ctx, expect.Parents())
					req.NoError(err)
					req.Len(trace, len(msgs))
					for i, blk := range trace {
						req.Equal(expect.Height(), blk.BlockNumber, "block %d", i)
						expectTxHash, err := ethtypes.EthHashFromCid(msgs[i].Cid)
						req.NoError(err)
						req.Equal(expectTxHash, blk.TransactionHash, "block %d", i)
					}
				}
			},
		},

		{
			name: "EthTraceReplayBlockTransactions",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var trace []*ethtypes.EthTraceReplayBlockTransaction
				var err error
				expect := stableExecute(func() {
					trace, err = subject.EthTraceReplayBlockTransactions(ctx, blkParam, []string{"trace"})
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Nil(trace)
				} else {
					req.NoError(err)
					msgs, err := client.ChainGetMessagesInTipset(ctx, expect.Parents())
					req.NoError(err)
					req.Len(trace, len(msgs))
					for i, blk := range trace {
						expectTxHash, err := ethtypes.EthHashFromCid(msgs[i].Cid)
						req.NoError(err)
						req.Equal(expectTxHash, blk.TransactionHash, "block %d", i)
					}
				}
			},
		},

		{
			name: "EthFeeHistory",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				blkCount := 5

				var feeHistory ethtypes.EthFeeHistory
				var err error
				expect := stableExecute(func() {
					feeHistory, err = subject.EthFeeHistory(ctx, result.Wrap[jsonrpc.RawParams](
						json.Marshal([]interface{}{blkCount, blkParam}),
					).Assert(req.NoError))
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Equal(ethtypes.EthFeeHistory{}, feeHistory)
				} else {
					req.NoError(err)
					oldest := expect
					for range blkCount - 1 {
						// iterate through Parents() because we'll likely have null rounds in here so we can't
						// just us the height
						k := oldest.Parents()
						oldest, err = client.ChainGetTipSet(ctx, k)
						req.NoError(err)
					}
					req.Equal(ethtypes.EthUint64(oldest.Height()), feeHistory.OldestBlock)
				}
			},
		},

		{
			// This is not a great test because the gas limit values we produce are not likely to differ
			// between different tipsets so we're mostly testing that we can call with the various
			// blkParams, not so much that their values are what we expect for that blkParam.
			name: "EthEstimateGas",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))
				call := ethtypes.EthCall{From: &fundedEthAddr, Data: contractBytes}
				gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{
					BlkParam: &param,
					Tx:       call,
				})
				require.NoError(t, err)

				var egaslimit ethtypes.EthUint64
				expect := stableExecute(func() {
					egaslimit, err = subject.EthEstimateGas(ctx, gasParams)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
					req.Equal(ethtypes.EthUint64(0), egaslimit)
				} else {
					msg, err := call.ToFilecoinMessage()
					require.NoError(t, err)
					gaslimit, err := client.GasEstimateGasLimit(ctx, msg, expect.Key())
					require.NoError(t, err)
					gasLimitOverestimation := 1.25 // default messagepool config value
					req.Equal(int64(float64(gaslimit)*gasLimitOverestimation), int64(egaslimit))
				}
			},
		},

		{
			name: "EthCall",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))
				call := ethtypes.EthCall{
					From: &fundedEthAddr,
					To:   &contractEthAddr,
					Data: kit.CalcFuncSignature("getCounter()"),
				}

				var ret ethtypes.EthBytes
				var err error
				expect := stableExecute(func() {
					ret, err = subject.EthCall(ctx, call, param)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else if expect.Height() < preF3FinalizedEpoch {
					req.NoError(err)
				} else {
					req.NoError(err)
					expected, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
					require.NoError(t, err)
					req.Equal(ethtypes.EthBytes(expected), ret)
				}
			},
		},

		{
			name: "EthGetBlockTransactionCountByNumber",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var ret ethtypes.EthUint64
				var err error
				expect := stableExecute(func() {
					ret, err = subject.EthGetBlockTransactionCountByNumber(ctx, blkParam)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else {
					req.NoError(err)
					msgs, err := client.ChainGetMessagesInTipset(ctx, expect.Parents())
					req.NoError(err)
					req.Equal(int(ret), len(msgs))
				}
			},
		},

		{
			name: "EthGetBlockByNumber",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var ret ethtypes.EthBlock
				var err error
				expect := stableExecute(func() {
					ret, err = subject.EthGetBlockByNumber(ctx, blkParam, false)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else {
					req.NoError(err)
					req.Equal(int(ret.Number), int(expect.Height()))
				}
			},
		},

		{
			name: "EthGetTransactionByBlockNumberAndIndex",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var ret *ethtypes.EthTx
				var err error
				expect := stableExecute(func() {
					ret, err = subject.EthGetTransactionByBlockNumberAndIndex(ctx, blkParam, ethtypes.EthUint64(0))
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else {
					msgs, err2 := client.ChainGetMessagesInTipset(ctx, expect.Parents())
					req.NoError(err2)
					if len(msgs) == 0 {
						req.ErrorContains(err, "tipset contains 0 messages")
					} else {
						req.NoError(err)
						req.NotNil(ret)
						req.Equal(int(*ret.BlockNumber), int(expect.Height()))
					}
				}
			},
		},

		{
			name: "EthGetTransactionCount",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				minerId, err := address.IDFromAddress(miner.ActorAddr)
				req.NoError(err)
				ethAddr := ethtypes.EthAddressFromActorID(abi.ActorID(minerId))
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))

				var ret ethtypes.EthUint64
				expect := stableExecute(func() {
					ret, err = subject.EthGetTransactionCount(ctx, ethAddr, param)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else {
					req.NoError(err)
					msgs, err := client.ChainGetMessagesInTipset(ctx, expect.Parents())
					req.NoError(err)
					var cnt int
					if len(msgs) > 0 {
						for _, msg := range msgs {
							if msg.Message.From == miner.ActorAddr {
								cnt++
							}
						}
					}
					req.Equal(int(ret), cnt)
				}
			},
		},

		{
			name: "EthGetBlockReceipts",
			execute: func(req *require.Assertions, subject ethApi, blkParam string, stableExecute func(func()) *types.TipSet, expectErr string) {
				var param ethtypes.EthBlockNumberOrHash
				req.NoError(param.UnmarshalJSON([]byte(`"` + blkParam + `"`)))

				var ret []*ethtypes.EthTxReceipt
				var err error
				expect := stableExecute(func() {
					ret, err = subject.EthGetBlockReceipts(ctx, param)
				})

				if expectErr != "" {
					req.ErrorContains(err, expectErr)
				} else {
					req.NoError(err)
					msgs, err := client.ChainGetMessagesInTipset(ctx, expect.Parents())
					req.NoError(err)
					req.Len(ret, len(msgs))
					if len(msgs) > 0 {
						req.Equal(int(ret[0].BlockNumber), int(expect.Height()))
					}
				}
			},
		},
	}

	for _, state := range testStates {
		t.Run(state.name, func(t *testing.T) {
			if state.setup != nil {
				state.setup(t)
			}
			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					t.Run("v1", func(t *testing.T) {
						stableExecute := kit.MakeStableExecute(ctx, t, state.wantTipSetV1)
						subject := client
						test.execute(require.New(t), subject, state.blkParam, stableExecute, state.wantErrV1)
					})

					t.Run("v2", func(t *testing.T) {
						stableExecute := kit.MakeStableExecute(ctx, t, state.wantTipSetV2)
						subject := client.V2
						test.execute(require.New(t), subject, state.blkParam, stableExecute, state.wantErrV2)
					})
				})
			}
		})
	}
}
