module github.com/filecoin-project/lotus

go 1.24.7

retract v1.14.0 // Accidentally force-pushed tag, use v1.14.1+ instead.

retract v1.20.2 // Wrongfully cherry picked PR, use v1.20.2+ instead.

retract v1.28.0 // misses some bug fixes and features, use v1.28.1+ instead

replace github.com/filecoin-project/test-vectors => ./extern/test-vectors // provided via a git submodule

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi // provided via a git submodule

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/BurntSushi/toml v1.5.0
	github.com/GeertJohan/go.rice v1.0.3
	github.com/Gurpartap/async v0.0.0-20180927173644-4f7f499dd9ee
	github.com/Kubuxu/imtui v0.0.0-20210401140320-41663d68d0fa
	github.com/Masterminds/semver/v3 v3.4.0
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d
	github.com/alecthomas/jsonschema v0.0.0-20200530073317-71f438968921
	github.com/cheggaaa/pb/v3 v3.1.7
	github.com/chzyer/readline v1.5.1
	github.com/consensys/gnark-crypto v0.19.0
	github.com/containerd/cgroups v1.1.0
	github.com/coreos/go-systemd/v22 v22.6.0
	github.com/detailyang/go-fallocate v0.0.0-20180908115635-432fa640bd2e
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/docker/go-units v0.5.0
	github.com/drand/go-clients v0.2.3
	github.com/drand/kyber v1.3.1
	github.com/dustin/go-humanize v1.0.1
	github.com/elastic/go-elasticsearch/v7 v7.17.10
	github.com/elastic/go-sysinfo v1.15.4
	github.com/elastic/gosigar v0.14.3
	github.com/etclabscore/go-openrpc-reflect v0.0.37
	github.com/fatih/color v1.18.0
	github.com/filecoin-project/filecoin-ffi v1.34.0
	github.com/filecoin-project/go-address v1.2.0
	github.com/filecoin-project/go-amt-ipld/v4 v4.4.0
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-cbor-util v0.0.2
	github.com/filecoin-project/go-commp-utils/v2 v2.1.0
	github.com/filecoin-project/go-crypto v0.1.0
	github.com/filecoin-project/go-f3 v0.8.10
	github.com/filecoin-project/go-fil-commcid v0.3.1
	github.com/filecoin-project/go-hamt-ipld/v3 v3.4.1
	github.com/filecoin-project/go-jsonrpc v0.9.0
	github.com/filecoin-project/go-padreader v0.0.1
	github.com/filecoin-project/go-paramfetch v0.0.4
	github.com/filecoin-project/go-state-types v0.18.0-dev // dependency-check-ignore: unknown
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
	github.com/georgysavva/scany/v2 v2.1.4
	github.com/go-openapi/spec v0.19.11
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.7.0
	github.com/google/go-github/v66 v66.0.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/gregdhill/go-openrpc v0.0.0-20220114144539-ae6f44720487
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/hannahhoward/go-pubsub v1.0.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/arc/v2 v2.0.7
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/icza/backscanner v0.0.0-20210726202459-ac2ffc679f94
	github.com/influxdata/influxdb1-client v0.0.0-20200827194710-b269163b24ab
	github.com/invopop/jsonschema v0.13.0
	github.com/ipfs/bbloom v0.0.4
	github.com/ipfs/boxo v0.35.0
	github.com/ipfs/go-block-format v0.2.3
	github.com/ipfs/go-cid v0.5.0
	github.com/ipfs/go-datastore v0.9.0
	github.com/ipfs/go-ds-badger2 v0.1.5
	github.com/ipfs/go-ds-leveldb v0.5.2
	github.com/ipfs/go-ds-measure v0.2.2
	github.com/ipfs/go-fs-lock v0.1.1
	github.com/ipfs/go-ipld-cbor v0.2.1
	github.com/ipfs/go-ipld-format v0.6.3
	github.com/ipfs/go-log/v2 v2.8.2
	github.com/ipfs/go-metrics-interface v0.3.0
	github.com/ipfs/go-metrics-prometheus v0.1.0
	github.com/ipld/go-car v0.6.2
	github.com/ipld/go-car/v2 v2.15.0
	github.com/ipld/go-ipld-prime v0.21.0
	github.com/jackc/pgerrcode v0.0.0-20240316143900-6e2875d9b438
	github.com/jpillora/backoff v1.0.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/klauspost/compress v1.18.0
	github.com/koalacxr/quantile v0.0.1
	github.com/libp2p/go-buffer-pool v0.1.0
	github.com/libp2p/go-libp2p v0.44.0
	github.com/libp2p/go-libp2p-kad-dht v0.35.1
	github.com/libp2p/go-libp2p-pubsub v0.15.0
	github.com/libp2p/go-libp2p-record v0.3.1
	github.com/libp2p/go-libp2p-routing-helpers v0.7.5
	github.com/libp2p/go-maddr-filter v0.1.0
	github.com/libp2p/go-msgio v0.3.0
	github.com/manifoldco/promptui v0.9.0
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-base32 v0.1.0
	github.com/multiformats/go-multiaddr v0.16.1
	github.com/multiformats/go-multiaddr-dns v0.4.1
	github.com/multiformats/go-multicodec v0.10.0
	github.com/multiformats/go-multihash v0.2.3
	github.com/multiformats/go-varint v0.1.0
	github.com/open-rpc/meta-schema v0.0.0-20201029221707-1b72ef2ea333
	github.com/polydawn/refmt v0.89.0
	github.com/prometheus/client_golang v1.23.2
	github.com/puzpuzpuz/xsync/v2 v2.5.1
	github.com/raulk/clock v1.1.0
	github.com/raulk/go-watchdog v1.3.0
	github.com/samber/lo v1.47.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.11.1
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7 // dependency-check-ignore: unknown
	github.com/triplewz/poseidon v0.0.2
	github.com/urfave/cli/v2 v2.27.7
	github.com/whyrusleeping/bencher v0.0.0-20190829221104-bb6607aa8bba
	github.com/whyrusleeping/cbor-gen v0.3.1
	github.com/whyrusleeping/multiaddr-filter v0.0.0-20160516205228-e903e4adabd7
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/xorcare/golden v0.6.1-0.20191112154924-b87f686d7542 // dependency-check-ignore: unknown
	github.com/yugabyte/pgx/v5 v5.5.3-yb-5 // dependency-check-ignore: unknown
	github.com/zondax/ledger-filecoin-go v1.2.0
	github.com/zyedidia/generic v1.2.1
	go.opencensus.io v0.24.0
	go.opentelemetry.io/otel v1.38.0
	go.opentelemetry.io/otel/bridge/opencensus v1.28.0
	go.opentelemetry.io/otel/exporters/jaeger v1.14.0
	go.opentelemetry.io/otel/exporters/prometheus v0.50.0
	go.opentelemetry.io/otel/metric v1.38.0
	go.opentelemetry.io/otel/sdk v1.38.0
	go.opentelemetry.io/otel/sdk/metric v1.38.0
	go.uber.org/fx v1.24.0
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.43.0
	golang.org/x/mod v0.28.0
	golang.org/x/net v0.45.0
	golang.org/x/sync v0.17.0
	golang.org/x/sys v0.37.0
	golang.org/x/term v0.36.0
	golang.org/x/time v0.14.0
	golang.org/x/tools v0.37.0
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da
	gotest.tools v2.2.0+incompatible
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/GeertJohan/go.incremental v1.0.0 // indirect
	github.com/Jorropo/jsync v1.0.1 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/akavel/rsrc v0.8.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.20.0 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cilium/ebpf v0.9.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/cskr/pubsub v1.0.2 // indirect
	github.com/daaku/go.zipexe v1.0.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect; dependency-check-ignore: needed by github.com/drand/go-clients@v0.2.2
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dgraph-io/ristretto v0.2.0 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/drand/drand/v2 v2.1.3
	github.com/drand/kyber-bls12381 v0.3.3 // indirect
	github.com/elastic/go-windows v1.0.2 // indirect
	github.com/etclabscore/go-jsonschema-walk v0.0.6 // indirect
	github.com/filecoin-project/go-amt-ipld/v2 v2.1.0 // indirect
	github.com/filecoin-project/go-amt-ipld/v3 v3.1.0 // indirect
	github.com/filecoin-project/go-clock v0.1.0 // indirect
	github.com/filecoin-project/go-fil-commp-hashhash v0.2.0 // indirect
	github.com/filecoin-project/go-hamt-ipld v0.1.5 // indirect
	github.com/filecoin-project/go-hamt-ipld/v2 v2.0.0 // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.3 // indirect
	github.com/go-openapi/jsonreference v0.19.4 // indirect
	github.com/go-openapi/swag v0.19.11 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/huin/goupnp v1.3.0 // indirect
	github.com/iancoleman/orderedmap v0.1.0 // indirect
	github.com/ipfs/go-blockservice v0.5.2 // indirect
	github.com/ipfs/go-ipfs-blockstore v1.3.1 // indirect
	github.com/ipfs/go-ipfs-ds-help v1.1.1 // indirect
	github.com/ipfs/go-ipfs-exchange-interface v0.2.1 // indirect
	github.com/ipfs/go-ipfs-pq v0.0.3 // indirect
	github.com/ipfs/go-ipfs-util v0.0.3 // indirect
	github.com/ipfs/go-ipld-legacy v0.2.2 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-merkledag v0.11.0 // indirect
	github.com/ipfs/go-peertaskqueue v0.8.2 // indirect
	github.com/ipfs/go-verifcid v0.0.3 // indirect
	github.com/ipld/go-codec-dagpb v1.7.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/jbenet/go-temp-err-catcher v0.1.0 // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kilic/bls12-381 v0.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/koron/go-ssdp v0.0.6 // indirect
	github.com/libp2p/go-cidranger v1.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.3.0 // indirect
	github.com/libp2p/go-libp2p-asn-util v0.4.1 // indirect
	github.com/libp2p/go-libp2p-kbucket v0.8.0 // indirect
	github.com/libp2p/go-netroute v0.3.0 // indirect
	github.com/libp2p/go-reuseport v0.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magefile/mage v1.9.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/marten-seemann/tcp v0.0.0-20210406111302-dfbc87cc63fd // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/miekg/dns v1.1.68 // indirect
	github.com/mikioh/tcpinfo v0.0.0-20190314235526-30a79bb1804b // indirect
	github.com/mikioh/tcpopt v0.0.0-20190314235656-172688c1accc // indirect
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr-fmt v0.1.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multistream v0.6.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nikkolasg/hexjson v0.1.0 // indirect
	github.com/nkovacs/streamquote v1.0.0 // indirect
	github.com/opencontainers/runtime-spec v1.2.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/petar/GoLLRB v0.0.0-20210522233825-ae3b015fd3e9 // indirect
	github.com/pion/datachannel v1.5.10 // indirect
	github.com/pion/dtls/v2 v2.2.12 // indirect
	github.com/pion/interceptor v0.1.40 // indirect
	github.com/pion/logging v0.2.3 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.8.19 // indirect
	github.com/pion/sctp v1.8.39 // indirect
	github.com/pion/sdp/v3 v3.0.13 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.10 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect; dependency-check-ignore: needed by github.com/drand/go-clients@v0.2.2
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/prometheus/statsd_exporter v0.22.7 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/webtransport-go v0.9.0 // indirect; dependency-check-ignore: unknown
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/smartystreets/assertions v1.13.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/cast v1.9.2 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.0.1 // indirect
	github.com/whyrusleeping/cbor v0.0.0-20171005072247-63513f603b11 // indirect
	github.com/whyrusleeping/go-keyspace v0.0.0-20160322163242-5b898ac5add1 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/zondax/hid v0.9.2 // indirect
	github.com/zondax/ledger-go v1.0.1 // indirect
	gitlab.com/yawning/secp256k1-voi v0.0.0-20230925100816-f2616030848b // indirect
	gitlab.com/yawning/tuplehash v0.0.0-20230713102510-df83abbf9a02 // indirect
	go.dedis.ch/fixbuf v1.0.3 // indirect
	go.dedis.ch/kyber/v4 v4.0.0-pre2.0.20240924132404-4de33740016e // indirect; dependency-check-ignore: unknown
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/mock v0.5.2 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/exp v0.0.0-20250911091902-df9299821621 // indirect
	golang.org/x/text v0.30.0 // indirect
	gonum.org/v1/gonum v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/grpc v1.75.0 // indirect
	google.golang.org/protobuf v1.36.9 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.28 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb // indirect; dependency-check-ignore: required by github.com/elastic/go-sysinfo
	lukechampine.com/blake3 v1.4.1 // indirect
)

require (
	github.com/gammazero/chanqueue v1.1.1 // indirect
	github.com/gammazero/deque v1.1.0 // indirect
	github.com/ipfs/go-cidutil v0.1.0 // indirect
	github.com/ipfs/go-dsqueue v0.0.5 // indirect
	github.com/libp2p/go-yamux/v5 v5.0.1 // indirect
	github.com/onsi/gomega v1.36.3 // indirect
	github.com/pion/dtls/v3 v3.0.6 // indirect
	github.com/pion/ice/v4 v4.0.10 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/srtp/v3 v3.0.6 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.0.2 // indirect
	github.com/pion/webrtc/v4 v4.1.2 // indirect
	github.com/quic-go/quic-go v0.55.0 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zondax/golem v0.27.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/telemetry v0.0.0-20250908211612-aef8a434d053 // indirect
)
