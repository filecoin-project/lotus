package actors

import (
	"bytes"
	"context"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-amt-ipld"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

type StorageMarketActor struct{}

type smaMethods struct {
	Constructor                  uint64
	WithdrawBalance              uint64
	AddBalance                   uint64
	CheckLockedBalance           uint64
	PublishStorageDeals          uint64
	HandleCronAction             uint64
	SettleExpiredDeals           uint64
	ProcessStorageDealsPayment   uint64
	SlashStorageDealCollateral   uint64
	GetLastExpirationFromDealIDs uint64
	ActivateStorageDeals         uint64
	ComputeDataCommitment        uint64
}

var SMAMethods = smaMethods{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

func (sma StorageMarketActor) Exports() []interface{} {
	return []interface{}{
		2: sma.WithdrawBalance,
		3: sma.AddBalance,
		// 4: sma.CheckLockedBalance,
		5: sma.PublishStorageDeals,
		// 6: sma.HandleCronAction,
		// 7: sma.SettleExpiredDeals,
		// 8: sma.ProcessStorageDealsPayment,
		// 9: sma.SlashStorageDealCollateral,
		// 10: sma.GetLastExpirationFromDealIDs,
		11: sma.ActivateStorageDeals, // TODO: move under PublishStorageDeals after specs team approves
		12: sma.ComputeDataCommitment,
	}
}

type StorageParticipantBalance struct {
	Locked    types.BigInt
	Available types.BigInt
}

type StorageMarketState struct {
	Balances cid.Cid // hamt<addr, StorageParticipantBalance>
	Deals    cid.Cid // amt<StorageDeal>

	NextDealID uint64 // TODO: spec
}

// TODO: Drop in favour of car storage
type SerializationMode = uint64

const (
	SerializationUnixFSv0 = iota
	// IPLD / car
)

type StorageDealProposal struct {
	PieceRef           []byte // cid bytes // TODO: spec says to use cid.Cid, probably not a good idea
	PieceSize          uint64
	PieceSerialization SerializationMode // Needs to be here as it tells how data in the sector maps to PieceRef cid

	Client   address.Address
	Provider address.Address

	ProposalExpiration uint64
	Duration           uint64 // TODO: spec

	StoragePricePerEpoch types.BigInt
	StorageCollateral    types.BigInt

	ProposerSignature *types.Signature
}

func (sdp *StorageDealProposal) TotalStoragePrice() types.BigInt {
	return types.BigMul(sdp.StoragePricePerEpoch, types.NewInt(sdp.Duration))
}

type SignFunc = func(context.Context, []byte) (*types.Signature, error)

func (sdp *StorageDealProposal) Sign(ctx context.Context, sign SignFunc) error {
	if sdp.ProposerSignature != nil {
		return xerrors.New("signature already present in StorageDealProposal")
	}
	var buf bytes.Buffer
	if err := sdp.MarshalCBOR(&buf); err != nil {
		return err
	}
	sig, err := sign(ctx, buf.Bytes())
	if err != nil {
		return err
	}
	sdp.ProposerSignature = sig
	return nil
}

func (sdp *StorageDealProposal) Cid() (cid.Cid, error) {
	nd, err := cborutil.AsIpld(sdp)
	if err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), nil
}

func (sdp *StorageDealProposal) Verify() error {
	unsigned := *sdp
	unsigned.ProposerSignature = nil
	var buf bytes.Buffer
	if err := unsigned.MarshalCBOR(&buf); err != nil {
		return err
	}

	return sdp.ProposerSignature.Verify(sdp.Client, buf.Bytes())
}

func (d *StorageDeal) Sign(ctx context.Context, sign SignFunc) error {
	var buf bytes.Buffer
	if err := d.Proposal.MarshalCBOR(&buf); err != nil {
		return err
	}
	sig, err := sign(ctx, buf.Bytes())
	if err != nil {
		return err
	}
	d.CounterSignature = sig
	return nil
}

func (d *StorageDeal) Verify(proposerWorker address.Address) error {
	var buf bytes.Buffer
	if err := d.Proposal.MarshalCBOR(&buf); err != nil {
		return err
	}

	return d.CounterSignature.Verify(proposerWorker, buf.Bytes())
}

type StorageDeal struct {
	Proposal         StorageDealProposal
	CounterSignature *types.Signature
}

type OnChainDeal struct {
	Deal            StorageDeal
	ActivationEpoch uint64 // 0 = inactive
}

type WithdrawBalanceParams struct {
	Balance types.BigInt
}

func (sma StorageMarketActor) WithdrawBalance(act *types.Actor, vmctx types.VMContext, params *WithdrawBalanceParams) ([]byte, ActorError) {
	// TODO: (spec) this should be 2-stage

	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	b, bnd, err := GetMarketBalances(vmctx.Context(), vmctx.Ipld(), self.Balances, vmctx.Message().From)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not get balance")
	}

	balance := b[0]

	if balance.Available.LessThan(params.Balance) {
		return nil, aerrors.Newf(1, "can not withdraw more funds than available: %s > %s", params.Balance, b[0].Available)
	}

	balance.Available = types.BigSub(balance.Available, params.Balance)

	_, err = vmctx.Send(vmctx.Message().From, 0, params.Balance, nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "sending funds failed")
	}

	bcid, err := setMarketBalances(vmctx, bnd, map[address.Address]StorageParticipantBalance{
		vmctx.Message().From: balance,
	})
	if err != nil {
		return nil, err
	}

	self.Balances = bcid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	return nil, vmctx.Storage().Commit(old, nroot)
}

func (sma StorageMarketActor) AddBalance(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	b, bnd, err := GetMarketBalances(vmctx.Context(), vmctx.Ipld(), self.Balances, vmctx.Message().From)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not get balance")
	}

	balance := b[0]

	balance.Available = types.BigAdd(balance.Available, vmctx.Message().Value)

	bcid, err := setMarketBalances(vmctx, bnd, map[address.Address]StorageParticipantBalance{
		vmctx.Message().From: balance,
	})
	if err != nil {
		return nil, err
	}

	self.Balances = bcid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	return nil, vmctx.Storage().Commit(old, nroot)
}

func setMarketBalances(vmctx types.VMContext, nd *hamt.Node, set map[address.Address]StorageParticipantBalance) (cid.Cid, ActorError) {
	for addr, b := range set {
		balance := b // to stop linter complaining
		if err := nd.Set(vmctx.Context(), string(addr.Bytes()), &balance); err != nil {
			return cid.Undef, aerrors.HandleExternalError(err, "setting new balance")
		}
	}
	if err := nd.Flush(vmctx.Context()); err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "flushing balance hamt")
	}

	c, err := vmctx.Ipld().Put(vmctx.Context(), nd)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to balances storage")
	}
	return c, nil
}

func GetMarketBalances(ctx context.Context, store *hamt.CborIpldStore, rcid cid.Cid, addrs ...address.Address) ([]StorageParticipantBalance, *hamt.Node, ActorError) {
	nd, err := hamt.LoadNode(ctx, store, rcid)
	if err != nil {
		return nil, nil, aerrors.HandleExternalError(err, "failed to load miner set")
	}

	out := make([]StorageParticipantBalance, len(addrs))

	for i, a := range addrs {
		var balance StorageParticipantBalance
		err = nd.Find(ctx, string(a.Bytes()), &balance)
		switch err {
		case hamt.ErrNotFound:
			out[i] = StorageParticipantBalance{
				Locked:    types.NewInt(0),
				Available: types.NewInt(0),
			}
		case nil:
			out[i] = balance
		default:
			return nil, nil, aerrors.HandleExternalError(err, "failed to do set lookup")
		}

	}

	return out, nd, nil
}

/*
func (sma StorageMarketActor) CheckLockedBalance(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}
*/

type PublishStorageDealsParams struct {
	Deals []StorageDeal
}

type PublishStorageDealResponse struct {
	DealIDs []uint64
}

func (sma StorageMarketActor) PublishStorageDeals(act *types.Actor, vmctx types.VMContext, params *PublishStorageDealsParams) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	deals, err := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Deals)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "loading deals amt")
	}

	// todo: handle duplicate deals

	if len(params.Deals) == 0 {
		return nil, aerrors.New(1, "no storage deals in params.Deals")
	}

	out := PublishStorageDealResponse{
		DealIDs: make([]uint64, len(params.Deals)),
	}

	workerBytes, aerr := vmctx.Send(params.Deals[0].Proposal.Provider, MAMethods.GetWorkerAddr, types.NewInt(0), nil)
	if aerr != nil {
		return nil, aerr
	}
	providerWorker, err := address.NewFromBytes(workerBytes)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "parsing provider worker address bytes")
	}

	// TODO: REVIEW: Do we want to check if provider exists in the power actor?

	for i, deal := range params.Deals {
		if err := self.validateDeal(vmctx, deal, providerWorker); err != nil {
			return nil, err
		}

		err := deals.Set(self.NextDealID, &OnChainDeal{Deal: deal})
		if err != nil {
			return nil, aerrors.HandleExternalError(err, "setting deal in deal AMT")
		}
		out.DealIDs[i] = self.NextDealID

		self.NextDealID++
	}

	dealsCid, err := deals.Flush()
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "saving deals AMT")
	}

	self.Deals = dealsCid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "storing state failed")
	}

	aerr = vmctx.Storage().Commit(old, nroot)
	if aerr != nil {
		return nil, aerr
	}

	var outBuf bytes.Buffer
	if err := out.MarshalCBOR(&outBuf); err != nil {
		return nil, aerrors.HandleExternalError(err, "serialising output")
	}

	return outBuf.Bytes(), nil
}

func (st *StorageMarketState) validateDeal(vmctx types.VMContext, deal StorageDeal, providerWorker address.Address) aerrors.ActorError {
	if vmctx.BlockHeight() > deal.Proposal.ProposalExpiration {
		return aerrors.New(1, "deal proposal already expired")
	}

	if err := deal.Proposal.Verify(); err != nil {
		return aerrors.Absorb(err, 2, "verifying proposer signature")
	}

	err := deal.Verify(providerWorker)
	if err != nil {
		return aerrors.Absorb(err, 2, "verifying provider signature")
	}

	// TODO: maybe this is actually fine
	if vmctx.Message().From != providerWorker && vmctx.Message().From != deal.Proposal.Client {
		return aerrors.New(4, "message not sent by deal participant")
	}

	// TODO: do some caching (changes gas so needs to be in spec too)
	b, bnd, aerr := GetMarketBalances(vmctx.Context(), vmctx.Ipld(), st.Balances, deal.Proposal.Client, providerWorker)
	if aerr != nil {
		return aerrors.Wrap(aerr, "getting client, and provider balances")
	}
	clientBalance := b[0]
	providerBalance := b[1]

	totalPrice := deal.Proposal.TotalStoragePrice()

	if clientBalance.Available.LessThan(totalPrice) {
		return aerrors.Newf(5, "client doesn't have enough available funds to cover storage price; %d < %d", clientBalance.Available, totalPrice)
	}

	clientBalance = lockFunds(clientBalance, totalPrice)

	// TODO: REVIEW: Not clear who pays for this
	if providerBalance.Available.LessThan(deal.Proposal.StorageCollateral) {
		return aerrors.Newf(6, "provider doesn't have enough available funds to cover StorageCollateral; %d < %d", providerBalance.Available, deal.Proposal.StorageCollateral)
	}

	providerBalance = lockFunds(providerBalance, deal.Proposal.StorageCollateral)

	// TODO: piece checks (e.g. size > sectorSize)?

	bcid, aerr := setMarketBalances(vmctx, bnd, map[address.Address]StorageParticipantBalance{
		deal.Proposal.Client: clientBalance,
		providerWorker:       providerBalance,
	})
	if aerr != nil {
		return aerr
	}

	st.Balances = bcid

	return nil
}

type ActivateStorageDealsParams struct {
	Deals []uint64
}

func (sma StorageMarketActor) ActivateStorageDeals(act *types.Actor, vmctx types.VMContext, params *ActivateStorageDealsParams) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	deals, err := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Deals)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "loading deals amt")
	}

	for _, deal := range params.Deals {
		var dealInfo OnChainDeal
		if err := deals.Get(deal, &dealInfo); err != nil {
			if _, is := err.(*amt.ErrNotFound); is {
				return nil, aerrors.New(3, "deal not found")
			}
			return nil, aerrors.HandleExternalError(err, "getting deal info failed")
		}

		if vmctx.Message().From != dealInfo.Deal.Proposal.Provider {
			return nil, aerrors.New(1, "ActivateStorageDeals can only be called by the deal provider")
		}

		if vmctx.BlockHeight() > dealInfo.Deal.Proposal.ProposalExpiration {
			return nil, aerrors.New(2, "deal cannot be activated: proposal expired")
		}

		if dealInfo.ActivationEpoch > 0 {
			// this probably can't happen in practice
			return nil, aerrors.New(3, "deal already active")
		}

		dealInfo.ActivationEpoch = vmctx.BlockHeight()

		if err := deals.Set(deal, &dealInfo); err != nil {
			return nil, aerrors.HandleExternalError(err, "setting deal info in AMT failed")
		}
	}

	dealsCid, err := deals.Flush()
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "saving deals AMT")
	}

	self.Deals = dealsCid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "storing state failed")
	}

	aerr := vmctx.Storage().Commit(old, nroot)
	if aerr != nil {
		return nil, aerr
	}

	return nil, nil
}

type ProcessStorageDealsPaymentParams struct {
	DealIDs []uint64
}

func (sma StorageMarketActor) ProcessStorageDealsPayment(act *types.Actor, vmctx types.VMContext, params *ProcessStorageDealsPaymentParams) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	deals, err := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Deals)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "loading deals amt")
	}

	// TODO: Would be nice if send could assert actor type
	workerBytes, aerr := vmctx.Send(vmctx.Message().From, MAMethods.GetWorkerAddr, types.NewInt(0), nil)
	if aerr != nil {
		return nil, aerr
	}
	providerWorker, err := address.NewFromBytes(workerBytes)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "parsing provider worker address bytes")
	}

	for _, deal := range params.DealIDs {
		var dealInfo OnChainDeal
		if err := deals.Get(deal, &dealInfo); err != nil {
			if _, is := err.(*amt.ErrNotFound); is {
				return nil, aerrors.New(2, "deal not found")
			}
			return nil, aerrors.HandleExternalError(err, "getting deal info failed")
		}

		if dealInfo.Deal.Proposal.Provider != vmctx.Message().From {
			return nil, aerrors.New(3, "ProcessStorageDealsPayment can only be called by deal provider")
		}

		if vmctx.BlockHeight() < dealInfo.ActivationEpoch {
			// TODO: This is probably fatal
			return nil, aerrors.New(4, "ActivationEpoch lower than block height")
		}

		if vmctx.BlockHeight() > dealInfo.ActivationEpoch+dealInfo.Deal.Proposal.Duration {
			// Deal expired, miner should drop it
			// TODO: process payment for the remainder of last proving period
			return nil, nil
		}

		toPay := types.BigMul(dealInfo.Deal.Proposal.StoragePricePerEpoch, types.NewInt(build.SlashablePowerDelay))

		b, bnd, aerr := GetMarketBalances(vmctx.Context(), vmctx.Ipld(), self.Balances, dealInfo.Deal.Proposal.Client, providerWorker)
		if aerr != nil {
			return nil, aerr
		}
		clientBal := b[0]
		providerBal := b[1]

		clientBal.Locked, providerBal.Available = transferFunds(clientBal.Locked, providerBal.Available, toPay)

		// TODO: call set once
		bcid, aerr := setMarketBalances(vmctx, bnd, map[address.Address]StorageParticipantBalance{
			dealInfo.Deal.Proposal.Client: clientBal,
			providerWorker:                providerBal,
		})
		if aerr != nil {
			return nil, aerr
		}

		self.Balances = bcid
	}

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "storing state failed")
	}

	aerr = vmctx.Storage().Commit(old, nroot)
	if aerr != nil {
		return nil, aerr
	}

	return nil, nil
}

func lockFunds(p StorageParticipantBalance, amt types.BigInt) StorageParticipantBalance {
	p.Available, p.Locked = transferFunds(p.Available, p.Locked, amt)
	return p
}

func transferFunds(from, to, amt types.BigInt) (types.BigInt, types.BigInt) {
	// TODO: some asserts
	return types.BigSub(from, amt), types.BigAdd(to, amt)
}

type ComputeDataCommitmentParams struct {
	DealIDs    []uint64
	SectorSize uint64
}

func (sma StorageMarketActor) ComputeDataCommitment(act *types.Actor, vmctx types.VMContext, params *ComputeDataCommitmentParams) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	deals, err := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Deals)
	if err != nil {
		return nil, aerrors.HandleExternalError(err, "loading deals amt")
	}

	var pieces []sectorbuilder.PublicPieceInfo
	for _, deal := range params.DealIDs {
		var dealInfo OnChainDeal
		if err := deals.Get(deal, &dealInfo); err != nil {
			if _, is := err.(*amt.ErrNotFound); is {
				return nil, aerrors.New(3, "deal not found")
			}
			return nil, aerrors.HandleExternalError(err, "getting deal info failed")
		}

		if dealInfo.Deal.Proposal.Provider != vmctx.Message().From {
			return nil, aerrors.New(4, "referenced deal was not from caller")
		}

		var commP [32]byte
		copy(commP[:], dealInfo.Deal.Proposal.PieceRef)

		pieces = append(pieces, sectorbuilder.PublicPieceInfo{
			Size:  dealInfo.Deal.Proposal.PieceSize,
			CommP: commP,
		})
	}

	commd, err := sectorbuilder.GenerateDataCommitment(params.SectorSize, pieces)
	if err != nil {
		return nil, aerrors.Absorb(err, 5, "failed to generate data commitment from pieces")
	}

	return commd[:], nil
}

/*
func (sma StorageMarketActor) HandleCronAction(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) SettleExpiredDeals(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) SlashStorageDealCollateral(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) GetLastExpirationFromDealIDs(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}
*/
