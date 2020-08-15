module github.com/filecoin-project/oni/tvx

go 1.14

require (
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200716204036-cddc56607e1d
	github.com/filecoin-project/go-address v0.0.3
	github.com/filecoin-project/go-bitfield v0.2.0
	github.com/filecoin-project/go-crypto v0.0.0-20191218222705-effae4ea9f03
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200716160307-8f644712406f
	github.com/filecoin-project/lotus v0.4.3-0.20200814191300-4a0171d26aa5
	github.com/filecoin-project/sector-storage v0.0.0-20200810171746-eac70842d8e0
	github.com/filecoin-project/specs-actors v0.9.2
	github.com/hashicorp/go-multierror v1.1.0
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.4-0.20200624145336-a978cec6e834
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipfs-blockstore v1.0.1
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipld/go-car v0.1.1-0.20200526133713-1c7508d55aae
	github.com/ipsn/go-secp256k1 v0.0.0-20180726113642-9d62b9f0bc52
	github.com/libp2p/go-libp2p-core v0.6.1
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/multiformats/go-varint v0.0.6
	github.com/stretchr/testify v1.6.1
	github.com/urfave/cli/v2 v2.2.0
	github.com/whyrusleeping/cbor-gen v0.0.0-20200812213548-958ddffe352c
)

replace github.com/filecoin-project/filecoin-ffi => ../extra/filecoin-ffi

replace github.com/filecoin-project/sector-storage => github.com/filecoin-project/lotus/extern/sector-storage v0.0.0-20200814191300-4a0171d26aa5

replace github.com/supranational/blst => github.com/supranational/blst v0.1.2-alpha.1
