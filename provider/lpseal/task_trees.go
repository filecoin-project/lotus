package lpseal

import (
	"context"
	"github.com/filecoin-project/go-commp-utils/nonffi"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
	"github.com/ipfs/go-cid"
	"io"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type TreesTask struct {
	sp *SealPoller
	db *harmonydb.DB
	sc *lpffi.SealCalls

	max int
}

func NewTreesTask(sp *SealPoller, db *harmonydb.DB, sc *lpffi.SealCalls, maxTrees int) *TreesTask {
	return &TreesTask{
		sp: sp,
		db: db,
		sc: sc,

		max: maxTrees,
	}
}

func (t *TreesTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64                   `db:"sp_id"`
		SectorNumber int64                   `db:"sector_number"`
		RegSealProof abi.RegisteredSealProof `db:"reg_seal_proof"`
	}

	err = t.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number, reg_seal_proof
		FROM sectors_sdr_pipeline
		WHERE task_id_tree_r = $1 and task_id_tree_c = $1 and task_id_tree_d = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	if len(sectorParamsArr) != 1 {
		return false, xerrors.Errorf("expected 1 sector params, got %d", len(sectorParamsArr))
	}
	sectorParams := sectorParamsArr[0]

	var pieces []struct {
		PieceIndex int64  `db:"piece_index"`
		PieceCID   string `db:"piece_cid"`
		PieceSize  int64  `db:"piece_size"`

		DataUrl     *string `db:"data_url"`
		DataHeaders *[]byte `db:"data_headers"`
		DataRawSize *int64  `db:"data_raw_size"`
	}

	err = t.db.Select(ctx, &pieces, `
		SELECT piece_index, piece_cid, piece_size, data_url, data_headers, data_raw_size
		FROM sectors_sdr_initial_pieces
		WHERE sp_id = $1 AND sector_number = $2`, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("getting pieces: %w", err)
	}

	ssize, err := sectorParams.RegSealProof.SectorSize()
	if err != nil {
		return false, xerrors.Errorf("getting sector size: %w", err)
	}

	var commd cid.Cid
	var dataReader io.Reader
	var unpaddedData bool

	if len(pieces) > 0 {
		pieceInfos := make([]abi.PieceInfo, len(pieces))
		pieceReaders := make([]io.Reader, len(pieces))

		for i, p := range pieces {
			// make pieceInfo
			c, err := cid.Parse(p.PieceCID)
			if err != nil {
				return false, xerrors.Errorf("parsing piece cid: %w", err)
			}

			pieceInfos[i] = abi.PieceInfo{
				Size:     abi.PaddedPieceSize(p.PieceSize),
				PieceCID: c,
			}

			// make pieceReader
			if p.DataUrl != nil {
				pieceReaders[i], _ = padreader.New(&UrlPieceReader{
					Url:     *p.DataUrl,
					RawSize: *p.DataRawSize,
				}, uint64(*p.DataRawSize))
			} else { // padding piece (w/o fr32 padding, added in TreeD)
				pieceReaders[i] = nullreader.NewNullReader(abi.PaddedPieceSize(p.PieceSize).Unpadded())
			}
		}

		commd, err = nonffi.GenerateUnsealedCID(sectorParams.RegSealProof, pieceInfos)
		if err != nil {
			return false, xerrors.Errorf("computing CommD: %w", err)
		}

		dataReader = io.MultiReader(pieceReaders...)
		unpaddedData = true
	} else {
		commd = zerocomm.ZeroPieceCommitment(abi.PaddedPieceSize(ssize).Unpadded())
		dataReader = nullreader.NewNullReader(abi.UnpaddedPieceSize(ssize))
		unpaddedData = false // nullreader includes fr32 zero bits
	}

	sref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sectorParams.RegSealProof,
	}

	// D
	treeUnsealed, err := t.sc.TreeD(ctx, sref, abi.PaddedPieceSize(ssize), dataReader, unpaddedData)
	if err != nil {
		return false, xerrors.Errorf("computing tree d: %w", err)
	}

	// todo: Sooo tree-d contains exactly the unsealed data in the prefix
	//  when we finalize we can totally just truncate that file and move it to unsealed !!

	// R / C
	sealed, unsealed, err := t.sc.TreeRC(ctx, sref, commd)
	if err != nil {
		return false, xerrors.Errorf("computing tree r and c: %w", err)
	}

	if unsealed != treeUnsealed {
		return false, xerrors.Errorf("tree-d and tree-r/c unsealed CIDs disagree")
	}

	// todo synth porep

	// todo porep challenge check

	n, err := t.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_tree_r = true, after_tree_c = true, after_tree_d = true, tree_r_cid = $3, tree_d_cid = $4
		WHERE sp_id = $1 AND sector_number = $2`,
		sectorParams.SpID, sectorParams.SectorNumber, sealed, unsealed)
	if err != nil {
		return false, xerrors.Errorf("store sdr-trees success: updating pipeline: %w", err)
	}
	if n != 1 {
		return false, xerrors.Errorf("store sdr-trees success: updated %d rows", n)
	}

	return true, nil
}

func (t *TreesTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// todo reserve storage

	id := ids[0]
	return &id, nil
}

func (t *TreesTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  t.max,
		Name: "SDRTrees",
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 1,
			Ram: 8000 << 20, // todo
		},
		MaxFailures: 3,
		Follows:     nil,
	}
}

func (t *TreesTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	t.sp.pollers[pollerTrees].Set(taskFunc)
}

type UrlPieceReader struct {
	Url     string
	RawSize int64 // the exact number of bytes read, if we read more or less that's an error

	readSoFar int64
	active    io.ReadCloser // auto-closed on EOF
}

func (u *UrlPieceReader) Read(p []byte) (n int, err error) {
	// Check if we have already read the required amount of data
	if u.readSoFar >= u.RawSize {
		return 0, io.EOF
	}

	// If 'active' is nil, initiate the HTTP request
	if u.active == nil {
		resp, err := http.Get(u.Url)
		if err != nil {
			return 0, err
		}

		// Set 'active' to the response body
		u.active = resp.Body
	}

	// Calculate the maximum number of bytes we can read without exceeding RawSize
	toRead := u.RawSize - u.readSoFar
	if int64(len(p)) > toRead {
		p = p[:toRead]
	}

	n, err = u.active.Read(p)

	// Update the number of bytes read so far
	u.readSoFar += int64(n)

	// If the number of bytes read exceeds RawSize, return an error
	if u.readSoFar > u.RawSize {
		return n, xerrors.New("read beyond the specified RawSize")
	}

	// If EOF is reached, close the reader
	if err == io.EOF {
		cerr := u.active.Close()
		if cerr != nil {
			log.Errorf("error closing http piece reader: %s", cerr)
		}

		// if we're below the RawSize, return an unexpected EOF error
		if u.readSoFar < u.RawSize {
			return n, io.ErrUnexpectedEOF
		}
	}

	return n, err
}

var _ harmonytask.TaskInterface = &TreesTask{}
