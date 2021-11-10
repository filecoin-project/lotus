package modules

import (
	"github.com/filecoin-project/lotus/journal/alerting"
	"github.com/filecoin-project/lotus/lib/ulimit"
)

func CheckFdLimit(min uint64) func(al *alerting.Alerting) {
	return func(al *alerting.Alerting) {
		soft, _, err := ulimit.GetLimit()

		if err == ulimit.ErrUnsupported {
			log.Warn("FD limit monitoring not available")
			return
		}

		alert := al.AddAlertType("process", "fd-limit")
		if err != nil {
			al.Raise(alert, map[string]string{
				"message": "failed to get FD limit",
				"error":   err.Error(),
			})
		}

		if soft < min {
			al.Raise(alert, map[string]interface{}{
				"message":         "soft FD limit is low",
				"soft_limit":      soft,
				"recommended_min": min,
			})
		}
	}
}

// TODO: More things:
//  * Space in repo dirs (taking into account mounts)
//  * Miner
//    * Faulted partitions
//    * Low balances
//  * Market provider
//    * Reachability
//    * on-chain config
//  * Low memory (maybe)
//  * Network / sync issues
