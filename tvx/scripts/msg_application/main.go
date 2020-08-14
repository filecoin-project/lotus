package main

import (
	"github.com/filecoin-project/specs-actors/actors/abi"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

var (
	unknown      = MustNewIDAddr(10000000)
	balance1T    = abi.NewTokenAmount(1_000_000_000_000)
	transferAmnt = abi.NewTokenAmount(10)
)

func main() {
	g := NewGenerator()

	g.MessageVectorGroup("gas_cost",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-receipt-gas",
				Version: "v1",
				Desc:    "fail to cover gas cost for message receipt on chain",
			},
			Func: failCoverReceiptGasCost,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-onchainsize-gas",
				Version: "v1",
				Desc:    "not enough gas to pay message on-chain-size cost",
			},
			Func: failCoverOnChainSizeGasCost,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-transfer-accountcreation-gas",
				Version: "v1",
				Desc:    "fail not enough gas to cover account actor creation on transfer",
			},
			Func: failCoverTransferAccountCreationGasStepwise,
		})

	g.MessageVectorGroup("invalid_msgs",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-invalid-nonce",
				Version: "v1",
				Desc:    "invalid actor nonce",
			},
			Func: failInvalidActorNonce,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-invalid-receiver-method",
				Version: "v1",
				Desc:    "invalid receiver method",
			},
			Func: failInvalidReceiverMethod,
		},
	)

	g.MessageVectorGroup("unknown_actors",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-unknown-sender",
				Version: "v1",
				Desc:    "fail due to lack of gas when sender is unknown",
			},
			Func: failUnknownSender,
		},
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-unknown-receiver",
				Version: "v1",
				Desc:    "inexistent receiver",
				Comment: `Note that this test is not a valid message, since it is using
an unknown actor. However in the event that an invalid message isn't filtered by
block validation we need to ensure behaviour is consistent across VM implementations.`,
			},
			Func: failUnknownReceiver,
		},
	)

	g.MessageVectorGroup("actor_exec",
		&MessageVectorGenItem{
			Metadata: &schema.Metadata{
				ID:      "msg-apply-fail-actor-execution-illegal-arg",
				Version: "v1",
				Desc:    "abort during actor execution due to illegal argument",
			},
			Func: failActorExecutionAborted,
		},
	)

	g.Wait()
}
