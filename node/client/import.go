package client

import (
	"context"
	"go.uber.org/fx"
	"os"

	"github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
)

type LocalStorage struct {
	fx.In

	LocalDAG ipld.DAGService
}

func (s *LocalStorage) ClientImport(ctx context.Context, path string) (cid.Cid, error)  {
	f, err := os.Open(path)
	if err != nil {
		return cid.Undef, err
	}
	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, err
	}

	file, err := files.NewReaderPathFile(path, f, stat)
	if err != nil {
		return cid.Undef, err
	}

	bufferedDS := ipld.NewBufferedDAG(ctx, s.LocalDAG)

	params := ihelper.DagBuilderParams{
		Maxlinks:   ihelper.DefaultLinksPerBlock,
		RawLeaves:  true,
		CidBuilder: nil,
		Dagserv:    bufferedDS, // flush?
		NoCopy:     false,
	}

	db, err := params.New(chunker.DefaultSplitter(file))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), bufferedDS.Commit()
}
