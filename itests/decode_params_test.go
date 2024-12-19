package itests

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v10/eam"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/cli"
)

type marshalable interface {
	cbor.Marshaler
	cbor.Unmarshaler
}

type testCase struct {
	ActorKey  string
	MethodNum abi.MethodNum
	retVal    marshalable
}

// Used './lotus state replay --show-trace <msg-cid>' to get params/return to decode.
func TestDecodeParams(t *testing.T) {
	testCborBytes := abi.CborBytes([]byte{1, 2, 3})

	testCases := []testCase{
		{
			ActorKey:  manifest.EvmKey,
			MethodNum: builtin.MethodsEVM.InvokeContract,
			retVal:    &testCborBytes,
		},
		{
			ActorKey:  manifest.EamKey,
			MethodNum: builtin.MethodsEAM.CreateExternal,
			retVal:    &testCborBytes,
		},
	}

	for _, _tc := range testCases {
		tc := _tc
		t.Run(tc.ActorKey+" "+tc.MethodNum.String(), func(t *testing.T) {
			av, err := actorstypes.VersionForNetwork(buildconstants.TestNetworkVersion)
			require.NoError(t, err)
			actorCodeCid, found := actors.GetActorCodeID(av, tc.ActorKey)
			require.True(t, found)

			buf := bytes.NewBuffer(nil)
			if err := tc.retVal.MarshalCBOR(buf); err != nil {
				t.Fatal(err)
			}

			paramString, err := cli.JsonParams(actorCodeCid, tc.MethodNum, buf.Bytes())
			require.NoError(t, err)

			jsonParams, err := json.MarshalIndent(tc.retVal, "", "  ")
			require.NoError(t, err)
			require.Equal(t, string(jsonParams), paramString)
		})
	}
}

func TestDecodeReturn(t *testing.T) {
	testCborBytes := abi.CborBytes([]byte{1, 2, 3})

	robustAddr, err := address.NewIDAddress(12345)
	require.NoError(t, err)

	//ethAddr, err := ethtypes.ParseEthAddress("d4c5fb16488Aa48081296299d54b0c648C9333dA")
	//require.NoError(t, err)

	testReturn := eam.CreateExternalReturn{
		ActorID:       12345,
		RobustAddress: &robustAddr,
		EthAddress:    [20]byte{},
	}

	testCases := []testCase{
		{
			ActorKey:  manifest.EvmKey,
			MethodNum: builtin.MethodsEVM.InvokeContract,
			retVal:    &testCborBytes,
		},
		{
			ActorKey:  manifest.EamKey,
			MethodNum: builtin.MethodsEAM.CreateExternal,
			retVal:    &testReturn,
		},
	}

	for _, _tc := range testCases {
		tc := _tc
		t.Run(tc.ActorKey+" "+tc.MethodNum.String(), func(t *testing.T) {
			av, err := actorstypes.VersionForNetwork(buildconstants.TestNetworkVersion)
			require.NoError(t, err)
			actorCodeCid, found := actors.GetActorCodeID(av, tc.ActorKey)
			require.True(t, found)

			buf := bytes.NewBuffer(nil)
			if err := tc.retVal.MarshalCBOR(buf); err != nil {
				t.Fatal(err)
			}

			returnString, err := cli.JsonReturn(actorCodeCid, tc.MethodNum, buf.Bytes())
			require.NoError(t, err)

			jsonReturn, err := json.MarshalIndent(tc.retVal, "", "  ")
			require.NoError(t, err)
			require.Equal(t, string(jsonReturn), returnString)
		})
	}
}
