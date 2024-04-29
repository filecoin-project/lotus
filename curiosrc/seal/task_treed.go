package seal

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-commp-utils/nonffi"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type TreeDTask struct {
	sp *SealPoller
	db *harmonydb.DB
	sc *ffi.SealCalls

	max int
}

func (t *TreeDTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	if engine.Resources().Gpu > 0 {
		return &ids[0], nil
	}
	return nil, nil
}

func (t *TreeDTask) TypeDetails() harmonytask.TaskTypeDetails {
	ssize := abi.SectorSize(32 << 30) // todo task details needs taskID to get correct sector size
	if isDevnet {
		ssize = abi.SectorSize(2 << 20)
	}

	return harmonytask.TaskTypeDetails{
		Max:  t.max,
		Name: "TreeD",
		Cost: resources.Resources{
			Cpu:     1,
			Ram:     1 << 30,
			Gpu:     0,
			Storage: t.sc.Storage(t.taskToSector, storiface.FTNone, storiface.FTCache, ssize, storiface.PathSealing, 1.0),
		},
		MaxFailures: 3,
		Follows:     nil,
	}
}

func (t *TreeDTask) taskToSector(id harmonytask.TaskID) (ffi.SectorRef, error) {
	var refs []ffi.SectorRef

	err := t.db.Select(context.Background(), &refs, `SELECT sp_id, sector_number, reg_seal_proof FROM sectors_sdr_pipeline WHERE task_id_tree_d = $1`, id)
	if err != nil {
		return ffi.SectorRef{}, xerrors.Errorf("getting sector ref: %w", err)
	}

	if len(refs) != 1 {
		return ffi.SectorRef{}, xerrors.Errorf("expected 1 sector ref, got %d", len(refs))
	}

	return refs[0], nil
}

func (t *TreeDTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	t.sp.pollers[pollerTreeD].Set(taskFunc)
}

func NewTreeDTask(sp *SealPoller, db *harmonydb.DB, sc *ffi.SealCalls, maxTrees int) *TreeDTask {
	return &TreeDTask{
		sp: sp,
		db: db,
		sc: sc,

		max: maxTrees,
	}
}

func (t *TreeDTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64                   `db:"sp_id"`
		SectorNumber int64                   `db:"sector_number"`
		RegSealProof abi.RegisteredSealProof `db:"reg_seal_proof"`
	}

	err = t.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number, reg_seal_proof
		FROM sectors_sdr_pipeline
		WHERE task_id_tree_d = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	if len(sectorParamsArr) != 1 {
		return false, xerrors.Errorf("expected 1 sector params, got %d", len(sectorParamsArr))
	}
	sectorParams := sectorParamsArr[0]

	sref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sectorParams.RegSealProof,
	}

	// Fetch the Sector to local storage
	fsPaths, pathIds, release, err := t.sc.PreFetch(ctx, sref, &taskID)
	if err != nil {
		return false, xerrors.Errorf("failed to prefetch sectors: %w", err)
	}
	defer release()

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
		WHERE sp_id = $1 AND sector_number = $2 ORDER BY piece_index ASC`, sectorParams.SpID, sectorParams.SectorNumber)
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

	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			if err := c.Close(); err != nil {
				log.Errorw("error closing piece reader", "error", err)
			}
		}
	}()

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
				dataUrl := *p.DataUrl

				goUrl, err := url.Parse(dataUrl)
				if err != nil {
					return false, xerrors.Errorf("parsing data URL: %w", err)
				}

				if goUrl.Scheme == "pieceref" {
					// url is to a piece reference

					refNum, err := strconv.ParseInt(goUrl.Opaque, 10, 64)
					if err != nil {
						return false, xerrors.Errorf("parsing piece reference number: %w", err)
					}

					// get pieceID
					var pieceID []struct {
						PieceID storiface.PieceNumber `db:"piece_id"`
					}
					err = t.db.Select(ctx, &pieceID, `SELECT piece_id FROM parked_piece_refs WHERE ref_id = $1`, refNum)
					if err != nil {
						return false, xerrors.Errorf("getting pieceID: %w", err)
					}

					if len(pieceID) != 1 {
						return false, xerrors.Errorf("expected 1 pieceID, got %d", len(pieceID))
					}

					pr, err := t.sc.PieceReader(ctx, pieceID[0].PieceID)
					if err != nil {
						return false, xerrors.Errorf("getting piece reader: %w", err)
					}

					closers = append(closers, pr)

					pieceReaders[i], _ = padreader.New(pr, uint64(*p.DataRawSize))
				} else {
					pieceReaders[i], _ = padreader.New(&UrlPieceReader{
						Url:     dataUrl,
						RawSize: *p.DataRawSize,
					}, uint64(*p.DataRawSize))
				}

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

	// Generate Tree D
	err = t.sc.TreeD(ctx, sref, commd, abi.PaddedPieceSize(ssize), dataReader, unpaddedData, fsPaths, pathIds, release)
	if err != nil {
		return false, xerrors.Errorf("failed to generate TreeD: %w", err)
	}

	n, err := t.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_tree_d = true, tree_d_cid = $3 WHERE sp_id = $1 AND sector_number = $2`,
		sectorParams.SpID, sectorParams.SectorNumber, commd)
	if err != nil {
		return false, xerrors.Errorf("store TreeD success: updating pipeline: %w", err)
	}
	if n != 1 {
		return false, xerrors.Errorf("store TreeD success: updated %d rows", n)
	}

	return true, nil
}

type UrlPieceReader struct {
	Url     string
	RawSize int64 // the exact number of bytes read, if we read more or less that's an error

	readSoFar int64
	closed    bool
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
		u.closed = true
		if cerr != nil {
			log.Errorf("error closing http piece reader: %s", cerr)
		}

		// if we're below the RawSize, return an unexpected EOF error
		if u.readSoFar < u.RawSize {
			log.Errorw("unexpected EOF", "readSoFar", u.readSoFar, "rawSize", u.RawSize, "url", u.Url)
			return n, io.ErrUnexpectedEOF
		}
	}

	return n, err
}

func (u *UrlPieceReader) Close() error {
	if !u.closed {
		u.closed = true
		return u.active.Close()
	}

	return nil
}

var _ harmonytask.TaskInterface = &TreeDTask{}
