package kit

import (
	"os"
	"testing"
)

// EnvRunExpensiveTests is the environment variable that needs to be present
// and set to value "1" to enable running expensive tests outside of CI.
const EnvRunExpensiveTests = "LOTUS_RUN_EXPENSIVE_TESTS"

// Expensive marks a test as expensive, skipping it immediately if not running an
func Expensive(t *testing.T) {
	switch {
	case os.Getenv("CI") == "true":
		return
	case os.Getenv(EnvRunExpensiveTests) != "1":
		t.Skipf("skipping expensive test outside of CI; enable by setting env var %s=1", EnvRunExpensiveTests)
	}
}
