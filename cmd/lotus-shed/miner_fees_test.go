package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/filecoin-project/go-state-types/builtin/v16/util/smoothing"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestDecodeMinerCronParams(t *testing.T) {
	params := miner16.DeferredCronEventParams{
		EventPayload:            []byte{0x81, 0x00},
		RewardSmoothed:          smoothing.NewEstimate(big.NewInt(7), big.NewInt(1)),
		QualityAdjPowerSmoothed: smoothing.NewEstimate(big.NewInt(11), big.NewInt(2)),
	}
	var legacy bytes.Buffer
	require.NoError(t, params.MarshalCBOR(&legacy))

	for _, tc := range []struct {
		name    string
		version actorstypes.Version
		weight  uint64
	}{
		{"v16", actorstypes.Version16, 1_000_000_000_000_000_000},
		{"v19", actorstypes.Version19, 1_000_000_000_000_000_000},
		{"v20 zero", actorstypes.Version20, 0},
		{"v20 partial", actorstypes.Version20, 650_000_000_000_000_000},
		{"v20 full", actorstypes.Version20, 1_000_000_000_000_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := bytes.NewBuffer(bytes.Clone(legacy.Bytes()))
			if tc.version >= actorstypes.Version20 {
				payload.Bytes()[0] = 0x84
				require.NoError(t, cbg.WriteMajorTypeHeader(payload, cbg.MajUnsignedInt, tc.weight))
			}
			code, ok := actors.GetActorCodeID(tc.version, manifest.MinerKey)
			require.True(t, ok)
			trace := types.ExecutionTrace{
				Msg:          types.MessageTrace{Params: payload.Bytes()},
				InvokedActor: &types.ActorTrace{State: types.Actor{Code: code}},
			}
			decoded, err := decodeMinerCronParams(trace)
			require.NoError(t, err)
			require.Equal(t, params, decoded.DeferredCronEventParams)
			require.Equal(t, tc.weight, decoded.ImplicitStreamWeight)
		})
	}
}
