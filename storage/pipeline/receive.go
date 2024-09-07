package sealing

import (
	"bytes"
	"context"
	"errors"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func (m *Sealing) Receive(ctx context.Context, meta api.RemoteSectorMeta) error {
	m.inputLk.Lock()
	defer m.inputLk.Unlock()

	si, err := m.checkSectorMeta(ctx, meta)
	if err != nil {
		return err
	}

	exists, err := m.sectors.Has(uint64(meta.Sector.Number))
	if err != nil {
		return xerrors.Errorf("checking if sector exists: %w", err)
	}
	if exists {
		return xerrors.Errorf("sector %d state already exists", meta.Sector.Number)
	}

	err = m.sectors.Send(uint64(meta.Sector.Number), SectorReceive{
		State: si,
	})
	if err != nil {
		return xerrors.Errorf("receiving sector: %w", err)
	}

	return nil
}

func (m *Sealing) checkSectorMeta(ctx context.Context, meta api.RemoteSectorMeta) (SectorInfo, error) {
	{
		mid, err := address.IDFromAddress(m.maddr)
		if err != nil {
			panic(err)
		}

		if meta.Sector.Miner != abi.ActorID(mid) {
			return SectorInfo{}, xerrors.Errorf("sector for wrong actor - expected actor id %d, sector was for actor %d", mid, meta.Sector.Miner)
		}
	}

	{
		// initial sanity check, doesn't prevent races
		_, err := m.GetSectorInfo(meta.Sector.Number)
		if err != nil && !errors.Is(err, datastore.ErrNotFound) {
			return SectorInfo{}, err
		}
		if err == nil {
			return SectorInfo{}, xerrors.Errorf("sector with ID %d already exists in the sealing pipeline", meta.Sector.Number)
		}
	}

	{
		spt, err := m.currentSealProof(ctx)
		if err != nil {
			return SectorInfo{}, err
		}

		if meta.Type != spt {
			return SectorInfo{}, xerrors.Errorf("sector seal proof type doesn't match current seal proof type (%d!=%d)", meta.Type, spt)
		}
	}

	ts, err := m.Api.ChainHead(ctx)
	if err != nil {
		return SectorInfo{}, xerrors.Errorf("getting chain head: %w", err)
	}

	nv, err := m.Api.StateNetworkVersion(ctx, ts.Key())
	if err != nil {
		return SectorInfo{}, xerrors.Errorf("getting network version: %w", err)
	}

	var info SectorInfo
	var validatePoRep bool

	switch SectorState(meta.State) {
	case Proving, Available:
		if meta.CommitMessage != nil {
			if err := checkMessagePrefix(*meta.CommitMessage); err != nil {
				return SectorInfo{}, xerrors.Errorf("commit message prefix: %w", err)
			}

			info.CommitMessage = meta.CommitMessage
		}

		fallthrough
	case SubmitCommit:
		if meta.PreCommitDeposit == nil {
			return SectorInfo{}, xerrors.Errorf("sector PreCommitDeposit was null")
		}

		info.PreCommitDeposit = *meta.PreCommitDeposit
		info.PreCommitTipSet = meta.PreCommitTipSet
		if info.PreCommitMessage != nil {
			if err := checkMessagePrefix(*meta.PreCommitMessage); err != nil {
				return SectorInfo{}, xerrors.Errorf("commit message prefix: %w", err)
			}
			info.PreCommitMessage = meta.PreCommitMessage
		}

		// check provided seed
		if len(meta.SeedValue) != abi.RandomnessLength {
			return SectorInfo{}, xerrors.Errorf("seed randomness had wrong length %d", len(meta.SeedValue))
		}

		maddrBuf := new(bytes.Buffer)
		if err := m.maddr.MarshalCBOR(maddrBuf); err != nil {
			return SectorInfo{}, xerrors.Errorf("marshal miner address for seed check: %w", err)
		}
		rand, err := m.Api.StateGetRandomnessFromBeacon(ctx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, meta.SeedEpoch, maddrBuf.Bytes(), ts.Key())
		if err != nil {
			return SectorInfo{}, xerrors.Errorf("generating check seed: %w", err)
		}
		if !bytes.Equal(rand, meta.SeedValue) {
			return SectorInfo{}, xerrors.Errorf("provided(%x) and generated(%x) seeds differ", meta.SeedValue, rand)
		}

		info.SeedValue = meta.SeedValue
		info.SeedEpoch = meta.SeedEpoch

		info.Proof = meta.CommitProof
		validatePoRep = true

		fallthrough
	case PreCommitting:
		// check provided ticket
		if len(meta.TicketValue) != abi.RandomnessLength {
			return SectorInfo{}, xerrors.Errorf("ticket randomness had wrong length %d", len(meta.TicketValue))
		}

		maddrBuf := new(bytes.Buffer)
		if err := m.maddr.MarshalCBOR(maddrBuf); err != nil {
			return SectorInfo{}, xerrors.Errorf("marshal miner address for ticket check: %w", err)
		}
		rand, err := m.Api.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_SealRandomness, meta.TicketEpoch, maddrBuf.Bytes(), ts.Key())
		if err != nil {
			return SectorInfo{}, xerrors.Errorf("generating check ticket: %w", err)
		}
		if !bytes.Equal(rand, meta.TicketValue) {
			return SectorInfo{}, xerrors.Errorf("provided(%x) and generated(%x) tickets differ", meta.TicketValue, rand)
		}

		info.TicketValue = meta.TicketValue
		info.TicketEpoch = meta.TicketEpoch

		info.PreCommit1Out = meta.PreCommit1Out

		// check CommD/R
		if meta.CommD == nil || meta.CommR == nil {
			return SectorInfo{}, xerrors.Errorf("both CommR/CommD cids need to be set for sectors in PreCommitting and later states")
		}

		dp := meta.CommD.Prefix()
		if dp.Version != 1 || dp.Codec != cid.FilCommitmentUnsealed || dp.MhType != multihash.SHA2_256_TRUNC254_PADDED || dp.MhLength != 32 {
			return SectorInfo{}, xerrors.Errorf("CommD cid has wrong prefix")
		}

		rp := meta.CommR.Prefix()
		if rp.Version != 1 || rp.Codec != cid.FilCommitmentSealed || rp.MhType != multihash.POSEIDON_BLS12_381_A1_FC1 || rp.MhLength != 32 {
			return SectorInfo{}, xerrors.Errorf("CommR cid has wrong prefix")
		}

		info.CommD = meta.CommD
		info.CommR = meta.CommR

		if meta.DataSealed == nil {
			return SectorInfo{}, xerrors.Errorf("expected DataSealed to be set")
		}
		if meta.DataCache == nil {
			return SectorInfo{}, xerrors.Errorf("expected DataCache to be set")
		}
		info.RemoteDataSealed = meta.DataSealed // todo make head requests to check?
		info.RemoteDataCache = meta.DataCache

		if meta.RemoteCommit1Endpoint != "" {
			// validate the url
			if _, err := url.Parse(meta.RemoteCommit1Endpoint); err != nil {
				return SectorInfo{}, xerrors.Errorf("parsing remote c1 endpoint url: %w", err)
			}

			info.RemoteCommit1Endpoint = meta.RemoteCommit1Endpoint
		}

		if meta.RemoteCommit2Endpoint != "" {
			// validate the url
			if _, err := url.Parse(meta.RemoteCommit2Endpoint); err != nil {
				return SectorInfo{}, xerrors.Errorf("parsing remote c2 endpoint url: %w", err)
			}

			info.RemoteCommit2Endpoint = meta.RemoteCommit2Endpoint
		}

		// If we get a sector after PC2, and remote C1 endpoint is set, assume that we're getting finalized sector data
		if info.RemoteCommit1Endpoint != "" {
			info.RemoteDataFinalized = true
		}

		fallthrough
	case GetTicket, Packing:
		info.Return = ReturnState(meta.State)
		info.State = ReceiveSector

		info.SectorNumber = meta.Sector.Number
		info.Pieces = make([]SafeSectorPiece, len(meta.Pieces))
		info.SectorType = meta.Type

		for i, piece := range meta.Pieces {
			info.Pieces[i] = SafeSectorPiece{
				real: piece,
			}

			if !info.Pieces[i].HasDealInfo() {
				continue // cc
			}

			err := info.Pieces[i].DealInfo().Valid(nv)
			if err != nil {
				return SectorInfo{}, xerrors.Errorf("piece %d deal info invalid: %w", i, err)
			}
		}

		if meta.RemoteSealingDoneEndpoint != "" {
			// validate the url
			if _, err := url.Parse(meta.RemoteSealingDoneEndpoint); err != nil {
				return SectorInfo{}, xerrors.Errorf("parsing remote sealing-done endpoint url: %w", err)
			}

			info.RemoteSealingDoneEndpoint = meta.RemoteSealingDoneEndpoint
		}

		if err := checkPieces(ctx, m.maddr, meta.Sector.Number, info.Pieces, m.Api, false); err != nil {
			return SectorInfo{}, xerrors.Errorf("checking pieces: %w", err)
		}

		if meta.DataUnsealed == nil {
			return SectorInfo{}, xerrors.Errorf("expected DataUnsealed to be set")
		}
		info.RemoteDataUnsealed = meta.DataUnsealed

		// some late checks which require previous checks
		if validatePoRep {
			ok, err := m.verif.VerifySeal(proof.SealVerifyInfo{
				SealProof:             meta.Type,
				SectorID:              meta.Sector,
				DealIDs:               nil,
				Randomness:            meta.TicketValue,
				InteractiveRandomness: meta.SeedValue,
				Proof:                 meta.CommitProof,
				SealedCID:             *meta.CommR,
				UnsealedCID:           *meta.CommD,
			})
			if err != nil {
				return SectorInfo{}, xerrors.Errorf("validating seal proof: %w", err)
			}
			if !ok {
				return SectorInfo{}, xerrors.Errorf("seal proof invalid")
			}
		}

		return info, nil
	default:
		return SectorInfo{}, xerrors.Errorf("imported sector State in not supported")
	}
}

func (m *Sealing) handleReceiveSector(ctx statemachine.Context, sector SectorInfo) error {
	toFetch := map[storiface.SectorFileType]storiface.SectorLocation{}

	for fileType, data := range map[storiface.SectorFileType]*storiface.SectorLocation{
		storiface.FTUnsealed: sector.RemoteDataUnsealed,
		storiface.FTSealed:   sector.RemoteDataSealed,
		storiface.FTCache:    sector.RemoteDataCache,
	} {
		if data == nil {
			continue
		}

		if data.Local {
			// todo check exists
			continue
		}

		toFetch[fileType] = *data
	}

	if len(toFetch) > 0 {
		if err := m.sealer.DownloadSectorData(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber), sector.RemoteDataFinalized, toFetch); err != nil {
			return xerrors.Errorf("downloading sector data: %w", err) // todo send err event
		}
	}

	// todo data checks?

	return ctx.Send(SectorReceived{})
}

func checkMessagePrefix(c cid.Cid) error {
	p := c.Prefix()
	if p.Version != 1 || p.MhLength != 32 || p.MhType != multihash.BLAKE2B_MIN+31 || p.Codec != cid.DagCBOR {
		return xerrors.New("invalid message prefix")
	}
	return nil
}
