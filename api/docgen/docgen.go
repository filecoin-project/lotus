package docgen

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-f3/manifest"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var ExampleValues = map[reflect.Type]interface{}{
	reflect.TypeFor[api.MinerSubsystem](): api.MinerSubsystem(1),
	reflect.TypeFor[auth.Permission]():    auth.Permission("write"),
	reflect.TypeFor[string]():             "string value",
	reflect.TypeFor[uint64]():             uint64(42),
	reflect.TypeFor[byte]():               byte(7),
	reflect.TypeFor[[]byte]():             []byte("byte array"),
}

func addExample(v interface{}) {
	ExampleValues[reflect.TypeOf(v)] = v
}

func init() {
	c, err := cid.Decode("bafy2bzacea3wsdh6y3a36tb3skempjoxqpuyompjbmfeyf34fi3uy6uue42v4")
	if err != nil {
		panic(err)
	}

	ExampleValues[reflect.TypeFor[cid.Cid]()] = c
	ExampleValues[reflect.TypeFor[*cid.Cid]()] = &c

	c2, err := cid.Decode("bafy2bzacebp3shtrn43k7g3unredz7fxn4gj533d3o43tqn2p2ipxxhrvchve")
	if err != nil {
		panic(err)
	}

	tsk := types.NewTipSetKey(c, c2)

	ExampleValues[reflect.TypeFor[types.TipSetKey]()] = tsk

	addr, err := address.NewIDAddress(1234)
	if err != nil {
		panic(err)
	}

	ExampleValues[reflect.TypeFor[address.Address]()] = addr
	ExampleValues[reflect.TypeFor[*address.Address]()] = &addr

	var ts types.TipSet
	err = json.Unmarshal(tipsetSampleJson, &ts)
	if err != nil {
		panic(err)
	}
	addExample(&ts)

	pid, err := peer.Decode("12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXtf")
	if err != nil {
		panic(err)
	}
	addExample(pid)
	addExample(&pid)

	f3Lease := api.F3ParticipationLease{
		Network:      "filecoin",
		Issuer:       pid.String(),
		MinerID:      1234,
		FromInstance: 10,
		ValidityTerm: 15,
	}
	addExample(f3Lease)
	addExample(&f3Lease)

	ecchain := &gpbft.ECChain{
		TipSets: []*gpbft.TipSet{
			{
				Epoch:      0,
				Key:        tsk.Bytes(),
				PowerTable: c,
			},
		},
	}
	f3Cert := certs.FinalityCertificate{
		GPBFTInstance: 0,
		ECChain:       ecchain,
		SupplementalData: gpbft.SupplementalData{
			PowerTable: c,
		},
		Signers:   bitfield.NewFromSet([]uint64{2, 3, 5, 7}),
		Signature: []byte("UnDadaSeA"),
		PowerTableDelta: []certs.PowerTableDelta{
			{
				ParticipantID: 0,
				PowerDelta:    gpbft.StoragePower{},
				SigningKey:    []byte("BaRrelEYe"),
			},
		},
	}
	addExample(f3Cert)
	addExample(&f3Cert)

	block := blocks.Block(&blocks.BasicBlock{})
	ExampleValues[reflect.TypeFor[blocks.Block]()] = block
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
	addExample(gpbft.INITIAL_PHASE)

	addExample(network.ReachabilityPublic)
	addExample(buildconstants.TestNetworkVersion)
	allocationId := verifreg.AllocationId(0)
	addExample(allocationId)
	addExample(&allocationId)
	addExample(miner.SectorOnChainInfoFlags(0))
	addExample(map[verifreg.AllocationId]verifreg.Allocation{})
	claimId := verifreg.ClaimId(0)
	addExample(claimId)
	addExample(&claimId)
	addExample(map[verifreg.ClaimId]verifreg.Claim{})
	addExample(map[string]int{"name": 42})
	addExample(map[string]time.Time{"name": time.Unix(1615243938, 0).UTC()})
	addExample(abi.ActorID(1000))
	addExample(map[string]types.Actor{"t01236": ExampleValue("init", reflect.TypeFor[types.Actor](), nil).(types.Actor)})
	addExample(types.IpldOpGet)
	addExample(&types.TraceIpld{
		Op:   types.IpldOpGet,
		Cid:  c,
		Size: 123,
	})
	addExample(&types.ExecutionTrace{
		Msg:    ExampleValue("init", reflect.TypeOf(types.MessageTrace{}), nil).(types.MessageTrace),
		MsgRct: ExampleValue("init", reflect.TypeOf(types.ReturnTrace{}), nil).(types.ReturnTrace),
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

	si := uint64(12)
	addExample(&si)
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

	after := ethtypes.EthUint64(0)
	count := ethtypes.EthUint64(100)

	ethTraceFilterCriteria := ethtypes.EthTraceFilterCriteria{
		FromBlock:   pstring("latest"),
		ToBlock:     pstring("latest"),
		FromAddress: ethtypes.EthAddressList{ethaddr},
		ToAddress:   ethtypes.EthAddressList{ethaddr},
		After:       &after,
		Count:       &count,
	}
	addExample(&ethTraceFilterCriteria)
	addExample(ethTraceFilterCriteria)

	percent := types.Percent(123)
	addExample(percent)
	addExample(&percent)

	addExample(&miner.PieceActivationManifest{
		CID:                   c,
		Size:                  2032,
		VerifiedAllocationKey: nil,
		Notify:                nil,
	})

	addExample(&types.ActorEventBlock{
		Codec: 0x51,
		Value: []byte("ddata"),
	})

	addExample(&types.ActorEventFilter{
		Addresses: []address.Address{addr},
		Fields: map[string][]types.ActorEventBlock{
			"abc": {
				{
					Codec: 0x51,
					Value: []byte("ddata"),
				},
			},
		},
		FromHeight: epochPtr(1010),
		ToHeight:   epochPtr(1020),
	})
	addExample(&manifest.Manifest{})
	addExample(gpbft.NetworkName("filecoin"))
	addExample(gpbft.ActorID(1000))
	addExample(gpbft.InstanceProgress{
		Instant: gpbft.Instant{
			ID:    1413,
			Round: 1,
			Phase: gpbft.COMMIT_PHASE,
		},
		Input: ecchain,
	})
	addExample(types.TipSetSelectors.Finalized)
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
		case "Gateway":
			i = &v0api.GatewayStruct{}
			t = reflect.TypeOf(new(struct{ v0api.Gateway })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(v0api.GatewayStruct{}.Internal))
		default:
			panic("unknown type")
		}
	case "v2api":
		switch name {
		case "FullNode":
			i = &v2api.FullNodeStruct{}
			t = reflect.TypeOf(new(struct{ v2api.FullNode })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(v2api.FullNodeStruct{}.Internal))
		case "Gateway":
			i = &v2api.GatewayStruct{}
			t = reflect.TypeOf(new(struct{ v2api.Gateway })).Elem()
			permStruct = append(permStruct, reflect.TypeOf(v2api.GatewayStruct{}.Internal))
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
		} else if t.Elem().Kind() == reflect.String {
			str := "string value"
			return &str
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

func epochPtr(ei int64) *abi.ChainEpoch {
	ep := abi.ChainEpoch(ei)
	return &ep
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

type ApiASTInfo struct {
	Comments      map[string]string
	GroupComments map[string]string
}

func Generate(out io.Writer, iface, pkg string, ainfo ApiASTInfo) error {

	groups := make(map[string]*MethodGroup)

	_, t, permStruct := GetAPIType(iface, pkg)

	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)

		groupName := MethodGroupFromName(m.Name)

		g, ok := groups[groupName]
		if !ok {
			g = new(MethodGroup)
			g.Header = ainfo.GroupComments[groupName]
			g.GroupName = groupName
			groups[groupName] = g
		}

		var args []interface{}
		ft := m.Func.Type()
		for j := 2; j < ft.NumIn(); j++ {
			inp := ft.In(j)
			args = append(args, ExampleValue(m.Name, inp, nil))
		}

		v, err := json.MarshalIndent(args, "", "  ")
		if err != nil {
			panic(err)
		}

		outv := ExampleValue(m.Name, ft.Out(0), nil)

		ov, err := json.MarshalIndent(outv, "", "  ")
		if err != nil {
			panic(err)
		}

		g.Methods = append(g.Methods, &Method{
			Name:            m.Name,
			Comment:         ainfo.Comments[m.Name],
			InputExample:    string(v),
			ResponseExample: string(ov),
		})
	}

	var groupslice []*MethodGroup
	for _, g := range groups {
		groupslice = append(groupslice, g)
	}

	sort.Slice(groupslice, func(i, j int) bool {
		return groupslice[i].GroupName < groupslice[j].GroupName
	})

	if _, err := fmt.Fprintf(out, "# Groups\n"); err != nil {
		return err
	}

	for _, g := range groupslice {
		if _, err := fmt.Fprintf(out, "* [%s](#%s)\n", g.GroupName, g.GroupName); err != nil {
			return err
		}
		for _, method := range g.Methods {
			if _, err := fmt.Fprintf(out, "  * [%s](#%s)\n", method.Name, method.Name); err != nil {
				return err
			}
		}
	}

	for _, g := range groupslice {
		g := g
		if _, err := fmt.Fprintf(out, "## %s\n", g.GroupName); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "%s\n\n", g.Header); err != nil {
			return err
		}

		sort.Slice(g.Methods, func(i, j int) bool {
			return g.Methods[i].Name < g.Methods[j].Name
		})

		for _, m := range g.Methods {
			if _, err := fmt.Fprintf(out, "### %s\n", m.Name); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "%s\n\n", m.Comment); err != nil {
				return err
			}

			var meth reflect.StructField
			var ok bool
			for _, ps := range permStruct {
				meth, ok = ps.FieldByName(m.Name)
				if ok {
					break
				}
			}
			if !ok {
				return fmt.Errorf("no perms for method: %s", m.Name)
			}

			perms := meth.Tag.Get("perm")

			if _, err := fmt.Fprintf(out, "Perms: %s\n\n", perms); err != nil {
				return err
			}

			if strings.Count(m.InputExample, "\n") > 0 {
				if _, err := fmt.Fprintf(out, "Inputs:\n```json\n%s\n```\n\n", m.InputExample); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintf(out, "Inputs: `%s`\n\n", m.InputExample); err != nil {
				return err
			}

			if strings.Count(m.ResponseExample, "\n") > 0 {
				if _, err := fmt.Fprintf(out, "Response:\n```json\n%s\n```\n\n", m.ResponseExample); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintf(out, "Response: `%s`\n\n", m.ResponseExample); err != nil {
				return err
			}
		}
	}
	return nil
}

func ParseApiASTInfo(apiFile, iface, pkg, dir string) (ApiASTInfo, error) { //nolint:golint
	fset := token.NewFileSet()
	apiDir, err := filepath.Abs(dir)
	if err != nil {
		return ApiASTInfo{}, fmt.Errorf("./api filepath absolute error: %w", err)
	}
	apiFile, err = filepath.Abs(apiFile)
	if err != nil {
		return ApiASTInfo{}, fmt.Errorf("filepath absolute file: %s, err: %w", apiFile, err)
	}
	pkgs, err := parser.ParseDir(fset, apiDir, nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return ApiASTInfo{}, fmt.Errorf("parse error: %w", err)
	}

	ap := pkgs[pkg]

	f := ap.Files[apiFile]

	cmap := ast.NewCommentMap(fset, f, f.Comments)

	v := &Visitor{iface, make(map[string]ast.Node)}
	ast.Walk(v, ap)

	comments := make(map[string]string)
	groupDocs := make(map[string]string)
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
	return ApiASTInfo{
		Comments:      comments,
		GroupComments: groupDocs,
	}, nil
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

//go:embed tipset.json
var tipsetSampleJson []byte
