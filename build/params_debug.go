// +build debug

package build

import "os"

// Seconds
const BlockDelay = 4

// Blocks
const ProvingPeriodDuration uint64 = 40

func init() {
	os.Setenv("TRUST_PARAMS", "1")
}
