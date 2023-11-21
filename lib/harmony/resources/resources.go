package resources

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"sync/atomic"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/pbnjay/memory"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

var LOOKS_DEAD_TIMEOUT = 10 * time.Minute // Time w/o minute heartbeats

type Resources struct {
	Cpu       int
	Gpu       float64
	Ram       uint64
	MachineID int
}
type Reg struct {
	Resources
	shutdown atomic.Bool
}

var logger = logging.Logger("harmonytask")

var lotusRE = regexp.MustCompile("lotus-worker|lotus-harmony|yugabyted|yb-master|yb-tserver")

func Register(db *harmonydb.DB, hostnameAndPort string) (*Reg, error) {
	var reg Reg
	var err error
	reg.Resources, err = getResources()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	{ // Learn our owner_id while updating harmony_machines
		var ownerID *int

		// Upsert query with last_contact update, fetch the machine ID
		// (note this isn't a simple insert .. on conflict because host_and_port isn't unique)
		err := db.QueryRow(ctx, `
			WITH upsert AS (
				UPDATE harmony_machines
				SET cpu = $2, ram = $3, gpu = $4, last_contact = CURRENT_TIMESTAMP
				WHERE host_and_port = $1
				RETURNING id
			),
			inserted AS (
				INSERT INTO harmony_machines (host_and_port, cpu, ram, gpu, last_contact)
				SELECT $1, $2, $3, $4, CURRENT_TIMESTAMP
				WHERE NOT EXISTS (SELECT id FROM upsert)
				RETURNING id
			)
			SELECT id FROM upsert
			UNION ALL
			SELECT id FROM inserted;
		`, hostnameAndPort, reg.Cpu, reg.Ram, reg.Gpu).Scan(&ownerID)
		if err != nil {
			return nil, xerrors.Errorf("inserting machine entry: %w", err)
		}
		if ownerID == nil {
			return nil, xerrors.Errorf("no owner id")
		}

		reg.MachineID = *ownerID

		cleaned := CleanupMachines(context.Background(), db)
		logger.Infow("Cleaned up machines", "count", cleaned)
	}
	go func() {
		for {
			time.Sleep(time.Minute)
			if reg.shutdown.Load() {
				return
			}
			_, err := db.Exec(ctx, `UPDATE harmony_machines SET last_contact=CURRENT_TIMESTAMP`)
			if err != nil {
				logger.Error("Cannot keepalive ", err)
			}
		}
	}()

	return &reg, nil
}

func CleanupMachines(ctx context.Context, db *harmonydb.DB) int {
	ct, err := db.Exec(ctx,
		`DELETE FROM harmony_machines WHERE last_contact < CURRENT_TIMESTAMP - INTERVAL '1 MILLISECOND' * $1 `,
		LOOKS_DEAD_TIMEOUT.Milliseconds()) // ms enables unit testing to change timeout.
	if err != nil {
		logger.Warn("unable to delete old machines: ", err)
	}
	return ct
}

func (res *Reg) Shutdown() {
	res.shutdown.Store(true)
}

func getResources() (res Resources, err error) {
	b, err := exec.Command(`ps`, `-ef`).CombinedOutput()
	if err != nil {
		logger.Warn("Could not safety check for 2+ processes: ", err)
	} else {
		found := 0
		for _, b := range bytes.Split(b, []byte("\n")) {
			if lotusRE.Match(b) {
				found++
			}
		}
		if found > 1 {
			logger.Warn("lotus-provider's defaults are for running alone. Use task maximums or CGroups.")
		}
	}

	res = Resources{
		Cpu: runtime.NumCPU(),
		Ram: memory.FreeMemory(),
		Gpu: getGPUDevices(),
	}

	return res, nil
}

func DiskFree(path string) (uint64, error) {
	s := unix.Statfs_t{}
	err := unix.Statfs(path, &s)
	if err != nil {
		return 0, err
	}

	return s.Bfree * uint64(s.Bsize), nil
}
