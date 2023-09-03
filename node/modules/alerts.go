package modules

import (
	"os"
	"strconv"

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

func LegacyMarketsEOL(al *alerting.Alerting) {
	// Add alert if lotus-miner legacy markets subsystem is still in use
	alert := al.AddAlertType("system", "EOL")

	// Alert with a message to migrate to Boost or similar markets subsystems
	al.Raise(alert, map[string]string{
		"message": "The lotus-miner legacy markets subsystem is deprecated and will be removed in a future release. Please migrate to [Boost](https://boost.filecoin.io) or similar markets subsystems.",
	})
}

func CheckFvmConcurrency() func(al *alerting.Alerting) {
	return func(al *alerting.Alerting) {
		fvmConcurrency, ok := os.LookupEnv("LOTUS_FVM_CONCURRENCY")
		if !ok {
			return
		}

		fvmConcurrencyVal, err := strconv.Atoi(fvmConcurrency)
		if err != nil {
			alert := al.AddAlertType("process", "fvm-concurrency")
			al.Raise(alert, map[string]string{
				"message": "LOTUS_FVM_CONCURRENCY is not an integer",
				"error":   err.Error(),
			})
			return
		}

		// Raise alert if LOTUS_FVM_CONCURRENCY is set to a high value
		if fvmConcurrencyVal > 24 {
			alert := al.AddAlertType("process", "fvm-concurrency")
			al.Raise(alert, map[string]interface{}{
				"message":     "LOTUS_FVM_CONCURRENCY is set to a high value that can cause chain sync panics on network migrations/upgrades",
				"set_value":   fvmConcurrencyVal,
				"recommended": "24 or less during network upgrades",
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
