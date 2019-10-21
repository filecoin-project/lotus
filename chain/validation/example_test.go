package validation

import (
	"testing"

	"github.com/filecoin-project/chain-validation/pkg/suites"
)

func TestExample(t *testing.T) {
	driver := NewDriver()
	suites.Example(t, driver)
}
