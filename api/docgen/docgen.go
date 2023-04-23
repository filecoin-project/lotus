package docgen

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-graphsync"
	blocks "github.com/ipfs/go-libipfs/blocks"
	textselector "github.com/ipld/go-ipld-selector-text-lite"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	datatransfer "github.com/filecoin-project/go-data-transfer/v2"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo/imports"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var ExampleValues = map[reflect.Type]interface{}{
	reflect.TypeOf(api.MinerSubsystem(0)): api.MinerSubsystem(1),
	reflect.TypeOf(auth.Permission("")):   auth.Permission("write"),
	reflect.TypeOf(""):                    "string value",
	reflect.TypeOf(uint64(42)):            uint64(42),
	reflect.TypeOf(byte(7)):               byte(7),
	reflect.TypeOf([]byte{}):              []byte("byte array"),
}

func addExample(v interface{}) {
	ExampleValues[reflect.TypeOf(v)] = v
}

func init() {
	c, err := cid.Decode("bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4")
	if err != nil {
		panic(err)
	}

	ExampleValues[reflect.TypeOf(c)] = c
	ExampleValues[reflect.TypeOf(&c)] = &c

	c2, err := cid.Decode("bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve")
	if err != nil {
		panic(err)
	}

	tsk := types.NewTipSetKey(c, c2)

	ExampleValues[reflect.TypeOf(tsk)] = tsk

	addr, err := address.NewIDAddress(1234)
	if err != nil {
		panic(err)
	}

	ExampleValues[reflect.TypeOf(addr)] = addr

	pid, err := peer.Decode("12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf")
	if err != nil {
		panic(err)
	}
	addExample(pid)
	addExample(&pid)

	storeIDExample := imports.ID(50)
	textSelExample := textselector.Expression("Links/21/Hash/Links/42/Hash")
	apiSelExample := api.Selector("Links/21/Hash/Links/42/Hash")
	clientEvent := retrievalmarket.ClientEventDealAccepted

	block := blocks.Block(&blocks.BasicBlock{})
	ExampleValues[reflect.TypeOf(&block).Elem()] = block

	addExample(bitfield.NewFromSet([]uint64{5}))
	addExample(abi.RegisteredSealProof_StackedDrg32GiBV1_1)
	addExample(abi.RegisteredPoStProof_StackedDrgWindow32GiBV1)
	addExample(abi.ChainEpoch(10101))
	addExample(crypto.SigTypeBLS)
	addExample(types.KTBLS)
	addExample(int64(9))
	addExample(12.3)
	addExample(123)
	addExample(uintptr(0))
	addExample(abi.MethodNum(1))
	addExample(exitcode.ExitCode(0))
	addExample(crypto.DomainSeparationTag_ElectionProofProduction)
	addExample(true)
	addExample(abi.UnpaddedPieceSize(1024))
	addExample(abi.UnpaddedPieceSize(1024).Padded())
	addExample(abi.DealID(5432))
	addExample(abi.SectorNumber(9))
	addExample(abi.SectorSize(32 * 1024 * 1024 * 1024))
	addExample(api.MpoolChange(0))
	addExample(network.Connected)
	addExample(dtypes.NetworkName("lotus"))
	addExample(api.SyncStateStage(1))
	addExample(api.FullAPIVersion1)
	addExample(api.PCHInbound)
	addExample(time.Minute)
	addExample(graphsync.NewRequestID())
	addExample(datatransfer.TransferID(3))
	addExample(datatransfer.Ongoing)
	addExample(storeIDExample)
	addExample(&storeIDExample)
	addExample(clientEvent)
	addExample(&clientEvent)
	addExample(retrievalmarket.ClientEventDealAccepted)
	addExample(retrievalmarket.DealStatusNew)
	addExample(&textSelExample)
	addExample(&apiSelExample)
	addExample(network.ReachabilityPublic)
	addExample(build.TestNetworkVersion)
	allocationId := verifreg.AllocationId(0)
	addExample(allocationId)
	addExample(&allocationId)
	addExample(map[verifreg.AllocationId]verifreg.Allocation{})
	claimId := verifreg.ClaimId(0)
	addExample(claimId)
	addExample(&claimId)
	addExample(map[verifreg.ClaimId]verifreg.Claim{})
	addExample(map[string]int{"name": 42})
	addExample(map[string]time.Time{"name": time.Unix(1615243938, 0).UTC()})
	addExample(&types.ExecutionTrace{
		Msg:    ExampleValue("init", reflect.TypeOf(types.MessageTrace{}), nil).(types.MessageTrace),
		MsgRct: ExampleValue("init", reflect.TypeOf(types.ReturnTrace{}), nil).(types.ReturnTrace),
	})
	addExample(map[string]types.Actor{
		"t01236": ExampleValue("init", reflect.TypeOf(types.Actor{}), nil).(types.Actor),
	})
	addExample(map[string]api.MarketDeal{
		"t026363": ExampleValue("init", reflect.TypeOf(api.MarketDeal{}), nil).(api.MarketDeal),
	})
	addExample(map[string]*api.MarketDeal{
		"t026363": ExampleValue("init", reflect.TypeOf(&api.MarketDeal{}), nil).(*api.MarketDeal),
	})

	addExample(map[string]api.MarketBalance{
		"t026363": ExampleValue("init", reflect.TypeOf(api.MarketBalance{}), nil).(api.MarketBalance),
	})
	addExample(map[string]*pubsub.TopicScoreSnapshot{
		"/blocks": {
			TimeInMesh:               time.Minute,
			FirstMessageDeliveries:   122,
			MeshMessageDeliveries:    1234,
			InvalidMessageDeliveries: 3,
		},
	})
	addExample(map[string]metrics.Stats{
		"12D3KooWSXmXLJmBR1M7i9RW9GQPNUhZSzXKzxDHWtAgNuJAbyEJ": {
			RateIn:   100,
			RateOut:  50,
			TotalIn:  174000,
			TotalOut: 12500,
		},
	})
	addExample(map[protocol.ID]metrics.Stats{
		"/fil/hello/1.0.0": {
			RateIn:   100,
			RateOut:  50,
			TotalIn:  174000,
			TotalOut: 12500,
		},
	})

	maddr, err := multiaddr.NewMultiaddr("/ip4/52.36.61.156/tcp/1347/p2p/12D3KooWFETiESTf1v4PGUvtnxMAcEFMzLZbJGg4tjWfGEimYior")
	if err != nil {
		panic(err)
	}

	// because reflect.TypeOf(maddr) returns the concrete type...
	ExampleValues[reflect.TypeOf(struct{ A multiaddr.Multiaddr }{}).Field(0).Type] = maddr

	// miner specific
	addExample(filestore.Path(".lotusminer/fstmp123"))
	si := uint64(12)
	addExample(&si)
	addExample(retrievalmarket.DealID(5))
	addExample(abi.ActorID(1000))
	addExample(map[string]cid.Cid{})
	addExample(map[string][]api.SealedRef{
		"98000": {
			api.SealedRef{
				SectorID: 100,
				Offset:   10 << 20,
				Size:     1 << 20,
			},
		},
	})
	addExample(api.SectorState(sealing.Proving))
	addExample(storiface.ID("76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8"))
	addExample(storiface.FTUnsealed)
	addExample(storiface.PathSealing)
	addExample(map[storiface.ID][]storiface.Decl{
		"76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8": {
			{
				SectorID:       abi.SectorID{Miner: 1000, Number: 100},
				SectorFileType: storiface.FTSealed,
			},
		},
	})
	addExample(map[storiface.ID]string{
		"76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8": "/data/path",
	})
	addExample(map[uuid.UUID][]storiface.WorkerJob{
		uuid.MustParse("ef8d99a2-6865-4189-8ffa-9fef0f806eee"): {
			{
				ID: storiface.CallID{
					Sector: abi.SectorID{Miner: 1000, Number: 100},
					ID:     uuid.MustParse("76081ba0-61bd-45a5-bc08-af05f1c26e5d"),
				},
				Sector:   abi.SectorID{Miner: 1000, Number: 100},
				Task:     sealtasks.TTPreCommit2,
				RunWait:  0,
				Start:    time.Unix(1605172927, 0).UTC(),
				Hostname: "host",
			},
		},
	})
	addExample(map[uuid.UUID]storiface.WorkerStats{
		uuid.MustParse("ef8d99a2-6865-4189-8ffa-9fef0f806eee"): {
			Info: storiface.WorkerInfo{
				Hostname: "host",
				Resources: storiface.WorkerResources{
					MemPhysical: 256 << 30,
					MemUsed:     2 << 30,
					MemSwap:     120 << 30,
					MemSwapUsed: 2 << 30,
					CPUs:        64,
					GPUs:        []string{"aGPU 1337"},
					Resources:   storiface.ResourceTable,
				},
			},
			Enabled:    true,
			MemUsedMin: 0,
			MemUsedMax: 0,
			GpuUsed:    0,
			CpuUse:     0,
		},
	})
	addExample(storiface.ErrorCode(0))
	addExample(map[abi.SectorNumber]string{
		123: "can't acquire read lock",
	})
	addExample(json.RawMessage(`"json raw message"`))
	addExample(map[api.SectorState]int{
		api.SectorState(sealing.Proving): 120,
	})
	addExample([]abi.SectorNumber{123, 124})
	addExample([]storiface.SectorLock{
		{
			Sector: abi.SectorID{Number: 123, Miner: 1000},
			Write:  [storiface.FileTypes]uint{0, 0, 1},
			Read:   [storiface.FileTypes]uint{2, 3, 0},
		},
	})
	storifaceid := storiface.ID("1399aa04-2625-44b1-bad4-bd07b59b22c4")
	addExample(&storifaceid)

	// worker specific
	addExample(storiface.AcquireMove)
	addExample(storiface.UnpaddedByteIndex(abi.PaddedPieceSize(1 << 20).Unpadded()))
	addExample(map[sealtasks.TaskType]struct{}{
		sealtasks.TTPreCommit2: {},
	})
	addExample(sealtasks.TTCommit2)
	addExample(apitypes.OpenRPCDocument{
		"openrpc": "1.2.6",
		"info": map[string]interface{}{
			"title":   "Lotus RPC API",
			"version": "1.2.1/generated=2020-11-22T08:22:42-06:00",
		},
		"methods": []interface{}{},
	},
	)

	addExample(api.CheckStatusCode(0))
	addExample(map[string]interface{}{"abc": 123})
	addExample(api.MinerSubsystems{
		api.SubsystemMining,
		api.SubsystemSealing,
		api.SubsystemSectorStorage,
		api.SubsystemMarkets,
	})
	addExample(api.DagstoreShardResult{
		Key:   "baga6ea4seaqecmtz7iak33dsfshi627abz4i4665dfuzr3qfs4bmad6dx3iigdq",
		Error: "<error>",
	})
	addExample(api.DagstoreShardInfo{
		Key:   "baga6ea4seaqecmtz7iak33dsfshi627abz4i4665dfuzr3qfs4bmad6dx3iigdq",
		State: "ShardStateAvailable",
		Error: "<error>",
	})
	addExample(storiface.ResourceTable)
	addExample(network.ScopeStat{
		Memory:             123,
		NumStreamsInbound:  1,
		NumStreamsOutbound: 2,
		NumConnsInbound:    3,
		NumConnsOutbound:   4,
		NumFD:              5,
	})
	addExample(map[string]network.ScopeStat{
		"abc": {
			Memory:             123,
			NumStreamsInbound:  1,
			NumStreamsOutbound: 2,
			NumConnsInbound:    3,
			NumConnsOutbound:   4,
			NumFD:              5,
		},
	})
	addExample(api.NetLimit{
		Memory:          123,
		StreamsInbound:  1,
		StreamsOutbound: 2,
		Streams:         3,
		ConnsInbound:    3,
		ConnsOutbound:   4,
		Conns:           4,
		FD:              5,
	})

	addExample(map[string]bitfield.BitField{
		"": bitfield.NewFromSet([]uint64{5, 6, 7, 10}),
	})
	addExample(&api.RaftStateData{
		NonceMap: make(map[address.Address]uint64),
		MsgUuids: make(map[uuid.UUID]*types.SignedMessage),
	})

	addExample(http.Header{
		"Authorization": []string{"Bearer ey.."},
	})

	addExample(map[storiface.SectorFileType]storiface.SectorLocation{
		storiface.FTSealed: {
			Local:   false,
			URL:     "https://example.com/sealingservice/sectors/s-f0123-12345",
			Headers: nil,
		},
	})

	ethint := ethtypes.EthUint64(5)
	addExample(ethint)
	addExample(&ethint)

	ethaddr, _ := ethtypes.ParseEthAddress("0x5CbEeCF99d3fDB3f25E309Cc264f240bb0664031")
	addExample(ethaddr)
	addExample(&ethaddr)

	ethhash, _ := ethtypes.EthHashFromCid(c)
	addExample(ethhash)
	addExample(&ethhash)

	ethFeeHistoryReward := [][]ethtypes.EthBigInt{}
	addExample(&ethFeeHistoryReward)

	addExample(&uuid.UUID{})

	filterid := ethtypes.EthFilterID(ethhash)
	addExample(filterid)
	addExample(&filterid)

	subid := ethtypes.EthSubscriptionID(ethhash)
	addExample(subid)
	addExample(&subid)

	pstring := func(s string) *string { return &s }
	addExample(&ethtypes.EthFilterSpec{
		FromBlock: pstring("2301220"),
		Address:   []ethtypes.EthAddress{ethaddr},
	})

	percent := types.Percent(123)
	addExample(percent)
	addExample(&percent)
}

func GetAPIType(name, pkg string) (i interface{}, t reflect.Type, permStruct []reflect.Type) {
	switch pkg {
	case "api": // latest
		switch name {
		case "FullNode":
			i = &api.FullNodeStruct{}
			t = reflect.TypeOf(new(struct{ api.FullNode })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(api.FullNodeStruct{}.Internal))
			permStruct = append(permStruct, reflect.TypeOf(api.CommonStruct{}.Internal))
			permStruct = append(permStruct, reflect.TypeOf(api.NetStruct{}.Internal))
		case "StorageMiner":
			i = &api.StorageMinerStruct{}
			t = reflect.TypeOf(new(struct{ api.StorageMiner })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(api.StorageMinerStruct{}.Internal))
			permStruct = append(permStruct, reflect.TypeOf(api.CommonStruct{}.Internal))
			permStruct = append(permStruct, reflect.TypeOf(api.NetStruct{}.Internal))
		case "Worker":
			i = &api.WorkerStruct{}
			t = reflect.TypeOf(new(struct{ api.Worker })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(api.WorkerStruct{}.Internal))
		case "Gateway":
			i = &api.GatewayStruct{}
			t = reflect.TypeOf(new(struct{ api.Gateway })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(api.GatewayStruct{}.Internal))
		default:
			panic("unknown type")
		}
	case "v0api":
		switch name {
		case "FullNode":
			i = v0api.FullNodeStruct{}
			t = reflect.TypeOf(new(struct{ v0api.FullNode })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(v0api.FullNodeStruct{}.Internal))
			permStruct = append(permStruct, reflect.TypeOf(v0api.CommonStruct{}.Internal))
			permStruct = append(permStruct, reflect.TypeOf(v0api.NetStruct{}.Internal))
		default:
			panic("unknown type")
		}
	}
	return
}

func ExampleValue(method string, t, parent reflect.Type) interface{} {
	v, ok := ExampleValues[t]
	if ok {
		return v
	}

	switch t.Kind() {
	case reflect.Slice:
		out := reflect.New(t).Elem()
		out = reflect.Append(out, reflect.ValueOf(ExampleValue(method, t.Elem(), t)))
		return out.Interface()
	case reflect.Chan:
		return ExampleValue(method, t.Elem(), nil)
	case reflect.Struct:
		es := exampleStruct(method, t, parent)
		v := reflect.ValueOf(es).Elem().Interface()
		ExampleValues[t] = v
		return v
	case reflect.Array:
		out := reflect.New(t).Elem()
		for i := 0; i < t.Len(); i++ {
			out.Index(i).Set(reflect.ValueOf(ExampleValue(method, t.Elem(), t)))
		}
		return out.Interface()

	case reflect.Ptr:
		if t.Elem().Kind() == reflect.Struct {
			es := exampleStruct(method, t.Elem(), t)
			ExampleValues[t] = es
			return es
		}
	case reflect.Interface:
		return struct{}{}
	}

	panic(fmt.Sprintf("No example value for type: %s (method '%s')", t, method))
}

func exampleStruct(method string, t, parent reflect.Type) interface{} {
	ns := reflect.New(t)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type == parent {
			continue
		}

		if f.IsExported() {
			ns.Elem().Field(i).Set(reflect.ValueOf(ExampleValue(method, f.Type, t)))
		}
	}

	return ns.Interface()
}

type Visitor struct {
	Root    string
	Methods map[string]ast.Node
}

func (v *Visitor) Visit(node ast.Node) ast.Visitor {
	st, ok := node.(*ast.TypeSpec)
	if !ok {
		return v
	}

	if st.Name.Name != v.Root {
		return nil
	}

	iface := st.Type.(*ast.InterfaceType)
	for _, m := range iface.Methods.List {
		if len(m.Names) > 0 {
			v.Methods[m.Names[0].Name] = m
		}
	}

	return v
}

const NoComment = "There are not yet any comments for this method."

func ParseApiASTInfo(apiFile, iface, pkg, dir string) (comments map[string]string, groupDocs map[string]string) { //nolint:golint
	fset := token.NewFileSet()
	apiDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Println("./api filepath absolute error: ", err)
		return
	}
	apiFile, err = filepath.Abs(apiFile)
	if err != nil {
		fmt.Println("filepath absolute error: ", err, "file:", apiFile)
		return
	}
	pkgs, err := parser.ParseDir(fset, apiDir, nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		fmt.Println("parse error: ", err)
		return
	}

	ap := pkgs[pkg]

	f := ap.Files[apiFile]

	cmap := ast.NewCommentMap(fset, f, f.Comments)

	v := &Visitor{iface, make(map[string]ast.Node)}
	ast.Walk(v, ap)

	comments = make(map[string]string)
	groupDocs = make(map[string]string)
	for mn, node := range v.Methods {
		filteredComments := cmap.Filter(node).Comments()
		if len(filteredComments) == 0 {
			comments[mn] = NoComment
		} else {
			for _, c := range filteredComments {
				if strings.HasPrefix(c.Text(), "MethodGroup:") {
					parts := strings.Split(c.Text(), "\n")
					groupName := strings.TrimSpace(parts[0][12:])
					comment := strings.Join(parts[1:], "\n")
					groupDocs[groupName] = comment

					break
				}
			}

			l := len(filteredComments) - 1
			if len(filteredComments) > 1 {
				l = len(filteredComments) - 2
			}
			last := filteredComments[l].Text()
			if !strings.HasPrefix(last, "MethodGroup:") {
				comments[mn] = last
			} else {
				comments[mn] = NoComment
			}
		}
	}
	return comments, groupDocs
}

type MethodGroup struct {
	GroupName string
	Header    string
	Methods   []*Method
}

type Method struct {
	Comment         string
	Name            string
	InputExample    string
	ResponseExample string
}

func MethodGroupFromName(mn string) string {
	i := strings.IndexFunc(mn[1:], func(r rune) bool {
		return unicode.IsUpper(r)
	})
	if i < 0 {
		return ""
	}
	return mn[:i+1]
}
