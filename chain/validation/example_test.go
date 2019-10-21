package validation

import (
	"testing"

	"github.com/filecoin-project/chain-validation/pkg/suites"
)

func TestExample(t *testing.T) {
	factories := NewFactories()
	suites.Example(t, factories)
}
