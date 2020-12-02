package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	filestore2 "github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-multistore"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var ExampleValues = map[reflect.Type]interface{}{
	reflect.TypeOf(auth.Permission("")): auth.Permission("write"),
	reflect.TypeOf(""):                  "string value",
	reflect.TypeOf(uint64(42)):          uint64(42),
	reflect.TypeOf(byte(7)):             byte(7),
	reflect.TypeOf([]byte{}):            []byte("byte array"),
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
	addExample(filestore.StatusFileChanged)
	addExample(abi.SectorNumber(9))
	addExample(abi.SectorSize(32 * 1024 * 1024 * 1024))
	addExample(api.MpoolChange(0))
	addExample(network.Connected)
	addExample(dtypes.NetworkName("lotus"))
	addExample(api.SyncStateStage(1))
	addExample(build.FullAPIVersion)
	addExample(api.PCHInbound)
	addExample(time.Minute)
	addExample(datatransfer.TransferID(3))
	addExample(datatransfer.Ongoing)
	addExample(multistore.StoreID(50))
	addExample(retrievalmarket.ClientEventDealAccepted)
	addExample(retrievalmarket.DealStatusNew)
	addExample(network.ReachabilityPublic)
	addExample(build.NewestNetworkVersion)
	addExample(&types.ExecutionTrace{
		Msg:    exampleValue("init", reflect.TypeOf(&types.Message{}), nil).(*types.Message),
		MsgRct: exampleValue("init", reflect.TypeOf(&types.MessageReceipt{}), nil).(*types.MessageReceipt),
	})
	addExample(map[string]types.Actor{
		"t01236": exampleValue("init", reflect.TypeOf(types.Actor{}), nil).(types.Actor),
	})
	addExample(map[string]api.MarketDeal{
		"t026363": exampleValue("init", reflect.TypeOf(api.MarketDeal{}), nil).(api.MarketDeal),
	})
	addExample(map[string]api.MarketBalance{
		"t026363": exampleValue("init", reflect.TypeOf(api.MarketBalance{}), nil).(api.MarketBalance),
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
	addExample(filestore2.Path(".lotusminer/fstmp123"))
	si := multistore.StoreID(12)
	addExample(&si)
	addExample(retrievalmarket.DealID(5))
	addExample(abi.ActorID(1000))
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
	addExample(stores.ID("76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8"))
	addExample(storiface.FTUnsealed)
	addExample(storiface.PathSealing)
	addExample(map[stores.ID][]stores.Decl{
		"76f1988b-ef30-4d7e-b3ec-9a627f4ba5a8": {
			{
				SectorID:       abi.SectorID{Miner: 1000, Number: 100},
				SectorFileType: storiface.FTSealed,
			},
		},
	})
	addExample(map[stores.ID]string{
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
					MemSwap:     120 << 30,
					MemReserved: 2 << 30,
					CPUs:        64,
					GPUs:        []string{"aGPU 1337"},
				},
			},
			Enabled:    true,
			MemUsedMin: 0,
			MemUsedMax: 0,
			GpuUsed:    false,
			CpuUse:     0,
		},
	})
	addExample(storiface.ErrorCode(0))
	addExample(map[abi.SectorNumber]string{
		123: "can't acquire read lock",
	})

	// worker specific
	addExample(storiface.AcquireMove)
	addExample(storiface.UnpaddedByteIndex(abi.PaddedPieceSize(1 << 20).Unpadded()))
	addExample(map[sealtasks.TaskType]struct{}{
		sealtasks.TTPreCommit2: {},
	})
	addExample(sealtasks.TTCommit2)
}

func exampleValue(method string, t, parent reflect.Type) interface{} {
	v, ok := ExampleValues[t]
	if ok {
		return v
	}

	switch t.Kind() {
	case reflect.Slice:
		out := reflect.New(t).Elem()
		reflect.Append(out, reflect.ValueOf(exampleValue(method, t.Elem(), t)))
		return out.Interface()
	case reflect.Chan:
		return exampleValue(method, t.Elem(), nil)
	case reflect.Struct:
		es := exampleStruct(method, t, parent)
		v := reflect.ValueOf(es).Elem().Interface()
		ExampleValues[t] = v
		return v
	case reflect.Array:
		out := reflect.New(t).Elem()
		for i := 0; i < t.Len(); i++ {
			out.Index(i).Set(reflect.ValueOf(exampleValue(method, t.Elem(), t)))
		}
		return out.Interface()

	case reflect.Ptr:
		if t.Elem().Kind() == reflect.Struct {
			es := exampleStruct(method, t.Elem(), t)
			//ExampleValues[t] = es
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
		if strings.Title(f.Name) == f.Name {
			ns.Elem().Field(i).Set(reflect.ValueOf(exampleValue(method, f.Type, t)))
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

const noComment = "There are not yet any comments for this method."

func parseApiASTInfo(apiFile, iface string) (map[string]string, map[string]string) { //nolint:golint
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "./api", nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		fmt.Println("parse error: ", err)
	}

	ap := pkgs["api"]

	f := ap.Files[apiFile]

	cmap := ast.NewCommentMap(fset, f, f.Comments)

	v := &Visitor{iface, make(map[string]ast.Node)}
	ast.Walk(v, pkgs["api"])

	groupDocs := make(map[string]string)
	out := make(map[string]string)
	for mn, node := range v.Methods {
		cs := cmap.Filter(node).Comments()
		if len(cs) == 0 {
			out[mn] = noComment
		} else {
			for _, c := range cs {
				if strings.HasPrefix(c.Text(), "MethodGroup:") {
					parts := strings.Split(c.Text(), "\n")
					groupName := strings.TrimSpace(parts[0][12:])
					comment := strings.Join(parts[1:], "\n")
					groupDocs[groupName] = comment

					break
				}
			}

			last := cs[len(cs)-1].Text()
			if !strings.HasPrefix(last, "MethodGroup:") {
				out[mn] = last
			} else {
				out[mn] = noComment
			}
		}
	}
	return out, groupDocs
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

func methodGroupFromName(mn string) string {
	i := strings.IndexFunc(mn[1:], func(r rune) bool {
		return unicode.IsUpper(r)
	})
	if i < 0 {
		return ""
	}
	return mn[:i+1]
}

func main() {
	comments, groupComments := parseApiASTInfo(os.Args[1], os.Args[2])

	groups := make(map[string]*MethodGroup)

	var t reflect.Type
	var permStruct, commonPermStruct reflect.Type

	switch os.Args[2] {
	case "FullNode":
		t = reflect.TypeOf(new(struct{ api.FullNode })).Elem()
		permStruct = reflect.TypeOf(apistruct.FullNodeStruct{}.Internal)
		commonPermStruct = reflect.TypeOf(apistruct.CommonStruct{}.Internal)
	case "StorageMiner":
		t = reflect.TypeOf(new(struct{ api.StorageMiner })).Elem()
		permStruct = reflect.TypeOf(apistruct.StorageMinerStruct{}.Internal)
		commonPermStruct = reflect.TypeOf(apistruct.CommonStruct{}.Internal)
	case "WorkerAPI":
		t = reflect.TypeOf(new(struct{ api.WorkerAPI })).Elem()
		permStruct = reflect.TypeOf(apistruct.WorkerStruct{}.Internal)
		commonPermStruct = reflect.TypeOf(apistruct.WorkerStruct{}.Internal)
	default:
		panic("unknown type")
	}

	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)

		groupName := methodGroupFromName(m.Name)

		g, ok := groups[groupName]
		if !ok {
			g = new(MethodGroup)
			g.Header = groupComments[groupName]
			g.GroupName = groupName
			groups[groupName] = g
		}

		var args []interface{}
		ft := m.Func.Type()
		for j := 2; j < ft.NumIn(); j++ {
			inp := ft.In(j)
			args = append(args, exampleValue(m.Name, inp, nil))
		}

		v, err := json.MarshalIndent(args, "", "  ")
		if err != nil {
			panic(err)
		}

		outv := exampleValue(m.Name, ft.Out(0), nil)

		ov, err := json.MarshalIndent(outv, "", "  ")
		if err != nil {
			panic(err)
		}

		g.Methods = append(g.Methods, &Method{
			Name:            m.Name,
			Comment:         comments[m.Name],
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

	fmt.Printf("# Groups\n")

	for _, g := range groupslice {
		fmt.Printf("* [%s](#%s)\n", g.GroupName, g.GroupName)
		for _, method := range g.Methods {
			fmt.Printf("  * [%s](#%s)\n", method.Name, method.Name)
		}
	}

	for _, g := range groupslice {
		g := g
		fmt.Printf("## %s\n", g.GroupName)
		fmt.Printf("%s\n\n", g.Header)

		sort.Slice(g.Methods, func(i, j int) bool {
			return g.Methods[i].Name < g.Methods[j].Name
		})

		for _, m := range g.Methods {
			fmt.Printf("### %s\n", m.Name)
			fmt.Printf("%s\n\n", m.Comment)

			meth, ok := permStruct.FieldByName(m.Name)
			if !ok {
				meth, ok = commonPermStruct.FieldByName(m.Name)
				if !ok {
					panic("no perms for method: " + m.Name)
				}
			}

			perms := meth.Tag.Get("perm")

			fmt.Printf("Perms: %s\n\n", perms)

			if strings.Count(m.InputExample, "\n") > 0 {
				fmt.Printf("Inputs:\n```json\n%s\n```\n\n", m.InputExample)
			} else {
				fmt.Printf("Inputs: `%s`\n\n", m.InputExample)
			}

			if strings.Count(m.ResponseExample, "\n") > 0 {
				fmt.Printf("Response:\n```json\n%s\n```\n\n", m.ResponseExample)
			} else {
				fmt.Printf("Response: `%s`\n\n", m.ResponseExample)
			}
		}
	}
}
