package kit

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	verifregtypes13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	datacap2 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

// SendFunds sends funds from the default wallet of the specified sender node
// to the recipient address.
func SendFunds(ctx context.Context, t *testing.T, sender *TestFullNode, recipient address.Address, amount abi.TokenAmount) {
	senderAddr, err := sender.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	msg := &types.Message{
		From:  senderAddr,
		To:    recipient,
		Value: amount,
	}

	sm, err := sender.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	sender.WaitMsg(ctx, sm.Cid())
}

func (f *TestFullNode) WaitMsg(ctx context.Context, msg cid.Cid) {
	res, err := f.StateWaitMsg(ctx, msg, 3, api.LookbackNoLimit, true)
	require.NoError(f.t, err)

	require.EqualValues(f.t, 0, res.Receipt.ExitCode, "message did not successfully execute")
}

// SetupVerifiedClients assumes that rootKey has been set in the ensemble's genesis and that
// verifierKey and verifiedClientKeys exist as accounts. It first sets up the verifier with datacap
// to allocate, and then allocates datacap to each of the verified clients.
// It returns the address of the verifier and the addresses of the verified clients.
func SetupVerifiedClients(ctx context.Context, t *testing.T, client *TestFullNode, rootKey *key.Key, verifierKey *key.Key, verifiedClientKeys []*key.Key) (verifierAddr address.Address, ret []address.Address) {
	// import the root key.
	rootAddr, err := client.WalletImport(ctx, &rootKey.KeyInfo)
	require.NoError(t, err)

	// import the verifiers' keys.
	verifierAddr, err = client.WalletImport(ctx, &verifierKey.KeyInfo)
	require.NoError(t, err)

	// import the verified client's key.
	for _, k := range verifiedClientKeys {
		verifiedClientAddr, err := client.WalletImport(ctx, &k.KeyInfo)
		require.NoError(t, err)
		ret = append(ret, verifiedClientAddr)
	}

	allowance := big.NewInt(100000000000)
	params, aerr := actors.SerializeParams(&verifreg.AddVerifierParams{Address: verifierAddr, Allowance: allowance})
	require.NoError(t, aerr)

	msg := &types.Message{
		From:   rootAddr,
		To:     verifreg.Address,
		Method: verifreg.Methods.AddVerifier,
		Params: params,
		Value:  big.Zero(),
	}

	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err, "AddVerifier failed")

	res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	verifierAllowance, err := client.StateVerifierStatus(ctx, verifierAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, allowance, *verifierAllowance)

	// assign datacap to clients
	for _, ad := range ret {
		initialDatacap := big.NewInt(10000)

		params, aerr = actors.SerializeParams(&verifreg.AddVerifiedClientParams{Address: ad, Allowance: initialDatacap})
		require.NoError(t, aerr)

		msg = &types.Message{
			From:   verifierAddr,
			To:     verifreg.Address,
			Method: verifreg.Methods.AddVerifiedClient,
			Params: params,
			Value:  big.Zero(),
		}

		sm, err = client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		res, err = client.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)
	}

	return
}

func SetupAllocation(
	ctx context.Context,
	t *testing.T,
	node api.FullNode,
	minerId uint64,
	dc abi.PieceInfo,
	verifiedClientAddr address.Address,
	expiration abi.ChainEpoch, // set to zero if we want to use maximum
	termMax abi.ChainEpoch, // set to zero if we want to use maximum
) (clientID abi.ActorID, allocationID verifregtypes13.AllocationId) {
	if termMax == 0 {
		termMax = verifreg.MaximumVerifiedAllocationTerm
	}
	if expiration == 0 {
		expiration = verifreg.MaximumVerifiedAllocationExpiration
	}

	var requests []verifreg.AllocationRequest

	allocationRequest := verifreg.AllocationRequest{
		Provider:   abi.ActorID(minerId),
		Data:       dc.PieceCID,
		Size:       dc.Size,
		TermMin:    verifreg.MinimumVerifiedAllocationTerm,
		TermMax:    termMax,
		Expiration: expiration,
	}
	requests = append(requests, allocationRequest)

	allocationRequests := verifreg.AllocationRequests{
		Allocations: requests,
	}

	receiverParams, aerr := actors.SerializeParams(&allocationRequests)
	require.NoError(t, aerr)

	transferParams, aerr := actors.SerializeParams(&datacap2.TransferParams{
		To:           builtin.VerifiedRegistryActorAddr,
		Amount:       big.Mul(big.NewInt(int64(dc.Size)), builtin.TokenPrecision),
		OperatorData: receiverParams,
	})
	require.NoError(t, aerr)

	msg := &types.Message{
		To:     builtin.DatacapActorAddr,
		From:   verifiedClientAddr,
		Method: datacap.Methods.TransferExported,
		Params: transferParams,
		Value:  big.Zero(),
	}

	sm, err := node.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	res, err := node.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	// check that we have an allocation
	allocations, err := node.StateGetAllocations(ctx, verifiedClientAddr, types.EmptyTSK)
	require.NoError(t, err)

	for key, value := range allocations {
		vkey := verifregtypes13.AllocationId(key)
		if dc.PieceCID.Equals(value.Data) && vkey >= allocationID {
			allocationID = verifregtypes13.AllocationId(key)
			clientID = value.Client
		}
	}

	require.NotEqual(t, verifreg.AllocationId(0), allocationID) // found it in there
	return clientID, allocationID
}
