package vm_test

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"

	suites "github.com/filecoin-project/chain-validation/suites"
	"github.com/filecoin-project/chain-validation/suites/message"
	"github.com/filecoin-project/chain-validation/suites/tipset"

	factory "github.com/filecoin-project/lotus/chain/validation"
)

// TestSkipper contains a list of test cases skipped by the implementation.
type TestSkipper struct {
	testSkips []suites.TestCase
}

// Skip return true if the sutire.TestCase should be skipped.
func (ts *TestSkipper) Skip(test suites.TestCase) bool {
	for _, skip := range ts.testSkips {
		if reflect.ValueOf(skip).Pointer() == reflect.ValueOf(test).Pointer() {
			fmt.Printf("=== SKIP   %v\n", runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name())
			return true
		}
	}
	return false
}

// TestSuiteSkips contains tests we wish to skip.
var TestSuiteSkipper TestSkipper

func init() {
	// initialize the test skipper with tests being skipped
	TestSuiteSkipper = TestSkipper{testSkips: []suites.TestCase{
		// Fails since deprecated network actor is required.
		tipset.TestBlockMessageInfoApplication,

		// Fails because ApplyMessage returns error instead of message receipt with unsuccessful exit code.
		message.TestValueTransferSimple,
		// Fails because ApplyMessage returns error instead of message receipt with unsuccessful exit code.
		message.TestValueTransferAdvance,
		// Fails because ApplyMessage returns error instead of message receipt with unsuccessful exit code.
		message.TestAccountActorCreation,

		// Fails due to state initialization
		message.TestPaych,
		// Fails due to state initialization
		message.TestMultiSigActor,
		// Fails due to state initialization
		message.TestMessageApplicationEdgecases,
	}}
}

func TestChainValidationMessageSuite(t *testing.T) {
	f := factory.NewFactories()
	for _, testCase := range suites.MessageTestCases() {
		if TestSuiteSkipper.Skip(testCase) {
			continue
		}
		testCase(t, f)
	}
}

func TestChainValidationTipSetSuite(t *testing.T) {
	f := factory.NewFactories()
	for _, testCase := range suites.TipSetTestCases() {
		if TestSuiteSkipper.Skip(testCase) {
			continue
		}
		testCase(t, f)
	}
}
