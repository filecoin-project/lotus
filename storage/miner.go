package storage

import (
	"context"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("storageminer")

type storageMinerApi interface {
	// Call a read only method on actors (no interaction with the chain required)
	StateCall(context.Context, *types.Message, *types.TipSet) (*api.MethodCall, error)
	StateMinerWorker(context.Context, address.Address, *types.TipSet) (address.Address, error)
	StateMinerElectionPeriodStart(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error)
	StateMinerSectors(context.Context, address.Address, *types.TipSet) ([]*api.ChainSectorInfo, error)
	StateMinerProvingSet(context.Context, address.Address, *types.TipSet) ([]*api.ChainSectorInfo, error)
	StateMinerSectorSize(context.Context, address.Address, *types.TipSet) (uint64, error)
	StateWaitMsg(context.Context, cid.Cid) (*api.MsgWait, error) // TODO: removeme eventually
	StateGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error)
	StateGetReceipt(context.Context, cid.Cid, *types.TipSet) (*types.MessageReceipt, error)
	StateMarketStorageDeal(context.Context, uint64, *types.TipSet) (*actors.OnChainDeal, error)
	StateMinerFaults(context.Context, address.Address, *types.TipSet) ([]uint64, error)

	MpoolPushMessage(context.Context, *types.Message) (*types.SignedMessage, error)

	ChainHead(context.Context) (*types.TipSet, error)
	ChainNotify(context.Context) (<-chan []*store.HeadChange, error)
	ChainGetRandomness(context.Context, types.TipSetKey, int64) ([]byte, error)
	ChainGetTipSetByHeight(context.Context, uint64, *types.TipSet) (*types.TipSet, error)
	ChainGetBlockMessages(context.Context, cid.Cid) (*api.BlockMessages, error)
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)

	WalletSign(context.Context, address.Address, []byte) (*types.Signature, error)
	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
}

type SectorBuilderEpp struct {
	sb sectorbuilder.Interface
}

func NewElectionPoStProver(sb sectorbuilder.Interface) *SectorBuilderEpp {
	return &SectorBuilderEpp{sb}
}

var _ gen.ElectionPoStProver = (*SectorBuilderEpp)(nil)

func (epp *SectorBuilderEpp) GenerateCandidates(ctx context.Context, ssi sectorbuilder.SortedPublicSectorInfo, rand []byte) ([]sectorbuilder.EPostCandidate, error) {
	start := time.Now()
	var faults []uint64 // TODO

	var randbuf [32]byte
	copy(randbuf[:], rand)
	cds, err := epp.sb.GenerateEPostCandidates(ssi, randbuf, faults)
	if err != nil {
		return nil, err
	}
	log.Infof("Generate candidates took %s", time.Since(start))
	return cds, nil
}

func (epp *SectorBuilderEpp) ComputeProof(ctx context.Context, ssi sectorbuilder.SortedPublicSectorInfo, rand []byte, winners []sectorbuilder.EPostCandidate) ([]byte, error) {
	if build.InsecurePoStValidation {
		log.Warn("Generating fake EPost proof! You should only see this while running tests!")
		return []byte("valid proof"), nil
	}
	start := time.Now()
	proof, err := epp.sb.ComputeElectionPoSt(ssi, rand, winners)
	if err != nil {
		return nil, err
	}
	log.Infof("ComputeElectionPost took %s", time.Since(start))
	return proof, nil
}
