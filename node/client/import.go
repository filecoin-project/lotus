package client

import (
	"context"
	"github.com/filecoin-project/go-lotus/api"
	"github.com/ipfs/go-filestore"
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

	LocalDAG  ipld.DAGService
	Filestore *filestore.Filestore
}

func (s *LocalStorage) ClientImport(ctx context.Context, path string) (cid.Cid, error) {
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
		Dagserv:    bufferedDS,
		NoCopy:     true,
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

func (s *LocalStorage) ClientListImports(ctx context.Context) ([]api.Import, error) {
	next, err := filestore.ListAll(s.Filestore, false)
	if err != nil {
		return nil, err
	}

	// TODO: make this less very bad by tracking root cids instead of using ListAll

	out := make([]api.Import, 0)
	for {
		r := next()
		if r == nil {
			return out, nil
		}
		if r.Offset != 0 {
			continue
		}
		out = append(out, api.Import{
			Status:   r.Status,
			Key:      r.Key,
			FilePath: r.FilePath,
			Size:     r.Size,
		})
	}
}
