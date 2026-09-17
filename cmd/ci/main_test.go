package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileCallsFunction(t *testing.T) {
	dir := t.TempDir()

	commentOnly := filepath.Join(dir, "comment_only_test.go")
	if err := os.WriteFile(commentOnly, []byte(`package main

// kit.RealProofs()
func TestCommentOnly(t *testing.T) {}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if fileCallsFunction(commentOnly, "RealProofs") {
		t.Fatal("comment-only RealProofs marker should not match")
	}

	withCall := filepath.Join(dir, "with_call_test.go")
	if err := os.WriteFile(withCall, []byte(`package main

func TestWithCall(t *testing.T) {
	_ = kit.RealProofs()
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if !fileCallsFunction(withCall, "RealProofs") {
		t.Fatal("RealProofs call should match")
	}
}

func TestGetNeedsParametersScansTestFiles(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(wd, "..", ".."))

	for _, tc := range []struct {
		name string
		want bool
	}{
		{name: "itest-api", want: true},
		{name: "itest-api_v2", want: false},
		{name: "itest-direct_data_onboard", want: true},
		{name: "itest-direct_data_onboard_verified", want: false},
		{name: "unit-cli", want: false},
		{name: "unit-storage", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := getNeedsParameters(tc.name); got != tc.want {
				t.Fatalf("getNeedsParameters(%q) = %t, want %t", tc.name, got, tc.want)
			}
		})
	}
}
