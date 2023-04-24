package shared

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
)

// Deprecated: AllSelector is a compatibility alias for an entire DAG non-matching-selector.
// Use github.com/ipld/go-ipld-prime/traversal/selector/parse.CommonSelector_ExploreAllRecursively instead.
func AllSelector() datamodel.Node { return selectorparse.CommonSelector_ExploreAllRecursively }
