package x

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func GetMinerID(miner api.StorageMiner) (string, error) {
	implMiner, ok := miner.(*impl.StorageMinerAPI)
	if !ok {
		return "", nil
	}
	minerID, err := modules.MinerID(dtypes.MinerAddress(implMiner.Miner.Address()))
	if err != nil {
		return "", err
	}
	return abi.ActorID(minerID).String(), nil
}
