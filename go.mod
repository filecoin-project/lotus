module github.com/filecoin-project/lotus

go 1.18

retract v1.14.0 // Accidentally force-pushed tag, use v1.14.1+ instead.

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.0
	github.com/BurntSushi/toml v1.1.0
	github.com/DataDog/zstd v1.4.5
	github.com/GeertJohan/go.rice v1.0.3
	github.com/Gurpartap/async v0.0.0-20180927173644-4f7f499dd9ee
	github.com/Kubuxu/imtui v0.0.0-20210401140320-41663d68d0fa
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d
	github.com/alecthomas/jsonschema v0.0.0-20200530073317-71f438968921
	github.com/buger/goterm v1.0.3
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/containerd/cgroups v1.0.4
	github.com/coreos/go-systemd/v22 v22.4.0
	github.com/detailyang/go-fallocate v0.0.0-20180908115635-432fa640bd2e
	github.com/dgraph-io/badger/v2 v2.2007.3
	github.com/docker/go-units v0.5.0
	github.com/drand/drand v1.3.0
	github.com/drand/kyber v1.1.7
	github.com/dustin/go-humanize v1.0.0
	github.com/elastic/go-elasticsearch/v7 v7.14.0
	github.com/elastic/go-sysinfo v1.7.0
	github.com/elastic/gosigar v0.14.2
	github.com/etclabscore/go-openrpc-reflect v0.0.36
	github.com/fatih/color v1.13.0
	github.com/filecoin-project/dagstore v0.5.2
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v1.1.0
	github.com/filecoin-project/go-amt-ipld/v4 v4.0.0
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-cbor-util v0.0.1
	github.com/filecoin-project/go-commp-utils v0.1.3
	github.com/filecoin-project/go-crypto v0.0.1
	github.com/filecoin-project/go-data-transfer v1.15.2
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-fil-commp-hashhash v0.1.0
	github.com/filecoin-project/go-fil-markets v1.25.2
	github.com/filecoin-project/go-jsonrpc v0.2.0
	github.com/filecoin-project/go-legs v0.4.4
	github.com/filecoin-project/go-padreader v0.0.1
	github.com/filecoin-project/go-paramfetch v0.0.4
	github.com/filecoin-project/go-state-types v0.10.0-alpha-11
	github.com/filecoin-project/go-statemachine v1.0.2
	github.com/filecoin-project/go-statestore v0.2.0
	github.com/filecoin-project/go-storedcounter v0.1.0
	github.com/filecoin-project/index-provider v0.9.1
	github.com/filecoin-project/pubsub v1.0.0
	github.com/filecoin-project/specs-actors v0.9.15
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v3 v3.1.2
	github.com/filecoin-project/specs-actors/v4 v4.0.2
	github.com/filecoin-project/specs-actors/v5 v5.0.6
	github.com/filecoin-project/specs-actors/v6 v6.0.2
	github.com/filecoin-project/specs-actors/v7 v7.0.1
	github.com/filecoin-project/specs-actors/v8 v8.0.1
	github.com/filecoin-project/test-vectors/schema v0.0.5
	github.com/gbrlsnchs/jwt/v3 v3.0.1
	github.com/gdamore/tcell/v2 v2.2.0
	github.com/go-kit/kit v0.12.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru v0.5.4
	github.com/hashicorp/raft v1.1.1
	github.com/hashicorp/raft-boltdb v0.0.0-20171010151810-6e5ba93211ea
	github.com/icza/backscanner v0.0.0-20210726202459-ac2ffc679f94
	github.com/influxdata/influxdb1-client v0.0.0-20200827194710-b269163b24ab
	github.com/ipfs/bbloom v0.0.4
	github.com/ipfs/go-bitswap v0.10.2
	github.com/ipfs/go-block-format v0.0.3
	github.com/ipfs/go-blockservice v0.4.0
	github.com/ipfs/go-cid v0.3.2
	github.com/ipfs/go-cidutil v0.1.0
	github.com/ipfs/go-datastore v0.6.0
	github.com/ipfs/go-ds-badger2 v0.1.2
	github.com/ipfs/go-ds-leveldb v0.5.0
	github.com/ipfs/go-ds-measure v0.2.0
	github.com/ipfs/go-fs-lock v0.0.7
	github.com/ipfs/go-graphsync v0.13.2
	github.com/ipfs/go-ipfs-blockstore v1.2.0
	github.com/ipfs/go-ipfs-blocksutil v0.0.1
	github.com/ipfs/go-ipfs-chunker v0.0.5
	github.com/ipfs/go-ipfs-ds-help v1.1.0
	github.com/ipfs/go-ipfs-exchange-interface v0.2.0
	github.com/ipfs/go-ipfs-exchange-offline v0.3.0
	github.com/ipfs/go-ipfs-files v0.1.1
	github.com/ipfs/go-ipfs-http-client v0.4.0
	github.com/ipfs/go-ipfs-routing v0.2.1
	github.com/ipfs/go-ipfs-util v0.0.2
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/ipfs/go-ipld-format v0.4.0
	github.com/ipfs/go-log/v2 v2.5.1
	github.com/ipfs/go-merkledag v0.8.1
	github.com/ipfs/go-metrics-interface v0.0.1
	github.com/ipfs/go-metrics-prometheus v0.0.2
	github.com/ipfs/go-unixfs v0.4.0
	github.com/ipfs/go-unixfsnode v1.4.0
	github.com/ipfs/interface-go-ipfs-core v0.7.0
	github.com/ipld/go-car v0.4.0
	github.com/ipld/go-car/v2 v2.5.0
	github.com/ipld/go-codec-dagpb v1.5.0
	github.com/ipld/go-ipld-prime v0.18.0
	github.com/ipld/go-ipld-selector-text-lite v0.0.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/koalacxr/quantile v0.0.1
	github.com/libp2p/go-buffer-pool v0.1.0
	github.com/libp2p/go-libp2p v0.23.2
	github.com/libp2p/go-libp2p-consensus v0.0.1
	github.com/libp2p/go-libp2p-gorpc v0.4.0
	github.com/libp2p/go-libp2p-kad-dht v0.18.0
	github.com/libp2p/go-libp2p-pubsub v0.8.2
	github.com/libp2p/go-libp2p-raft v0.1.8
	github.com/libp2p/go-libp2p-record v0.2.0
	github.com/libp2p/go-libp2p-routing-helpers v0.2.3
	github.com/libp2p/go-maddr-filter v0.1.0
	github.com/libp2p/go-msgio v0.2.0
	github.com/mattn/go-isatty v0.0.16
	github.com/mattn/go-sqlite3 v1.14.16
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-base32 v0.1.0
	github.com/multiformats/go-multiaddr v0.7.0
	github.com/multiformats/go-multiaddr-dns v0.3.1
	github.com/multiformats/go-multibase v0.1.1
	github.com/multiformats/go-multihash v0.2.1
	github.com/multiformats/go-varint v0.0.6
	github.com/open-rpc/meta-schema v0.0.0-20201029221707-1b72ef2ea333
	github.com/polydawn/refmt v0.0.0-20201211092308-30ac6d18308e
	github.com/prometheus/client_golang v1.13.0
	github.com/raulk/clock v1.1.0
	github.com/raulk/go-watchdog v1.3.0
	github.com/stretchr/testify v1.8.0
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/urfave/cli/v2 v2.16.3
	github.com/whyrusleeping/bencher v0.0.0-20190829221104-bb6607aa8bba
	github.com/whyrusleeping/cbor-gen v0.0.0-20221021053955-c138aae13722
	github.com/whyrusleeping/ledger-filecoin-go v0.9.1-0.20201010031517-c3dcc1bddce4
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/xorcare/golden v0.6.1-0.20191112154924-b87f686d7542
	go.opencensus.io v0.23.0
	go.opentelemetry.io/otel v1.11.1
	go.opentelemetry.io/otel/bridge/opencensus v0.33.0
	go.opentelemetry.io/otel/exporters/jaeger v1.2.0
	go.opentelemetry.io/otel/sdk v1.11.1
	go.uber.org/fx v1.15.0
	go.uber.org/multierr v1.8.0
	go.uber.org/zap v1.23.0
	golang.org/x/crypto v0.0.0-20220829220503-c86fa9a7ed90
	golang.org/x/exp v0.0.0-20220916125017-b168a2c6b86b
	golang.org/x/net v0.0.0-20220920183852-bf014ff85ad5
	golang.org/x/sync v0.0.0-20220907140024-f12130a52804
	golang.org/x/sys v0.0.0-20220919091848-fb04ddd9f9c8
	golang.org/x/time v0.0.0-20220722155302-e5dcc9cfc0b9
	golang.org/x/tools v0.1.12
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gotest.tools v2.2.0+incompatible
)

require (
	github.com/GeertJohan/go.incremental v1.0.0 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/Stebalien/go-bitfield v0.0.1 // indirect
	github.com/akavel/rsrc v0.8.0 // indirect
	github.com/alecthomas/units v0.0.0-20210927113745-59d0afb8317a // indirect
	github.com/armon/go-metrics v0.3.9 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bep/debounce v1.2.1 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cilium/ebpf v0.4.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20170627025303-887ab5e44cc3 // indirect
	github.com/cskr/pubsub v1.0.2 // indirect
	github.com/daaku/go.zipexe v1.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dgryski/go-farm v0.0.0-20190423205320-6a90982ecee2 // indirect
	github.com/drand/kyber-bls12381 v0.2.1 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/etclabscore/go-jsonschema-walk v0.0.6 // indirect
	github.com/filecoin-project/go-amt-ipld/v2 v2.1.0 // indirect
	github.com/filecoin-project/go-amt-ipld/v3 v3.1.0 // indirect
	github.com/filecoin-project/go-commp-utils/nonffi v0.0.0-20220905160352-62059082a837 // indirect
	github.com/filecoin-project/go-ds-versioning v0.1.2 // indirect
	github.com/filecoin-project/go-hamt-ipld v0.1.5 // indirect
	github.com/filecoin-project/go-hamt-ipld/v2 v2.0.0 // indirect
	github.com/filecoin-project/go-hamt-ipld/v3 v3.1.0 // indirect
	github.com/filecoin-project/storetheindex v0.4.30-0.20221114113647-683091f8e893 // indirect
	github.com/flynn/noise v1.0.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/go-kit/log v0.2.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/go-openapi/jsonpointer v0.19.3 // indirect
	github.com/go-openapi/jsonreference v0.19.4 // indirect
	github.com/go-openapi/spec v0.19.11 // indirect
	github.com/go-openapi/swag v0.19.11 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/hannahhoward/cbor-gen-for v0.0.0-20200817222906-ea96cece81f1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v0.16.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/huin/goupnp v1.0.3 // indirect
	github.com/iancoleman/orderedmap v0.1.0 // indirect
	github.com/ipfs/go-bitfield v1.0.0 // indirect
	github.com/ipfs/go-filestore v1.2.0 // indirect
	github.com/ipfs/go-ipfs-cmds v0.7.0 // indirect
	github.com/ipfs/go-ipfs-config v0.18.0 // indirect
	github.com/ipfs/go-ipfs-delay v0.0.1 // indirect
	github.com/ipfs/go-ipfs-posinfo v0.0.1 // indirect
	github.com/ipfs/go-ipfs-pq v0.0.2 // indirect
	github.com/ipfs/go-ipld-legacy v0.1.1 // indirect
	github.com/ipfs/go-ipns v0.3.0 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-path v0.3.0 // indirect
	github.com/ipfs/go-peertaskqueue v0.8.0 // indirect
	github.com/ipfs/go-verifcid v0.0.2 // indirect
	github.com/ipld/go-ipld-adl-hamt v0.0.0-20220616142416-9004dbd839e0 // indirect
	github.com/ipsn/go-secp256k1 v0.0.0-20180726113642-9d62b9f0bc52 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/kilic/bls12-381 v0.0.0-20200820230200-6b2c19996391 // indirect
	github.com/klauspost/compress v1.15.10 // indirect
	github.com/klauspost/cpuid/v2 v2.1.1 // indirect
	github.com/koron/go-ssdp v0.0.3 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.2.0 // indirect
	github.com/libp2p/go-libp2p-connmgr v0.4.0 // indirect
	github.com/libp2p/go-libp2p-core v0.20.1 // indirect
	github.com/libp2p/go-libp2p-gostream v0.5.0 // indirect
	github.com/libp2p/go-libp2p-kbucket v0.5.0 // indirect
	github.com/libp2p/go-libp2p-noise v0.5.0 // indirect
	github.com/libp2p/go-libp2p-peerstore v0.8.0 // indirect
	github.com/libp2p/go-libp2p-tls v0.5.0 // indirect
	github.com/libp2p/go-nat v0.1.0 // indirect
	github.com/libp2p/go-netroute v0.2.0 // indirect
	github.com/libp2p/go-openssl v0.1.0 // indirect
	github.com/libp2p/go-reuseport v0.2.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.0 // indirect
	github.com/lucas-clemente/quic-go v0.29.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.0.3 // indirect
	github.com/magefile/mage v1.9.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.2 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.0 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.10 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base36 v0.1.0 // indirect
	github.com/multiformats/go-multiaddr-fmt v0.1.0 // indirect
	github.com/multiformats/go-multicodec v0.6.0 // indirect
	github.com/multiformats/go-multistream v0.3.3 // indirect
	github.com/nikkolasg/hexjson v0.0.0-20181101101858-78e39397e00c // indirect
	github.com/nkovacs/streamquote v1.0.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/petar/GoLLRB v0.0.0-20210522233825-ae3b015fd3e9 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/rivo/uniseg v0.1.0 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil v2.18.12+incompatible // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spacemonkeygo/spacelog v0.0.0-20180420211403-2296661a0572 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/twmb/murmur3 v1.1.6 // indirect
	github.com/uber/jaeger-client-go v2.25.0+incompatible // indirect
	github.com/ugorji/go/codec v1.2.6 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.0.1 // indirect
	github.com/whyrusleeping/cbor v0.0.0-20171005072247-63513f603b11 // indirect
	github.com/whyrusleeping/chunker v0.0.0-20181014151217-fe64bd25879f // indirect
	github.com/whyrusleeping/go-keyspace v0.0.0-20160322163242-5b898ac5add1 // indirect
	github.com/whyrusleeping/timecache v0.0.0-20160911033111-cfcb2f1abfee // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/zondax/hid v0.9.1 // indirect
	github.com/zondax/ledger-go v0.12.1 // indirect
	go.opentelemetry.io/otel/metric v0.33.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.33.0 // indirect
	go.opentelemetry.io/otel/trace v1.11.1 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/dig v1.12.0 // indirect
	go4.org v0.0.0-20200411211856-f5505b9728dd // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20210917145530-b395a37504d4 // indirect
	google.golang.org/grpc v1.45.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb // indirect
	lukechampine.com/blake3 v1.1.7 // indirect
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace github.com/filecoin-project/test-vectors => ./extern/test-vectors
