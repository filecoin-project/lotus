package test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
)

// APIBuilder is a function which is invoked in test suite to provide
// test nodes and networks
type APIBuilder func() api.API
type testSuite struct {
	makeNode APIBuilder
}

// TestApis is the entry point to API test suite
func TestApis(t *testing.T, b APIBuilder) {
	ts := testSuite{
		makeNode: b,
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
