module github.com/filecoin-project/oni/tvx

go 1.14

require (
	github.com/filecoin-project/go-address v0.0.3
	github.com/filecoin-project/go-state-types v0.0.0-20200911004822-964d6c679cfc
	github.com/filecoin-project/lotus v0.7.2-0.20200921170631-46ed0142db7a
	github.com/filecoin-project/specs-actors v0.9.10
	github.com/filecoin-project/test-vectors/schema v0.0.1
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.4-0.20200624145336-a978cec6e834
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipld/go-car v0.1.1-0.20200526133713-1c7508d55aae
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/multiformats/go-multiaddr-net v0.2.0
	github.com/urfave/cli/v2 v2.2.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20200826160007-0b9f6c5fb163
)

replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi

replace github.com/supranational/blst => github.com/supranational/blst v0.1.2-alpha.1
