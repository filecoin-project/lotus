package dtypes

import "github.com/filecoin-project/go-state-types/abi"

type DrandEnum int

type DrandSchedule []DrandPoint

type DrandPoint struct {
	Start  abi.ChainEpoch
	Config DrandConfig
}

type DrandConfig struct {
	Network       DrandEnum
	Servers       []string
	Relays        []string
	ChainInfoJSON string
}
