package conformance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"

	"github.com/filecoin-project/statediff"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/fatih/color"
	"github.com/ipld/go-car"
)

const (
	// EnvSkipConformance, if 1, skips the conformance test suite.
	EnvSkipConformance = "SKIP_CONFORMANCE"

	// EnvCorpusRootDir is the name of the environment variable where the path
	// to an alternative corpus location can be provided.
	//
	// The default is defaultCorpusRoot.
	EnvCorpusRootDir = "CORPUS_DIR"

	// defaultCorpusRoot is the directory where the test vector corpus is hosted.
	// It is mounted on the Lotus repo as a git submodule.
	//
	// When running this test, the corpus root can be overridden through the
	// -conformance.corpus CLI flag to run an alternate corpus.
	defaultCorpusRoot = "../extern/test-vectors/corpus"
)

// ignore is a set of paths relative to root to skip.
var ignore = map[string]struct{}{
	".git":        {},
	"schema.json": {},
}

// TestConformance is the entrypoint test that runs all test vectors found
// in the corpus root directory.
//
// It locates all json files via a recursive walk, skipping over the ignore set,
// as well as files beginning with _. It parses each file as a test vector, and
// runs it via the Driver.
func TestConformance(t *testing.T) {
	if skip := strings.TrimSpace(os.Getenv(EnvSkipConformance)); skip == "1" {
		t.SkipNow()
	}
	// corpusRoot is the effective corpus root path, taken from the `-conformance.corpus` CLI flag,
	// falling back to defaultCorpusRoot if not provided.
	corpusRoot := defaultCorpusRoot
	if dir := strings.TrimSpace(os.Getenv(EnvCorpusRootDir)); dir != "" {
		corpusRoot = dir
	}

	var vectors []string
	err := filepath.Walk(corpusRoot+"/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}

		filename := filepath.Base(path)
		rel, err := filepath.Rel(corpusRoot, path)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := ignore[rel]; ok {
			// skip over using the right error.
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			// dive into directories.
			return nil
		}
		if filepath.Ext(path) != ".json" {
			// skip if not .json.
			return nil
		}
		if ignored := strings.HasPrefix(filename, "_"); ignored {
			// ignore files starting with _.
			t.Logf("ignoring: %s", rel)
			return nil
		}
		vectors = append(vectors, rel)
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(vectors) == 0 {
		t.Fatalf("no test vectors found")
	}

	// Run a test for each vector.
	for _, v := range vectors {
		path := filepath.Join(corpusRoot, v)
		raw, err := ioutil.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read test raw file: %s", path)
		}

		var vector schema.TestVector
		err = json.Unmarshal(raw, &vector)
		if err != nil {
			t.Errorf("failed to parse test vector %s: %s; skipping", path, err)
			continue
		}

		t.Run(v, func(t *testing.T) {
			for _, h := range vector.Hints {
				if h == schema.HintIncorrect {
					t.Logf("skipping vector marked as incorrect: %s", vector.Meta.ID)
					t.SkipNow()
				}
			}

			// dispatch the execution depending on the vector class.
			switch vector.Class {
			case "message":
				executeMessageVector(t, &vector)
			default:
				t.Fatalf("test vector class not supported: %s", vector.Class)
			}
		})
	}
}

// executeMessageVector executes a message-class test vector.
func executeMessageVector(t *testing.T, vector *schema.TestVector) {
	var (
		ctx   = context.Background()
		epoch = vector.Pre.Epoch
		root  = vector.Pre.StateTree.RootCID
	)

	bs := blockstore.NewTemporary()

	// Read the base64-encoded CAR from the vector, and inflate the gzip.
	buf := bytes.NewReader(vector.CAR)
	r, err := gzip.NewReader(buf)
	if err != nil {
		t.Fatalf("failed to inflate gzipped CAR: %s", err)
	}
	defer r.Close() // nolint

	// Load the CAR embedded in the test vector into the Blockstore.
	_, err = car.LoadCar(bs, r)
	if err != nil {
		t.Fatalf("failed to load state tree car from test vector: %s", err)
	}

	// Create a new Driver.
	driver := NewDriver(ctx, vector)

	// Apply every message.
	for i, m := range vector.ApplyMessages {
		msg, err := types.DecodeMessage(m.Bytes)
		if err != nil {
			t.Fatalf("failed to deserialize message: %s", err)
		}

		// add an epoch if one's set.
		if m.Epoch != nil {
			epoch = *m.Epoch
		}

		// Execute the message.
		var ret *vm.ApplyRet
		ret, root, err = driver.ExecuteMessage(msg, root, bs, epoch)
		if err != nil {
			t.Fatalf("fatal failure when executing message: %s", err)
		}

		// Assert that the receipt matches what the test vector expects.
		receipt := vector.Post.Receipts[i]
		if expected, actual := receipt.ExitCode, ret.ExitCode; expected != actual {
			t.Errorf("exit code of msg %d did not match; expected: %s, got: %s", i, expected, actual)
		}
		if expected, actual := receipt.GasUsed, ret.GasUsed; expected != actual {
			t.Errorf("gas used of msg %d did not match; expected: %d, got: %d", i, expected, actual)
		}
		if expected, actual := []byte(receipt.ReturnValue), ret.Return; !bytes.Equal(expected, actual) {
			t.Errorf("return value of msg %d did not match; expected: %s, got: %s", i, base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(actual))
		}
	}

	// Once all messages are applied, assert that the final state root matches
	// the expected postcondition root.
	if root != vector.Post.StateTree.RootCID {
		color.NoColor = false // enable colouring.

		t.Errorf("wrong post root cid; expected %v, but got %v", vector.Post.StateTree.RootCID, root)

		var (
			a  = color.New(color.FgMagenta, color.Bold).Sprint("(A) expected final state")
			b  = color.New(color.FgYellow, color.Bold).Sprint("(B) actual final state")
			c  = color.New(color.FgCyan, color.Bold).Sprint("(C) initial state")
			d1 = color.New(color.FgGreen, color.Bold).Sprint("[Δ1]")
			d2 = color.New(color.FgGreen, color.Bold).Sprint("[Δ2]")
			d3 = color.New(color.FgGreen, color.Bold).Sprint("[Δ3]")
		)

		bold := color.New(color.Bold).SprintfFunc()

		// run state diffs.
		t.Log(bold("=== dumping 3-way diffs between %s, %s, %s ===", a, b, c))

		t.Log(bold("--- %s left: %s; right: %s ---", d1, a, b))
		t.Log(statediff.Diff(context.Background(), bs, vector.Post.StateTree.RootCID, root))

		t.Log(bold("--- %s left: %s; right: %s ---", d2, c, b))
		t.Log(statediff.Diff(context.Background(), bs, vector.Pre.StateTree.RootCID, root))

		t.Log(bold("--- %s left: %s; right: %s ---", d3, c, a))
		t.Log(statediff.Diff(context.Background(), bs, vector.Pre.StateTree.RootCID, vector.Post.StateTree.RootCID))
	}
}
