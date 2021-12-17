package types

import (
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"testing"
)

func TestParams(t *testing.T) {
	to, err := address.NewFromString("t1wmcrn4wzhihq6yzwnlo3e7f4rgcqzgmim5zhfkq")
	if err != nil {
		panic(err)
	}
	to2, err := address.NewFromString("t15ka4hwsfzojmwk3aeyyzuc73w6m7n3xl6a4f5ny")
	if err != nil {
		panic(err)
	}
	v1, err := ParseFIL("1")
	if err != nil {
		panic(err)
	}
	v2, err := ParseFIL("1")
	if err != nil {
		panic(err)
	}
	var r = Receive{
		To:     to,
		Value:  abi.TokenAmount(v1),
		Method: 0,
		Params: nil,
	}
	var r2 = Receive{
		To:     to2,
		Value:  abi.TokenAmount(v2),
		Method: 0,
		Params: nil,
	}
	//var test = ClassicalParams{Params:[]Receive{r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2}}
	//var test = ClassicalParams{Params:[]Receive{r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2}}
	//var test = ClassicalParams{Params:[]Receive{r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r,r,r,r,r,r,r,r,r,r,r,r,r,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2,r2}}
	var test = ClassicalParams{Params:[]Receive{r,r2}}
	by, err := json.Marshal(test)
	if err != nil {
		panic(err)
	}
	a := string(by)
	b, err := json.Marshal(a)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

//func TestParseParams(t *testing.T) {
//	var p = "{\"params\":[{\"To\":\"f1lhtzzv6zbi6bpnsu76kfvsrxgyz4ojyxs5cev3q\",\"Value\":\"1\",\"Method\":0,\"Params\":null},{\"To\":\"t1bf4luhhqnaieswiir7diadagdf2c4vuolntlzoq\",\"Value\":\"1\",\"Method\":0,\"Params\":null}]}"
//	var pa ClassicalParams
//	err := json.Unmarshal([]byte(p), &pa)
//	if err != nil {
//		panic(err)
//	}
//	fmt.Println(pa)
//}
