package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestCirciulationSupplyUpgrade(t *testing.T) {
	kit.QuietMiningLogs()
	ctx := context.Background()

	// Choosing something divisible by epochs per day to remove error with simple deal duration
	lockedClientBalance := big.Mul(abi.NewTokenAmount(8_640_000), abi.NewTokenAmount(1e18))
	lockedProviderBalance := big.Mul(abi.NewTokenAmount(1_000_000), abi.NewTokenAmount(1e18))
	var height0 abi.ChainEpoch
	var height1 abi.ChainEpoch
	// Lock collateral in market on nv22 network
	fullNode0, blockMiner0, ens0 := kit.EnsembleMinimal(t,
		kit.GenesisNetworkVersion(network.Version22),
		kit.MockProofs(),
	)
	{

		worker0 := blockMiner0.OwnerKey.Address
		ens0.InterconnectAll().BeginMining(50 * time.Millisecond)

		// Lock collateral in market actor
		wallet0, err := fullNode0.WalletDefaultAddress(ctx)
		require.NoError(t, err)

		// Add 1 FIL to cover provider collateral
		c00, err := fullNode0.MarketAddBalance(ctx, wallet0, wallet0, lockedClientBalance)
		require.NoError(t, err)
		fullNode0.WaitMsg(ctx, c00)
		c01, err := blockMiner0.FullNode.MarketAddBalance(ctx, worker0, blockMiner0.ActorAddr, lockedProviderBalance)
		require.NoError(t, err)
		fullNode0.WaitMsg(ctx, c01)

		psd0, err := fullNode0.MpoolPushMessage(ctx,
			makePSDMessage(
				ctx,
				t,
				blockMiner0.ActorAddr,
				worker0,
				wallet0,
				lockedProviderBalance,
				lockedClientBalance,
				fullNode0.WalletSign,
			),
			nil,
		)
		require.NoError(t, err)
		fullNode0.WaitMsg(ctx, psd0.Cid())
		head, err := fullNode0.ChainHead(ctx)
		require.NoError(t, err)
		height0 = head.Height()
	}

	// Lock collateral in market on nv23 network
	fullNode1, blockMiner1, ens1 := kit.EnsembleMinimal(t,
		kit.GenesisNetworkVersion(network.Version23),
		kit.MockProofs(),
	)
	{
		worker1 := blockMiner1.OwnerKey.Address
		ens1.InterconnectAll().BeginMining(50 * time.Millisecond)

		// Lock collateral in market actor
		wallet1, err := fullNode1.WalletDefaultAddress(ctx)
		require.NoError(t, err)
		c10, err := fullNode1.MarketAddBalance(ctx, wallet1, wallet1, lockedClientBalance)
		require.NoError(t, err)
		fullNode1.WaitMsg(ctx, c10)
		c11, err := blockMiner1.FullNode.MarketAddBalance(ctx, worker1, blockMiner1.ActorAddr, lockedProviderBalance)
		require.NoError(t, err)
		fullNode1.WaitMsg(ctx, c11)

		psd1, err := fullNode1.MpoolPushMessage(ctx,
			makePSDMessage(
				ctx,
				t,
				blockMiner1.ActorAddr,
				worker1,
				wallet1,
				lockedProviderBalance,
				lockedClientBalance,
				fullNode1.WalletSign,
			),
			nil,
		)
		require.NoError(t, err)
		fullNode1.WaitMsg(ctx, psd1.Cid())
		head, err := fullNode1.ChainHead(ctx)
		require.NoError(t, err)
		height1 = head.Height()
	}

	// Measure each circulating supply at the latest height where market balance was locked
	// This allows us to normalize against fluctuations in circulating supply based on the underlying
	// dynamics irrelevant to this change

	max := height0
	if height0 < height1 {
		max = height1
	}
	max = max + 1 // Measure supply at height after the deal locking funds was published

	// Let both chains catch up
	fullNode0.WaitTillChain(ctx, func(ts *types.TipSet) bool {
		return ts.Height() >= max
	})
	fullNode1.WaitTillChain(ctx, func(ts *types.TipSet) bool {
		return ts.Height() >= max
	})

	ts0, err := fullNode0.ChainGetTipSetByHeight(ctx, max, types.EmptyTSK)
	require.NoError(t, err)
	ts1, err := fullNode1.ChainGetTipSetByHeight(ctx, max, types.EmptyTSK)
	require.NoError(t, err)

	nv22Supply, err := fullNode0.StateVMCirculatingSupplyInternal(ctx, ts0.Key())
	require.NoError(t, err, "Failed to fetch nv22 circulating supply")
	nv23Supply, err := fullNode1.StateVMCirculatingSupplyInternal(ctx, ts1.Key())
	require.NoError(t, err, "Failed to fetch nv23 circulating supply")

	// Unfortunately there's still some non-determinism in supply dynamics so check for equality within a tolerance
	tolerance := big.Mul(abi.NewTokenAmount(1000), abi.NewTokenAmount(1e18))
	totalLocked := big.Sum(lockedClientBalance, lockedProviderBalance)
	diff := big.Sub(
		big.Sum(totalLocked, nv23Supply.FilLocked),
		nv22Supply.FilLocked,
	)
	assert.Equal(t, -1, big.Cmp(
		diff.Abs(),
		tolerance), "Difference from expected locked supply between versions exceeds tolerance")
}

// Message will be valid and lock funds but the data is fake so the deal will never be activated
func makePSDMessage(
	ctx context.Context,
	t *testing.T,
	provider,
	worker,
	client address.Address,
	providerLocked,
	clientLocked abi.TokenAmount,
	signFunc func(context.Context, address.Address, []byte) (*crypto.Signature, error)) *types.Message {

	dummyCid, err := cid.Parse("baga6ea4seaqflae5c3k2odz4sqfufmrmoegplhk5jbq3co4fgmmy56yc2qfh4aq")
	require.NoError(t, err)

	duration := 2880 * 200 // 200 days
	ppe := big.Div(clientLocked, big.NewInt(2880*200))
	proposal := market.DealProposal{
		PieceCID:             dummyCid,
		PieceSize:            abi.PaddedPieceSize(128),
		VerifiedDeal:         false,
		Client:               client,
		Provider:             provider,
		ClientCollateral:     big.Zero(),
		ProviderCollateral:   providerLocked,
		StartEpoch:           10000,
		EndEpoch:             10000 + abi.ChainEpoch(duration),
		StoragePricePerEpoch: ppe,
	}
	buf := bytes.NewBuffer(nil)
	require.NoError(t, proposal.MarshalCBOR(buf))
	sig, err := signFunc(ctx, client, buf.Bytes())
	require.NoError(t, err)
	// Publish storage deal
	params, err := actors.SerializeParams(&market.PublishStorageDealsParams{
		Deals: []market.ClientDealProposal{
			{
				Proposal:        proposal,
				ClientSignature: *sig,
			},
		},
	})
	require.NoError(t, err)
	return &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   worker,
		Value:  types.NewInt(0),
		Method: builtin.MethodsMarket.PublishStorageDeals,
		Params: params,
	}
}
