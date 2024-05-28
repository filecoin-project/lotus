package conformance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"

	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/boxo/blockservice"
	offline "github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

// FallbackBlockstoreGetter is a fallback blockstore to use for resolving CIDs
// unknown to the test vector. This is rarely used, usually only needed
// when transplanting vectors across versions. This is an interface tighter
// than ChainModuleAPI. It can be backed by a FullAPI client.
var FallbackBlockstoreGetter interface {
	ChainReadObj(context.Context, cid.Cid) ([]byte, error)
}

var TipsetVectorOpts struct {
	// PipelineBaseFee pipelines the basefee in multi-tipset vectors from one
	// tipset to another. Basefees in the vector are ignored, except for that of
	// the first tipset. UNUSED.
	PipelineBaseFee bool

	// OnTipsetApplied contains callback functions called after a tipset has been
	// applied.
	OnTipsetApplied []func(bs blockstore.Blockstore, params *ExecuteTipsetParams, res *ExecuteTipsetResult)
}

type GasPricingRestoreFn func()

// adjustGasPricing adjusts the global gas price mapping to make sure that the
// gas pricelist for vector's network version is used at the vector's epoch.
// Because it manipulates a global, it returns a function that reverts the
// change. The caller MUST invoke this function or the test vector runner will
// become invalid.
func adjustGasPricing(vectorEpoch abi.ChainEpoch, vectorNv network.Version) GasPricingRestoreFn {
	// Stash the current pricing mapping.
	// Ok to take a reference instead of a copy, because we override the map
	// with a new one below.
	var old = vm.Prices

	// Resolve the epoch at which the vector network version kicks in.
	var epoch abi.ChainEpoch = math.MaxInt64
	if vectorNv == network.Version0 {
		// genesis is not an upgrade.
		epoch = 0
	} else {
		for _, u := range filcns.DefaultUpgradeSchedule() {
			if u.Network == vectorNv {
				epoch = u.Height
				break
			}
		}
	}

	if epoch == math.MaxInt64 {
		panic(fmt.Sprintf("could not resolve network version %d to height", vectorNv))
	}

	// Find the right pricelist for this network version.
	pricelist := vm.PricelistByEpoch(epoch)

	// Override the pricing mapping by setting the relevant pricelist for the
	// network version at the epoch where the vector runs.
	vm.Prices = map[abi.ChainEpoch]vm.Pricelist{
		vectorEpoch: pricelist,
	}

	// Return a function to restore the original mapping.
	return func() {
		vm.Prices = old
	}
}

// ExecuteMessageVector executes a message-class test vector.
func ExecuteMessageVector(r Reporter, vector *schema.TestVector, variant *schema.Variant) (diffs []string, err error) {
	var (
		ctx       = context.Background()
		baseEpoch = abi.ChainEpoch(variant.Epoch)
		nv        = network.Version(variant.NetworkVersion)
		root      = vector.Pre.StateTree.RootCID
	)

	// Load the CAR into a new temporary Blockstore.
	bs, err := LoadBlockstore(vector.CAR)
	if err != nil {
		r.Fatalf("failed to load the vector CAR: %w", err)
	}

	// Create a new Driver.
	driver := NewDriver(ctx, vector.Selector, DriverOpts{DisableVMFlush: true})

	// Monkey patch the gas pricing.
	revertFn := adjustGasPricing(baseEpoch, nv)
	defer revertFn()

	// Apply every message.
	for i, m := range vector.ApplyMessages {
		msg, err := types.DecodeMessage(m.Bytes)
		if err != nil {
			r.Fatalf("failed to deserialize message: %s", err)
		}

		// add the epoch offset if one is set.
		if m.EpochOffset != nil {
			baseEpoch += abi.ChainEpoch(*m.EpochOffset)
		}

		// Execute the message.
		var ret *vm.ApplyRet
		ret, root, err = driver.ExecuteMessage(bs, ExecuteMessageParams{
			Preroot:        root,
			Epoch:          baseEpoch,
			Message:        msg,
			BaseFee:        BaseFeeOrDefault(vector.Pre.BaseFee),
			CircSupply:     CircSupplyOrDefault(vector.Pre.CircSupply),
			Rand:           NewReplayingRand(r, vector.Randomness),
			NetworkVersion: nv,
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
		ierr := fmt.Errorf("wrong post root cid; expected %v, but got %v", expected, actual)
		r.Errorf(ierr.Error())
		err = multierror.Append(err, ierr)
		diffs = dumpThreeWayStateDiff(r, vector, bs, root)
	}
	return diffs, err
}

// ExecuteTipsetVector executes a tipset-class test vector.
func ExecuteTipsetVector(r Reporter, vector *schema.TestVector, variant *schema.Variant) (diffs []string, err error) {
	var (
		ctx       = context.Background()
		baseEpoch = abi.ChainEpoch(variant.Epoch)
		root      = vector.Pre.StateTree.RootCID
		tmpds     = ds.NewMapDatastore()
	)

	// Load the vector CAR into a new temporary Blockstore.
	bs, err := LoadBlockstore(vector.CAR)
	if err != nil {
		r.Fatalf("failed to load the vector CAR: %w", err)
		return nil, err
	}

	// Create a new Driver.
	driver := NewDriver(ctx, vector.Selector, DriverOpts{})

	// Apply every tipset.
	var receiptsIdx int
	var prevEpoch = baseEpoch
	for i, ts := range vector.ApplyTipsets {
		ts := ts // capture
		execEpoch := baseEpoch + abi.ChainEpoch(ts.EpochOffset)
		params := ExecuteTipsetParams{
			Preroot:     root,
			ParentEpoch: prevEpoch,
			Tipset:      &ts,
			ExecEpoch:   execEpoch,
			Rand:        NewReplayingRand(r, vector.Randomness),
		}
		ret, err := driver.ExecuteTipset(bs, tmpds, params)
		if err != nil {
			r.Fatalf("failed to apply tipset %d: %s", i, err)
			return nil, err
		}

		// invoke callbacks.
		for _, cb := range TipsetVectorOpts.OnTipsetApplied {
			cb(bs, &params, ret)
		}

		for j, v := range ret.AppliedResults {
			AssertMsgResult(r, vector.Post.Receipts[receiptsIdx], v, fmt.Sprintf("%d of tipset %d", j, i))
			receiptsIdx++
		}

		// Compare the receipts root.
		if expected, actual := vector.Post.ReceiptsRoots[i], ret.ReceiptsRoot; expected != actual {
			ierr := fmt.Errorf("post receipts root doesn't match; expected: %s, was: %s", expected, actual)
			r.Errorf(ierr.Error())
			err = multierror.Append(err, ierr)
		}

		prevEpoch = execEpoch
		root = ret.PostStateRoot
	}

	// Once all messages are applied, assert that the final state root matches
	// the expected postcondition root.
	if expected, actual := vector.Post.StateTree.RootCID, root; expected != actual {
		ierr := fmt.Errorf("wrong post root cid; expected %v, but got %v", expected, actual)
		r.Errorf(ierr.Error())
		err = multierror.Append(err, ierr)
		diffs = dumpThreeWayStateDiff(r, vector, bs, root)
	}
	return diffs, err
}

// AssertMsgResult compares a message result. It takes the expected receipt
// encoded in the vector, the actual receipt returned by Lotus, and a message
// label to log in the assertion failure message to facilitate debugging.
func AssertMsgResult(r Reporter, expected *schema.Receipt, actual *vm.ApplyRet, label string) {
	r.Helper()

	applyret := actual
	if expected, actual := exitcode.ExitCode(expected.ExitCode), actual.ExitCode; expected != actual {
		r.Errorf("exit code of msg %s did not match; expected: %s, got: %s", label, expected, actual)
		r.Errorf("\t\\==> actor error: %s", applyret.ActorErr)
	}
	if expected, actual := expected.GasUsed, actual.GasUsed; expected != actual {
		r.Errorf("gas used of msg %s did not match; expected: %d, got: %d", label, expected, actual)
	}
	if expected, actual := []byte(expected.ReturnValue), actual.Return; !bytes.Equal(expected, actual) {
		r.Errorf("return value of msg %s did not match; expected: %s, got: %s", label, base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(actual))
	}
}

func dumpThreeWayStateDiff(r Reporter, vector *schema.TestVector, bs blockstore.Blockstore, actual cid.Cid) []string {
	// check if statediff exists; if not, skip.
	if err := exec.Command("statediff", "--help").Run(); err != nil {
		r.Log("could not dump 3-way state tree diff upon test failure: statediff command not found")
		r.Log("install statediff with:")
		r.Log("$ git clone https://github.com/filecoin-project/statediff.git")
		r.Log("$ cd statediff")
		r.Log("$ go generate ./...")
		r.Log("$ go install ./cmd/statediff")
		return nil
	}

	tmpCar, err := writeStateToTempCAR(bs,
		vector.Pre.StateTree.RootCID,
		vector.Post.StateTree.RootCID,
		actual,
	)
	if err != nil {
		r.Fatalf("failed to write temporary state CAR: %s", err)
		return nil
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

	diff := func(left, right cid.Cid) string {
		cmd := exec.Command("statediff", "car", "--file", tmpCar, left.String(), right.String())
		b, err := cmd.CombinedOutput()
		if err != nil {
			r.Fatalf("statediff failed: %s", err)
		}
		return string(b)
	}

	bold := color.New(color.Bold).SprintfFunc()

	r.Log(bold("-----BEGIN STATEDIFF-----"))

	// run state diffs.
	r.Log(bold("=== dumping 3-way diffs between %s, %s, %s ===", a, b, c))

	r.Log(bold("--- %s left: %s; right: %s ---", d1, a, b))
	diffA := diff(vector.Post.StateTree.RootCID, actual)
	r.Log(bold("----------BEGIN STATEDIFF A----------"))
	r.Log(diffA)
	r.Log(bold("----------END STATEDIFF A----------"))

	r.Log(bold("--- %s left: %s; right: %s ---", d2, c, b))
	diffB := diff(vector.Pre.StateTree.RootCID, actual)
	r.Log(bold("----------BEGIN STATEDIFF B----------"))
	r.Log(diffB)
	r.Log(bold("----------END STATEDIFF B----------"))

	r.Log(bold("--- %s left: %s; right: %s ---", d3, c, a))
	diffC := diff(vector.Pre.StateTree.RootCID, vector.Post.StateTree.RootCID)
	r.Log(bold("----------BEGIN STATEDIFF C----------"))
	r.Log(diffC)
	r.Log(bold("----------END STATEDIFF C----------"))

	r.Log(bold("-----END STATEDIFF-----"))

	return []string{diffA, diffB, diffC}
}

// writeStateToTempCAR writes the provided roots to a temporary CAR that'll be
// cleaned up via t.Cleanup(). It returns the full path of the temp file.
func writeStateToTempCAR(bs blockstore.Blockstore, roots ...cid.Cid) (string, error) {
	tmp, err := os.CreateTemp("", "lotus-tests-*.car")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file to dump CAR for diffing: %w", err)
	}

	carWalkFn := func(nd format.Node) (out []*format.Link, err error) {
		for _, link := range nd.Links() {
			if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
				continue
			}
			// ignore things we don't have, the state tree is incomplete.
			if has, err := bs.Has(context.TODO(), link.Cid); err != nil {
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

func LoadBlockstore(vectorCAR schema.Base64EncodedBytes) (blockstore.Blockstore, error) {
	bs := blockstore.Blockstore(blockstore.NewMemory())

	// Read the base64-encoded CAR from the vector, and inflate the gzip.
	buf := bytes.NewReader(vectorCAR)
	r, err := gzip.NewReader(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to inflate gzipped CAR: %s", err)
	}
	defer r.Close() // nolint

	// Load the CAR embedded in the test vector into the Blockstore.
	_, err = car.LoadCar(context.TODO(), bs, r)
	if err != nil {
		return nil, fmt.Errorf("failed to load state tree car from test vector: %s", err)
	}

	if FallbackBlockstoreGetter != nil {
		fbs := &blockstore.FallbackStore{Blockstore: bs}
		fbs.SetFallback(func(ctx context.Context, c cid.Cid) (blocks.Block, error) {
			b, err := FallbackBlockstoreGetter.ChainReadObj(ctx, c)
			if err != nil {
				return nil, err
			}
			return blocks.NewBlockWithCid(b, c)
		})
		bs = fbs
	}

	return bs, nil
}
