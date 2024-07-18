package kit

import (
	"os"
	"testing"
)

// EnvRunExpensiveTests is the environment variable that needs to be present
// and set to value "1" to enable running expensive tests outside of CI.
const EnvRunExpensiveTests = "LOTUS_RUN_EXPENSIVE_TESTS"

// EnvRunVeryExpensiveTests is the environment variable that needs to be present
// and set to value "1" to enable running very expensive tests outside of CI.
// A "very expensive" test is one that is expected to take too long to run in
// a standard CI setup, and should be skipped unless explicitly enabled.
const EnvRunVeryExpensiveTests = "LOTUS_RUN_VERY_EXPENSIVE_TESTS"

// Expensive marks a test as expensive, skipping it immediately if not running in a CI environment
// or if the LOTUS_RUN_EXPENSIVE_TESTS environment variable is not set to "1".
func Expensive(t *testing.T) {
	switch {
	case os.Getenv("CI") == "true":
		return
	case os.Getenv(EnvRunExpensiveTests) != "1":
		t.Skipf("skipping expensive test outside of CI; enable by setting env var %s=1", EnvRunExpensiveTests)
	}
}

// VeryExpensive marks a test as VERY expensive, skipping it immediately if the
// LOTUS_RUN_VERY_EXPENSIVE_TESTS environment variable is not set to "1".
func VeryExpensive(t *testing.T) {
	if os.Getenv(EnvRunVeryExpensiveTests) != "1" {
		t.Skipf("skipping VERY expensive test outside of CI; enable by setting env var %s=1", EnvRunVeryExpensiveTests)
	}
}
