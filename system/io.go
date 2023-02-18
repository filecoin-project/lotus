package system

import (
	"os"
	"strings"
)

var BadgerFsyncDisable bool

func init() {
	//
	// Tri-state environment variable LOTUS_CHAIN_BADGERSTORE_DISABLE_FSYNC
	// - unset == the default (currently fsync enabled)
	// - set with a false-y value == fsync enabled no matter what a future default is
	// - set with any other value == fsync is disabled ignored defaults (recommended for day-to-day use)
	//
	if nosyncBs, nosyncBsSet := os.LookupEnv("LOTUS_CHAIN_BADGERSTORE_DISABLE_FSYNC"); nosyncBsSet {
		nosyncBs = strings.ToLower(nosyncBs)
		if nosyncBs == "" || nosyncBs == "0" || nosyncBs == "false" || nosyncBs == "no" {
			BadgerFsyncDisable = false
		} else {
			BadgerFsyncDisable = true
		}
	}
}
