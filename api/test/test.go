package test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
)

type NodeBuilder func() api.API
type testSuite struct {
	makeNode NodeBuilder
}

func TestApis(t *testing.T, nb NodeBuilder) {
	ts := testSuite{
		makeNode: nb,
	}

	t.Run("version", ts.testVersion)
}

func (ts *testSuite) testVersion(t *testing.T) {
	ctx := context.Background()
	fc := ts.makeNode()

	v, err := fc.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != build.Version {
		t.Error("Version didn't work properly")
	}
}
