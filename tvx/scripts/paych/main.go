package main

import "github.com/filecoin-project/specs-actors/actors/abi"

var (
	initialBal = abi.NewTokenAmount(200_000_000_000)
	toSend     = abi.NewTokenAmount(10_000)
)

func main() {
	happyPathCreate()
	happyPathUpdate()
	happyPathCollect()
}
