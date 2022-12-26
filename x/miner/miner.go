package miner

import (
	"errors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/x/conf"
)

func GetMinerID(miner api.StorageMiner) (string, error) {
	implMiner, ok := miner.(*impl.StorageMinerAPI)
	if !ok {
		return "", errors.New("StorageMiner not impl from StorageMinerAPI")
	}
	minerID, err := modules.MinerID(dtypes.MinerAddress(implMiner.Miner.Address()))
	if err != nil {
		return "", err
	}
	prefix := "t0"
	if conf.X.Net == "mainnet" {
		prefix = "f0"
	}
	return prefix + abi.ActorID(minerID).String(), nil
}
