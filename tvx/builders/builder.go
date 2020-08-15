package builders

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/oni/tvx/lotus"
	"github.com/filecoin-project/oni/tvx/schema"
	ostate "github.com/filecoin-project/oni/tvx/state"
)

type Stage string

const (
	StagePreconditions = Stage("preconditions")
	StageApplies       = Stage("applies")
	StageChecks        = Stage("checks")
	StageFinished      = Stage("finished")
)

func init() {
	// disable logs, as we need a clean stdout output.
	log.SetOutput(os.Stderr)
	log.SetPrefix(">>> ")

	_ = os.Setenv("LOTUS_DISABLE_VM_BUF", "iknowitsabadidea")
}

// TODO use stage.Surgeon with non-proxying blockstore.
type Builder struct {
	Actors    *Actors
	Assert    *Asserter
	Messages  *Messages
	Driver    *lotus.Driver
	PreRoot   cid.Cid
	PostRoot  cid.Cid
	CurrRoot  cid.Cid
	Wallet    *Wallet
	StateTree *state.StateTree
	Stores    *ostate.Stores

	vector schema.TestVector
	stage  Stage
}

// MessageVector creates a builder for a message-class vector.
func MessageVector(metadata *schema.Metadata) *Builder {
	stores := ostate.NewLocalStores(context.Background())

	// Create a brand new state tree.
	st, err := state.NewStateTree(stores.CBORStore)
	if err != nil {
		panic(err)
	}

	b := &Builder{
		stage:     StagePreconditions,
		Stores:    stores,
		StateTree: st,
		PreRoot:   cid.Undef,
		Driver:    lotus.NewDriver(context.Background()),
	}

	b.Wallet = newWallet()
	b.Assert = newAsserter(b, StagePreconditions)
	b.Actors = newActors(b)
	b.Messages = &Messages{b: b}

	b.vector.Class = schema.ClassMessage
	b.vector.Meta = metadata
	b.vector.Pre = &schema.Preconditions{}
	b.vector.Post = &schema.Postconditions{}

	b.initializeZeroState()

	return b
}

func (b *Builder) CommitPreconditions() {
	if b.stage != StagePreconditions {
		panic("called CommitPreconditions at the wrong time")
	}

	// capture the preroot after applying all preconditions.
	preroot := b.FlushState()

	b.vector.Pre.Epoch = 0
	b.vector.Pre.StateTree = &schema.StateTree{RootCID: preroot}

	b.CurrRoot, b.PreRoot = preroot, preroot
	b.stage = StageApplies
	b.Assert = newAsserter(b, StageApplies)
}

func (b *Builder) CommitApplies() {
	if b.stage != StageApplies {
		panic("called CommitApplies at the wrong time")
	}

	for _, am := range b.Messages.All() {
		// apply all messages that are pending application.
		if am.Result == nil {
			b.applyMessage(am)
		}
	}

	b.PostRoot = b.CurrRoot
	b.vector.Post.StateTree = &schema.StateTree{RootCID: b.CurrRoot}
	b.stage = StageChecks
	b.Assert = newAsserter(b, StageChecks)
}

// applyMessage executes the provided message via the driver, records the new
// root, refreshes the state tree, and updates the underlying vector with the
// message and its receipt.
func (b *Builder) applyMessage(am *ApplicableMessage) {
	var err error
	am.Result, b.CurrRoot, err = b.Driver.ExecuteMessage(am.Message, b.CurrRoot, b.Stores.Blockstore, am.Epoch)
	b.Assert.NoError(err)

	// replace the state tree.
	b.StateTree, err = state.LoadStateTree(b.Stores.CBORStore, b.CurrRoot)
	b.Assert.NoError(err)

	b.vector.ApplyMessages = append(b.vector.ApplyMessages, schema.Message{
		Bytes: MustSerialize(am.Message),
		Epoch: &am.Epoch,
	})
	b.vector.Post.Receipts = append(b.vector.Post.Receipts, &schema.Receipt{
		ExitCode:    am.Result.ExitCode,
		ReturnValue: am.Result.Return,
		GasUsed:     am.Result.GasUsed,
	})
}

func (b *Builder) Finish(w io.Writer) {
	if b.stage != StageChecks {
		panic("called Finish at the wrong time")
	}

	out := new(bytes.Buffer)
	gw := gzip.NewWriter(out)
	if err := b.WriteCAR(gw, b.vector.Pre.StateTree.RootCID, b.vector.Post.StateTree.RootCID); err != nil {
		panic(err)
	}
	if err := gw.Flush(); err != nil {
		panic(err)
	}
	if err := gw.Close(); err != nil {
		panic(err)
	}

	b.vector.CAR = out.Bytes()

	b.stage = StageFinished
	b.Assert = nil

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(b.vector); err != nil {
		panic(err)
	}
}

// WriteCAR recursively writes the tree referenced by the root as assert CAR into the
// supplied io.Writer.
//
// TODO use state.Surgeon instead. (This is assert copy of Surgeon#WriteCAR).
func (b *Builder) WriteCAR(w io.Writer, roots ...cid.Cid) error {
	carWalkFn := func(nd format.Node) (out []*format.Link, err error) {
		for _, link := range nd.Links() {
			if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
				continue
			}
			out = append(out, link)
		}
		return out, nil
	}

	return car.WriteCarWithWalker(context.Background(), b.Stores.DAGService, roots, w, carWalkFn)
}

func (b *Builder) FlushState() cid.Cid {
	preroot, err := b.StateTree.Flush(context.Background())
	if err != nil {
		panic(err)
	}
	return preroot
}
