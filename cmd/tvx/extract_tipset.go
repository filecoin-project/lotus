package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log"

	"github.com/filecoin-project/test-vectors/schema"
	"github.com/ipfs/go-cid"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/conformance"
)

func doExtractTipset(opts extractOpts) error {
	ctx := context.Background()

	if opts.tsk == "" {
		return fmt.Errorf("tipset key cannot be empty")
	}

	if opts.retain != "accessed-cids" {
		return fmt.Errorf("tipset extraction only supports 'accessed-cids' state retention")
	}

	ts, err := lcli.ParseTipSetRef(ctx, FullAPI, opts.tsk)
	if err != nil {
		return fmt.Errorf("failed to fetch tipset: %w", err)
	}

	log.Printf("tipset block count: %d", len(ts.Blocks()))

	var blocks []schema.Block
	for _, b := range ts.Blocks() {
		msgs, err := FullAPI.ChainGetBlockMessages(ctx, b.Cid())
		if err != nil {
			return fmt.Errorf("failed to get block messages (cid: %s): %w", b.Cid(), err)
		}

		log.Printf("block %s has %d messages", b.Cid(), len(msgs.Cids))

		packed := make([]schema.Base64EncodedBytes, 0, len(msgs.Cids))
		for _, m := range msgs.BlsMessages {
			b, err := m.Serialize()
			if err != nil {
				return fmt.Errorf("failed to serialize message: %w", err)
			}
			packed = append(packed, b)
		}
		for _, m := range msgs.SecpkMessages {
			b, err := m.Message.Serialize()
			if err != nil {
				return fmt.Errorf("failed to serialize message: %w", err)
			}
			packed = append(packed, b)
		}
		blocks = append(blocks, schema.Block{
			MinerAddr: b.Miner,
			WinCount:  b.ElectionProof.WinCount,
			Messages:  packed,
		})
	}

	var (
		// create a read-through store that uses ChainGetObject to fetch unknown CIDs.
		pst = NewProxyingStores(ctx, FullAPI)
		g   = NewSurgeon(ctx, FullAPI, pst)
	)

	driver := conformance.NewDriver(ctx, schema.Selector{}, conformance.DriverOpts{
		DisableVMFlush: true,
	})

	// this is the root of the state tree we start with.
	root := ts.ParentState()
	log.Printf("base state tree root CID: %s", root)

	basefee := ts.Blocks()[0].ParentBaseFee
	log.Printf("basefee: %s", basefee)

	tipset := schema.Tipset{
		BaseFee: *basefee.Int,
		Blocks:  blocks,
	}

	// recordingRand will record randomness so we can embed it in the test vector.
	recordingRand := conformance.NewRecordingRand(new(conformance.LogReporter), FullAPI)

	log.Printf("using state retention strategy: %s", extractFlags.retain)

	tbs, ok := pst.Blockstore.(TracingBlockstore)
	if !ok {
		return fmt.Errorf("requested 'accessed-cids' state retention, but no tracing blockstore was present")
	}

	tbs.StartTracing()

	params := conformance.ExecuteTipsetParams{
		Preroot:     ts.ParentState(),
		ParentEpoch: ts.Height() - 1,
		Tipset:      &tipset,
		ExecEpoch:   ts.Height(),
		Rand:        recordingRand,
	}
	result, err := driver.ExecuteTipset(pst.Blockstore, pst.Datastore, params)
	if err != nil {
		return fmt.Errorf("failed to execute tipset: %w", err)
	}

	accessed := tbs.FinishTracing()

	// write a CAR with the accessed state into a buffer.
	var (
		out = new(bytes.Buffer)
		gw  = gzip.NewWriter(out)
	)
	if err := g.WriteCARIncluding(gw, accessed, ts.ParentState(), result.PostStateRoot); err != nil {
		return err
	}
	if err = gw.Flush(); err != nil {
		return err
	}
	if err = gw.Close(); err != nil {
		return err
	}

	codename := GetProtocolCodename(ts.Height())
	nv, err := FullAPI.StateNetworkVersion(ctx, ts.Key())
	if err != nil {
		return err
	}

	version, err := FullAPI.Version(ctx)
	if err != nil {
		return err
	}

	ntwkName, err := FullAPI.StateNetworkName(ctx)
	if err != nil {
		return err
	}

	vector := schema.TestVector{
		Class: schema.ClassTipset,
		Meta: &schema.Metadata{
			ID: opts.id,
			Gen: []schema.GenerationData{
				{Source: fmt.Sprintf("network:%s", ntwkName)},
				{Source: fmt.Sprintf("tipset:%s", ts.Key())},
				{Source: "github.com/filecoin-project/lotus", Version: version.String()}},
		},
		Selector: schema.Selector{
			schema.SelectorMinProtocolVersion: codename,
		},
		Randomness: recordingRand.Recorded(),
		CAR:        out.Bytes(),
		Pre: &schema.Preconditions{
			Variants: []schema.Variant{
				{ID: codename, Epoch: int64(ts.Height()), NetworkVersion: uint(nv)},
			},
			BaseFee: basefee.Int,
			StateTree: &schema.StateTree{
				RootCID: ts.ParentState(),
			},
		},
		ApplyTipsets: []schema.Tipset{tipset},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{
				RootCID: result.PostStateRoot,
			},
			ReceiptsRoots: []cid.Cid{result.ReceiptsRoot},
		},
	}

	for _, res := range result.AppliedResults {
		vector.Post.Receipts = append(vector.Post.Receipts, &schema.Receipt{
			ExitCode:    int64(res.ExitCode),
			ReturnValue: res.Return,
			GasUsed:     res.GasUsed,
		})
	}

	return writeVector(vector, opts.file)
}
