package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/fatih/color"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/conformance"
)

func doExtractMessage(opts extractOpts) error {
	ctx := context.Background()

	if opts.cid == "" {
		return fmt.Errorf("missing message CID")
	}

	mcid, err := cid.Decode(opts.cid)
	if err != nil {
		return err
	}

	msg, execTs, incTs, err := resolveFromChain(ctx, FullAPI, mcid, opts.block)
	if err != nil {
		return fmt.Errorf("failed to resolve message and tipsets from chain: %w", err)
	}

	// Assumes that the desired message isn't at the boundary of network versions.
	// Otherwise this will be inaccurate. But it's such a tiny edge case that
	// it's not worth spending the time to support boundary messages unless
	// actually needed.
	nv, err := FullAPI.StateNetworkVersion(ctx, incTs.Key())
	if err != nil {
		return fmt.Errorf("failed to resolve network version from inclusion height: %w", err)
	}

	// get the circulating supply before the message was executed.
	circSupplyDetail, err := FullAPI.StateVMCirculatingSupplyInternal(ctx, incTs.Key())
	if err != nil {
		return fmt.Errorf("failed while fetching circulating supply: %w", err)
	}

	circSupply := circSupplyDetail.FilCirculating

	log.Printf("message was executed in tipset: %s", execTs.Key())
	log.Printf("message was included in tipset: %s", incTs.Key())
	log.Printf("network version at inclusion: %d", nv)
	log.Printf("circulating supply at inclusion tipset: %d", circSupply)
	log.Printf("finding precursor messages using mode: %s", opts.precursor)

	// Fetch messages in canonical order from inclusion tipset.
	msgs, err := FullAPI.ChainGetParentMessages(ctx, execTs.Blocks()[0].Cid())
	if err != nil {
		return fmt.Errorf("failed to fetch messages in canonical order from inclusion tipset: %w", err)
	}

	related, found, err := findMsgAndPrecursors(ctx, opts.precursor, mcid, msg.From, msg.To, msgs)
	if err != nil {
		return fmt.Errorf("failed while finding message and precursors: %w", err)
	}

	if !found {
		return fmt.Errorf("message not found; precursors found: %d", len(related))
	}

	var (
		precursors     = related[:len(related)-1]
		precursorsCids []cid.Cid
	)

	for _, p := range precursors {
		precursorsCids = append(precursorsCids, p.Cid())
	}

	log.Println(color.GreenString("found message; precursors (count: %d): %v", len(precursors), precursorsCids))

	var (
		// create a read-through store that uses ChainGetObject to fetch unknown CIDs.
		pst = NewProxyingStores(ctx, FullAPI)
		g   = NewSurgeon(ctx, FullAPI, pst)
	)

	driver := conformance.NewDriver(ctx, schema.Selector{}, conformance.DriverOpts{
		DisableVMFlush: true,
	})

	// this is the root of the state tree we start with.
	root := incTs.ParentState()
	log.Printf("base state tree root CID: %s", root)

	basefee := incTs.Blocks()[0].ParentBaseFee
	log.Printf("basefee: %s", basefee)

	// on top of that state tree, we apply all precursors.
	log.Printf("number of precursors to apply: %d", len(precursors))
	for i, m := range precursors {
		log.Printf("applying precursor %d, cid: %s", i, m.Cid())
		_, root, err = driver.ExecuteMessage(pst.Blockstore, conformance.ExecuteMessageParams{
			Preroot:    root,
			Epoch:      incTs.Height(),
			Message:    m,
			CircSupply: circSupplyDetail.FilCirculating,
			BaseFee:    basefee,
			// recorded randomness will be discarded.
			Rand:           conformance.NewRecordingRand(new(conformance.LogReporter), FullAPI),
			NetworkVersion: nv,
		})
		if err != nil {
			return fmt.Errorf("failed to execute precursor message: %w", err)
		}
	}

	var (
		preroot   cid.Cid
		postroot  cid.Cid
		applyret  *vm.ApplyRet
		carWriter func(w io.Writer) error
		retention = opts.retain

		// recordingRand will record randomness so we can embed it in the test vector.
		recordingRand = conformance.NewRecordingRand(new(conformance.LogReporter), FullAPI)
	)

	log.Printf("using state retention strategy: %s", retention)
	log.Printf("now applying requested message: %s", msg.Cid())
	switch retention {
	case "accessed-cids":
		tbs, ok := pst.Blockstore.(TracingBlockstore)
		if !ok {
			return fmt.Errorf("requested 'accessed-cids' state retention, but no tracing blockstore was present")
		}

		tbs.StartTracing()

		preroot = root
		applyret, postroot, err = driver.ExecuteMessage(pst.Blockstore, conformance.ExecuteMessageParams{
			Preroot:        preroot,
			Epoch:          incTs.Height(),
			Message:        msg,
			CircSupply:     circSupplyDetail.FilCirculating,
			BaseFee:        basefee,
			Rand:           recordingRand,
			NetworkVersion: nv,
		})
		if err != nil {
			return fmt.Errorf("failed to execute message: %w", err)
		}
		accessed := tbs.FinishTracing()
		carWriter = func(w io.Writer) error {
			return g.WriteCARIncluding(w, accessed, preroot, postroot)
		}

	case "accessed-actors":
		log.Printf("calculating accessed actors")
		// get actors accessed by message.
		retain, err := g.GetAccessedActors(ctx, FullAPI, mcid)
		if err != nil {
			return fmt.Errorf("failed to calculate accessed actors: %w", err)
		}
		// also append the reward actor and the burnt funds actor.
		retain = append(retain, reward.Address, builtin.BurntFundsActorAddr, init_.Address)
		log.Printf("calculated accessed actors: %v", retain)

		// get the masked state tree from the root,
		preroot, err = g.GetMaskedStateTree(root, retain)
		if err != nil {
			return err
		}
		applyret, postroot, err = driver.ExecuteMessage(pst.Blockstore, conformance.ExecuteMessageParams{
			Preroot:    preroot,
			Epoch:      incTs.Height(),
			Message:    msg,
			CircSupply: circSupplyDetail.FilCirculating,
			BaseFee:    basefee,
			Rand:       recordingRand,
		})
		if err != nil {
			return fmt.Errorf("failed to execute message: %w", err)
		}
		carWriter = func(w io.Writer) error {
			return g.WriteCAR(w, preroot, postroot)
		}

	default:
		return fmt.Errorf("unknown state retention option: %s", retention)
	}

	log.Printf("message applied; preroot: %s, postroot: %s", preroot, postroot)
	log.Println("performing sanity check on receipt")

	// TODO sometimes this returns a nil receipt and no error ¯\_(ツ)_/¯
	//  ex: https://filfox.info/en/message/bafy2bzacebpxw3yiaxzy2bako62akig46x3imji7fewszen6fryiz6nymu2b2
	//  This code is lenient and skips receipt comparison in case of a nil receipt.
	rec, err := FullAPI.StateSearchMsg(ctx, execTs.Key(), mcid, api.LookbackNoLimit, false)
	if err != nil {
		return fmt.Errorf("failed to find receipt on chain: %w", err)
	}
	log.Printf("found receipt: %+v", rec)

	// generate the schema receipt; if we got
	var receipt *schema.Receipt
	if rec != nil {
		receipt = &schema.Receipt{
			ExitCode:    int64(rec.Receipt.ExitCode),
			ReturnValue: rec.Receipt.Return,
			GasUsed:     rec.Receipt.GasUsed,
		}

		reporter := new(conformance.LogReporter)
		conformance.AssertMsgResult(reporter, receipt, applyret, "as locally executed")
		if reporter.Failed() {
			if opts.ignoreSanityChecks {
				log.Println(color.YellowString("receipt sanity check failed; proceeding anyway"))
			} else {
				log.Println(color.RedString("receipt sanity check failed; aborting"))
				return fmt.Errorf("vector generation aborted")
			}
		} else {
			log.Println(color.GreenString("receipt sanity check succeeded"))
		}

	} else {
		receipt = &schema.Receipt{
			ExitCode:    int64(applyret.ExitCode),
			ReturnValue: applyret.Return,
			GasUsed:     applyret.GasUsed,
		}
		log.Println(color.YellowString("skipping receipts comparison; we got back a nil receipt from lotus"))
	}

	log.Println("generating vector")
	msgBytes, err := msg.Serialize()
	if err != nil {
		return err
	}

	var (
		out = new(bytes.Buffer)
		gw  = gzip.NewWriter(out)
	)
	if err := carWriter(gw); err != nil {
		return err
	}
	if err = gw.Flush(); err != nil {
		return err
	}
	if err = gw.Close(); err != nil {
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

	codename := GetProtocolCodename(execTs.Height())

	// Write out the test vector.
	vector := schema.TestVector{
		Class: schema.ClassMessage,
		Meta: &schema.Metadata{
			ID: opts.id,
			// TODO need to replace schema.GenerationData with a more flexible
			//  data structure that makes no assumption about the traceability
			//  data that's being recorded; a flexible map[string]string
			//  would do.
			Gen: []schema.GenerationData{
				{Source: fmt.Sprintf("network:%s", ntwkName)},
				{Source: fmt.Sprintf("message:%s", msg.Cid().String())},
				{Source: fmt.Sprintf("inclusion_tipset:%s", incTs.Key().String())},
				{Source: fmt.Sprintf("execution_tipset:%s", execTs.Key().String())},
				{Source: "github.com/filecoin-project/lotus", Version: version.String()}},
		},
		Selector: schema.Selector{
			schema.SelectorMinProtocolVersion: codename,
		},
		Randomness: recordingRand.Recorded(),
		CAR:        out.Bytes(),
		Pre: &schema.Preconditions{
			Variants: []schema.Variant{
				{ID: codename, Epoch: int64(incTs.Height()), NetworkVersion: uint(nv)},
			},
			CircSupply: circSupply.Int,
			BaseFee:    basefee.Int,
			StateTree: &schema.StateTree{
				RootCID: preroot,
			},
		},
		ApplyMessages: []schema.Message{{Bytes: msgBytes}},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{
				RootCID: postroot,
			},
			Receipts: []*schema.Receipt{
				{
					ExitCode:    int64(applyret.ExitCode),
					ReturnValue: applyret.Return,
					GasUsed:     applyret.GasUsed,
				},
			},
		},
	}
	return writeVector(&vector, opts.file)
}

// resolveFromChain queries the chain for the provided message, using the block CID to
// speed up the query, if provided
func resolveFromChain(ctx context.Context, api lapi.FullNode, mcid cid.Cid, block string) (msg *types.Message, execTs *types.TipSet, incTs *types.TipSet, err error) {
	// Extract the full message.
	msg, err = api.ChainGetMessage(ctx, mcid)
	if err != nil {
		return nil, nil, nil, err
	}

	log.Printf("found message with CID %s: %+v", mcid, msg)

	if block == "" {
		log.Printf("locating message in blockchain")

		// Locate the message.
		msgInfo, err := api.StateSearchMsg(ctx, types.EmptyTSK, mcid, lapi.LookbackNoLimit, false)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to locate message: %w", err)
		}
		if msgInfo == nil {
			return nil, nil, nil, fmt.Errorf("failed to locate message: not found")
		}

		log.Printf("located message at tipset %s (height: %d) with exit code: %s", msgInfo.TipSet, msgInfo.Height, msgInfo.Receipt.ExitCode)

		execTs, incTs, err = fetchThisAndPrevTipset(ctx, api, msgInfo.TipSet)
		return msg, execTs, incTs, err
	}

	bcid, err := cid.Decode(block)
	if err != nil {
		return nil, nil, nil, err
	}

	log.Printf("message inclusion block CID was provided; scanning around it: %s", bcid)

	blk, err := api.ChainGetBlock(ctx, bcid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get block: %w", err)
	}

	// types.EmptyTSK hints to use the HEAD.
	execTs, err = api.ChainGetTipSetByHeight(ctx, blk.Height+1, types.EmptyTSK)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get message execution tipset (%d) : %w", blk.Height+1, err)
	}

	// walk back from the execTs instead of HEAD, to save time.
	incTs, err = api.ChainGetTipSetByHeight(ctx, blk.Height, execTs.Key())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get message inclusion tipset (%d): %w", blk.Height, err)
	}

	return msg, execTs, incTs, nil
}

// fetchThisAndPrevTipset returns the full tipset identified by the key, as well
// as the previous tipset. In the context of vector generation, the target
// tipset is the one where a message was executed, and the previous tipset is
// the one where the message was included.
func fetchThisAndPrevTipset(ctx context.Context, api v1api.FullNode, target types.TipSetKey) (targetTs *types.TipSet, prevTs *types.TipSet, err error) {
	// get the tipset on which this message was "executed" on.
	// https://github.com/filecoin-project/lotus/issues/2847
	targetTs, err = api.ChainGetTipSet(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	// get the previous tipset, on which this message was mined,
	// i.e. included on-chain.
	prevTs, err = api.ChainGetTipSet(ctx, targetTs.Parents())
	if err != nil {
		return nil, nil, err
	}
	return targetTs, prevTs, nil
}

// findMsgAndPrecursors ranges through the canonical messages slice, locating
// the target message and returning precursors in accordance to the supplied
// mode.
func findMsgAndPrecursors(ctx context.Context, mode string, msgCid cid.Cid, sender address.Address, recipient address.Address, msgs []api.Message) (related []*types.Message, found bool, err error) {
	// Resolve addresses to IDs for canonicality.
	senderID := mustResolveAddr(ctx, sender)
	recipientID := mustResolveAddr(ctx, recipient)

	// Range through messages, selecting only the precursors based on selection mode.
	for _, m := range msgs {
		msgSenderID := mustResolveAddr(ctx, m.Message.From)
		msgRecipientID := mustResolveAddr(ctx, m.Message.To)

		switch {
		case mode == PrecursorSelectAll:
			fallthrough
		case mode == PrecursorSelectParticipants &&
			(msgSenderID == senderID ||
				msgRecipientID == recipientID ||
				msgSenderID == recipientID ||
				msgRecipientID == senderID):
			related = append(related, m.Message)
		}

		// this message is the target; we're done.
		if m.Cid == msgCid {
			return related, true, nil
		}
	}

	// this could happen because a block contained related messages, but not
	// the target (that is, messages with a lower nonce, but ultimately not the
	// target).
	return related, false, nil
}

var addressCache = make(map[address.Address]address.Address)

func mustResolveAddr(ctx context.Context, addr address.Address) address.Address {
	if resolved, ok := addressCache[addr]; ok {
		return resolved
	}
	id, err := FullAPI.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		panic(fmt.Errorf("failed to resolve addr: %w", err))
	}
	addressCache[addr] = id
	return id
}
