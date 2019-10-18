package deals

import (
	"context"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
	"runtime"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	cbor "github.com/ipfs/go-ipld-cbor"
	unixfile "github.com/ipfs/go-unixfs/file"
	inet "github.com/libp2p/go-libp2p-core/network"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/lib/cborrpc"
)

func (c *Client) failDeal(id cid.Cid, cerr error) {
	if cerr == nil {
		_, f, l, _ := runtime.Caller(1)
		cerr = xerrors.Errorf("unknown error (fail called at %s:%d)", f, l)
	}

	s, ok := c.conns[id]
	if ok {
		_ = s.Reset()
		delete(c.conns, id)
	}

	// TODO: store in some sort of audit log
	log.Errorf("deal %s failed: %s", id, cerr)
}

func (c *Client) commP(ctx context.Context, data cid.Cid) ([]byte, int64, error) {
	root, err := c.dag.Get(ctx, data)
	if err != nil {
		log.Errorf("failed to get file root for deal: %s", err)
		return nil, 0, err
	}

	n, err := unixfile.NewUnixfsFile(ctx, c.dag, root)
	if err != nil {
		log.Errorf("cannot open unixfs file: %s", err)
		return nil, 0, err
	}

	uf, ok := n.(files.File)
	if !ok {
		// TODO: we probably got directory, how should we handle this in unixfs mode?
		return nil, 0, xerrors.New("unsupported unixfs type")
	}

	size, err := uf.Size()
	if err != nil {
		return nil, 0, err
	}

	commP, err := sectorbuilder.GeneratePieceCommitment(uf, uint64(size))
	if err != nil {
		return nil, 0, err
	}

	return commP[:], size, err
}

func (c *Client) sendProposal(s inet.Stream, proposal StorageDealProposal, from address.Address) error {
	log.Info("Sending deal proposal")

	msg, err := cbor.DumpObject(proposal)
	if err != nil {
		return err
	}
	sig, err := c.w.Sign(context.TODO(), from, msg)
	if err != nil {
		return err
	}

	signedProposal := &SignedStorageDealProposal{
		Proposal:  proposal,
		Signature: sig,
	}

	return cborrpc.WriteCborRPC(s, signedProposal)
}

func (c *Client) readStorageDealResp(deal ClientDeal) (*StorageDealResponse, error) {
	s, ok := c.conns[deal.ProposalCid]
	if !ok {
		// TODO: Try to re-establish the connection using query protocol
		return nil, xerrors.Errorf("no connection to miner")
	}

	var resp SignedStorageDealResponse
	if err := cborrpc.ReadCborRPC(s, &resp); err != nil {
		log.Errorw("failed to read StorageDealResponse message", "error", err)
		return nil, err
	}

	// TODO: verify signature

	if resp.Response.Proposal != deal.ProposalCid {
		return nil, xerrors.New("miner responded to a wrong proposal")
	}

	return &resp.Response, nil
}
