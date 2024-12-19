package itests

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	migration "github.com/filecoin-project/go-state-types/builtin/v9/migration/test"
	verifregtypes "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestGetAllocationForPendingDeal(t *testing.T) {
	rootKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifierKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	verifiedClientKey, err := key.GenerateKey(types.KTBLS)
	require.NoError(t, err)

	bal, err := types.ParseFIL("100fil")
	require.NoError(t, err)

	node, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
		kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
		kit.Account(verifierKey, abi.NewTokenAmount(bal.Int64())), // assign some balance to the verifier so they can send an AddClient message.
		kit.Account(verifiedClientKey, abi.NewTokenAmount(bal.Int64())),
	)

	ens.InterconnectAll().BeginMining(250 * time.Millisecond)

	api := node.FullNode.(*impl.FullNodeAPI)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get VRH
	vrh, err := api.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
	fmt.Println(vrh.String())
	require.NoError(t, err)

	// import the root key.
	rootAddr, err := api.WalletImport(ctx, &rootKey.KeyInfo)
	require.NoError(t, err)

	// import the verifier's key.
	verifierAddr, err := api.WalletImport(ctx, &verifierKey.KeyInfo)
	require.NoError(t, err)

	// import the verified client's key.
	verifiedClientAddr, err := api.WalletImport(ctx, &verifiedClientKey.KeyInfo)
	require.NoError(t, err)

	params, err := actors.SerializeParams(&verifregtypes.AddVerifierParams{Address: verifierAddr, Allowance: big.NewInt(100000000000)})
	require.NoError(t, err)

	msg := &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := api.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err, "AddVerifier failed")

	res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// assign datacap to a client
	datacap := big.NewInt(10000)

	params, err = actors.SerializeParams(&verifregtypes.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
	require.NoError(t, err)

	msg = &types.Message{
		From:   verifierAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifiedClient,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err = api.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// check datacap balance
	dcap, err := api.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, *dcap, datacap)

	pieceCid := migration.MakeCID("1", &markettypes.PieceCIDPrefix)

	label, err := markettypes.NewLabelFromString("")
	require.NoError(t, err)

	dealProposal := markettypes.DealProposal{
		PieceCID:             pieceCid,
		PieceSize:            1024,
		Client:               verifiedClientAddr,
		Provider:             miner.ActorAddr,
		Label:                label,
		StartEpoch:           abi.ChainEpoch(1000_000),
		EndEpoch:             abi.ChainEpoch(2000_000),
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   big.Zero(),
		ClientCollateral:     big.Zero(),
		VerifiedDeal:         true,
	}

	serializedProposal := new(bytes.Buffer)
	err = dealProposal.MarshalCBOR(serializedProposal)
	require.NoError(t, err)

	sig, err := api.WalletSign(ctx, verifiedClientAddr, serializedProposal.Bytes())
	require.NoError(t, err)

	publishDealParams := markettypes.PublishStorageDealsParams{
		Deals: []markettypes.ClientDealProposal{{
			Proposal: dealProposal,
			ClientSignature: crypto.Signature{
				Type: crypto.SigTypeBLS,
				Data: sig.Data,
			},
		}},
	}

	serializedParams := new(bytes.Buffer)
	require.NoError(t, publishDealParams.MarshalCBOR(serializedParams))

	m, err := node.MpoolPushMessage(ctx, &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   miner.OwnerKey.Address,
		Value:  types.FromFil(0),
		Method: builtin.MethodsMarket.PublishStorageDeals,
		Params: serializedParams.Bytes(),
	}, nil)
	require.NoError(t, err)

	r, err := node.StateWaitMsg(ctx, m.Cid(), 2, lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

	ret, err := market.DecodePublishStorageDealsReturn(r.Receipt.Return, buildconstants.TestNetworkVersion)
	require.NoError(t, err)
	valid, _, err := ret.IsDealValid(0)
	require.NoError(t, err)
	require.True(t, valid)
	dealIds, err := ret.DealIDs()
	require.NoError(t, err)

	allocation, err := api.StateGetAllocationForPendingDeal(ctx, dealIds[0], types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, dealProposal.PieceCID, allocation.Data)

	allocations, err := api.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)
	for _, alloc := range allocations {
		require.Equal(t, alloc, *allocation)
	}

	marketDeal, err := api.StateMarketStorageDeal(ctx, dealIds[0], types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, marketDeal.State.SectorStartEpoch, abi.ChainEpoch(-1))
}
