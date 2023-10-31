package resources

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sync/atomic"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/pbnjay/memory"
	"golang.org/x/sys/unix"

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
		var ownerID []int
		err := db.Select(ctx, &ownerID, `SELECT id FROM harmony_machines WHERE host_and_port=$1`, hostnameAndPort)
		if err != nil {
			return nil, fmt.Errorf("could not read from harmony_machines: %w", err)
		}
		if len(ownerID) == 0 {
			err = db.QueryRow(ctx, `INSERT INTO harmony_machines 
		(host_and_port, cpu, ram, gpu, gpuram) VALUES
		($1,$2,$3,$4,0) RETURNING id`,
				hostnameAndPort, reg.Cpu, reg.Ram, reg.Gpu).Scan(&reg.Resources.MachineID)
			if err != nil {
				return nil, err
			}

		} else {
			reg.MachineID = ownerID[0]
			_, err := db.Exec(ctx, `UPDATE harmony_machines SET
		   cpu=$1, ram=$2, gpu=$3 WHERE id=$5`,
				reg.Cpu, reg.Ram, reg.Gpu, reg.Resources.MachineID)
			if err != nil {
				return nil, err
			}
		}
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
	ct, err := db.Exec(ctx, `DELETE FROM harmony_machines WHERE last_contact < $1`,
		time.Now().Add(-1*LOOKS_DEAD_TIMEOUT))
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
