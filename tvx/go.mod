module github.com/filecoin-project/oni/tvx

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/lotus v0.4.3-0.20200801235920-43491cb7edfd
	github.com/filecoin-project/sector-storage v0.0.0-20200730203805-7153e1dd05b5
	github.com/filecoin-project/specs-actors v0.8.6
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.4-0.20200624145336-a978cec6e834
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipld/go-car v0.1.1-0.20200526133713-1c7508d55aae
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/multiformats/go-multihash v0.0.14
	github.com/urfave/cli/v2 v2.2.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20200723185710-6a3894a6352b
)

replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi
