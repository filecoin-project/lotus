module github.com/filecoin-project/go-lotus

go 1.12

require (
	contrib.go.opencensus.io/exporter/jaeger v0.1.0
	github.com/BurntSushi/toml v0.3.1
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4 // indirect
	github.com/filecoin-project/go-bls-sigs v0.0.0-20190718224239-4bc4b8a7bbf8
	github.com/filecoin-project/go-leb128 v0.0.0-20190212224330-8d79a5489543
	github.com/filecoin-project/go-sectorbuilder v0.0.0-00010101000000-000000000000
	github.com/gbrlsnchs/jwt/v3 v3.0.0-beta.1
	github.com/gorilla/websocket v1.4.0
	github.com/ipfs/go-bitswap v0.1.5
	github.com/ipfs/go-block-format v0.0.2
	github.com/ipfs/go-blockservice v0.1.2
	github.com/ipfs/go-car v0.0.1
	github.com/ipfs/go-cid v0.0.3
	github.com/ipfs/go-datastore v0.0.5
	github.com/ipfs/go-ds-badger v0.0.5
	github.com/ipfs/go-filestore v0.0.2
	github.com/ipfs/go-fs-lock v0.0.1
	github.com/ipfs/go-hamt-ipld v0.0.10
	github.com/ipfs/go-ipfs-blockstore v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.1
	github.com/ipfs/go-ipfs-exchange-interface v0.0.1
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipfs-files v0.0.3
	github.com/ipfs/go-ipfs-routing v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.3
	github.com/ipfs/go-ipld-format v0.0.2
	github.com/ipfs/go-log v0.0.2-0.20190708183747-9c9fd6111bea
	github.com/ipfs/go-merkledag v0.1.0
	github.com/ipfs/go-unixfs v0.1.0
	github.com/ipsn/go-secp256k1 v0.0.0-20180726113642-9d62b9f0bc52
	github.com/libp2p/go-eventbus v0.0.3 // indirect
	github.com/libp2p/go-libp2p v0.2.0
	github.com/libp2p/go-libp2p-circuit v0.1.0
	github.com/libp2p/go-libp2p-connmgr v0.1.0
	github.com/libp2p/go-libp2p-core v0.0.6
	github.com/libp2p/go-libp2p-discovery v0.1.0
	github.com/libp2p/go-libp2p-kad-dht v0.1.1
	github.com/libp2p/go-libp2p-mplex v0.2.1
	github.com/libp2p/go-libp2p-peerstore v0.1.2-0.20190621130618-cfa9bb890c1a
	github.com/libp2p/go-libp2p-pnet v0.1.0
	github.com/libp2p/go-libp2p-pubsub v0.1.0
	github.com/libp2p/go-libp2p-quic-transport v0.1.1
	github.com/libp2p/go-libp2p-record v0.1.0
	github.com/libp2p/go-libp2p-routing-helpers v0.1.0
	github.com/libp2p/go-libp2p-secio v0.1.0
	github.com/libp2p/go-libp2p-swarm v0.1.1 // indirect
	github.com/libp2p/go-libp2p-tls v0.1.0
	github.com/libp2p/go-libp2p-yamux v0.2.1
	github.com/libp2p/go-maddr-filter v0.0.5
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.0.4
	github.com/multiformats/go-multiaddr-dns v0.0.3
	github.com/multiformats/go-multiaddr-net v0.0.1
	github.com/multiformats/go-multihash v0.0.6
	github.com/pkg/errors v0.8.1
	github.com/polydawn/refmt v0.0.0-20190731040541-eff0b363297a
	github.com/prometheus/common v0.6.0
	github.com/smartystreets/assertions v1.0.1 // indirect
	github.com/smartystreets/goconvey v0.0.0-20190710185942-9d28bd7c0945 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/whyrusleeping/pubsub v0.0.0-20131020042734-02de8aa2db3d
	github.com/whyrusleeping/sharray v0.0.0-20190718051354-e41931821e33
	go.opencensus.io v0.22.0
	go.uber.org/dig v1.7.0 // indirect
	go.uber.org/fx v1.9.0
	go.uber.org/goleak v0.10.0 // indirect
	go4.org v0.0.0-20190313082347-94abd6928b1d // indirect
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4 // indirect
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80 // indirect
	golang.org/x/sync v0.0.0-20190423024810-112230192c58 // indirect
	golang.org/x/sys v0.0.0-20190730183949-1393eb018365 // indirect
	golang.org/x/xerrors v0.0.0-20190717185122-a985d3407aa7
	gopkg.in/urfave/cli.v2 v2.0.0-20180128182452-d3ae77c26ac8
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
)

replace github.com/filecoin-project/go-bls-sigs => ./extern/go-bls-sigs

replace github.com/filecoin-project/go-sectorbuilder => ./extern/go-sectorbuilder
