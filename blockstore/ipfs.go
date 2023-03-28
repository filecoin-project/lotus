package blockstore

import (
	"bytes"
	"context"
	"io/ioutil"

	"github.com/ipfs/go-cid"
	httpapi "github.com/ipfs/go-ipfs-http-client"
	blocks "github.com/ipfs/go-libipfs/blocks"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

type IPFSBlockstore struct {
	ctx             context.Context
	api, offlineAPI iface.CoreAPI
}

var _ BasicBlockstore = (*IPFSBlockstore)(nil)

func NewLocalIPFSBlockstore(ctx context.Context, onlineMode bool) (Blockstore, error) {
	localApi, err := httpapi.NewLocalApi()
	if err != nil {
		return nil, xerrors.Errorf("getting local ipfs api: %w", err)
	}
	api, err := localApi.WithOptions(options.Api.Offline(!onlineMode))
	if err != nil {
		return nil, xerrors.Errorf("setting offline mode: %s", err)
	}

	offlineAPI := api
	if onlineMode {
		offlineAPI, err = localApi.WithOptions(options.Api.Offline(true))
		if err != nil {
			return nil, xerrors.Errorf("applying offline mode: %s", err)
		}
	}

	bs := &IPFSBlockstore{
		ctx:        ctx,
		api:        api,
		offlineAPI: offlineAPI,
	}

	return Adapt(bs), nil
}

func NewRemoteIPFSBlockstore(ctx context.Context, maddr multiaddr.Multiaddr, onlineMode bool) (Blockstore, error) {
	httpApi, err := httpapi.NewApi(maddr)
	if err != nil {
		return nil, xerrors.Errorf("setting remote ipfs api: %w", err)
	}
	api, err := httpApi.WithOptions(options.Api.Offline(!onlineMode))
	if err != nil {
		return nil, xerrors.Errorf("applying offline mode: %s", err)
	}

	offlineAPI := api
	if onlineMode {
		offlineAPI, err = httpApi.WithOptions(options.Api.Offline(true))
		if err != nil {
			return nil, xerrors.Errorf("applying offline mode: %s", err)
		}
	}

	bs := &IPFSBlockstore{
		ctx:        ctx,
		api:        api,
		offlineAPI: offlineAPI,
	}

	return Adapt(bs), nil
}

func (i *IPFSBlockstore) DeleteBlock(ctx context.Context, cid cid.Cid) error {
	return xerrors.Errorf("not supported")
}

func (i *IPFSBlockstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	_, err := i.offlineAPI.Block().Stat(ctx, path.IpldPath(cid))
	if err != nil {
		// The underlying client is running in Offline mode.
		// Stat() will fail with an err if the block isn't in the
		// blockstore. If that's the case, return false without
		// an error since that's the original intention of this method.
		if err.Error() == "blockservice: key not found" {
			return false, nil
		}
		return false, xerrors.Errorf("getting ipfs block: %w", err)
	}

	return true, nil
}

func (i *IPFSBlockstore) Get(ctx context.Context, cid cid.Cid) (blocks.Block, error) {
	rd, err := i.api.Block().Get(ctx, path.IpldPath(cid))
	if err != nil {
		return nil, xerrors.Errorf("getting ipfs block: %w", err)
	}

	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	return blocks.NewBlockWithCid(data, cid)
}

func (i *IPFSBlockstore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	st, err := i.api.Block().Stat(ctx, path.IpldPath(cid))
	if err != nil {
		return 0, xerrors.Errorf("getting ipfs block: %w", err)
	}

	return st.Size(), nil
}

func (i *IPFSBlockstore) Put(ctx context.Context, block blocks.Block) error {
	mhd, err := multihash.Decode(block.Cid().Hash())
	if err != nil {
		return err
	}

	_, err = i.api.Block().Put(ctx, bytes.NewReader(block.RawData()),
		options.Block.Hash(mhd.Code, mhd.Length),
		options.Block.Format(multihash.Codes[block.Cid().Type()]))
	return err
}

func (i *IPFSBlockstore) PutMany(ctx context.Context, blocks []blocks.Block) error {
	// TODO: could be done in parallel

	for _, block := range blocks {
		if err := i.Put(ctx, block); err != nil {
			return err
		}
	}

	return nil
}

func (i *IPFSBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, xerrors.Errorf("not supported")
}

func (i *IPFSBlockstore) HashOnRead(enabled bool) {
	return // TODO: We could technically support this, but..
}
