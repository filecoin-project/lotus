package itests

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"testing"
	"time"

	"github.com/docker/go-units"
	"github.com/gbrlsnchs/jwt/v3"
	"github.com/google/uuid"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	miner2 "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/cmd/curio/rpc"
	"github.com/filecoin-project/lotus/cmd/curio/tasks"
	"github.com/filecoin-project/lotus/curiosrc/market/lmrpc"
	"github.com/filecoin-project/lotus/curiosrc/seal"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestCurioNewActor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	full, miner, esemble := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs(),
		kit.WithSectorIndexDB(),
	)

	esemble.Start()
	blockTime := 100 * time.Millisecond
	esemble.BeginMining(blockTime)

	db := miner.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB

	var titles []string
	err := db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	require.NoError(t, err)
	require.NotEmpty(t, titles)
	require.NotContains(t, titles, "base")

	addr := miner.OwnerKey.Address
	sectorSizeInt, err := units.RAMInBytes("8MiB")
	require.NoError(t, err)

	maddr, err := spcli.CreateStorageMiner(ctx, full, addr, addr, addr, abi.SectorSize(sectorSizeInt), 0)
	require.NoError(t, err)

	err = deps.CreateMinerConfig(ctx, full, db, []string{maddr.String()}, "FULL NODE API STRING")
	require.NoError(t, err)

	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	require.NoError(t, err)
	require.Contains(t, titles, "base")
	baseCfg := config.DefaultCurioConfig()
	var baseText string

	err = db.QueryRow(ctx, "SELECT config FROM harmony_config WHERE title='base'").Scan(&baseText)
	require.NoError(t, err)
	_, err = deps.LoadConfigWithUpgrades(baseText, baseCfg)
	require.NoError(t, err)

	require.NotNil(t, baseCfg.Addresses)
	require.GreaterOrEqual(t, len(baseCfg.Addresses), 1)

	require.Contains(t, baseCfg.Addresses[0].MinerAddresses, maddr.String())
}

func TestCurioHappyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	full, miner, esemble := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.WithSectorIndexDB(),
	)

	esemble.Start()
	blockTime := 100 * time.Millisecond
	esemble.BeginMining(blockTime)

	err := miner.LogSetLevel(ctx, "*", "ERROR")
	require.NoError(t, err)

	err = full.LogSetLevel(ctx, "*", "ERROR")
	require.NoError(t, err)

	db := miner.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB

	token, err := full.AuthNew(ctx, api.AllPermissions)
	require.NoError(t, err)

	fapi := fmt.Sprintf("%s:%s", string(token), full.ListenAddr)

	var titles []string
	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	require.NoError(t, err)
	require.NotEmpty(t, titles)
	require.NotContains(t, titles, "base")

	addr := miner.OwnerKey.Address
	sectorSizeInt, err := units.RAMInBytes("2KiB")
	require.NoError(t, err)

	maddr, err := spcli.CreateStorageMiner(ctx, full, addr, addr, addr, abi.SectorSize(sectorSizeInt), 0)
	require.NoError(t, err)

	err = deps.CreateMinerConfig(ctx, full, db, []string{maddr.String()}, fapi)
	require.NoError(t, err)

	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	require.NoError(t, err)
	require.Contains(t, titles, "base")
	baseCfg := config.DefaultCurioConfig()
	var baseText string

	err = db.QueryRow(ctx, "SELECT config FROM harmony_config WHERE title='base'").Scan(&baseText)
	require.NoError(t, err)
	_, err = deps.LoadConfigWithUpgrades(baseText, baseCfg)
	require.NoError(t, err)

	require.NotNil(t, baseCfg.Addresses)
	require.GreaterOrEqual(t, len(baseCfg.Addresses), 1)

	require.Contains(t, baseCfg.Addresses[0].MinerAddresses, maddr.String())

	temp := os.TempDir()
	dir, err := os.MkdirTemp(temp, "curio")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(dir)
	}()

	capi, enginerTerm, closure, finishCh := ConstructCurioTest(ctx, t, dir, db, full, maddr, baseCfg)
	defer enginerTerm()
	defer closure()

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	mi, err := full.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	nv, err := full.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)

	wpt := mi.WindowPoStProofType
	spt, err := miner2.PreferredSealProofTypeFromWindowPoStType(nv, wpt, false)
	require.NoError(t, err)

	num, err := seal.AllocateSectorNumbers(ctx, full, db, maddr, 1, func(tx *harmonydb.Tx, numbers []abi.SectorNumber) (bool, error) {
		for _, n := range numbers {
			_, err := tx.Exec("insert into sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) values ($1, $2, $3)", mid, n, spt)
			if err != nil {
				return false, xerrors.Errorf("inserting into sectors_sdr_pipeline: %w", err)
			}
		}
		return true, nil
	})
	require.Len(t, num, 1)
	// TODO: add DDO deal, f05 deal 2 MiB each in the sector

	var sectorParamsArr []struct {
		SpID         int64 `db:"sp_id"`
		SectorNumber int64 `db:"sector_number"`
	}

	require.Eventuallyf(t, func() bool {
		err = db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number
		FROM sectors_sdr_pipeline
		WHERE after_commit_msg_success = True`)
		require.NoError(t, err)
		return len(sectorParamsArr) == 1
	}, 5*time.Minute, 1*time.Second, "sector did not finish sealing in 5 minutes")

	require.Equal(t, sectorParamsArr[0].SectorNumber, int64(0))
	require.Equal(t, sectorParamsArr[0].SpID, int64(mid))

	_ = capi.Shutdown(ctx)

	<-finishCh
}

func createCliContext(dir string) (*cli.Context, error) {
	// Define flags for the command
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Usage:   "host address and port the worker api will listen on",
			Value:   "0.0.0.0:12300",
			EnvVars: []string{"LOTUS_WORKER_LISTEN"},
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.BoolFlag{
			Name:   "halt-after-init",
			Usage:  "only run init, then return",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "manage-fdlimit",
			Usage: "manage open file limit",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "storage-json",
			Usage: "path to json file containing storage config",
			Value: "~/.curio/storage.json",
		},
		&cli.StringFlag{
			Name:  "journal",
			Usage: "path to journal files",
			Value: "~/.curio/",
		},
		&cli.StringSliceFlag{
			Name:    "layers",
			Aliases: []string{"l", "layer"},
			Usage:   "list of layers to be interpreted (atop defaults)",
		},
	}

	// Set up the command with flags
	command := &cli.Command{
		Name:  "simulate",
		Flags: flags,
		Action: func(c *cli.Context) error {
			fmt.Println("Listen address:", c.String("listen"))
			fmt.Println("No-sync:", c.Bool("nosync"))
			fmt.Println("Halt after init:", c.Bool("halt-after-init"))
			fmt.Println("Manage file limit:", c.Bool("manage-fdlimit"))
			fmt.Println("Storage config path:", c.String("storage-json"))
			fmt.Println("Journal path:", c.String("journal"))
			fmt.Println("Layers:", c.StringSlice("layers"))
			return nil
		},
	}

	// Create a FlagSet and populate it
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	for _, f := range flags {
		if err := f.Apply(set); err != nil {
			return nil, xerrors.Errorf("Error applying flag: %s\n", err)
		}
	}

	curioDir := path.Join(dir, "curio")
	cflag := fmt.Sprintf("--storage-json=%s", curioDir)

	storage := path.Join(dir, "storage.json")
	sflag := fmt.Sprintf("--journal=%s", storage)

	// Parse the flags with test values
	err := set.Parse([]string{"--listen=0.0.0.0:12345", "--nosync", "--manage-fdlimit", sflag, cflag, "--layers=seal", "--layers=post"})
	if err != nil {
		return nil, xerrors.Errorf("Error setting flag: %s\n", err)
	}

	// Create a cli.Context from the FlagSet
	app := cli.NewApp()
	ctx := cli.NewContext(app, set, nil)
	ctx.Command = command

	return ctx, nil
}

func ConstructCurioTest(ctx context.Context, t *testing.T, dir string, db *harmonydb.DB, full v1api.FullNode, maddr address.Address, cfg *config.CurioConfig) (api.Curio, func(), jsonrpc.ClientCloser, <-chan struct{}) {
	cctx, err := createCliContext(dir)
	require.NoError(t, err)

	shutdownChan := make(chan struct{})

	{
		var ctxclose func()
		ctx, ctxclose = context.WithCancel(ctx)
		go func() {
			<-shutdownChan
			ctxclose()
		}()
	}

	dependencies := &deps.Deps{}
	dependencies.DB = db
	dependencies.Full = full
	seal.SetDevnet(true)
	err = os.Setenv("CURIO_REPO_PATH", dir)
	require.NoError(t, err)
	err = dependencies.PopulateRemainingDeps(ctx, cctx, false)
	require.NoError(t, err)

	taskEngine, err := tasks.StartTasks(ctx, dependencies)
	require.NoError(t, err)

	dependencies.Cfg.Subsystems.BoostAdapters = []string{fmt.Sprintf("%s:127.0.0.1:32000", maddr)}
	err = lmrpc.ServeCurioMarketRPCFromConfig(dependencies.DB, dependencies.Full, dependencies.Cfg)
	require.NoError(t, err)

	go func() {
		err = rpc.ListenAndServe(ctx, dependencies, shutdownChan) // Monitor for shutdown.
		require.NoError(t, err)
	}()

	finishCh := node.MonitorShutdown(shutdownChan)

	var machines []string
	err = db.Select(ctx, &machines, `select host_and_port from harmony_machines`)
	require.NoError(t, err)

	require.Len(t, machines, 1)
	laddr, err := net.ResolveTCPAddr("tcp", machines[0])
	require.NoError(t, err)

	ma, err := manet.FromNetAddr(laddr)
	require.NoError(t, err)

	var apiToken []byte
	{
		type jwtPayload struct {
			Allow []auth.Permission
		}

		p := jwtPayload{
			Allow: api.AllPermissions,
		}

		sk, err := base64.StdEncoding.DecodeString(cfg.Apis.StorageRPCSecret)
		require.NoError(t, err)

		apiToken, err = jwt.Sign(&p, jwt.NewHS256(sk))
		require.NoError(t, err)
	}

	ctoken := fmt.Sprintf("%s:%s", string(apiToken), ma)
	err = os.Setenv("CURIO_API_INFO", ctoken)
	require.NoError(t, err)

	capi, ccloser, err := rpc.GetCurioAPI(&cli.Context{})
	require.NoError(t, err)

	scfg := storiface.LocalStorageMeta{
		ID:         storiface.ID(uuid.New().String()),
		Weight:     10,
		CanSeal:    true,
		CanStore:   true,
		MaxStorage: 0,
		Groups:     []string{},
		AllowTo:    []string{},
	}

	err = capi.StorageInit(ctx, dir, scfg)
	require.NoError(t, err)

	err = capi.StorageAddLocal(ctx, dir)
	require.NoError(t, err)

	err = capi.LogSetLevel(ctx, "harmonytask", "DEBUG")

	return capi, taskEngine.GracefullyTerminate, ccloser, finishCh
}
