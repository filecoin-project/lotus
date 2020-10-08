package conformance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
)

// ExecuteMessageVector executes a message-class test vector.
func ExecuteMessageVector(r Reporter, vector *schema.TestVector) {
	var (
		ctx   = context.Background()
		epoch = vector.Pre.Epoch
		root  = vector.Pre.StateTree.RootCID
	)

	// Load the CAR into a new temporary Blockstore.
	bs, err := LoadVectorCAR(vector.CAR)
	if err != nil {
		r.Fatalf("failed to load the vector CAR: %w", err)
	}

	// Create a new Driver.
	driver := NewDriver(ctx, vector.Selector, DriverOpts{DisableVMFlush: true})

	// Apply every message.
	for i, m := range vector.ApplyMessages {
		msg, err := types.DecodeMessage(m.Bytes)
		if err != nil {
			r.Fatalf("failed to deserialize message: %s", err)
		}

		// add an epoch if one's set.
		if m.Epoch != nil {
			epoch = *m.Epoch
		}

		// Execute the message.
		var ret *vm.ApplyRet
		ret, root, err = driver.ExecuteMessage(bs, ExecuteMessageParams{
			Preroot:    root,
			Epoch:      abi.ChainEpoch(epoch),
			Message:    msg,
			BaseFee:    BaseFeeOrDefault(vector.Pre.BaseFee),
			CircSupply: CircSupplyOrDefault(vector.Pre.CircSupply),
			Rand:       NewReplayingRand(r, vector.Randomness),
		})
		if err != nil {
			r.Fatalf("fatal failure when executing message: %s", err)
		}

		// Assert that the receipt matches what the test vector expects.
		AssertMsgResult(r, vector.Post.Receipts[i], ret, strconv.Itoa(i))
	}

	// Once all messages are applied, assert that the final state root matches
	// the expected postcondition root.
	if expected, actual := vector.Post.StateTree.RootCID, root; expected != actual {
		r.Errorf("wrong post root cid; expected %v, but got %v", expected, actual)
		dumpThreeWayStateDiff(r, vector, bs, root)
		r.FailNow()
	}
}

// ExecuteTipsetVector executes a tipset-class test vector.
func ExecuteTipsetVector(r Reporter, vector *schema.TestVector) {
	var (
		ctx       = context.Background()
		prevEpoch = vector.Pre.Epoch
		root      = vector.Pre.StateTree.RootCID
		tmpds     = ds.NewMapDatastore()
	)

	// Load the vector CAR into a new temporary Blockstore.
	bs, err := LoadVectorCAR(vector.CAR)
	if err != nil {
		r.Fatalf("failed to load the vector CAR: %w", err)
	}

	// Create a new Driver.
	driver := NewDriver(ctx, vector.Selector, DriverOpts{})

	// Apply every tipset.
	var receiptsIdx int
	for i, ts := range vector.ApplyTipsets {
		ts := ts // capture
		ret, err := driver.ExecuteTipset(bs, tmpds, root, abi.ChainEpoch(prevEpoch), &ts)
		if err != nil {
			r.Fatalf("failed to apply tipset %d message: %s", i, err)
		}

		for j, v := range ret.AppliedResults {
			AssertMsgResult(r, vector.Post.Receipts[receiptsIdx], v, fmt.Sprintf("%d of tipset %d", j, i))
			receiptsIdx++
		}

		// Compare the receipts root.
		if expected, actual := vector.Post.ReceiptsRoots[i], ret.ReceiptsRoot; expected != actual {
			r.Errorf("post receipts root doesn't match; expected: %s, was: %s", expected, actual)
		}

		prevEpoch = ts.Epoch
		root = ret.PostStateRoot
	}

	// Once all messages are applied, assert that the final state root matches
	// the expected postcondition root.
	if expected, actual := vector.Post.StateTree.RootCID, root; expected != actual {
		r.Errorf("wrong post root cid; expected %v, but got %v", expected, actual)
		dumpThreeWayStateDiff(r, vector, bs, root)
		r.FailNow()
	}
}

// AssertMsgResult compares a message result. It takes the expected receipt
// encoded in the vector, the actual receipt returned by Lotus, and a message
// label to log in the assertion failure message to facilitate debugging.
func AssertMsgResult(r Reporter, expected *schema.Receipt, actual *vm.ApplyRet, label string) {
	r.Helper()

	if expected, actual := exitcode.ExitCode(expected.ExitCode), actual.ExitCode; expected != actual {
		r.Errorf("exit code of msg %s did not match; expected: %s, got: %s", label, expected, actual)
	}
	if expected, actual := expected.GasUsed, actual.GasUsed; expected != actual {
		r.Errorf("gas used of msg %s did not match; expected: %d, got: %d", label, expected, actual)
	}
	if expected, actual := []byte(expected.ReturnValue), actual.Return; !bytes.Equal(expected, actual) {
		r.Errorf("return value of msg %s did not match; expected: %s, got: %s", label, base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(actual))
	}
}

func dumpThreeWayStateDiff(r Reporter, vector *schema.TestVector, bs blockstore.Blockstore, actual cid.Cid) {
	// check if statediff exists; if not, skip.
	if err := exec.Command("statediff", "--help").Run(); err != nil {
		r.Log("could not dump 3-way state tree diff upon test failure: statediff command not found")
		r.Log("install statediff with:")
		r.Log("$ git clone https://github.com/filecoin-project/statediff.git")
		r.Log("$ cd statediff")
		r.Log("$ go generate ./...")
		r.Log("$ go install ./cmd/statediff")
		return
	}

	tmpCar, err := writeStateToTempCAR(bs,
		vector.Pre.StateTree.RootCID,
		vector.Post.StateTree.RootCID,
		actual,
	)
	if err != nil {
		r.Fatalf("failed to write temporary state CAR: %s", err)
	}
	defer os.RemoveAll(tmpCar) //nolint:errcheck

	color.NoColor = false // enable colouring.

	var (
		a  = color.New(color.FgMagenta, color.Bold).Sprint("(A) expected final state")
		b  = color.New(color.FgYellow, color.Bold).Sprint("(B) actual final state")
		c  = color.New(color.FgCyan, color.Bold).Sprint("(C) initial state")
		d1 = color.New(color.FgGreen, color.Bold).Sprint("[Δ1]")
		d2 = color.New(color.FgGreen, color.Bold).Sprint("[Δ2]")
		d3 = color.New(color.FgGreen, color.Bold).Sprint("[Δ3]")
	)

	printDiff := func(left, right cid.Cid) {
		cmd := exec.Command("statediff", "car", "--file", tmpCar, left.String(), right.String())
		b, err := cmd.CombinedOutput()
		if err != nil {
			r.Fatalf("statediff failed: %s", err)
		}
		r.Log(string(b))
	}

	bold := color.New(color.Bold).SprintfFunc()

	// run state diffs.
	r.Log(bold("=== dumping 3-way diffs between %s, %s, %s ===", a, b, c))

	r.Log(bold("--- %s left: %s; right: %s ---", d1, a, b))
	printDiff(vector.Post.StateTree.RootCID, actual)

	r.Log(bold("--- %s left: %s; right: %s ---", d2, c, b))
	printDiff(vector.Pre.StateTree.RootCID, actual)

	r.Log(bold("--- %s left: %s; right: %s ---", d3, c, a))
	printDiff(vector.Pre.StateTree.RootCID, vector.Post.StateTree.RootCID)
}

// writeStateToTempCAR writes the provided roots to a temporary CAR that'll be
// cleaned up via t.Cleanup(). It returns the full path of the temp file.
func writeStateToTempCAR(bs blockstore.Blockstore, roots ...cid.Cid) (string, error) {
	tmp, err := ioutil.TempFile("", "lotus-tests-*.car")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file to dump CAR for diffing: %w", err)
	}

	carWalkFn := func(nd format.Node) (out []*format.Link, err error) {
		for _, link := range nd.Links() {
			if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
				continue
			}
			// ignore things we don't have, the state tree is incomplete.
			if has, err := bs.Has(link.Cid); err != nil {
				return nil, err
			} else if has {
				out = append(out, link)
			}
		}
		return out, nil
	}

	var (
		offl    = offline.Exchange(bs)
		blkserv = blockservice.New(bs, offl)
		dserv   = merkledag.NewDAGService(blkserv)
	)

	err = car.WriteCarWithWalker(context.Background(), dserv, roots, tmp, carWalkFn)
	if err != nil {
		return "", fmt.Errorf("failed to dump CAR for diffing: %w", err)
	}
	_ = tmp.Close()
	return tmp.Name(), nil
}

func LoadVectorCAR(vectorCAR schema.Base64EncodedBytes) (blockstore.Blockstore, error) {
	bs := blockstore.NewTemporary()

	// Read the base64-encoded CAR from the vector, and inflate the gzip.
	buf := bytes.NewReader(vectorCAR)
	r, err := gzip.NewReader(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to inflate gzipped CAR: %s", err)
	}
	defer r.Close() // nolint

	// Load the CAR embedded in the test vector into the Blockstore.
	_, err = car.LoadCar(bs, r)
	if err != nil {
		return nil, fmt.Errorf("failed to load state tree car from test vector: %s", err)
	}
	return bs, nil
}
