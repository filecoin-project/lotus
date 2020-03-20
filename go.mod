module github.com/filecoin-project/lotus

go 1.13

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	contrib.go.opencensus.io/exporter/prometheus v0.1.0
	github.com/BurntSushi/toml v0.3.1
	github.com/GeertJohan/go.rice v1.0.0
	github.com/Gurpartap/async v0.0.0-20180927173644-4f7f499dd9ee
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/coreos/go-systemd/v22 v22.0.0
	github.com/docker/go-units v0.4.0
	github.com/filecoin-project/chain-validation v0.0.6-0.20200320210432-2793319e9867
	github.com/filecoin-project/filecoin-ffi v0.0.0-20200304181354-4446ff8a1bb9
	github.com/filecoin-project/go-address v0.0.2-0.20200218010043-eb9bb40ed5be
	github.com/filecoin-project/go-amt-ipld/v2 v2.0.1-0.20200131012142-05d80eeccc5e
	github.com/filecoin-project/go-cbor-util v0.0.0-20191219014500-08c40a1e63a2
	github.com/filecoin-project/go-crypto v0.0.0-20191218222705-effae4ea9f03
	github.com/filecoin-project/go-data-transfer v0.0.0-20191219005021-4accf56bd2ce
	github.com/filecoin-project/go-fil-commcid v0.0.0-20200208005934-2b8bd03caca5
	github.com/filecoin-project/go-fil-markets v0.0.0-20200318012938-6403a5bda668
	github.com/filecoin-project/go-padreader v0.0.0-20200210211231-548257017ca6
	github.com/filecoin-project/go-paramfetch v0.0.2-0.20200218225740-47c639bab663
	github.com/filecoin-project/go-sectorbuilder v0.0.2-0.20200314022627-38af9db49ba2
	github.com/filecoin-project/go-statemachine v0.0.0-20200226041606-2074af6d51d9
	github.com/filecoin-project/go-statestore v0.1.0
	github.com/filecoin-project/specs-actors v0.0.0-20200312030511-3f5510bf6130
	github.com/filecoin-project/specs-storage v0.0.0-20200303233430-1a5a408f7513
	github.com/gbrlsnchs/jwt/v3 v3.0.0-beta.1
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/websocket v1.4.1
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/influxdata/influxdb1-client v0.0.0-20190809212627-fc22c7df067e
	github.com/ipfs/go-bitswap v0.1.8
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.3-0.20190908200855-f22eea50656c
	github.com/ipfs/go-car v0.0.3-0.20200304012825-b6769248bfef
	github.com/ipfs/go-cid v0.0.5
	github.com/ipfs/go-datastore v0.4.2
	github.com/ipfs/go-ds-badger2 v0.0.0-20200211201106-609c9d2a39c7
	github.com/ipfs/go-filestore v0.0.2
	github.com/ipfs/go-fs-lock v0.0.1
	github.com/ipfs/go-graphsync v0.0.4
	github.com/ipfs/go-hamt-ipld v0.0.15-0.20200204200533-99b8553ef242
	github.com/ipfs/go-ipfs-blockstore v0.1.4
	github.com/ipfs/go-ipfs-chunker v0.0.1
	github.com/ipfs/go-ipfs-ds-help v0.1.1
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.4
	github.com/ipfs/go-ipfs-routing v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200204214505-252690b78669
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v1.0.2
	github.com/ipfs/go-log/v2 v2.0.2
	github.com/ipfs/go-merkledag v0.2.4
	github.com/ipfs/go-path v0.0.7
	github.com/ipfs/go-unixfs v0.2.2-0.20190827150610-868af2e9e5cb
	github.com/ipld/go-ipld-prime v0.0.2-0.20191108012745-28a82f04c785
	github.com/lib/pq v1.2.0
	github.com/libp2p/go-libp2p v0.5.2
	github.com/libp2p/go-libp2p-circuit v0.1.4
	github.com/libp2p/go-libp2p-connmgr v0.1.0
	github.com/libp2p/go-libp2p-core v0.3.0
	github.com/libp2p/go-libp2p-discovery v0.2.0
	github.com/libp2p/go-libp2p-kad-dht v0.1.1
	github.com/libp2p/go-libp2p-mplex v0.2.1
	github.com/libp2p/go-libp2p-peer v0.2.0
	github.com/libp2p/go-libp2p-peerstore v0.1.4
	github.com/libp2p/go-libp2p-pubsub v0.2.6
	github.com/libp2p/go-libp2p-quic-transport v0.1.1
	github.com/libp2p/go-libp2p-record v0.1.1
	github.com/libp2p/go-libp2p-routing-helpers v0.1.0
	github.com/libp2p/go-libp2p-secio v0.2.1
	github.com/libp2p/go-libp2p-swarm v0.2.2
	github.com/libp2p/go-libp2p-tls v0.1.0
	github.com/libp2p/go-libp2p-yamux v0.2.1
	github.com/libp2p/go-maddr-filter v0.0.5
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/minio/sha256-simd v0.1.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.2.0
	github.com/multiformats/go-multiaddr-dns v0.2.0
	github.com/multiformats/go-multiaddr-net v0.1.2
	github.com/multiformats/go-multihash v0.0.13
	github.com/opentracing/opentracing-go v1.1.0
	github.com/prometheus/common v0.4.0
	github.com/stretchr/testify v1.4.0
	github.com/whyrusleeping/bencher v0.0.0-20190829221104-bb6607aa8bba
	github.com/whyrusleeping/cbor-gen v0.0.0-20200222160900-51052a1e8191
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/whyrusleeping/pubsub v0.0.0-20131020042734-02de8aa2db3d
	go.opencensus.io v0.22.2
	go.uber.org/dig v1.8.0 // indirect
	go.uber.org/fx v1.9.0
	go.uber.org/goleak v1.0.0 // indirect
	go.uber.org/multierr v1.4.0
	go.uber.org/zap v1.13.0
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	gopkg.in/urfave/cli.v2 v2.0.0-20180128182452-d3ae77c26ac8
	gotest.tools v2.2.0+incompatible
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
)

replace github.com/golangci/golangci-lint => github.com/golangci/golangci-lint v1.18.0

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace github.com/coreos/go-systemd => github.com/coreos/go-systemd/v22 v22.0.0
