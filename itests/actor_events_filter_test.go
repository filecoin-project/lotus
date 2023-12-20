package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestGetActorEvents(t *testing.T) {
	//require := require.New(t)
	kit.QuietAllLogsExcept("events", "messagepool")

	blockTime := 100 * time.Millisecond

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up the test fixture with a standard list of invocations
	contract1, contract2, invocations := prepareEventMatrixInvocations(ctx, t, client)
	fmt.Printf("contract1:%s; contract2:%s\n", contract1, contract2)

	cf1, err := contract1.ToFilecoinAddress()
	if err != nil {
		panic(err)
	}

	cf2, err := contract2.ToFilecoinAddress()
	if err != nil {
		panic(err)
	}

	fmt.Printf("contract1 f4 is:%s; contract2 f4 is:%s\n", cf1.String(), cf2.String())

	testCases := getCombinationFilterTestCases(contract1, contract2, "0x0")

	messages := invokeAndWaitUntilAllOnChain(t, client, invocations)

	// f410fiy2dwcbbvc5c6xwwrhlwgi2dby4rzgamxllpgva

	for _, tc := range testCases {
		tc := tc // appease the lint despot
		t.Run(tc.name, func(t *testing.T) {

			res, err := client.EthGetLogs(ctx, tc.spec)
			require.NoError(t, err)

			/*ch, _ := client.SubscribeActorEvents(ctx, &types.SubActorEventFilter{
				Prefill: true,
				ActorEventFilter: types.ActorEventFilter{
					MinEpoch: 0,
					MaxEpoch: 1000,
				},
			})

			for i := range ch {
				fmt.Println("Hello Chan", i.Entries[0].Key, i.Entries[0].Codec, i.EmitterAddr.String())
			}*/

			res2, _ := client.GetActorEvents(ctx, &types.ActorEventFilter{
				MinEpoch:  0,
				MaxEpoch:  -1,
				Addresses: []address.Address{cf2},
				//EthAddresses: []ethtypes.EthAddress{
				//	contract1,
				//},
			})
			for _, res := range res2 {
				res := res
				fmt.Println("Emitter Address is", res.EmitterAddr.String())
				for _, entry := range res.Entries {
					fmt.Println("Hello", entry.Key, entry.Codec, string(entry.Value))
				}

			}
			fmt.Println("Hello", res2[0].Entries[0].Key, res2[0].Entries[0].Codec, res2[0].EmitterAddr.String())

			elogs, err := parseEthLogsFromFilterResult(res)
			require.NoError(t, err)
			AssertEthLogs(t, elogs, tc.expected, messages)
		})
	}
}
