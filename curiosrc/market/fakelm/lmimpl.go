package fakelm

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/curiosrc/market"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/paths"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	lpiece "github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type LMRPCProvider struct {
	si   paths.SectorIndex
	full api.FullNode

	maddr   address.Address // lotus-miner RPC is single-actor
	minerID abi.ActorID

	ssize abi.SectorSize

	pi   market.Ingester
	db   *harmonydb.DB
	conf *config.CurioConfig
}

func NewLMRPCProvider(si paths.SectorIndex, full api.FullNode, maddr address.Address, minerID abi.ActorID, ssize abi.SectorSize, pi market.Ingester, db *harmonydb.DB, conf *config.CurioConfig) *LMRPCProvider {
	return &LMRPCProvider{
		si:      si,
		full:    full,
		maddr:   maddr,
		minerID: minerID,
		ssize:   ssize,
		pi:      pi,
		db:      db,
		conf:    conf,
	}
}

func (l *LMRPCProvider) ActorAddress(ctx context.Context) (address.Address, error) {
	return l.maddr, nil
}

func (l *LMRPCProvider) WorkerJobs(ctx context.Context) (map[uuid.UUID][]storiface.WorkerJob, error) {
	// correct enough
	return map[uuid.UUID][]storiface.WorkerJob{}, nil
}

func (l *LMRPCProvider) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	var ssip []struct {
		PieceCID *string `db:"piece_cid"`
		DealID   *int64  `db:"f05_deal_id"`
		Complete bool    `db:"after_commit_msg_success"`
		Failed   bool    `db:"failed"`
		SDR      bool    `db:"after_sdr"`
		PoRep    bool    `db:"after_porep"`
	}

	err := l.db.Select(ctx, &ssip, `
						WITH CheckCommit AS (
						SELECT
							sp_id,
							sector_number,
							after_commit_msg,
							failed,
							after_sdr,
							after_porep,
							after_commit_msg_success
						FROM
							sectors_sdr_pipeline
						WHERE
							sp_id = $1 AND sector_number = $2
						),
						MetaPieces AS (
							SELECT
								mp.piece_cid,
								mp.f05_deal_id,
								cc.after_commit_msg_success,
								cc.failed,
								cc.after_sdr,
								cc.after_porep
							FROM
								sectors_meta_pieces mp
							INNER JOIN
								CheckCommit cc ON mp.sp_id = cc.sp_id AND mp.sector_num = cc.sector_number
							WHERE
								cc.after_commit_msg IS TRUE
						),
						InitialPieces AS (
							SELECT
								ip.piece_cid,
								ip.f05_deal_id,
								cc.after_commit_msg_success,
								cc.failed,
								cc.after_sdr,
								cc.after_porep
							FROM
								sectors_sdr_initial_pieces ip
							INNER JOIN
								CheckCommit cc ON ip.sp_id = cc.sp_id AND ip.sector_number = cc.sector_number
							WHERE
								cc.after_commit_msg IS FALSE
						),
						FallbackPieces AS (
							SELECT
								op.piece_cid,
								op.f05_deal_id,
								FALSE as after_commit_msg_success,
								FALSE as failed,
								FALSE as after_sdr,
								FALSE as after_porep
							FROM
								open_sector_pieces op
							WHERE
								op.sp_id = $1 AND op.sector_number = $2
								AND NOT EXISTS (SELECT 1 FROM sectors_sdr_pipeline sp WHERE sp.sp_id = op.sp_id AND sp.sector_number = op.sector_number)
						)
						SELECT * FROM MetaPieces
						UNION ALL
						SELECT * FROM InitialPieces
						UNION ALL
						SELECT * FROM FallbackPieces;`, l.minerID, sid)
	if err != nil {
		return api.SectorInfo{}, err
	}

	var deals []abi.DealID
	if len(ssip) > 0 {
		for _, d := range ssip {
			if d.DealID != nil {
				deals = append(deals, abi.DealID(*d.DealID))
			}
		}
	}

	spt, err := miner.SealProofTypeFromSectorSize(l.ssize, network.Version20, false) // good enough, just need this for ssize anyways
	if err != nil {
		return api.SectorInfo{}, err
	}

	ret := api.SectorInfo{
		SectorID:             sid,
		CommD:                nil,
		CommR:                nil,
		Proof:                nil,
		Deals:                deals,
		Pieces:               nil,
		Ticket:               api.SealTicket{},
		Seed:                 api.SealSeed{},
		PreCommitMsg:         nil,
		CommitMsg:            nil,
		Retries:              0,
		ToUpgrade:            false,
		ReplicaUpdateMessage: nil,
		LastErr:              "",
		Log:                  nil,
		SealProof:            spt,
		Activation:           0,
		Expiration:           0,
		DealWeight:           big.Zero(),
		VerifiedDealWeight:   big.Zero(),
		InitialPledge:        big.Zero(),
		OnTime:               0,
		Early:                0,
	}

	// If no rows found i.e. sector doesn't exist in DB
	//assign ssip[0] to a local variable for easier reading.
	currentSSIP := ssip[0]

	switch {
	case len(ssip) == 0:
		ret.State = api.SectorState(sealing.UndefinedSectorState)
	case currentSSIP.Failed:
		ret.State = api.SectorState(sealing.FailedUnrecoverable)
	case !currentSSIP.SDR:
		ret.State = api.SectorState(sealing.WaitDeals)
	case currentSSIP.SDR && !currentSSIP.PoRep:
		ret.State = api.SectorState(sealing.PreCommit1)
	case currentSSIP.SDR && currentSSIP.PoRep && !currentSSIP.Complete:
		ret.State = api.SectorState(sealing.PreCommit2)
	case currentSSIP.Complete:
		ret.State = api.SectorState(sealing.Proving)
	default:
		return api.SectorInfo{}, nil
	}
	return ret, nil
}

func (l *LMRPCProvider) SectorsList(ctx context.Context) ([]abi.SectorNumber, error) {
	decls, err := l.si.StorageList(ctx)
	if err != nil {
		return nil, err
	}

	var out []abi.SectorNumber
	for _, decl := range decls {
		for _, s := range decl {
			if s.Miner != l.minerID {
				continue
			}

			out = append(out, s.SectorID.Number)
		}
	}

	return out, nil
}

type sectorParts struct {
	sealed, unsealed, cache bool
	inStorage               bool
}

func (l *LMRPCProvider) SectorsSummary(ctx context.Context) (map[api.SectorState]int, error) {
	decls, err := l.si.StorageList(ctx)
	if err != nil {
		return nil, err
	}

	states := map[abi.SectorID]sectorParts{}
	for si, decll := range decls {
		sinfo, err := l.si.StorageInfo(ctx, si)
		if err != nil {
			return nil, err
		}

		for _, decl := range decll {
			if decl.Miner != l.minerID {
				continue
			}

			state := states[abi.SectorID{Miner: decl.Miner, Number: decl.SectorID.Number}]
			state.sealed = state.sealed || decl.Has(storiface.FTSealed)
			state.unsealed = state.unsealed || decl.Has(storiface.FTUnsealed)
			state.cache = state.cache || decl.Has(storiface.FTCache)
			state.inStorage = state.inStorage || sinfo.CanStore
			states[abi.SectorID{Miner: decl.Miner, Number: decl.SectorID.Number}] = state
		}
	}

	out := map[api.SectorState]int{}
	for _, state := range states {
		switch {
		case state.sealed && state.inStorage:
			out[api.SectorState(sealing.Proving)]++
		default:
			// not even close to correct, but good enough for now
			out[api.SectorState(sealing.PreCommit1)]++
		}
	}

	return out, nil
}

func (l *LMRPCProvider) SectorsListInStates(ctx context.Context, want []api.SectorState) ([]abi.SectorNumber, error) {
	decls, err := l.si.StorageList(ctx)
	if err != nil {
		return nil, err
	}

	wantProving, wantPrecommit1 := false, false
	for _, s := range want {
		switch s {
		case api.SectorState(sealing.Proving):
			wantProving = true
		case api.SectorState(sealing.PreCommit1):
			wantPrecommit1 = true
		}
	}

	states := map[abi.SectorID]sectorParts{}

	for si, decll := range decls {
		sinfo, err := l.si.StorageInfo(ctx, si)
		if err != nil {
			return nil, err
		}

		for _, decl := range decll {
			if decl.Miner != l.minerID {
				continue
			}

			state := states[abi.SectorID{Miner: decl.Miner, Number: decl.SectorID.Number}]
			state.sealed = state.sealed || decl.Has(storiface.FTSealed)
			state.unsealed = state.unsealed || decl.Has(storiface.FTUnsealed)
			state.cache = state.cache || decl.Has(storiface.FTCache)
			state.inStorage = state.inStorage || sinfo.CanStore
			states[abi.SectorID{Miner: decl.Miner, Number: decl.SectorID.Number}] = state
		}
	}
	var out []abi.SectorNumber

	for id, state := range states {
		switch {
		case state.sealed && state.inStorage:
			if wantProving {
				out = append(out, id.Number)
			}
		default:
			// not even close to correct, but good enough for now
			if wantPrecommit1 {
				out = append(out, id.Number)
			}
		}
	}

	return out, nil
}

func (l *LMRPCProvider) StorageRedeclareLocal(ctx context.Context, id *storiface.ID, b bool) error {
	// so this rescans and redeclares sectors on lotus-miner; whyyy is boost even calling this?

	return nil
}

func (l *LMRPCProvider) IsUnsealed(ctx context.Context, sectorNum abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (bool, error) {
	sectorID := abi.SectorID{Miner: l.minerID, Number: sectorNum}

	si, err := l.si.StorageFindSector(ctx, sectorID, storiface.FTUnsealed, 0, false)
	if err != nil {
		return false, err
	}

	// yes, yes, technically sectors can be partially unsealed, but that is never done in practice
	// and can't even be easily done with the current implementation
	return len(si) > 0, nil
}

func (l *LMRPCProvider) ComputeDataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error) {
	return abi.PieceInfo{}, xerrors.Errorf("not supported")
}

func (l *LMRPCProvider) SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d api.PieceDealInfo) (api.SectorOffset, error) {
	if d.DealProposal.PieceSize != abi.PaddedPieceSize(l.ssize) {
		return api.SectorOffset{}, xerrors.Errorf("only full-sector pieces are supported")
	}

	return api.SectorOffset{}, xerrors.Errorf("not supported, use AllocatePieceToSector")
}

func (l *LMRPCProvider) AllocatePieceToSector(ctx context.Context, maddr address.Address, piece lpiece.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error) {
	return l.pi.AllocatePieceToSector(ctx, maddr, piece, rawSize, source, header)
}

func (l *LMRPCProvider) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	type jwtPayload struct {
		Allow []auth.Permission
	}

	p := jwtPayload{
		Allow: perms,
	}

	sk, err := base64.StdEncoding.DecodeString(l.conf.Apis.StorageRPCSecret)
	if err != nil {
		return nil, xerrors.Errorf("decode secret: %w", err)
	}

	return jwt.Sign(&p, jwt.NewHS256(sk))
}

var _ MinimalLMApi = &LMRPCProvider{}
