package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestRules(t *testing.T) {
	ctx := context.Background()

	makeRule := func(ruleStr string) Filter {
		var r interface{}
		require.NoError(t, json.Unmarshal([]byte(ruleStr), &r))

		filter, err := ParseRule(ctx, r)
		require.NoError(t, err)
		return filter
	}

	a1, err := addr.NewIDAddress(1234)
	require.NoError(t, err)
	a2, err := addr.NewIDAddress(2345)
	require.NoError(t, err)

	testMsgSignRule := func(name string, f Filter, expect error, setup func(map[FilterParam]interface{})) {
		t.Run(name, func(t *testing.T) {
			action := map[FilterParam]interface{}{
				ParamAction:   ActionSign,
				ParamSignType: api.MTChainMsg,

				ParamSource:      a1,
				ParamDestination: a2,
				ParamValue:       types.FromFil(1),
				ParamMethod:      abi.MethodNum(0),
				ParamMaxFee:      0,
			}

			if setup != nil {
				setup(action)
			}

			res := f(action)
			require.ErrorIs(t, res, expect)
		})
	}

	f := makeRule(`{"Accept":{}}`)
	testMsgSignRule("accept", f, ErrAccept, nil)

	f = makeRule(`{"Reject":{}}`)
	testMsgSignRule("reject", f, ErrReject, nil)

	f = makeRule(`{"AnyAccepts":[
		{"Accept": {}},
		{"Reject": {}}
	]}`)
	testMsgSignRule("any-accept-first", f, ErrAccept, nil)

	f = makeRule(`{"AnyAccepts":[
		{"Reject": {}},
		{"Accept": {}}
	]}`)
	testMsgSignRule("any-reject-first", f, ErrReject, nil)

	f = makeRule(`{"AnyAccepts":[
		{"Sign": {"Message": {"Method":{"Method":[2], "Next": {"Reject":{}}}}}},
		{"Accept": {}}
	]}`)
	testMsgSignRule("any-message-match", f, ErrReject, func(m map[FilterParam]interface{}) {
		m[ParamMethod] = abi.MethodNum(2)
	})

	f = makeRule(`{"AnyAccepts":[
		{"Sign": {"Message": {"Method":{"Method":[2], "Next": {"Reject":{}}}}}}
	]}`)
	testMsgSignRule("any-none", f, nil, nil)

	f = makeRule(`{"AnyAccepts":[
		{"Sign": {"Message": {"Method":{"Method":[2], "Next": {"Reject":{}}}}}},
		{"Accept": {}}
	]}`)
	testMsgSignRule("any-accept-second", f, ErrAccept, nil)

	f = makeRule(`
		{"Sign": {"Message": {"Destination":{"Addr":["f02345"], "Next": {"Accept":{}}}}}}
	`)
	testMsgSignRule("dest", f, ErrAccept, nil)

	f = makeRule(`
		{"Sign": {"Message": {"Destination":{"Addr":["f01234"], "Next": {"Accept":{}}}}}}
	`)
	testMsgSignRule("wrong-dest", f, nil, nil)

	f = makeRule(`
		{"Sign": {"Message": {"Source":{"Addr":["f01234"], "Next": {"Accept":{}}}}}}
	`)
	testMsgSignRule("wrong-dest", f, ErrAccept, nil)

	f = makeRule(`
		{"Sign": {"Message": {"Source":{"Addr":["f01234"], "Next": {"Accept":{}}}}}}
	`)
	testMsgSignRule("wrong-dest", f, ErrAccept, nil)

	withVal := func(v uint64) func(m map[FilterParam]interface{}) {
		return func(m map[FilterParam]interface{}) {
			m[ParamValue] = types.FromFil(v)
		}
	}

	f = makeRule(`{"AnyAccepts":[
		{"Sign": {"Message": {"Value":{"LessThan":"2", "Next": {"Accept":{}}}}}},
		{"Sign": {"Message": {"Value":{"MoreThan":"4", "Next": {"Reject":{}}}}}}
	]}`)
	testMsgSignRule("value-less", f, ErrAccept, withVal(1))
	testMsgSignRule("value-between", f, nil, withVal(3))
	testMsgSignRule("value-more", f, ErrReject, withVal(5))
}
