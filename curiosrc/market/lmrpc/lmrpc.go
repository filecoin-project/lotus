package lmrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/yugabyte/pgx/v5"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	cumarket "github.com/filecoin-project/lotus/curiosrc/market"
	"github.com/filecoin-project/lotus/curiosrc/market/fakelm"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/nullreader"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/paths"
	lpiece "github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("lmrpc")

const backpressureWaitTime = 30 * time.Second

func ServeCurioMarketRPCFromConfig(db *harmonydb.DB, full api.FullNode, cfg *config.CurioConfig) error {
	return forEachMarketRPC(cfg, func(maddr string, listen string) error {
		addr, err := address.NewFromString(maddr)
		if err != nil {
			return xerrors.Errorf("parsing actor address: %w", err)
		}

		go func() {
			err := ServeCurioMarketRPC(db, full, addr, cfg, listen)
			if err != nil {
				log.Errorf("failed to serve market rpc: %s", err)
			}
		}()

		return nil
	})
}

func MakeTokens(cfg *config.CurioConfig) (map[address.Address]string, error) {
	out := map[address.Address]string{}

	err := forEachMarketRPC(cfg, func(smaddr string, listen string) error {
		ctx := context.Background()

		laddr, err := net.ResolveTCPAddr("tcp", listen)
		if err != nil {
			return xerrors.Errorf("net resolve: %w", err)
		}

		if len(laddr.IP) == 0 || laddr.IP.IsUnspecified() {
			return xerrors.Errorf("market rpc server listen address must be a specific address, not %s (probably missing bind IP)", listen)
		}

		// need minimal provider with just the config
		lp := fakelm.NewLMRPCProvider(nil, nil, address.Undef, 0, 0, nil, nil, cfg)

		tok, err := lp.AuthNew(ctx, api.AllPermissions)
		if err != nil {
			return err
		}

		// parse listen into multiaddr
		ma, err := manet.FromNetAddr(laddr)
		if err != nil {
			return xerrors.Errorf("net from addr (%v): %w", laddr, err)
		}

		maddr, err := address.NewFromString(smaddr)
		if err != nil {
			return xerrors.Errorf("parsing actor address: %w", err)
		}

		token := fmt.Sprintf("%s:%s", tok, ma)
		out[maddr] = token

		return nil
	})

	return out, err
}

func forEachMarketRPC(cfg *config.CurioConfig, cb func(string, string) error) error {
	for n, server := range cfg.Subsystems.BoostAdapters {
		n := n

		// server: [f0.. actor address]:[bind address]
		// bind address is either a numeric port or a full address

		// first split at first : to get the actor address and the bind address
		split := strings.SplitN(server, ":", 2)

		// if the split length is not 2, return an error
		if len(split) != 2 {
			return fmt.Errorf("bad market rpc server config %d %s, expected [f0.. actor address]:[bind address]", n, server)
		}

		// get the actor address and the bind address
		strMaddr, strListen := split[0], split[1]

		maddr, err := address.NewFromString(strMaddr)
		if err != nil {
			return xerrors.Errorf("parsing actor address: %w", err)
		}

		// check the listen address
		if strListen == "" {
			return fmt.Errorf("bad market rpc server config %d %s, expected [f0.. actor address]:[bind address]", n, server)
		}
		// if listen address is numeric, prepend the default host
		if _, err := strconv.Atoi(strListen); err == nil {
			strListen = "0.0.0.0:" + strListen
		}
		// check if the listen address is a valid address
		if _, _, err := net.SplitHostPort(strListen); err != nil {
			return fmt.Errorf("bad market rpc server config %d %s, expected [f0.. actor address]:[bind address]", n, server)
		}

		log.Infow("Starting market RPC server", "actor", maddr, "listen", strListen)

		if err := cb(strMaddr, strListen); err != nil {
			return err
		}
	}

	return nil
}

func ServeCurioMarketRPC(db *harmonydb.DB, full api.FullNode, maddr address.Address, conf *config.CurioConfig, listen string) error {
	ctx := context.Background()

	pin, err := cumarket.NewPieceIngester(ctx, db, full, maddr, false, time.Duration(conf.Ingest.MaxDealWaitTime))
	if err != nil {
		return xerrors.Errorf("starting piece ingestor")
	}

	si := paths.NewDBIndex(nil, db)

	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return xerrors.Errorf("getting miner id: %w", err)
	}

	mi, err := full.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting miner info: %w", err)
	}

	lp := fakelm.NewLMRPCProvider(si, full, maddr, abi.ActorID(mid), mi.SectorSize, pin, db, conf)

	laddr, err := net.ResolveTCPAddr("tcp", listen)
	if err != nil {
		return xerrors.Errorf("net resolve: %w", err)
	}

	if len(laddr.IP) == 0 || laddr.IP.IsUnspecified() {
		return xerrors.Errorf("market rpc server listen address must be a specific address, not %s (probably missing bind IP)", listen)
	}
	rootUrl := url.URL{
		Scheme: "http",
		Host:   laddr.String(),
	}

	ast := api.StorageMinerStruct{}

	ast.CommonStruct.Internal.Version = func(ctx context.Context) (api.APIVersion, error) {
		return api.APIVersion{
			Version:    "curio-proxy-v0",
			APIVersion: api.MinerAPIVersion0,
			BlockDelay: build.BlockDelaySecs,
		}, nil
	}

	pieceInfoLk := new(sync.Mutex)
	pieceInfos := map[uuid.UUID][]pieceInfo{}

	ast.CommonStruct.Internal.AuthNew = lp.AuthNew
	ast.Internal.ActorAddress = lp.ActorAddress
	ast.Internal.WorkerJobs = lp.WorkerJobs
	ast.Internal.SectorsStatus = lp.SectorsStatus
	ast.Internal.SectorsList = lp.SectorsList
	ast.Internal.SectorsSummary = lp.SectorsSummary
	ast.Internal.SectorsListInStates = lp.SectorsListInStates
	ast.Internal.StorageRedeclareLocal = lp.StorageRedeclareLocal
	ast.Internal.ComputeDataCid = lp.ComputeDataCid
	ast.Internal.SectorAddPieceToAny = sectorAddPieceToAnyOperation(maddr, rootUrl, conf, pieceInfoLk, pieceInfos, pin, db, mi.SectorSize)
	ast.Internal.StorageList = si.StorageList
	ast.Internal.StorageDetach = si.StorageDetach
	ast.Internal.StorageReportHealth = si.StorageReportHealth
	ast.Internal.StorageDeclareSector = si.StorageDeclareSector
	ast.Internal.StorageDropSector = si.StorageDropSector
	ast.Internal.StorageFindSector = si.StorageFindSector
	ast.Internal.StorageInfo = si.StorageInfo
	ast.Internal.StorageBestAlloc = si.StorageBestAlloc
	ast.Internal.StorageLock = si.StorageLock
	ast.Internal.StorageTryLock = si.StorageTryLock
	ast.Internal.StorageGetLocks = si.StorageGetLocks
	ast.Internal.SectorStartSealing = pin.SectorStartSealing

	var pieceHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		// /piece?piece_id=xxxx
		pieceUUID := r.URL.Query().Get("piece_id")

		pu, err := uuid.Parse(pieceUUID)
		if err != nil {
			http.Error(w, "bad piece id", http.StatusBadRequest)
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}

		fmt.Printf("%s request for piece from %s\n", pieceUUID, r.RemoteAddr)

		pieceInfoLk.Lock()
		pis, ok := pieceInfos[pu]
		if !ok {
			http.Error(w, "piece not found", http.StatusNotFound)
			log.Warnw("piece not found", "piece_uuid", pu)
			pieceInfoLk.Unlock()
			return
		}

		// pop
		pi := pis[0]
		pis = pis[1:]

		pieceInfos[pu] = pis
		if len(pis) == 0 {
			delete(pieceInfos, pu)
		}

		pieceInfoLk.Unlock()

		start := time.Now()

		pieceData := io.LimitReader(io.MultiReader(
			pi.data,
			nullreader.Reader{},
		), int64(pi.size))

		n, err := io.Copy(w, pieceData)
		close(pi.done)

		took := time.Since(start)
		mbps := float64(n) / (1024 * 1024) / took.Seconds()

		if err != nil {
			log.Errorf("copying piece data: %s", err)
			return
		}

		log.Infow("piece served", "piece_uuid", pu, "size", float64(n)/(1024*1024), "duration", took, "speed", mbps)
	}

	finalApi := proxy.LoggingAPI[api.StorageMiner, api.StorageMinerStruct](&ast)

	mh, err := node.MinerHandler(finalApi, false) // todo permissioned
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/piece", pieceHandler)
	mux.Handle("/", mh) // todo: create a method for sealNow for sectors

	server := &http.Server{
		Addr:         listen,
		Handler:      mux,
		ReadTimeout:  48 * time.Hour,
		WriteTimeout: 48 * time.Hour, // really high because we block until pieces are saved in PiecePark
	}

	return server.ListenAndServe()
}

type pieceInfo struct {
	data storiface.Data
	size abi.UnpaddedPieceSize

	done chan struct{}
}

func sectorAddPieceToAnyOperation(maddr address.Address, rootUrl url.URL, conf *config.CurioConfig, pieceInfoLk *sync.Mutex, pieceInfos map[uuid.UUID][]pieceInfo, pin *cumarket.PieceIngester, db *harmonydb.DB, ssize abi.SectorSize) func(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data, deal lpiece.PieceDealInfo) (api.SectorOffset, error) {
	return func(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data, deal lpiece.PieceDealInfo) (api.SectorOffset, error) {
		if (deal.PieceActivationManifest == nil && deal.DealProposal == nil) || (deal.PieceActivationManifest != nil && deal.DealProposal != nil) {
			return api.SectorOffset{}, xerrors.Errorf("deal info must have either deal proposal or piece manifest")
		}

		origPieceData := pieceData
		defer func() {
			closer, ok := origPieceData.(io.Closer)
			if !ok {
				log.Warnf("DataCid: cannot close pieceData reader %T because it is not an io.Closer", origPieceData)
				return
			}
			if err := closer.Close(); err != nil {
				log.Warnw("closing pieceData in DataCid", "error", err)
			}
		}()

		pi := pieceInfo{
			data: pieceData,
			size: pieceSize,

			done: make(chan struct{}),
		}

		pieceUUID := uuid.New()

		if deal.DealProposal != nil {
			log.Infow("piece assign request", "piece_cid", deal.PieceCID().String(), "provider", deal.DealProposal.Provider, "piece_uuid", pieceUUID)
		}

		pieceInfoLk.Lock()
		pieceInfos[pieceUUID] = append(pieceInfos[pieceUUID], pi)
		pieceInfoLk.Unlock()

		// /piece?piece_cid=xxxx
		dataUrl := rootUrl
		dataUrl.Path = "/piece"
		dataUrl.RawQuery = "piece_id=" + pieceUUID.String()

		// add piece entry
		refID, pieceWasCreated, err := addPieceEntry(ctx, db, conf, deal, pieceSize, dataUrl, ssize)
		if err != nil {
			return api.SectorOffset{}, err
		}

		// wait for piece to be parked
		if pieceWasCreated {
			<-pi.done
		} else {
			// If the piece was not created, we need to close the done channel
			close(pi.done)

			closeDataReader(pieceData)
		}

		{
			// piece park is either done or currently happening from another AP call
			// now we need to make sure that the piece is definitely parked successfully
			// - in case of errors we return, and boost should be able to retry the call

			// * If piece is completed, return
			// * If piece is not completed but has null taskID, wait
			// * If piece has a non-null taskID
			//   * If the task is in harmony_tasks, wait
			//   * Otherwise look for an error in harmony_task_history and return that

			for {
				var taskID *int64
				var complete bool
				err := db.QueryRow(ctx, `SELECT pp.task_id, pp.complete
												FROM parked_pieces pp
												JOIN parked_piece_refs ppr ON pp.id = ppr.piece_id
												WHERE ppr.ref_id = $1;`, refID).Scan(&taskID, &complete)
				if err != nil {
					return api.SectorOffset{}, xerrors.Errorf("getting piece park status: %w", err)
				}

				if complete {
					break
				}

				if taskID == nil {
					// piece is not parked yet
					time.Sleep(5 * time.Second)
					continue
				}

				// check if task is in harmony_tasks
				var taskName string
				err = db.QueryRow(ctx, `SELECT name FROM harmony_task WHERE id = $1`, *taskID).Scan(&taskName)
				if err == nil {
					// task is in harmony_tasks, wait
					time.Sleep(5 * time.Second)
					continue
				}
				if err != pgx.ErrNoRows {
					return api.SectorOffset{}, xerrors.Errorf("checking park-piece task in harmony_tasks: %w", err)
				}

				// task is not in harmony_tasks, check harmony_task_history (latest work_end)
				var taskError string
				var taskResult bool
				err = db.QueryRow(ctx, `SELECT result, err FROM harmony_task_history WHERE task_id = $1 ORDER BY work_end DESC LIMIT 1`, *taskID).Scan(&taskResult, &taskError)
				if err != nil {
					return api.SectorOffset{}, xerrors.Errorf("checking park-piece task history: %w", err)
				}
				if !taskResult {
					return api.SectorOffset{}, xerrors.Errorf("park-piece task failed: %s", taskError)
				}
				return api.SectorOffset{}, xerrors.Errorf("park task succeeded but piece is not marked as complete")
			}
		}

		pieceIDUrl := url.URL{
			Scheme: "pieceref",
			Opaque: fmt.Sprintf("%d", refID),
		}

		// make a sector
		so, err := pin.AllocatePieceToSector(ctx, maddr, deal, int64(pieceSize), pieceIDUrl, nil)
		if err != nil {
			return api.SectorOffset{}, err
		}

		log.Infow("piece assigned to sector", "piece_cid", deal.PieceCID().String(), "sector", so.Sector, "offset", so.Offset)

		return so, nil
	}
}

func addPieceEntry(ctx context.Context, db *harmonydb.DB, conf *config.CurioConfig, deal lpiece.PieceDealInfo, pieceSize abi.UnpaddedPieceSize, dataUrl url.URL, ssize abi.SectorSize) (int64, bool, error) {
	var refID int64
	var pieceWasCreated bool

	for {
		var backpressureWait bool

		comm, err := db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
			// BACKPRESSURE
			wait, err := maybeApplyBackpressure(tx, conf.Ingest, ssize)
			if err != nil {
				return false, xerrors.Errorf("backpressure checks: %w", err)
			}
			if wait {
				backpressureWait = true
				return false, nil
			}

			var pieceID int64
			// Attempt to select the piece ID first
			err = tx.QueryRow(`SELECT id FROM parked_pieces WHERE piece_cid = $1`, deal.PieceCID().String()).Scan(&pieceID)

			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					// Piece does not exist, attempt to insert
					err = tx.QueryRow(`
							INSERT INTO parked_pieces (piece_cid, piece_padded_size, piece_raw_size)
							VALUES ($1, $2, $3)
							ON CONFLICT (piece_cid) DO NOTHING
							RETURNING id`, deal.PieceCID().String(), int64(pieceSize.Padded()), int64(pieceSize)).Scan(&pieceID)
					if err != nil {
						return false, xerrors.Errorf("inserting new parked piece and getting id: %w", err)
					}
					pieceWasCreated = true // New piece was created
				} else {
					// Some other error occurred during select
					return false, xerrors.Errorf("checking existing parked piece: %w", err)
				}
			} else {
				pieceWasCreated = false // Piece already exists, no new piece was created
			}

			// Add parked_piece_ref
			err = tx.QueryRow(`INSERT INTO parked_piece_refs (piece_id, data_url)
        			VALUES ($1, $2) RETURNING ref_id`, pieceID, dataUrl.String()).Scan(&refID)
			if err != nil {
				return false, xerrors.Errorf("inserting parked piece ref: %w", err)
			}

			// If everything went well, commit the transaction
			return true, nil // This will commit the transaction
		}, harmonydb.OptionRetry())
		if err != nil {
			return refID, pieceWasCreated, xerrors.Errorf("inserting parked piece: %w", err)
		}
		if !comm {
			if backpressureWait {
				// Backpressure was applied, wait and try again
				select {
				case <-time.After(backpressureWaitTime):
				case <-ctx.Done():
					return refID, pieceWasCreated, xerrors.Errorf("context done while waiting for backpressure: %w", ctx.Err())
				}
				continue
			}

			return refID, pieceWasCreated, xerrors.Errorf("piece tx didn't commit")
		}

		break
	}
	return refID, pieceWasCreated, nil
}

func closeDataReader(pieceData storiface.Data) {
	go func() {
		// close the data reader (drain to eof if it's not a closer)
		if closer, ok := pieceData.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Warnw("closing pieceData in DataCid", "error", err)
			}
		} else {
			log.Warnw("pieceData is not an io.Closer", "type", fmt.Sprintf("%T", pieceData))

			_, err := io.Copy(io.Discard, pieceData)
			if err != nil {
				log.Warnw("draining pieceData in DataCid", "error", err)
			}
		}
	}()
}

func maybeApplyBackpressure(tx *harmonydb.Tx, cfg config.CurioIngestConfig, ssize abi.SectorSize) (wait bool, err error) {
	var bufferedSDR, bufferedTrees, bufferedPoRep, waitDealSectors int
	err = tx.QueryRow(`
	WITH BufferedSDR AS (
    SELECT COUNT(p.task_id_sdr) - COUNT(t.owner_id) AS buffered_sdr_count
    FROM sectors_sdr_pipeline p
    LEFT JOIN harmony_task t ON p.task_id_sdr = t.id
    WHERE p.after_sdr = false
	),
	BufferedTrees AS (
    SELECT COUNT(p.task_id_tree_r) - COUNT(t.owner_id) AS buffered_trees_count
    FROM sectors_sdr_pipeline p
    LEFT JOIN harmony_task t ON p.task_id_tree_r = t.id
    WHERE p.after_sdr = true AND p.after_tree_r = false
	),
	BufferedPoRep AS (
    SELECT COUNT(p.task_id_porep) - COUNT(t.owner_id) AS buffered_porep_count
    FROM sectors_sdr_pipeline p
    LEFT JOIN harmony_task t ON p.task_id_porep = t.id
    WHERE p.after_tree_r = true AND p.after_porep = false
	),
	WaitDealSectors AS (
    SELECT COUNT(DISTINCT sip.sector_number) AS wait_deal_sectors_count
    FROM sectors_sdr_initial_pieces sip
    LEFT JOIN sectors_sdr_pipeline sp ON sip.sp_id = sp.sp_id AND sip.sector_number = sp.sector_number
    WHERE sp.sector_number IS NULL
	)
	SELECT
    (SELECT buffered_sdr_count FROM BufferedSDR) AS total_buffered_sdr,
    (SELECT buffered_trees_count FROM BufferedTrees) AS buffered_trees_count,
    (SELECT buffered_porep_count FROM BufferedPoRep) AS buffered_porep_count,
    (SELECT wait_deal_sectors_count FROM WaitDealSectors) AS wait_deal_sectors_count
`).Scan(&bufferedSDR, &bufferedTrees, &bufferedPoRep, &waitDealSectors)
	if err != nil {
		return false, xerrors.Errorf("counting buffered sectors: %w", err)
	}

	var pieceSizes []abi.PaddedPieceSize

	err = tx.Select(&pieceSizes, `SELECT piece_padded_size FROM parked_pieces WHERE complete = false;`)
	if err != nil {
		return false, xerrors.Errorf("getting in-process pieces")
	}

	sectors := sectorCount(pieceSizes, abi.PaddedPieceSize(ssize))
	if cfg.MaxQueueDealSector != 0 && waitDealSectors+sectors > cfg.MaxQueueDealSector {
		log.Debugw("backpressure", "reason", "too many wait deal sectors", "wait_deal_sectors", waitDealSectors, "max", cfg.MaxQueueDealSector)
		return true, nil
	}

	if bufferedSDR > cfg.MaxQueueSDR {
		log.Debugw("backpressure", "reason", "too many SDR tasks", "buffered", bufferedSDR, "max", cfg.MaxQueueSDR)
		return true, nil
	}
	if cfg.MaxQueueTrees != 0 && bufferedTrees > cfg.MaxQueueTrees {
		log.Debugw("backpressure", "reason", "too many tree tasks", "buffered", bufferedTrees, "max", cfg.MaxQueueTrees)
		return true, nil
	}
	if cfg.MaxQueuePoRep != 0 && bufferedPoRep > cfg.MaxQueuePoRep {
		log.Debugw("backpressure", "reason", "too many PoRep tasks", "buffered", bufferedPoRep, "max", cfg.MaxQueuePoRep)
		return true, nil
	}

	return false, nil
}

func sectorCount(sizes []abi.PaddedPieceSize, targetSize abi.PaddedPieceSize) int {
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i] > sizes[j]
	})

	sectors := make([]abi.PaddedPieceSize, 0)

	for _, size := range sizes {
		placed := false
		for i := range sectors {
			if sectors[i]+size <= targetSize {
				sectors[i] += size
				placed = true
				break
			}
		}
		if !placed {
			sectors = append(sectors, size)
		}
	}

	return len(sectors)
}
