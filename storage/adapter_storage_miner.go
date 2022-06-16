package storage

import (
	"bytes"
	"context"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	pipeline "github.com/filecoin-project/lotus/storage/pipeline"
)

var _ pipeline.SealingAPI = new(SealingAPIAdapter)

type SealingAPIAdapter struct {
	delegate fullNodeFilteredAPI
}

func NewSealingAPIAdapter(api fullNodeFilteredAPI) SealingAPIAdapter {
	return SealingAPIAdapter{delegate: api}
}

func (s SealingAPIAdapter) StateMinerPreCommitDepositForPower(ctx context.Context, a address.Address, pci minertypes.SectorPreCommitInfo, tsk types.TipSetKey) (big.Int, error) {
	return s.delegate.StateMinerPreCommitDepositForPower(ctx, a, pci, tsk)
}

func (s SealingAPIAdapter) StateMinerInitialPledgeCollateral(ctx context.Context, a address.Address, pci minertypes.SectorPreCommitInfo, tsk types.TipSetKey) (big.Int, error) {
	return s.delegate.StateMinerInitialPledgeCollateral(ctx, a, pci, tsk)
}

func (s SealingAPIAdapter) StateMinerInfo(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	return s.delegate.StateMinerInfo(ctx, maddr, tsk)
}

func (s SealingAPIAdapter) StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (big.Int, error) {
	return s.delegate.StateMinerAvailableBalance(ctx, maddr, tsk)
}

func (s SealingAPIAdapter) StateMinerDeadlines(ctx context.Context, maddr address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	return s.delegate.StateMinerDeadlines(ctx, maddr, tsk)
}

func (s SealingAPIAdapter) StateMinerSectorAllocated(ctx context.Context, maddr address.Address, sid abi.SectorNumber, tsk types.TipSetKey) (bool, error) {
	return s.delegate.StateMinerSectorAllocated(ctx, maddr, sid, tsk)
}

func (s SealingAPIAdapter) StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	return s.delegate.StateSectorGetInfo(ctx, maddr, sectorNumber, tsk)
}

func (s SealingAPIAdapter) StateMinerPartitions(ctx context.Context, maddr address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error) {
	return s.delegate.StateMinerPartitions(ctx, maddr, dlIdx, tsk)
}

func (s SealingAPIAdapter) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return s.delegate.StateLookupID(ctx, addr, tsk)
}

func (s SealingAPIAdapter) StateMarketStorageDeal(ctx context.Context, dealID abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	return s.delegate.StateMarketStorageDeal(ctx, dealID, tsk)
}

func (s SealingAPIAdapter) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	return s.delegate.StateNetworkVersion(ctx, tsk)
}

func (s SealingAPIAdapter) StateMinerProvingDeadline(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	return s.delegate.StateMinerProvingDeadline(ctx, maddr, tsk)
}

func (s SealingAPIAdapter) ChainGetMessage(ctx context.Context, mc cid.Cid) (*types.Message, error) {
	return s.delegate.ChainGetMessage(ctx, mc)
}

func (s SealingAPIAdapter) StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	return s.delegate.StateGetRandomnessFromBeacon(ctx, personalization, randEpoch, entropy, tsk)
}

func (s SealingAPIAdapter) StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	return s.delegate.StateGetRandomnessFromTickets(ctx, personalization, randEpoch, entropy, tsk)
}

func (s SealingAPIAdapter) ChainReadObj(ctx context.Context, ocid cid.Cid) ([]byte, error) {
	return s.delegate.ChainReadObj(ctx, ocid)
}

func (s SealingAPIAdapter) StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	return s.delegate.StateWaitMsg(ctx, cid, confidence, limit, allowReplaced)

}

func (s SealingAPIAdapter) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	return s.delegate.StateSearchMsg(ctx, from, msg, limit, allowReplaced)
}

func (s SealingAPIAdapter) StateComputeDataCommitment(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tsk types.TipSetKey) (cid.Cid, error) {

	nv, err := s.delegate.StateNetworkVersion(ctx, tsk)
	if err != nil {
		return cid.Cid{}, err
	}

	var ccparams []byte
	if nv < network.Version13 {
		ccparams, err = actors.SerializeParams(&market2.ComputeDataCommitmentParams{
			DealIDs:    deals,
			SectorType: sectorType,
		})
	} else {
		ccparams, err = actors.SerializeParams(&market5.ComputeDataCommitmentParams{
			Inputs: []*market5.SectorDataSpec{
				{
					DealIDs:    deals,
					SectorType: sectorType,
				},
			},
		})
	}

	if err != nil {
		return cid.Undef, xerrors.Errorf("computing params for ComputeDataCommitment: %w", err)
	}

	ccmt := &types.Message{
		To:     market.Address,
		From:   maddr,
		Value:  types.NewInt(0),
		Method: market.Methods.ComputeDataCommitment,
		Params: ccparams,
	}
	r, err := s.delegate.StateCall(ctx, ccmt, tsk)
	if err != nil {
		return cid.Undef, xerrors.Errorf("calling ComputeDataCommitment: %w", err)
	}
	if r.MsgRct.ExitCode != 0 {
		return cid.Undef, xerrors.Errorf("receipt for ComputeDataCommitment had exit code %d", r.MsgRct.ExitCode)
	}

	if nv < network.Version13 {
		var c cbg.CborCid
		if err := c.UnmarshalCBOR(bytes.NewReader(r.MsgRct.Return)); err != nil {
			return cid.Undef, xerrors.Errorf("failed to unmarshal CBOR to CborCid: %w", err)
		}

		return cid.Cid(c), nil
	}

	var cr market5.ComputeDataCommitmentReturn
	if err := cr.UnmarshalCBOR(bytes.NewReader(r.MsgRct.Return)); err != nil {
		return cid.Undef, xerrors.Errorf("failed to unmarshal CBOR to CborCid: %w", err)
	}

	if len(cr.CommDs) != 1 {
		return cid.Undef, xerrors.Errorf("CommD output must have 1 entry")
	}

	return cid.Cid(cr.CommDs[0]), nil
}

func (s SealingAPIAdapter) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*minertypes.SectorPreCommitOnChainInfo, error) {

	act, err := s.delegate.StateGetActor(ctx, maddr, tsk)
	if err != nil {
		return nil, xerrors.Errorf("handleSealFailed(%d): temp error: %+v", sectorNumber, err)
	}

	stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(s.delegate))

	state, err := miner.Load(stor, act)
	if err != nil {
		return nil, xerrors.Errorf("handleSealFailed(%d): temp error: loading miner state: %+v", sectorNumber, err)
	}

	pci, err := state.GetPrecommittedSector(sectorNumber)
	if err != nil {
		return nil, err
	}
	if pci == nil {
		set, err := state.IsAllocated(sectorNumber)
		if err != nil {
			return nil, xerrors.Errorf("checking if sector is allocated: %w", err)
		}
		if set {
			return nil, pipeline.ErrSectorAllocated
		}

		return nil, nil
	}

	return pci, nil
}

func (s SealingAPIAdapter) StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*pipeline.SectorLocation, error) {

	l, err := s.delegate.StateSectorPartition(ctx, maddr, sectorNumber, tsk)
	if err != nil {
		return nil, err
	}
	if l != nil {
		return &pipeline.SectorLocation{
			Deadline:  l.Deadline,
			Partition: l.Partition,
		}, nil
	}

	return nil, nil // not found
}

func (s SealingAPIAdapter) StateMarketStorageDealProposal(ctx context.Context, dealID abi.DealID, tsk types.TipSetKey) (market.DealProposal, error) {
	deal, err := s.delegate.StateMarketStorageDeal(ctx, dealID, tsk)
	if err != nil {
		return market.DealProposal{}, err
	}

	return deal.Proposal, nil
}

func (s SealingAPIAdapter) ChainHead(ctx context.Context) (types.TipSetKey, abi.ChainEpoch, error) {
	head, err := s.delegate.ChainHead(ctx)
	if err != nil {
		return types.EmptyTSK, 0, err
	}

	return head.Key(), head.Height(), nil
}

func (s SealingAPIAdapter) ChainBaseFee(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	ts, err := s.delegate.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return big.Zero(), err
	}

	return ts.Blocks()[0].ParentBaseFee, nil
}

func (s SealingAPIAdapter) SendMsg(ctx context.Context, from, to address.Address, method abi.MethodNum, value, maxFee abi.TokenAmount, params []byte) (cid.Cid, error) {
	msg := types.Message{
		To:     to,
		From:   from,
		Value:  value,
		Method: method,
		Params: params,
	}

	smsg, err := s.delegate.MpoolPushMessage(ctx, &msg, &api.MessageSendSpec{MaxFee: maxFee})
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}
