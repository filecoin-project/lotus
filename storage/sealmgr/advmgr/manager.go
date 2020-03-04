package advmgr

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealmgr"
)

type LocalStorage interface {
	GetStorage() (config.StorageConfig, error)
	SetStorage(config.StorageConfig) error
}

type Path struct {
	ID string
	Weight uint64

	LocalPath string

	CanSeal bool
	CanStore bool
}

type Worker interface {
	sealmgr.Worker

	Paths() []Path
}

type Manager struct {
	workers []sealmgr.Worker

	sectorbuilder.Prover
}

func New(ls LocalStorage, cfg *sectorbuilder.Config) (*Manager, error) {
	stor := &storage{
		localStorage: ls,
	}
	if err := stor.open(); err != nil {
		return nil, err
	}

	mid, err := address.IDFromAddress(cfg.Miner)
	if err != nil {
		return nil, xerrors.Errorf("getting miner id: %w", err)
	}

	prover, err := sectorbuilder.New(&readonlyProvider{stor: stor, miner: abi.ActorID(mid)}, cfg)
	if err != nil {
		return nil, xerrors.Errorf("creating prover instance: %w", err)
	}

	m := &Manager{
		workers:      nil,

		Prover: prover,
	}

	return m, nil
}

func (m Manager) SectorSize() abi.SectorSize {
	panic("implement me")
}

func (m Manager) NewSector() (abi.SectorNumber, error) {
	panic("implement me")
}

func (m Manager) ReadPieceFromSealedSector(context.Context, abi.SectorNumber, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error) {
	panic("implement me")
}

func (m Manager) AddPiece(context.Context, abi.UnpaddedPieceSize, abi.SectorNumber, io.Reader, []abi.UnpaddedPieceSize) (abi.PieceInfo, error) {
	panic("implement me")
}

func (m Manager) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out []byte, err error) {
	panic("implement me")
}

func (m Manager) SealPreCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	panic("implement me")
}

func (m Manager) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (output []byte, err error) {
	panic("implement me")
}

func (m Manager) SealCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (proof []byte, err error) {
	panic("implement me")
}

func (m Manager) FinalizeSector(context.Context, abi.SectorNumber) error {
	panic("implement me")
}

var _ sealmgr.Manager = &Manager{}
