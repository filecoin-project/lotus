package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/conformance"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/oni/tvx/state"
)

var extractMsgFlags struct {
	cid    string
	file   string
	retain string
}

var extractMsgCmd = &cli.Command{
	Name:        "extract-message",
	Description: "generate a message-class test vector by extracting it from a network",
	Action:      runExtractMsg,
	Flags: []cli.Flag{
		&apiFlag,
		&cli.StringFlag{
			Name:        "cid",
			Usage:       "message CID to generate test vector from",
			Required:    true,
			Destination: &extractMsgFlags.cid,
		},
		&cli.StringFlag{
			Name:        "file",
			Usage:       "output file",
			Required:    true,
			Destination: &extractMsgFlags.file,
		},
		&cli.StringFlag{
			Name:        "state-retain",
			Usage:       "state retention policy; values: 'accessed-cids' (default), 'accessed-actors'",
			Value:       "accessed-actors",
			Destination: &extractMsgFlags.retain,
		},
	},
}

func runExtractMsg(c *cli.Context) error {
	// LOTUS_DISABLE_VM_BUF disables what's called "VM state tree buffering",
	// which stashes write operations in a BufferedBlockstore
	// (https://github.com/filecoin-project/lotus/blob/b7a4dbb07fd8332b4492313a617e3458f8003b2a/lib/bufbstore/buf_bstore.go#L21)
	// such that they're not written until the VM is actually flushed.
	//
	// For some reason, the standard behaviour was not working for me (raulk),
	// and disabling it (such that the state transformations are written immediately
	// to the blockstore) worked.
	_ = os.Setenv("LOTUS_DISABLE_VM_BUF", "iknowitsabadidea")

	ctx := context.Background()

	// get the output file.
	if extractMsgFlags.file == "" {
		return fmt.Errorf("output file required")
	}

	mcid, err := cid.Decode(extractMsgFlags.cid)
	if err != nil {
		return err
	}

	// Make the client.
	api, err := makeClient(c)
	if err != nil {
		return err
	}

	log.Printf("locating message with CID: %s...", mcid)

	// Locate the message.
	msgInfo, err := api.StateSearchMsg(ctx, mcid)
	if err != nil {
		return fmt.Errorf("failed to locate message: %w", err)
	}

	log.Printf("located message at tipset %s (height: %d) with exit code: %s", msgInfo.TipSet, msgInfo.Height, msgInfo.Receipt.ExitCode)

	// Extract the full message.
	msg, err := api.ChainGetMessage(ctx, mcid)
	if err != nil {
		return err
	}

	log.Printf("full message: %+v", msg)

	execTs, incTs, err := findRelevantTipsets(ctx, api, msgInfo.TipSet)
	if err != nil {
		return err
	}

	log.Printf("message was executed in tipset: %s", execTs.Key())
	log.Printf("message was included in tipset: %s", incTs.Key())
	log.Printf("finding precursor messages...")

	var allmsgs []*types.Message
	for _, b := range incTs.Blocks() {
		messages, err := api.ChainGetBlockMessages(ctx, b.Cid())
		if err != nil {
			return err
		}

		related, found, err := findMsgAndPrecursors(messages, msg)
		if err != nil {
			return fmt.Errorf("invariant failed while scanning messages in block %s: %w", b.Cid(), err)
		}

		if found {
			var mcids []cid.Cid
			for _, m := range related {
				mcids = append(mcids, m.Cid())
			}
			log.Printf("found message in block %s; precursors: %v", b.Cid(), mcids[:len(mcids)-1])
			allmsgs = related
			break
		}

		log.Printf("message not found in block %s; precursors found: %d", b.Cid(), len(related))
	}

	if allmsgs == nil {
		// Message was not found; abort.
		return fmt.Errorf("did not find a block containing the message")
	}

	precursors := allmsgs[:len(allmsgs)-1]

	var (
		// create a read through store that uses ChainGetObject to fetch unknown CIDs.
		pst = state.NewProxyingStores(ctx, api)
		g   = state.NewSurgeon(ctx, api, pst)
	)

	driver := conformance.NewDriver(ctx, schema.Selector{})

	// this is the root of the state tree we start with.
	root := incTs.ParentState()
	log.Printf("base state tree root CID: %s", root)

	// on top of that state tree, we apply all precursors.
	log.Printf("precursors to apply: %d", len(precursors))
	for i, m := range precursors {
		log.Printf("applying precursor %d, cid: %s", i, m.Cid())
		_, root, err = driver.ExecuteMessage(pst.Blockstore, root, execTs.Height(), m)
		if err != nil {
			return fmt.Errorf("failed to execute precursor message: %w", err)
		}
	}

	var (
		preroot cid.Cid
		postroot cid.Cid
	)

	if extractMsgFlags.retain == "accessed-actors" {
		log.Printf("calculating accessed actors...")
		// get actors accessed by message.
		retain, err := g.GetAccessedActors(ctx, api, mcid)
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
		_, postroot, err = driver.ExecuteMessage(pst.Blockstore, preroot, execTs.Height(), msg)
		if err != nil {
			return fmt.Errorf("failed to execute message: %w", err)
		}
	}

	msgBytes, err := msg.Serialize()
	if err != nil {
		return err
	}

	// don't fetch additional content that wasn't accessed yet during car spidering / generation.
	type onlineblockstore interface {
		SetOnline(bool)
	}
	if ob, ok := pst.Blockstore.(onlineblockstore); ok {
		ob.SetOnline(false)
	}

	out := new(bytes.Buffer)
	gw := gzip.NewWriter(out)
	if err := g.WriteCAR(gw, preroot, postroot); err != nil {
		return err
	}
	if err = gw.Flush(); err != nil {
		return err
	}
	if err = gw.Close(); err != nil {
		return err
	}

	version, err := api.Version(ctx)
	if err != nil {
		return err
	}

	// Write out the test vector.
	vector := schema.TestVector{
		Class:    schema.ClassMessage,
		Selector: schema.Selector(map[string]string{}),
		Meta: &schema.Metadata{
			ID:      "TK",
			Version: "TK",
			Gen: []schema.GenerationData{schema.GenerationData{
				Source:  msg.Cid().String(),
				Version: version.String(),
			}},
		},
		CAR: out.Bytes(),
		Pre: &schema.Preconditions{
			Epoch: int64(execTs.Height()),
			StateTree: &schema.StateTree{
				RootCID: preroot,
			},
		},
		ApplyMessages: []schema.Message{{Bytes: msgBytes}},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{
				RootCID: postroot,
			},
		},
	}

	file, err := os.Create(extractMsgFlags.file)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&vector); err != nil {
		return err
	}

	return nil
}

func findRelevantTipsets(ctx context.Context, api api.FullNode, execTsk types.TipSetKey) (execTs *types.TipSet, incTs *types.TipSet, err error) {
	// get the tipset on which this message was "executed" on.
	// https://github.com/filecoin-project/lotus/issues/2847
	execTs, err = api.ChainGetTipSet(ctx, execTsk)
	if err != nil {
		return nil, nil, err
	}
	// get the previous tipset, on which this message was mined,
	// i.e. included on-chain.
	incTs, err = api.ChainGetTipSet(ctx, execTs.Parents())
	if err != nil {
		return nil, nil, err
	}
	return execTs, incTs, nil
}

// findMsgAndPrecursors scans the messages in a block to locate the supplied
// message, looking into the BLS or SECP section depending on the sender's
// address type.
//
// It returns any precursors (if they exist), and the found message (if found),
// in a slice.
//
// It also returns a boolean indicating whether the message was actually found.
//
// This function also asserts invariants, and if those fail, it returns an error.
func findMsgAndPrecursors(messages *api.BlockMessages, target *types.Message) (related []*types.Message, found bool, err error) {
	// Decide which block of messages to process, depending on whether the
	// sender is a BLS or a SECP account.
	input := messages.BlsMessages
	if senderKind := target.From.Protocol(); senderKind == address.SECP256K1 {
		input = make([]*types.Message, 0, len(messages.SecpkMessages))
		for _, sm := range messages.SecpkMessages {
			input = append(input, &sm.Message)
		}
	}

	for _, other := range input {
		if other.From != target.From {
			continue
		}

		// this message is from the same sender, so it's related.
		related = append(related, other)

		if other.Nonce > target.Nonce {
			return nil, false, fmt.Errorf("a message with nonce higher than the target was found before the target; offending mcid: %s", other.Cid())
		}

		// this message is the target; we're done.
		if other.Cid() == target.Cid() {
			return related, true, nil
		}
	}

	// this could happen because a block contained related messages, but not
	// the target (that is, messages with a lower nonce, but ultimately not the
	// target).
	return related, false, nil
}
