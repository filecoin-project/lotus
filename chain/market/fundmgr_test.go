package market

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	tutils "github.com/filecoin-project/specs-actors/support/testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

type fakeAPI struct {
	returnedBalance    api.MarketBalance
	returnedBalanceErr error
	signature          crypto.Signature
	receivedMessage    *types.Message
	pushMessageErr     error
	lookupIDErr        error
}

func (fapi *fakeAPI) StateLookupID(_ context.Context, addr address.Address, _ types.TipSetKey) (address.Address, error) {
	return addr, fapi.lookupIDErr
}
func (fapi *fakeAPI) StateMarketBalance(context.Context, address.Address, types.TipSetKey) (api.MarketBalance, error) {
	return fapi.returnedBalance, fapi.returnedBalanceErr
}

func (fapi *fakeAPI) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	fapi.receivedMessage = msg
	return &types.SignedMessage{
		Message:   *msg,
		Signature: fapi.signature,
	}, fapi.pushMessageErr
}

func addFundsMsg(toAdd abi.TokenAmount, addr address.Address, wallet address.Address) *types.Message {
	params, _ := actors.SerializeParams(&addr)
	return &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   wallet,
		Value:  toAdd,
		Method: builtin.MethodsMarket.AddBalance,
		Params: params,
	}
}

type expectedResult struct {
	addAmt    abi.TokenAmount
	shouldAdd bool
	err       error
}

func TestAddFunds(t *testing.T) {
	ctx := context.Background()
	testCases := map[string]struct {
		returnedBalanceErr error
		returnedBalance    api.MarketBalance
		addAmounts         []abi.TokenAmount
		pushMessageErr     error
		expectedResults    []expectedResult
		lookupIDErr        error
	}{
		"succeeds, trivial case": {
			returnedBalance: api.MarketBalance{Escrow: abi.NewTokenAmount(0), Locked: abi.NewTokenAmount(0)},
			addAmounts:      []abi.TokenAmount{abi.NewTokenAmount(100)},
			expectedResults: []expectedResult{
				{
					addAmt:    abi.NewTokenAmount(100),
					shouldAdd: true,
					err:       nil,
				},
			},
		},
		"succeeds, money already present": {
			returnedBalance: api.MarketBalance{Escrow: abi.NewTokenAmount(150), Locked: abi.NewTokenAmount(50)},
			addAmounts:      []abi.TokenAmount{abi.NewTokenAmount(100)},
			expectedResults: []expectedResult{
				{
					shouldAdd: false,
					err:       nil,
				},
			},
		},
		"succeeds, multiple adds": {
			returnedBalance: api.MarketBalance{Escrow: abi.NewTokenAmount(150), Locked: abi.NewTokenAmount(50)},
			addAmounts:      []abi.TokenAmount{abi.NewTokenAmount(100), abi.NewTokenAmount(200), abi.NewTokenAmount(250), abi.NewTokenAmount(250)},
			expectedResults: []expectedResult{
				{
					shouldAdd: false,
					err:       nil,
				},
				{
					addAmt:    abi.NewTokenAmount(100),
					shouldAdd: true,
					err:       nil,
				},
				{
					addAmt:    abi.NewTokenAmount(50),
					shouldAdd: true,
					err:       nil,
				},
				{
					shouldAdd: false,
					err:       nil,
				},
			},
		},
		"error on market balance": {
			returnedBalanceErr: errors.New("something went wrong"),
			addAmounts:         []abi.TokenAmount{abi.NewTokenAmount(100)},
			expectedResults: []expectedResult{
				{
					err: errors.New("something went wrong"),
				},
			},
		},
		"error on push message": {
			returnedBalance: api.MarketBalance{Escrow: abi.NewTokenAmount(0), Locked: abi.NewTokenAmount(0)},
			pushMessageErr:  errors.New("something went wrong"),
			addAmounts:      []abi.TokenAmount{abi.NewTokenAmount(100)},
			expectedResults: []expectedResult{
				{
					err: errors.New("something went wrong"),
				},
			},
		},
		"error looking up address": {
			lookupIDErr: errors.New("something went wrong"),
			addAmounts:  []abi.TokenAmount{abi.NewTokenAmount(100)},
			expectedResults: []expectedResult{
				{
					err: errors.New("something went wrong"),
				},
			},
		},
	}

	for testCase, data := range testCases {
		//nolint:scopelint
		t.Run(testCase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			sig := make([]byte, 100)
			_, err := rand.Read(sig)
			require.NoError(t, err)
			fapi := &fakeAPI{
				returnedBalance:    data.returnedBalance,
				returnedBalanceErr: data.returnedBalanceErr,
				signature: crypto.Signature{
					Type: crypto.SigTypeUnknown,
					Data: sig,
				},
				pushMessageErr: data.pushMessageErr,
				lookupIDErr:    data.lookupIDErr,
			}
			fundMgr := newFundMgr(fapi)
			addr := tutils.NewIDAddr(t, uint64(rand.Uint32()))
			wallet := tutils.NewIDAddr(t, uint64(rand.Uint32()))
			for i, amount := range data.addAmounts {
				fapi.receivedMessage = nil
				_, err := fundMgr.EnsureAvailable(ctx, addr, wallet, amount)
				expected := data.expectedResults[i]
				if expected.err == nil {
					require.NoError(t, err)
					if expected.shouldAdd {
						expectedMessage := addFundsMsg(expected.addAmt, addr, wallet)
						require.Equal(t, expectedMessage, fapi.receivedMessage)
					} else {
						require.Nil(t, fapi.receivedMessage)
					}
				} else {
					require.EqualError(t, err, expected.err.Error())
				}
			}
		})
	}
}
