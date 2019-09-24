package deals

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-sectorbuilder/sealing_state"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/storage/sectorblocks"
)

type minerHandlerFunc func(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error)

func (h *Handler) handle(ctx context.Context, deal MinerDeal, cb minerHandlerFunc, next api.DealState) {
	go func() {
		mut, err := cb(ctx, deal)

		if err == nil && next == api.DealNoUpdate {
			return
		}

		select {
		case h.updated <- minerDealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
			mut:      mut,
		}:
		case <-h.stop:
		}
	}()
}

// ACCEPTED

func (h *Handler) validateVouchers(ctx context.Context, deal MinerDeal, paych address.Address) error {
	curHead, err := h.full.ChainHead(ctx)
	if err != nil {
		return err
	}
	if len(deal.Proposal.Payment.Vouchers) == 0 {
		return xerrors.Errorf("no payment vouchers for deal")
	}

	increments := deal.Proposal.Duration / uint64(len(deal.Proposal.Payment.Vouchers))

	startH := deal.Proposal.Payment.Vouchers[0].TimeLock - (deal.Proposal.Duration / increments)
	if startH > curHead.Height()+build.DealVoucherSkewLimit {
		return xerrors.Errorf("deal starts too far into the future")
	}

	vspec := VoucherSpec(deal.Proposal.Duration, deal.Proposal.TotalPrice, startH, nil)

	lane := deal.Proposal.Payment.Vouchers[0].Lane

	for i, voucher := range deal.Proposal.Payment.Vouchers {
		err := h.full.PaychVoucherCheckValid(ctx, deal.Proposal.Payment.PayChActor, voucher)
		if err != nil {
			return xerrors.Errorf("validating payment voucher %d: %w", i, err)
		}

		if voucher.Extra == nil {
			return xerrors.Errorf("validating payment voucher %d: voucher.Extra not set")
		}

		if voucher.Extra.Actor != deal.Proposal.MinerAddress {
			return xerrors.Errorf("validating payment voucher %d: extra params actor didn't match miner address in proposal: '%s' != '%s'", i, voucher.Extra.Actor, deal.Proposal.MinerAddress)
		}
		if voucher.Extra.Method != actors.MAMethods.PaymentVerifyInclusion {
			return xerrors.Errorf("validating payment voucher %d: expected extra method %d, got %d", i, actors.MAMethods.PaymentVerifyInclusion, voucher.Extra.Method)
		}

		var inclChallenge actors.PieceInclVoucherData
		if err := cbor.DecodeInto(voucher.Extra.Data, &inclChallenge); err != nil {
			return xerrors.Errorf("validating payment voucher %d: failed to decode storage voucher data for verification: %w", i, err)
		}
		if inclChallenge.PieceSize.Uint64() != deal.Proposal.Size {
			return xerrors.Errorf("validating payment voucher %d: paych challenge piece size didn't match deal proposal size: %d != %d", i, inclChallenge.PieceSize.Uint64(), deal.Proposal.Size)
		}
		if !bytes.Equal(inclChallenge.CommP, deal.Proposal.CommP) {
			return xerrors.Errorf("validating payment voucher %d: paych challenge commP didn't match deal proposal", i)
		}

		maxClose := curHead.Height() + (increments * uint64(i+1)) + build.DealVoucherSkewLimit
		if voucher.MinCloseHeight > maxClose {
			return xerrors.Errorf("validating payment voucher %d: MinCloseHeight too high (%d), max expected: %d", i, voucher.MinCloseHeight, maxClose)
		}

		if voucher.TimeLock > maxClose {
			return xerrors.Errorf("validating payment voucher %d: TimeLock too high (%d), max expected: %d", i, voucher.TimeLock, maxClose)
		}

		if len(voucher.Merges) > 0 {
			return xerrors.Errorf("validating payment voucher %d: didn't expect any merges", i)
		}
		if voucher.Amount.LessThan(vspec[i].Amount) {
			return xerrors.Errorf("validating payment voucher %d: not enough funds in the voucher: %s < %s; vl=%d vsl=%d", i, voucher.Amount, vspec[i].Amount, len(deal.Proposal.Payment.Vouchers), len(vspec))
		}
	}

	minPrice := types.BigMul(types.BigMul(h.pricePerByteBlock, types.NewInt(deal.Proposal.Size)), types.NewInt(deal.Proposal.Duration))
	if types.BigCmp(minPrice, deal.Proposal.TotalPrice) > 0 {
		return xerrors.Errorf("minimum price: %s", minPrice)
	}

	// TODO: This is mildly racy, we need a way to check lane and submit voucher
	//  atomically
	laneState, err := h.full.PaychLaneState(ctx, paych, lane)
	if err != nil {
		return xerrors.Errorf("looking up payment channel lane: %w", err)
	}

	if laneState.Closed {
		return xerrors.New("lane closed")
	}

	if laneState.Redeemed.GreaterThan(types.NewInt(0)) {
		return xerrors.New("used lanes unsupported")
	}

	if laneState.Nonce > 0 {
		return xerrors.New("used lanes unsupported")
	}

	return nil
}

func (h *Handler) accept(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	switch deal.Proposal.SerializationMode {
	//case SerializationRaw:
	//case SerializationIPLD:
	case SerializationUnixFs:
	default:
		return nil, xerrors.Errorf("deal proposal with unsupported serialization: %s", deal.Proposal.SerializationMode)
	}

	if deal.Proposal.Payment.ChannelMessage != nil {
		log.Info("waiting for channel message to appear on chain")
		if _, err := h.full.ChainWaitMsg(ctx, *deal.Proposal.Payment.ChannelMessage); err != nil {
			return nil, xerrors.Errorf("waiting for paych message: %w", err)
		}
	}

	if err := h.validateVouchers(ctx, deal, deal.Proposal.Payment.PayChActor); err != nil {
		return nil, err
	}

	for i, voucher := range deal.Proposal.Payment.Vouchers {
		// TODO: Set correct minAmount
		if _, err := h.full.PaychVoucherAdd(ctx, deal.Proposal.Payment.PayChActor, voucher, nil, types.NewInt(0)); err != nil {
			return nil, xerrors.Errorf("consuming payment voucher %d: %w", i, err)
		}
	}

	log.Info("fetching data for a deal")
	err := h.sendSignedResponse(StorageDealResponse{
		State:    api.DealAccepted,
		Message:  "",
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		return nil, err
	}

	return nil, merkledag.FetchGraph(ctx, deal.Ref, h.dag)
}

// STAGED

func (h *Handler) staged(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	err := h.sendSignedResponse(StorageDealResponse{
		State:    api.DealStaged,
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	root, err := h.dag.Get(ctx, deal.Ref)
	if err != nil {
		return nil, xerrors.Errorf("failed to get file root for deal: %s", err)
	}

	// TODO: abstract this away into ReadSizeCloser + implement different modes
	n, err := unixfile.NewUnixfsFile(ctx, h.dag, root)
	if err != nil {
		return nil, xerrors.Errorf("cannot open unixfs file: %s", err)
	}

	uf, ok := n.(sectorblocks.UnixfsReader)
	if !ok {
		// we probably got directory, unsupported for now
		return nil, xerrors.Errorf("unsupported unixfs file type")
	}

	sectorID, err := h.secst.AddUnixfsPiece(deal.Proposal.PieceRef, uf, deal.Proposal.Duration)
	if err != nil {
		return nil, xerrors.Errorf("AddPiece failed: %s", err)
	}

	log.Warnf("New Sector: %d", sectorID)
	return func(deal *MinerDeal) {
		deal.SectorID = sectorID
	}, nil
}

// SEALING

func getInclusionProof(ref string, status sectorbuilder.SectorSealingStatus) (PieceInclusionProof, error) {
	for i, p := range status.Pieces {
		if p.Key == ref {
			return PieceInclusionProof{
				Position:      uint64(i),
				ProofElements: p.InclusionProof,
			}, nil
		}
	}
	return PieceInclusionProof{}, xerrors.Errorf("pieceInclusionProof for %s in sector %d not found", ref, status.SectorID)
}

func (h *Handler) waitSealed(ctx context.Context, deal MinerDeal) (sectorbuilder.SectorSealingStatus, error) {
	status, err := h.secst.WaitSeal(ctx, deal.SectorID)
	if err != nil {
		return sectorbuilder.SectorSealingStatus{}, err
	}

	switch status.State {
	case sealing_state.Sealed:
	case sealing_state.Failed:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sealing sector %d for deal %s (ref=%s) failed: %s", deal.SectorID, deal.ProposalCid, deal.Ref, status.SealErrorMsg)
	case sealing_state.Pending:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sector status was 'pending' after call to WaitSeal (for sector %d)", deal.SectorID)
	case sealing_state.Sealing:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sector status was 'wait' after call to WaitSeal (for sector %d)", deal.SectorID)
	default:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("unknown SealStatusCode: %d", status.SectorID)
	}

	return status, nil
}

func (h *Handler) sealing(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	status, err := h.waitSealed(ctx, deal)
	if err != nil {
		return nil, err
	}

	// TODO: don't hardcode unixfs
	ip, err := getInclusionProof(string(sectorblocks.SerializationUnixfs0)+deal.Ref.String(), status)
	if err != nil {
		return nil, err
	}

	proof := &actors.InclusionProof{
		Sector: deal.SectorID,
		Proof:  ip.ProofElements,
	}
	proofB, err := cbor.DumpObject(proof)
	if err != nil {
		return nil, err
	}

	// store proofs for channels
	for i, v := range deal.Proposal.Payment.Vouchers {
		if v.Extra.Method == actors.MAMethods.PaymentVerifyInclusion {
			// TODO: Set correct minAmount
			if _, err := h.full.PaychVoucherAdd(ctx, deal.Proposal.Payment.PayChActor, v, proofB, types.NewInt(0)); err != nil {
				return nil, xerrors.Errorf("storing payment voucher %d proof: %w", i, err)
			}
		}
	}

	err = h.sendSignedResponse(StorageDealResponse{
		State:               api.DealSealing,
		Proposal:            deal.ProposalCid,
		PieceInclusionProof: ip,
		CommD:               status.CommD[:],
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	return nil, nil
}

func (h *Handler) complete(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	mcid, err := h.commt.WaitCommit(ctx, deal.Proposal.MinerAddress, deal.SectorID)
	if err != nil {
		log.Warnf("Waiting for sector commitment message: %s", err)
	}

	err = h.sendSignedResponse(StorageDealResponse{
		State:    api.DealComplete,
		Proposal: deal.ProposalCid,

		SectorCommitMessage: &mcid,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	return nil, nil
}
