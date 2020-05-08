module github.com/filecoin-project/lotus

go 1.13

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/BurntSushi/toml v0.3.1
	github.com/GeertJohan/go.rice v1.0.0
	github.com/Gurpartap/async v0.0.0-20180927173644-4f7f499dd9ee
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/davidlazar/go-crypto v0.0.0-20190912175916-7055855a373f // indirect
	github.com/docker/go-units v0.4.0
	github.com/drand/drand v0.8.1
	github.com/fatih/color v1.8.0
	github.com/filecoin-project/chain-validation v0.0.6-0.20200506234205-5fe7d4aab7f9
	github.com/filecoin-project/filecoin-ffi v0.0.0-20200427223233-a0014b17f124
	github.com/filecoin-project/go-address v0.0.2-0.20200504173055-8b6f2fb2b3ef
	github.com/filecoin-project/go-amt-ipld/v2 v2.0.1-0.20200424220931-6263827e49f2
	github.com/filecoin-project/go-bitfield v0.0.0-20200416002808-b3ee67ec9060
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-crypto v0.0.0-20191218222705-effae4ea9f03
	github.com/filecoin-project/go-data-transfer v0.3.0
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200208005934-2b8bd03caca5
	github.com/filecoin-project/go-fil-markets v0.2.1
	github.com/filecoin-project/go-padreader v0.0.0-20200210211231-548257017ca6
	github.com/filecoin-project/go-paramfetch v0.0.2-0.20200505180321-973f8949ea8e
	github.com/filecoin-project/go-statestore v0.1.0
	github.com/filecoin-project/go-storedcounter v0.0.0-20200421200003-1c99c62e8a5b
	github.com/filecoin-project/sector-storage v0.0.0-20200508170218-e721b8910cbb
	github.com/filecoin-project/specs-actors v0.4.0
	github.com/filecoin-project/specs-storage v0.0.0-20200417134612-61b2d91a6102
	github.com/filecoin-project/storage-fsm v0.0.0-20200427182014-01487d5ad3c8
	github.com/gbrlsnchs/jwt/v3 v3.0.0-beta.1
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.4
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/influxdata/influxdb1-client v0.0.0-20190809212627-fc22c7df067e
	github.com/ipfs/go-bitswap v0.2.8
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.3
	github.com/ipfs/go-cid v0.0.6-0.20200501230655-7c82f3b81c00
	github.com/ipfs/go-datastore v0.4.4
	github.com/ipfs/go-ds-badger2 v0.1.0
	github.com/ipfs/go-filestore v1.0.0
	github.com/ipfs/go-fs-lock v0.0.1
	github.com/ipfs/go-graphsync v0.0.6-0.20200504202014-9d5f2c26a103
	github.com/ipfs/go-hamt-ipld v0.1.1-0.20200501020327-d53d20a7063e
	github.com/ipfs/go-ipfs-blockstore v1.0.0
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.0.0
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.8
	github.com/ipfs/go-ipfs-http-client v0.0.5
	github.com/ipfs/go-ipfs-routing v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-log v1.0.4
	github.com/ipfs/go-log/v2 v2.0.5
	github.com/ipfs/go-merkledag v0.3.1
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-unixfs v0.2.4
	github.com/ipfs/interface-go-ipfs-core v0.2.3
	github.com/ipld/go-car v0.1.1-0.20200430185908-8ff2e52a4c88
	github.com/ipld/go-ipld-prime v0.0.2-0.20200428162820-8b59dc292b8e
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/lib/pq v1.2.0
	github.com/libp2p/go-eventbus v0.1.0
	github.com/libp2p/go-libp2p v0.8.1
	github.com/libp2p/go-libp2p-circuit v0.2.1
	github.com/libp2p/go-libp2p-connmgr v0.1.1
	github.com/libp2p/go-libp2p-core v0.5.3
	github.com/libp2p/go-libp2p-discovery v0.4.0
	github.com/libp2p/go-libp2p-kad-dht v0.7.6
	github.com/libp2p/go-libp2p-mplex v0.2.3
	github.com/libp2p/go-libp2p-peer v0.2.0
	github.com/libp2p/go-libp2p-peerstore v0.2.3
	github.com/libp2p/go-libp2p-protocol v0.1.0
	github.com/libp2p/go-libp2p-pubsub v0.2.7-0.20200505181014-5bbe37191afb
	github.com/libp2p/go-libp2p-quic-transport v0.1.1
	github.com/libp2p/go-libp2p-record v0.1.2
	github.com/libp2p/go-libp2p-routing-helpers v0.2.1
	github.com/libp2p/go-libp2p-secio v0.2.2
	github.com/libp2p/go-libp2p-swarm v0.2.3
	github.com/libp2p/go-libp2p-tls v0.1.3
	github.com/libp2p/go-libp2p-yamux v0.2.7
	github.com/libp2p/go-maddr-filter v0.0.5
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.2.1
	github.com/multiformats/go-multiaddr-dns v0.2.0
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/multiformats/go-multihash v0.0.13
	github.com/opentracing/opentracing-go v1.1.0
	github.com/stretchr/testify v1.5.1
	github.com/whyrusleeping/bencher v0.0.0-20190829221104-bb6607aa8bba
	github.com/whyrusleeping/cbor-gen v0.0.0-20200504204219-64967432584d
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/whyrusleeping/pubsub v0.0.0-20131020042734-02de8aa2db3d
	go.opencensus.io v0.22.3
	go.uber.org/dig v1.8.0 // indirect
	go.uber.org/fx v1.9.0
	go.uber.org/multierr v1.5.0
	go.uber.org/zap v1.14.1
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a // indirect
	golang.org/x/sys v0.0.0-20200427175716-29b57079015a
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	gopkg.in/urfave/cli.v2 v2.0.0-20180128182452-d3ae77c26ac8
	gotest.tools v2.2.0+incompatible
)

replace github.com/golangci/golangci-lint => github.com/golangci/golangci-lint v1.18.0

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace github.com/coreos/go-systemd => github.com/coreos/go-systemd/v22 v22.0.0
