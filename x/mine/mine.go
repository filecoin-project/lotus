package mine

import (
	"time"

	"hlm-ipfs/x/infras"
	"hlm-ipfs/x/logger"

	"github.com/filecoin-project/lotus/x/conf"
)

type MineInfo struct {
	Miner        string
	BaseEpoch    int64
	CurrentEpoch int64
	BeaconEpoch  int64
	Beacon       string
	Eligible     bool
	Late         bool
	WnPostID     int64
	TotalPower   string
	MinerPower   string
	WinCount     int32
	MsgCount     int32
	BlockCid     string
	Message      string
	StartTime    time.Time
	SubmitTime   time.Time
}

func NewMineInfo() *MineInfo {
	return &MineInfo{
		Miner:     conf.X.ID,
		StartTime: time.Now(),
	}
}

func PublishMine(info *MineInfo) {
	if info == nil {
		return
	}
	go func() {
		if err := conf.KV.LPush("lotus:x:mine:infos", infras.ToJson(info)).Err(); err != nil {
			logger.Error("publish mine-info error", "err", err)
		}
	}()
}
