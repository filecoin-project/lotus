package cli

import (
	logging "github.com/ipfs/go-log/v2"
)

func init() {
	_ = logging.SetLogLevel("watchdog", "ERROR")
}
