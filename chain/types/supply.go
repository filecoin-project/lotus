package types

import "github.com/filecoin-project/go-state-types/abi"

type CirculatingSupply struct {
	FilVested           abi.TokenAmount
	FilMined            abi.TokenAmount
	FilBurnt            abi.TokenAmount
	FilLocked           abi.TokenAmount
	FilCirculating      abi.TokenAmount
	FilReserveDisbursed abi.TokenAmount
}
