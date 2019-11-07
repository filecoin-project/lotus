package deals

import (
	"context"
	"runtime"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/lib/padreader"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
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
	log.Errorf("deal %s failed: %+v", id, cerr)
}

func (c *Client) commP(ctx context.Context, data cid.Cid) ([]byte, uint64, error) {
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

	s, err := uf.Size()
	if err != nil {
		return nil, 0, err
	}

	pr, psize := padreader.New(uf, uint64(s))

	commp, err := sectorbuilder.GeneratePieceCommitment(pr, psize)
	if err != nil {
		return nil, 0, xerrors.Errorf("generating CommP: %w", err)
	}

	return commp[:], psize, nil
}

func (c *Client) readStorageDealResp(deal ClientDeal) (*Response, error) {
	s, ok := c.conns[deal.ProposalCid]
	if !ok {
		// TODO: Try to re-establish the connection using query protocol
		return nil, xerrors.Errorf("no connection to miner")
	}

	var resp SignedResponse
	if err := cborrpc.ReadCborRPC(s, &resp); err != nil {
		log.Errorw("failed to read Response message", "error", err)
		return nil, err
	}

	// TODO: verify signature

	if resp.Response.Proposal != deal.ProposalCid {
		return nil, xerrors.Errorf("miner responded to a wrong proposal: %s != %s", resp.Response.Proposal, deal.ProposalCid)
	}

	return &resp.Response, nil
}

func (c *Client) disconnect(deal ClientDeal) error {
	s, ok := c.conns[deal.ProposalCid]
	if !ok {
		return nil
	}

	err := s.Close()
	delete(c.conns, deal.ProposalCid)
	return err
}
