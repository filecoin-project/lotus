package modules

import (
	"net"
	"os"
	"strconv"
	"syscall"

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

func CheckUDPBufferSize(wanted int) func(al *alerting.Alerting) {
	return func(al *alerting.Alerting) {
		conn, err := net.Dial("udp", "localhost:0")
		if err != nil {
			alert := al.AddAlertType("process", "udp-buffer-size")
			al.Raise(alert, map[string]string{
				"message": "Failed to create UDP connection",
				"error":   err.Error(),
			})
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				log.Warnf("Failed to close connection: %s", err)
			}
		}()

		udpConn, ok := conn.(*net.UDPConn)
		if !ok {
			alert := al.AddAlertType("process", "udp-buffer-size")
			al.Raise(alert, map[string]string{
				"message": "Failed to cast connection to UDPConn",
			})
			return
		}

		file, err := udpConn.File()
		if err != nil {
			alert := al.AddAlertType("process", "udp-buffer-size")
			al.Raise(alert, map[string]string{
				"message": "Failed to get file descriptor from UDPConn",
				"error":   err.Error(),
			})
			return
		}
		defer func() {
			if err := file.Close(); err != nil {
				log.Warnf("Failed to close file: %s", err)
			}
		}()

		size, err := syscall.GetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
		if err != nil {
			alert := al.AddAlertType("process", "udp-buffer-size")
			al.Raise(alert, map[string]string{
				"message": "Failed to get UDP buffer size",
				"error":   err.Error(),
			})
			return
		}

		if size < wanted {
			alert := al.AddAlertType("process", "udp-buffer-size")
			al.Raise(alert, map[string]interface{}{
				"message":      "UDP buffer size is low",
				"current_size": size,
				"wanted_size":  wanted,
				"help":         "See https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes for details.",
			})
		}
	}
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
