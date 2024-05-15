package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/conformance"
)

func doExtractTipset(opts extractOpts) error {
	ctx := context.Background()

	if opts.retain != "accessed-cids" {
		return fmt.Errorf("tipset extraction only supports 'accessed-cids' state retention")
	}

	if opts.tsk == "" {
		return fmt.Errorf("tipset key cannot be empty")
	}

	ss := strings.Split(opts.tsk, "..")
	switch len(ss) {
	case 1: // extracting a single tipset.
		ts, err := lcli.ParseTipSetRef(ctx, FullAPI, opts.tsk)
		if err != nil {
			return fmt.Errorf("failed to fetch tipset: %w", err)
		}
		v, err := extractTipsets(ctx, ts)
		if err != nil {
			return err
		}
		return writeVector(v, opts.file)

	case 2: // extracting a range of tipsets.
		left, err := lcli.ParseTipSetRef(ctx, FullAPI, ss[0])
		if err != nil {
			return fmt.Errorf("failed to fetch tipset %s: %w", ss[0], err)
		}
		right, err := lcli.ParseTipSetRef(ctx, FullAPI, ss[1])
		if err != nil {
			return fmt.Errorf("failed to fetch tipset %s: %w", ss[1], err)
		}

		// resolve the tipset range.
		tss, err := resolveTipsetRange(ctx, left, right)
		if err != nil {
			return err
		}

		// are squashing all tipsets into a single multi-tipset vector?
		if opts.squash {
			vector, err := extractTipsets(ctx, tss...)
			if err != nil {
				return err
			}
			return writeVector(vector, opts.file)
		}

		// we are generating a single-tipset vector per tipset.
		vectors, err := extractIndividualTipsets(ctx, tss...)
		if err != nil {
			return err
		}
		return writeVectors(opts.file, vectors...)

	default:
		return fmt.Errorf("unrecognized tipset format")
	}
}

func resolveTipsetRange(ctx context.Context, left *types.TipSet, right *types.TipSet) (tss []*types.TipSet, err error) {
	// start from the right tipset and walk back the chain until the left tipset, inclusive.
	for curr := right; curr.Key() != left.Parents(); {
		tss = append(tss, curr)
		curr, err = FullAPI.ChainGetTipSet(ctx, curr.Parents())
		if err != nil {
			return nil, fmt.Errorf("failed to get tipset %s (height: %d): %w", curr.Parents(), curr.Height()-1, err)
		}
	}
	// reverse the slice.
	for i, j := 0, len(tss)-1; i < j; i, j = i+1, j-1 {
		tss[i], tss[j] = tss[j], tss[i]
	}
	return tss, nil
}

func extractIndividualTipsets(ctx context.Context, tss ...*types.TipSet) (vectors []*schema.TestVector, err error) {
	for _, ts := range tss {
		v, err := extractTipsets(ctx, ts)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, v)
	}
	return vectors, nil
}

func extractTipsets(ctx context.Context, tss ...*types.TipSet) (*schema.TestVector, error) {
	var (
		// create a read-through store that uses ChainGetObject to fetch unknown CIDs.
		pst = NewProxyingStores(ctx, FullAPI)
		g   = NewSurgeon(ctx, FullAPI, pst)

		// recordingRand will record randomness so we can embed it in the test vector.
		recordingRand = conformance.NewRecordingRand(new(conformance.LogReporter), FullAPI)
	)

	tbs, ok := pst.Blockstore.(TracingBlockstore)
	if !ok {
		return nil, fmt.Errorf("requested 'accessed-cids' state retention, but no tracing blockstore was present")
	}

	driver := conformance.NewDriver(ctx, schema.Selector{}, conformance.DriverOpts{
		DisableVMFlush: true,
	})

	base := tss[0]
	last := tss[len(tss)-1]

	// this is the root of the state tree we start with.
	root := base.ParentState()
	log.Printf("base state tree root CID: %s", root)

	codename := GetProtocolCodename(base.Height())
	nv, err := FullAPI.StateNetworkVersion(ctx, base.Key())
	if err != nil {
		return nil, err
	}

	version, err := FullAPI.Version(ctx)
	if err != nil {
		return nil, err
	}

	ntwkName, err := FullAPI.StateNetworkName(ctx)
	if err != nil {
		return nil, err
	}

	vector := schema.TestVector{
		Class: schema.ClassTipset,
		Meta: &schema.Metadata{
			ID: fmt.Sprintf("@%d..@%d", base.Height(), last.Height()),
			Gen: []schema.GenerationData{
				{Source: fmt.Sprintf("network:%s", ntwkName)},
				{Source: "github.com/filecoin-project/lotus", Version: version.String()}},
			// will be completed by extra tipset stamps.
		},
		Selector: schema.Selector{
			schema.SelectorMinProtocolVersion: codename,
		},
		Pre: &schema.Preconditions{
			Variants: []schema.Variant{
				{ID: codename, Epoch: int64(base.Height()), NetworkVersion: uint(nv)},
			},
			StateTree: &schema.StateTree{
				RootCID: base.ParentState(),
			},
		},
		Post: &schema.Postconditions{
			StateTree: new(schema.StateTree),
		},
	}

	tbs.StartTracing()

	roots := []cid.Cid{base.ParentState()}
	for i, ts := range tss {
		log.Printf("tipset %s block count: %d", ts.Key(), len(ts.Blocks()))

		var blocks []schema.Block
		for _, b := range ts.Blocks() {
			msgs, err := FullAPI.ChainGetBlockMessages(ctx, b.Cid())
			if err != nil {
				return nil, fmt.Errorf("failed to get block messages (cid: %s): %w", b.Cid(), err)
			}

			log.Printf("block %s has %d messages", b.Cid(), len(msgs.Cids))

			packed := make([]schema.Base64EncodedBytes, 0, len(msgs.Cids))
			for _, m := range msgs.BlsMessages {
				b, err := m.Serialize()
				if err != nil {
					return nil, fmt.Errorf("failed to serialize message: %w", err)
				}
				packed = append(packed, b)
			}
			for _, m := range msgs.SecpkMessages {
				b, err := m.Message.Serialize()
				if err != nil {
					return nil, fmt.Errorf("failed to serialize message: %w", err)
				}
				packed = append(packed, b)
			}
			blocks = append(blocks, schema.Block{
				MinerAddr: b.Miner,
				WinCount:  b.ElectionProof.WinCount,
				Messages:  packed,
			})
		}

		basefee := base.Blocks()[0].ParentBaseFee
		log.Printf("tipset basefee: %s", basefee)

		tipset := schema.Tipset{
			BaseFee:     *basefee.Int,
			Blocks:      blocks,
			EpochOffset: int64(i),
		}

		params := conformance.ExecuteTipsetParams{
			Preroot:     roots[len(roots)-1],
			ParentEpoch: ts.Height() - 1,
			Tipset:      &tipset,
			ExecEpoch:   ts.Height(),
			Rand:        recordingRand,
		}

		result, err := driver.ExecuteTipset(pst.Blockstore, pst.Datastore, params)
		if err != nil {
			return nil, fmt.Errorf("failed to execute tipset: %w", err)
		}

		roots = append(roots, result.PostStateRoot)

		// update the vector.
		vector.ApplyTipsets = append(vector.ApplyTipsets, tipset)
		vector.Post.ReceiptsRoots = append(vector.Post.ReceiptsRoots, result.ReceiptsRoot)

		for _, res := range result.AppliedResults {
			vector.Post.Receipts = append(vector.Post.Receipts, &schema.Receipt{
				ExitCode:    int64(res.ExitCode),
				ReturnValue: res.Return,
				GasUsed:     res.GasUsed,
			})
		}

		vector.Meta.Gen = append(vector.Meta.Gen, schema.GenerationData{
			Source: "tipset:" + ts.Key().String(),
		})
	}

	accessed := tbs.FinishTracing()

	//
	// ComputeBaseFee(ctx, baseTs)

	// write a CAR with the accessed state into a buffer.
	var (
		out = new(bytes.Buffer)
		gw  = gzip.NewWriter(out)
	)
	if err := g.WriteCARIncluding(gw, accessed, roots...); err != nil {
		return nil, err
	}
	if err = gw.Flush(); err != nil {
		return nil, err
	}
	if err = gw.Close(); err != nil {
		return nil, err
	}

	vector.Randomness = recordingRand.Recorded()
	vector.Post.StateTree.RootCID = roots[len(roots)-1]
	vector.CAR = out.Bytes()

	return &vector, nil
}
