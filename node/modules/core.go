package modules

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gbrlsnchs/jwt/v3"
	logging "github.com/ipfs/go-log/v2"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/raulk/go-watchdog"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/addrutil"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/system"
)

const (
	// EnvWatchdogDisabled is an escape hatch to disable the watchdog explicitly
	// in case an OS/kernel appears to report incorrect information. The
	// watchdog will be disabled if the value of this env variable is 1.
	EnvWatchdogDisabled = "LOTUS_DISABLE_WATCHDOG"
)

const (
	JWTSecretName   = "auth-jwt-private" //nolint:gosec
	KTJwtHmacSecret = "jwt-hmac-secret"  //nolint:gosec
)

var (
	log         = logging.Logger("modules")
	logWatchdog = logging.Logger("watchdog")
)

type Genesis func() (*types.BlockHeader, error)

// RecordValidator provides namesys compatible routing record validator
func RecordValidator(ps peerstore.Peerstore) record.Validator {
	return record.NamespacedValidator{
		"pk": record.PublicKeyValidator{},
	}
}

// MemoryConstraints returns the memory constraints configured for this system.
func MemoryConstraints() system.MemoryConstraints {
	constraints := system.GetMemoryConstraints()
	log.Infow("memory limits initialized",
		"max_mem_heap", constraints.MaxHeapMem,
		"total_system_mem", constraints.TotalSystemMem,
		"effective_mem_limit", constraints.EffectiveMemLimit)
	return constraints
}

// MemoryWatchdog starts the memory watchdog, applying the computed resource
// constraints.
func MemoryWatchdog(lr repo.LockedRepo, lc fx.Lifecycle, constraints system.MemoryConstraints) {
	if os.Getenv(EnvWatchdogDisabled) == "1" {
		log.Infof("memory watchdog is disabled via %s", EnvWatchdogDisabled)
		return
	}

	// configure heap profile capture so that one is captured per episode where
	// utilization climbs over 90% of the limit. A maximum of 10 heapdumps
	// will be captured during life of this process.
	watchdog.HeapProfileDir = filepath.Join(lr.Path(), "heapprof")
	watchdog.HeapProfileMaxCaptures = 10
	watchdog.HeapProfileThreshold = 0.9
	watchdog.Logger = logWatchdog

	policy := watchdog.NewWatermarkPolicy(0.50, 0.60, 0.70, 0.85, 0.90, 0.925, 0.95)

	// Try to initialize a watchdog in the following order of precedence:
	// 1. If a max heap limit has been provided, initialize a heap-driven watchdog.
	// 2. Else, try to initialize a cgroup-driven watchdog.
	// 3. Else, try to initialize a system-driven watchdog.
	// 4. Else, log a warning that the system is flying solo, and return.

	addStopHook := func(stopFn func()) {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				stopFn()
				return nil
			},
		})
	}

	// 1. If user has set max heap limit, apply it.
	if maxHeap := constraints.MaxHeapMem; maxHeap != 0 {
		const minGOGC = 10
		err, stopFn := watchdog.HeapDriven(maxHeap, minGOGC, policy)
		if err == nil {
			log.Infof("initialized heap-driven watchdog; max heap: %d bytes", maxHeap)
			addStopHook(stopFn)
			return
		}
		log.Warnf("failed to initialize heap-driven watchdog; err: %s", err)
		log.Warnf("trying a cgroup-driven watchdog")
	}

	// 2. cgroup-driven watchdog.
	err, stopFn := watchdog.CgroupDriven(5*time.Second, policy)
	if err == nil {
		log.Infof("initialized cgroup-driven watchdog")
		addStopHook(stopFn)
		return
	}
	log.Warnf("failed to initialize cgroup-driven watchdog; err: %s", err)
	log.Warnf("trying a system-driven watchdog")

	// 3. system-driven watchdog.
	err, stopFn = watchdog.SystemDriven(0, 5*time.Second, policy) // 0 calculates the limit automatically.
	if err == nil {
		log.Infof("initialized system-driven watchdog")
		addStopHook(stopFn)
		return
	}

	// 4. log the failure
	log.Warnf("failed to initialize system-driven watchdog; err: %s", err)
	log.Warnf("system running without a memory watchdog")
}

type JwtPayload struct {
	Allow []auth.Permission
}

func APISecret(keystore types.KeyStore, lr repo.LockedRepo) (*dtypes.APIAlg, error) {
	key, err := keystore.Get(JWTSecretName)

	if errors.Is(err, types.ErrKeyInfoNotFound) {
		log.Warn("Generating new API secret")

		sk, err := io.ReadAll(io.LimitReader(rand.Reader, 32))
		if err != nil {
			return nil, err
		}

		key = types.KeyInfo{
			Type:       KTJwtHmacSecret,
			PrivateKey: sk,
		}

		if err := keystore.Put(JWTSecretName, key); err != nil {
			return nil, xerrors.Errorf("writing API secret: %w", err)
		}

		// TODO: make this configurable
		p := JwtPayload{
			Allow: api.AllPermissions,
		}

		cliToken, err := jwt.Sign(&p, jwt.NewHS256(key.PrivateKey))
		if err != nil {
			return nil, err
		}

		if err := lr.SetAPIToken(cliToken); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, xerrors.Errorf("could not get JWT Token: %w", err)
	}

	return (*dtypes.APIAlg)(jwt.NewHS256(key.PrivateKey)), nil
}

func ConfigBootstrap(peers []string) func() (dtypes.BootstrapPeers, error) {
	return func() (dtypes.BootstrapPeers, error) {
		return addrutil.ParseAddresses(context.TODO(), peers)
	}
}

func BuiltinBootstrap() (dtypes.BootstrapPeers, error) {
	return build.BuiltinBootstrap()
}

func DrandBootstrap(ds dtypes.DrandSchedule) (dtypes.DrandBootstrap, error) {
	// TODO: retry resolving, don't fail if at least one resolve succeeds
	var res []peer.AddrInfo
	for _, d := range ds {
		addrs, err := addrutil.ParseAddresses(context.TODO(), d.Config.Relays)
		if err != nil {
			log.Errorf("resolving drand relays addresses: %+v", err)
			continue
		}
		res = append(res, addrs...)
	}
	return res, nil
}

func NewDefaultMaxFeeFunc(r repo.LockedRepo) dtypes.DefaultMaxFeeFunc {
	return func() (out abi.TokenAmount, err error) {
		err = readNodeCfg(r, func(cfg *config.FullNode) {
			out = abi.TokenAmount(cfg.Fees.DefaultMaxFee)
		})
		return
	}
}

func readNodeCfg(r repo.LockedRepo, accessor func(node *config.FullNode)) error {
	raw, err := r.Config()
	if err != nil {
		return err
	}

	cfg, ok := raw.(*config.FullNode)
	if !ok {
		return xerrors.New("expected config.FullNode")
	}

	accessor(cfg)

	return nil
}
