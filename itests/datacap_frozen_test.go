package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	datacap19 "github.com/filecoin-project/go-state-types/builtin/v19/datacap"
	verifreg19 "github.com/filecoin-project/go-state-types/builtin/v19/verifreg"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestDatacapFrozen asserts that FIP-0118 refuses every mutating entry point on the datacap and
// verified registry actors.
// Read-only methods (GetClaims, Balance, Name, ...) are untouched by the FIP.
func TestDatacapFrozen(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(2 * time.Millisecond)
	client.WaitTillChain(ctx, kit.HeightAtLeast(5))

	nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.GreaterOrEqual(t, nv, network.Version29)

	from, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	var (
		verifregAddr = builtin.VerifiedRegistryActorAddr
		datacapAddr  = builtin.DatacapActorAddr
		amount       = big.Mul(big.NewInt(1<<30), builtin.TokenPrecision)
		allowance    = verifreg19.DataCap(big.NewInt(1 << 30))
		// well-formed enough to deserialize; the actor never gets as far as verifying it
		signature = crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: make([]byte, 65)}
		// FRC-46 token receiver hook payload type, per the frc46_token library
		frc46 = verifreg19.ReceiverType(builtin.MustGenerateFRCMethodNum("FRC46"))
	)

	type frozenMethod struct {
		name   string
		to     address.Address
		method abi.MethodNum
		params cbor.Marshaler
		// callerGated methods reject an account sender at caller validation, ahead of the
		// FIP-0118 refusal, so only the exit code is observable from a plain wallet.
		callerGated bool
	}

	cases := []frozenMethod{
		{
			name: "verifreg/AddVerifier", to: verifregAddr, method: verifreg.Methods.AddVerifier,
			params: &verifreg19.AddVerifierParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/RemoveVerifier", to: verifregAddr, method: verifreg.Methods.RemoveVerifier,
			params: &from,
		}, {
			name: "verifreg/AddVerifiedClient", to: verifregAddr, method: verifreg.Methods.AddVerifiedClient,
			params: &verifreg19.AddVerifiedClientParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/AddVerifiedClientExported", to: verifregAddr, method: verifreg.Methods.AddVerifiedClientExported,
			params: &verifreg19.AddVerifiedClientParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/RemoveVerifiedClientDataCap", to: verifregAddr, method: verifreg.Methods.RemoveVerifiedClientDataCap,
			params: &verifreg19.RemoveDataCapParams{
				VerifiedClientToRemove: from,
				DataCapAmountToRemove:  allowance,
				VerifierRequest1:       verifreg19.RemoveDataCapRequest{Verifier: from, VerifierSignature: signature},
				VerifierRequest2:       verifreg19.RemoveDataCapRequest{Verifier: from, VerifierSignature: signature},
			},
		}, {
			name: "verifreg/RemoveExpiredAllocations", to: verifregAddr, method: verifreg.Methods.RemoveExpiredAllocations,
			params: &verifreg19.RemoveExpiredAllocationsParams{Client: abi.ActorID(100)},
		}, {
			name: "verifreg/RemoveExpiredAllocationsExported", to: verifregAddr, method: verifreg.Methods.RemoveExpiredAllocationsExported,
			params: &verifreg19.RemoveExpiredAllocationsParams{Client: abi.ActorID(100)},
		}, {
			name: "verifreg/ClaimAllocations", to: verifregAddr, method: verifreg.Methods.ClaimAllocations,
			params: &verifreg19.ClaimAllocationsParams{AllOrNothing: true}, callerGated: true,
		}, {
			name: "verifreg/ExtendClaimTerms", to: verifregAddr, method: verifreg.Methods.ExtendClaimTerms,
			params: &verifreg19.ExtendClaimTermsParams{},
		}, {
			name: "verifreg/ExtendClaimTermsExported", to: verifregAddr, method: verifreg.Methods.ExtendClaimTermsExported,
			params: &verifreg19.ExtendClaimTermsParams{},
		}, {
			name: "verifreg/RemoveExpiredClaims", to: verifregAddr, method: verifreg.Methods.RemoveExpiredClaims,
			params: &verifreg19.RemoveExpiredClaimsParams{Provider: abi.ActorID(1000)},
		}, {
			name: "verifreg/RemoveExpiredClaimsExported", to: verifregAddr, method: verifreg.Methods.RemoveExpiredClaimsExported,
			params: &verifreg19.RemoveExpiredClaimsParams{Provider: abi.ActorID(1000)},
		}, {
			// the allocation path: datacap tokens transferred to verifreg land here
			name: "verifreg/UniversalReceiverHook", to: verifregAddr, method: verifreg.Methods.UniversalReceiverHook,
			params: &verifreg19.UniversalReceiverParams{Type_: frc46}, callerGated: true,
		},

		{
			name: "datacap/Mint", to: datacapAddr, method: datacap.Methods.MintExported,
			params: &datacap19.MintParams{To: from, Amount: amount},
		}, {
			name: "datacap/Destroy", to: datacapAddr, method: datacap.Methods.DestroyExported,
			params: &datacap19.DestroyParams{Owner: from, Amount: amount},
		}, {
			name: "datacap/Transfer", to: datacapAddr, method: datacap.Methods.TransferExported,
			params: &datacap19.TransferParams{To: verifregAddr, Amount: amount},
		}, {
			name: "datacap/TransferFrom", to: datacapAddr, method: datacap.Methods.TransferFromExported,
			params: &datacap19.TransferFromParams{From: from, To: verifregAddr, Amount: amount},
		}, {
			name: "datacap/IncreaseAllowance", to: datacapAddr, method: datacap.Methods.IncreaseAllowanceExported,
			params: &datacap19.IncreaseAllowanceParams{Operator: from, Increase: amount},
		}, {
			name: "datacap/DecreaseAllowance", to: datacapAddr, method: datacap.Methods.DecreaseAllowanceExported,
			params: &datacap19.DecreaseAllowanceParams{Operator: from, Decrease: amount},
		}, {
			name: "datacap/RevokeAllowance", to: datacapAddr, method: datacap.Methods.RevokeAllowanceExported,
			params: &datacap19.RevokeAllowanceParams{Operator: from},
		}, {
			name: "datacap/Burn", to: datacapAddr, method: datacap.Methods.BurnExported,
			params: &datacap19.BurnParams{Amount: amount},
		}, {
			name: "datacap/BurnFrom", to: datacapAddr, method: datacap.Methods.BurnFromExported,
			params: &datacap19.BurnFromParams{Owner: from, Amount: amount},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, aerr := actors.SerializeParams(tc.params)
			require.NoError(t, aerr)

			res, err := client.StateCall(ctx, &types.Message{
				From:   from,
				To:     tc.to,
				Method: tc.method,
				Params: params,
				Value:  big.Zero(),
			}, types.EmptyTSK)
			require.NoError(t, err)

			require.Equal(t, exitcode.ErrForbidden, res.MsgRct.ExitCode, "error was: %s", res.Error)
			if !tc.callerGated {
				require.Contains(t, res.Error, "FIP-0118")
			}
		})
	}
}
