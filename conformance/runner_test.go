package conformance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"

	"github.com/ipld/go-car"
)

const (
	// defaultRoot is default root at where the message vectors are hosted.
	//
	// You can run this test with the -vectors.root flag to execute
	// a custom corpus.
	defaultRoot = "../extern/conformance-vectors"
)

var (
	// root is the effective root path, taken from the `-vectors.root` CLI flag,
	// falling back to defaultRoot if not provided.
	root string
	// ignore is a set of paths relative to root to skip.
	ignore = map[string]struct{}{
		".git":        {},
		"schema.json": {},
	}
)

func init() {
	// read the alternative root from the -vectors.root CLI flag.
	flag.StringVar(&root, "vectors.root", defaultRoot, "root directory containing test vectors")
}

// TestConformance is the entrypoint test that runs all test vectors found
// in the root directory.
//
// It locates all json files via a recursive walk, skipping over the ignore set,
// as well as files beginning with _. It parses each file as a test vector, and
// runs it via the Driver.
func TestConformance(t *testing.T) {
	var vectors []string
	err := filepath.Walk(root+"/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}

		filename := filepath.Base(path)
		rel, err := filepath.Rel(root, path)
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
		v := v
		t.Run(v, func(t *testing.T) {
			path := filepath.Join(root, v)
			raw, err := ioutil.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read test raw file: %s", path)
			}

			var vector TestVector
			err = json.Unmarshal(raw, &vector)
			if err != nil {
				t.Fatalf("failed to parse test raw: %s", err)
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
func executeMessageVector(t *testing.T, vector *TestVector) {
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
	driver := NewDriver(ctx)

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

		var ret *vm.ApplyRet
		ret, root, err = driver.ExecuteMessage(msg, root, bs, epoch)
		if err != nil {
			t.Fatalf("fatal failure when executing message: %s", err)
		}

		receipt := vector.Post.Receipts[i]
		if expected, actual := receipt.ExitCode, ret.ExitCode; expected != actual {
			t.Errorf("exit code of msg %d did not match; expected: %s, got: %s", i, expected, actual)
		}
		if expected, actual := receipt.GasUsed, ret.GasUsed; expected != actual {
			t.Errorf("gas used of msg %d did not match; expected: %d, got: %d", i, expected, actual)
		}
	}
	if root != vector.Post.StateTree.RootCID {
		t.Errorf("wrong post root cid; expected %vector , but got %vector", vector.Post.StateTree.RootCID, root)
	}
}
