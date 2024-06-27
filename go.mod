module github.com/filecoin-project/lotus

go 1.21

retract v1.14.0 // Accidentally force-pushed tag, use v1.14.1+ instead.

retract v1.20.2 // Wrongfully cherry picked PR, use v1.20.2+ instead.

replace github.com/filecoin-project/test-vectors => ./extern/test-vectors // provided via a git submodule

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi // provided via a git submodule

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/BurntSushi/toml v1.3.0
	github.com/DataDog/zstd v1.4.5
	github.com/GeertJohan/go.rice v1.0.3
	github.com/Gurpartap/async v0.0.0-20180927173644-4f7f499dd9ee
	github.com/Kubuxu/imtui v0.0.0-20210401140320-41663d68d0fa
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d
	github.com/alecthomas/jsonschema v0.0.0-20200530073317-71f438968921
	github.com/chzyer/readline v1.5.1
	github.com/containerd/cgroups v1.1.0
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/detailyang/go-fallocate v0.0.0-20180908115635-432fa640bd2e
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/docker/go-units v0.5.0
	github.com/drand/drand v1.5.11
	github.com/drand/kyber v1.3.0
	github.com/dustin/go-humanize v1.0.1
	github.com/elastic/go-elasticsearch/v7 v7.14.0
	github.com/elastic/go-sysinfo v1.7.0
	github.com/elastic/gosigar v0.14.2
	github.com/etclabscore/go-openrpc-reflect v0.0.36
	github.com/fatih/color v1.15.0
	github.com/filecoin-project/filecoin-ffi v1.28.0-rc2
	github.com/filecoin-project/go-address v1.1.0
	github.com/filecoin-project/go-amt-ipld/v4 v4.3.0
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-cbor-util v0.0.1
	github.com/filecoin-project/go-commp-utils v0.1.3
	github.com/filecoin-project/go-commp-utils/nonffi v0.0.0-20220905160352-62059082a837
	github.com/filecoin-project/go-crypto v0.0.1
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-hamt-ipld/v3 v3.1.0
	github.com/filecoin-project/go-jsonrpc v0.3.2
	github.com/filecoin-project/go-padreader v0.0.1
	github.com/filecoin-project/go-paramfetch v0.0.4
	github.com/filecoin-project/go-state-types v0.14.0-rc5
	github.com/filecoin-project/go-statemachine v1.0.3
	github.com/filecoin-project/go-statestore v0.2.0
	github.com/filecoin-project/go-storedcounter v0.1.0
	github.com/filecoin-project/pubsub v1.0.0
	github.com/filecoin-project/specs-actors v0.9.15
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v3 v3.1.2
	github.com/filecoin-project/specs-actors/v4 v4.0.2
	github.com/filecoin-project/specs-actors/v5 v5.0.6
	github.com/filecoin-project/specs-actors/v6 v6.0.2
	github.com/filecoin-project/specs-actors/v7 v7.0.1
	github.com/filecoin-project/specs-actors/v8 v8.0.1
	github.com/filecoin-project/test-vectors/schema v0.0.7
	github.com/gbrlsnchs/jwt/v3 v3.0.1
	github.com/gdamore/tcell/v2 v2.2.0
	github.com/georgysavva/scany/v2 v2.1.3
	github.com/go-openapi/spec v0.19.11
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.1
	github.com/gregdhill/go-openrpc v0.0.0-20220114144539-ae6f44720487
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/arc/v2 v2.0.7
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/icza/backscanner v0.0.0-20210726202459-ac2ffc679f94
	github.com/influxdata/influxdb1-client v0.0.0-20200827194710-b269163b24ab
	github.com/invopop/jsonschema v0.12.0
	github.com/ipfs/bbloom v0.0.4
	github.com/ipfs/boxo v0.20.0
	github.com/ipfs/go-block-format v0.2.0
	github.com/ipfs/go-cid v0.4.1
	github.com/ipfs/go-cidutil v0.1.0
	github.com/ipfs/go-datastore v0.6.0
	github.com/ipfs/go-ds-badger2 v0.1.3
	github.com/ipfs/go-ds-leveldb v0.5.0
	github.com/ipfs/go-ds-measure v0.2.0
	github.com/ipfs/go-fs-lock v0.0.7
	github.com/ipfs/go-ipld-cbor v0.1.0
	github.com/ipfs/go-ipld-format v0.6.0
	github.com/ipfs/go-log/v2 v2.5.1
	github.com/ipfs/go-metrics-interface v0.0.1
	github.com/ipfs/go-metrics-prometheus v0.0.2
	github.com/ipld/go-car v0.6.2
	github.com/ipld/go-car/v2 v2.13.1
	github.com/ipld/go-ipld-prime v0.21.0
	github.com/ipni/go-libipni v0.0.8
	github.com/jackc/pgerrcode v0.0.0-20240316143900-6e2875d9b438
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/klauspost/compress v1.17.8
	github.com/koalacxr/quantile v0.0.1
	github.com/libp2p/go-buffer-pool v0.1.0
	github.com/libp2p/go-libp2p v0.34.1
	github.com/libp2p/go-libp2p-kad-dht v0.25.2
	github.com/libp2p/go-libp2p-pubsub v0.11.0
	github.com/libp2p/go-libp2p-record v0.2.0
	github.com/libp2p/go-libp2p-routing-helpers v0.7.3
	github.com/libp2p/go-maddr-filter v0.1.0
	github.com/libp2p/go-msgio v0.3.0
	github.com/manifoldco/promptui v0.9.0
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-sqlite3 v1.14.16
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-base32 v0.1.0
	github.com/multiformats/go-multiaddr v0.12.4
	github.com/multiformats/go-multiaddr-dns v0.3.1
	github.com/multiformats/go-multicodec v0.9.0
	github.com/multiformats/go-multihash v0.2.3
	github.com/multiformats/go-varint v0.0.7
	github.com/open-rpc/meta-schema v0.0.0-20201029221707-1b72ef2ea333
	github.com/polydawn/refmt v0.89.0
	github.com/prometheus/client_golang v1.19.1
	github.com/puzpuzpuz/xsync/v2 v2.4.0
	github.com/raulk/clock v1.1.0
	github.com/raulk/go-watchdog v1.3.0
	github.com/samber/lo v1.39.0
	github.com/stretchr/testify v1.9.0
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/triplewz/poseidon v0.0.0-20230828015038-79d8165c88ed
	github.com/urfave/cli/v2 v2.25.5
	github.com/whyrusleeping/bencher v0.0.0-20190829221104-bb6607aa8bba
	github.com/whyrusleeping/cbor-gen v0.1.1
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/xorcare/golden v0.6.1-0.20191112154924-b87f686d7542
	github.com/yugabyte/pgx/v5 v5.5.3-yb-2
	github.com/zondax/ledger-filecoin-go v0.11.1
	github.com/zyedidia/generic v1.2.1
	go.opencensus.io v0.24.0
	go.opentelemetry.io/otel v1.26.0
	go.opentelemetry.io/otel/bridge/opencensus v0.39.0
	go.opentelemetry.io/otel/exporters/jaeger v1.14.0
	go.opentelemetry.io/otel/sdk v1.26.0
	go.uber.org/atomic v1.11.0
	go.uber.org/fx v1.21.1
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.23.0
	golang.org/x/net v0.25.0
	golang.org/x/sync v0.7.0
	golang.org/x/sys v0.20.0
	golang.org/x/term v0.20.0
	golang.org/x/time v0.5.0
	golang.org/x/tools v0.21.0
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028
	gopkg.in/cheggaaa/pb.v1 v1.0.28
	gotest.tools v2.2.0+incompatible
)

require (
	github.com/GeertJohan/go.incremental v1.0.0 // indirect
	github.com/Jorropo/jsync v1.0.1 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/akavel/rsrc v0.8.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cilium/ebpf v0.9.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20231225121904-e25f5bc08668 // indirect
	github.com/cskr/pubsub v1.0.2 // indirect
	github.com/daaku/go.zipexe v1.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0 // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/drand/kyber-bls12381 v0.3.1 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/etclabscore/go-jsonschema-walk v0.0.6 // indirect
	github.com/filecoin-project/go-amt-ipld/v2 v2.1.0 // indirect
	github.com/filecoin-project/go-amt-ipld/v3 v3.1.0 // indirect
	github.com/filecoin-project/go-hamt-ipld v0.1.5 // indirect
	github.com/filecoin-project/go-hamt-ipld/v2 v2.0.0 // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/go-openapi/jsonpointer v0.19.3 // indirect
	github.com/go-openapi/jsonreference v0.19.4 // indirect
	github.com/go-openapi/swag v0.19.11 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/pprof v0.0.0-20240509144519-723abb6459b7 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/huin/goupnp v1.3.0 // indirect
	github.com/iancoleman/orderedmap v0.1.0 // indirect
	github.com/ipfs/go-blockservice v0.5.2 // indirect
	github.com/ipfs/go-ipfs-blockstore v1.3.1 // indirect
	github.com/ipfs/go-ipfs-delay v0.0.1 // indirect
	github.com/ipfs/go-ipfs-ds-help v1.1.1 // indirect
	github.com/ipfs/go-ipfs-exchange-interface v0.2.1 // indirect
	github.com/ipfs/go-ipfs-pq v0.0.3 // indirect
	github.com/ipfs/go-ipfs-util v0.0.3 // indirect
	github.com/ipfs/go-ipld-legacy v0.2.1 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-merkledag v0.11.0 // indirect
	github.com/ipfs/go-peertaskqueue v0.8.1 // indirect
	github.com/ipfs/go-verifcid v0.0.3 // indirect
	github.com/ipld/go-codec-dagpb v1.6.0 // indirect
	github.com/ipsn/go-secp256k1 v0.0.0-20180726113642-9d62b9f0bc52 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d80cb9 // indirect
	github.com/jackc/pgx/v5 v5.4.1 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/koron/go-ssdp v0.0.4 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.4.1 // indirect
	github.com/libp2p/go-libp2p-kbucket v0.6.3 // indirect
	github.com/libp2p/go-nat v0.2.0 // indirect
	github.com/libp2p/go-netroute v0.2.1 // indirect
	github.com/libp2p/go-reuseport v0.4.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magefile/mage v1.9.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/miekg/dns v1.1.59 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr-fmt v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/nikkolasg/hexjson v0.1.0 // indirect
	github.com/nkovacs/streamquote v1.0.0 // indirect
	github.com/onsi/ginkgo/v2 v2.17.3 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/petar/GoLLRB v0.0.0-20210522233825-ae3b015fd3e9 // indirect
	github.com/pion/datachannel v1.5.6 // indirect
	github.com/pion/dtls/v2 v2.2.11 // indirect
	github.com/pion/ice/v2 v2.3.24 // indirect
	github.com/pion/interceptor v0.1.29 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.12 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.14 // indirect
	github.com/pion/rtp v1.8.6 // indirect
	github.com/pion/sctp v1.8.16 // indirect
	github.com/pion/sdp/v3 v3.0.9 // indirect
	github.com/pion/srtp/v2 v2.0.18 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.5 // indirect
	github.com/pion/turn/v2 v2.1.6 // indirect
	github.com/pion/webrtc/v3 v3.2.40 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.53.0 // indirect
	github.com/prometheus/procfs v0.15.0 // indirect
	github.com/prometheus/statsd_exporter v0.22.7 // indirect
	github.com/quic-go/qpack v0.4.0 // indirect
	github.com/quic-go/quic-go v0.44.0 // indirect
	github.com/quic-go/webtransport-go v0.8.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil v2.18.12+incompatible // indirect
	github.com/sirupsen/logrus v1.9.2 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.0.1 // indirect
	github.com/whyrusleeping/cbor v0.0.0-20171005072247-63513f603b11 // indirect
	github.com/whyrusleeping/chunker v0.0.0-20181014151217-fe64bd25879f // indirect
	github.com/whyrusleeping/go-keyspace v0.0.0-20160322163242-5b898ac5add1 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/zondax/hid v0.9.2 // indirect
	github.com/zondax/ledger-go v0.14.3 // indirect
	go.opentelemetry.io/otel/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.26.0 // indirect
	go.uber.org/dig v1.17.1 // indirect
	go.uber.org/mock v0.4.0 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	gonum.org/v1/gonum v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240515191416-fc5f0ca64291 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)
