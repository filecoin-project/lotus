package taskhelp

import (
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"

	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
)

var logger = logging.Logger("networkoptimizer")

// SubsetIf returns a subset of the slice for which the predicate is true.
// It does not allocate memory, but rearranges the list in place.
// A non-zero list input will always return a non-zero list.
// The return value is the subset and a boolean indicating whether the subset was sliced.
func SliceIfFound[T any](slice []T, f func(T) bool) ([]T, bool) {
	ct := 0
	for i, v := range slice {
		if f(v) {
			slice[ct], slice[i] = slice[i], slice[ct]
			ct++
		}
	}
	if ct == 0 {
		return slice, false
	}
	return slice[:ct], true
}

// NetworkOptimizer is a helper to decide if we should (CanAccept() true) a task
// even when we don't have the best conditions.
// It is used to optimize the network by making sure that the task is done
// by the most capable party.
//
// USAGE EXAMPLE:    (A job that gets data from Boost)
//
// n := &taskhelp.NetworkOptimizer{DB: db, TaskID: taskID}
// n.Prefer(hasLocalStorage, boostNodeIsOnMyIP,  boostNodeIsOnMySubnet) // Bools calculated elsewhere
// n.PreferFunc(diskIoQueueAvg1mUnder2Func, storageUnder70PercentFunc) // on-demand
// if !n.IsTimeToAct() { return nil} else {return taskID}
type NetworkOptimizer struct {
	// How long should we wait for other a hero to appear?
	*harmonydb.DB
	TaskID    harmonytask.TaskID
	boolFuncs []func() bool

	count int
	total int
}

// Prefer collects if all conditions are met.
func (n *NetworkOptimizer) Prefer(wanted ...bool) {
	n.count = lo.Reduce(wanted, func(a int, b bool, _ int) int {
		if b {
			a++
		}
		return a
	}, 0)
	n.total += len(wanted)
}

// Prefer collects if all conditions are met.
func (n *NetworkOptimizer) PreferFunc(wanted ...func() bool) {
	n.boolFuncs = append(n.boolFuncs, wanted...)
}

var minWait = 3 * harmonytask.POLL_DURATION
var fallbackWindow = 4 * harmonytask.POLL_DURATION

// IsTimeToAct returns true if the conditions are all met or after a certain time.
func (n *NetworkOptimizer) IsTimeToAct() bool {
	boolFuncsTrue := 0
	if n.count == n.total { ////////////////////// We are the perfect runner for the job
		if len(n.boolFuncs) > 0 {
			var somethingWasFalse bool
			_, boolFuncsTrue, somethingWasFalse = lo.FindIndexOf(n.boolFuncs, func(f func() bool) bool { return !f() })
			if !somethingWasFalse {
				return true
			}
		} else {
			return true
		}
	}
	var posted time.Time
	err := n.DB.QueryRow(context.Background(),
		"SELECT posted_time FROM harmony_task WHERE id = ?", n.TaskID).Scan(&posted)
	if err != nil {
		logger.Error("Error getting posted time: ", err)
		return false
	}
	taskAge := time.Since(posted)
	if taskAge < minWait { /////////////////////// Waiting for a hero
		return false
	}
	////////////////////////////////////////////// I'm X% better than the minimum.
	stepSz := fallbackWindow / time.Duration(n.total+len(n.boolFuncs))
	// howMuchLonger := taskAge - minimumTimeFallback
	targetAge := minWait + fallbackWindow - stepSz*time.Duration(n.count+boolFuncsTrue)
	if taskAge > targetAge {
		return true
	}

	// the 0th element was tested above
	// Cleverness: Avoid running expensive functions as machine gets busier (because we are a good fit earlier).
	for remaining := boolFuncsTrue + 1; remaining < len(n.boolFuncs); remaining++ {
		if n.boolFuncs[remaining]() {
			targetAge -= stepSz
			if taskAge > targetAge {
				return true
			}
		}
	}
	return false // We wait for a better job runner.
}
