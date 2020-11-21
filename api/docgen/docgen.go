package docgen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	datatransfer "github.com/filecoin-project/go-data-transfer"
	filecoin_filestore "github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-multistore"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	sector_stores "github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
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

	multistoreIDExample := multistore.StoreID(50)

	addExample(bitfield.NewFromSet([]uint64{5}))
	addExample(abi.RegisteredSealProof_StackedDrg32GiBV1)
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
	addExample(multistoreIDExample)
	addExample(&multistoreIDExample)
	addExample(retrievalmarket.ClientEventDealAccepted)
	addExample(retrievalmarket.DealStatusNew)
	addExample(network.ReachabilityPublic)
	addExample(build.NewestNetworkVersion)
	addExample(&types.ExecutionTrace{
		Msg:    ExampleValue(reflect.TypeOf(&types.Message{}), nil).(*types.Message),
		MsgRct: ExampleValue(reflect.TypeOf(&types.MessageReceipt{}), nil).(*types.MessageReceipt),
	})
	addExample(map[string]types.Actor{
		"t01236": ExampleValue(reflect.TypeOf(types.Actor{}), nil).(types.Actor),
	})
	addExample(map[string]api.MarketDeal{
		"t026363": ExampleValue(reflect.TypeOf(api.MarketDeal{}), nil).(api.MarketDeal),
	})
	addExample(map[string]api.MarketBalance{
		"t026363": ExampleValue(reflect.TypeOf(api.MarketBalance{}), nil).(api.MarketBalance),
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

	addExample(filecoin_filestore.Path("/path/to/file"))
	addExample(retrievalmarket.DealID(6))
	addExample(abi.ActorID(42))
	addExample(map[string][]api.SealedRef{
		"t026363": {
			{
				SectorID: abi.SectorNumber(42),
				Offset:   abi.PaddedPieceSize(42),
				Size:     abi.UnpaddedPieceSize(42),
			},
			{
				SectorID: abi.SectorNumber(43),
				Offset:   abi.PaddedPieceSize(43),
				Size:     abi.UnpaddedPieceSize(43),
			},
		},
	})
	addExample(api.SectorState(1))
	addExample(sector_stores.ID("abc123"))
	addExample(storiface.FTUnsealed)
	addExample(storiface.PathStorage)
	addExample(map[sector_stores.ID][]sector_stores.Decl{
		"12D3KooWSXmXLJmBR1M7i9RW9GQPNUhZSzXKzxDHWtAgNuJAbyFF": {
			{
				SectorID: abi.SectorID{
					Miner:  42,
					Number: 13,
				},
				SectorFileType: storiface.FTSealed,
			},
		},
	})
	addExample(map[sector_stores.ID]string{
		"12D3KooWSXmXLJmBR1M7i9RW9GQPNUhZSzXKzxDHWtAgNuJAbyFF": "12D3KooWSXmXLJmBR1M7i9RW9GQPNUhZSzXKzxDHWtAgNuJAbyAA",
	})
	addExample(sector_stores.HealthReport{
		Stat: fsutil.FsStat{},
		Err:  nil,
	})
	addExample(map[uuid.UUID][]storiface.WorkerJob{
		uuid.MustParse("f47ac10b-58cc-c372-8567-0e02b2c3d479"): {
			storiface.WorkerJob{
				ID:      storiface.CallID{},
				Sector:  abi.SectorID{},
				Task:    "",
				RunWait: 0,
				Start:   time.Time{},
			},
		},
	})
	addExample(map[uuid.UUID]storiface.WorkerStats{
		uuid.MustParse("f47ac10b-58cc-c372-8567-0e02b2c3d479"): storiface.WorkerStats{
			Info:       storiface.WorkerInfo{},
			Enabled:    false,
			MemUsedMin: 0,
			MemUsedMax: 0,
			GpuUsed:    false,
			CpuUse:     0,
		},
	})

	maddr, err := multiaddr.NewMultiaddr("/ip4/52.36.61.156/tcp/1347/p2p/12D3KooWFETiESTf1v4PGUvtnxMAcEFMzLZbJGg4tjWfGEimYior")
	if err != nil {
		panic(err)
	}

	// because reflect.TypeOf(maddr) returns the concrete type...
	ExampleValues[reflect.TypeOf(struct{ A multiaddr.Multiaddr }{}).Field(0).Type] = maddr

}

func ExampleValue(t, parent reflect.Type) interface{} {
	v, ok := ExampleValues[t]
	if ok {
		return v
	}

	switch t.Kind() {
	case reflect.Slice:
		out := reflect.New(t).Elem()
		reflect.Append(out, reflect.ValueOf(ExampleValue(t.Elem(), t)))
		return out.Interface()
	case reflect.Chan:
		return ExampleValue(t.Elem(), nil)
	case reflect.Struct:
		es := exampleStruct(t, parent)
		v := reflect.ValueOf(es).Elem().Interface()
		ExampleValues[t] = v
		return v
	case reflect.Array:
		out := reflect.New(t).Elem()
		for i := 0; i < t.Len(); i++ {
			out.Index(i).Set(reflect.ValueOf(ExampleValue(t.Elem(), t)))
		}
		return out.Interface()

	case reflect.Ptr:
		if t.Elem().Kind() == reflect.Struct {
			es := exampleStruct(t.Elem(), t)
			// ExampleValues[t] = es
			return es
		}
	case reflect.Interface:
		return struct{}{}
	}

	panic(fmt.Sprintf("No example value for type: %s %v %s %s", t, t, t.Kind().String(), t.PkgPath()))
}

func exampleStruct(t, parent reflect.Type) interface{} {
	ns := reflect.New(t)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type == parent {
			continue
		}
		if strings.Title(f.Name) == f.Name {
			ns.Elem().Field(i).Set(reflect.ValueOf(ExampleValue(f.Type, t)))
		}
	}

	return ns.Interface()
}

type Visitor struct {
	Methods map[string]ast.Node
}

func (v *Visitor) Visit(node ast.Node) ast.Visitor {
	st, ok := node.(*ast.TypeSpec)
	if !ok {
		return v
	}

	if st.Name.Name != "FullNode" {
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

func ParseApiASTInfo() (map[string]string, map[string]string) { //nolint:golint
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "./api", nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		fmt.Println("parse error: ", err)
	}

	ap := pkgs["api"]

	f := ap.Files["api/api_full.go"]

	cmap := ast.NewCommentMap(fset, f, f.Comments)

	v := &Visitor{make(map[string]ast.Node)}
	ast.Walk(v, pkgs["api"])

	groupDocs := make(map[string]string)
	out := make(map[string]string)
	for mn, node := range v.Methods {
		comments := cmap.Filter(node).Comments()
		if len(comments) == 0 {
			out[mn] = NoComment
		} else {
			for _, c := range comments {
				if strings.HasPrefix(c.Text(), "MethodGroup:") {
					parts := strings.Split(c.Text(), "\n")
					groupName := strings.TrimSpace(parts[0][12:])
					comment := strings.Join(parts[1:], "\n")
					groupDocs[groupName] = comment

					break
				}
			}

			last := comments[len(comments)-1].Text()
			if !strings.HasPrefix(last, "MethodGroup:") {
				out[mn] = last
			} else {
				out[mn] = NoComment
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

func MethodGroupFromName(mn string) string {
	i := strings.IndexFunc(mn[1:], func(r rune) bool {
		return unicode.IsUpper(r)
	})
	if i < 0 {
		return ""
	}
	return mn[:i+1]
}
