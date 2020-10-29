package openrpc

import (
	"go/ast"
	"net"
	"reflect"
	"time"

	"github.com/alecthomas/jsonschema"
	go_openrpc_reflect "github.com/etclabscore/go-openrpc-reflect"
	"github.com/filecoin-project/lotus/api/docgen"
	"github.com/filecoin-project/lotus/build"
	meta_schema "github.com/open-rpc/meta-schema"
)

var Comments, GroupDocs = docgen.ParseApiASTInfo()

// NewLotusOpenRPCDocument defines application-specific documentation and configuration for its OpenRPC document.
func NewLotusOpenRPCDocument() *go_openrpc_reflect.Document {
	d := &go_openrpc_reflect.Document{}

	// Register "Meta" document fields.
	// These include getters for
	// - Servers object
	// - Info object
	// - ExternalDocs object
	//
	// These objects represent server-specific data that cannot be
	// reflected.
	d.WithMeta(&go_openrpc_reflect.MetaT{
		GetServersFn: func() func(listeners []net.Listener) (*meta_schema.Servers, error) {
			return func(listeners []net.Listener) (*meta_schema.Servers, error) {
				return nil, nil
			}
		},
		GetInfoFn: func() (info *meta_schema.InfoObject) {
			info = &meta_schema.InfoObject{}
			title := "Lotus RPC API"
			info.Title = (*meta_schema.InfoObjectProperties)(&title)

			version := build.UserVersion() + "/generated=" + time.Now().Format(time.RFC3339)
			info.Version = (*meta_schema.InfoObjectVersion)(&version)
			return info
		},
		GetExternalDocsFn: func() (exdocs *meta_schema.ExternalDocumentationObject) {
			return nil // FIXME
		},
	})

	// Use a provided Ethereum default configuration as a base.
	appReflector := &go_openrpc_reflect.EthereumReflectorT{}

	// Install overrides for the json schema->type map fn used by the jsonschema reflect package.
	appReflector.FnSchemaTypeMap = func() func(ty reflect.Type) *jsonschema.Type {
		return func(ty reflect.Type) *jsonschema.Type {
			if ty.String() == "uintptr" {
				return &jsonschema.Type{Type: "number", Title: "uintptr-title"}
			}
			return nil
		}
	}

	appReflector.FnIsMethodEligible = func (m reflect.Method) bool {
		for i := 0; i < m.Func.Type().NumOut(); i++ {
			if m.Func.Type().Out(i).Kind() == reflect.Chan {
				return false
			}
		}
		return go_openrpc_reflect.EthereumReflector.IsMethodEligible(m)
	}
	appReflector.FnGetMethodName = func(moduleName string, r reflect.Value, m reflect.Method, funcDecl *ast.FuncDecl) (string, error) {
		if m.Name == "ID" {
			return moduleName + "_ID", nil
		}
		if moduleName == "rpc" && m.Name == "Discover" {
			return "rpc.discover", nil
		}

		return moduleName + "." + m.Name, nil
	}

	appReflector.FnGetMethodSummary = func(r reflect.Value, m reflect.Method, funcDecl *ast.FuncDecl) (string, error) {
		if v, ok := Comments[m.Name]; ok {
			return v, nil
		}
		return "", nil // noComment
	}

	appReflector.FnGetMethodExamples = func(r reflect.Value, m reflect.Method, funcDecl *ast.FuncDecl) (*meta_schema.MethodObjectExamples, error) {

		var args []interface{}
		ft := m.Func.Type()
		for j := 2; j < ft.NumIn(); j++ {
			inp := ft.In(j)
			args = append(args, docgen.ExampleValue(inp, nil))
		}
		params := []meta_schema.ExampleOrReference{}
		for _, p := range params {
			v := meta_schema.ExampleObjectValue(p)
			params = append(params, meta_schema.ExampleOrReference{
				ExampleObject:   &meta_schema.ExampleObject{Value: &v},
				ReferenceObject: nil,
			})
		}
		pairingParams := meta_schema.ExamplePairingObjectParams(params)

		outv := docgen.ExampleValue(ft.Out(0), nil)
		resultV := meta_schema.ExampleObjectValue(outv)
		result := &meta_schema.ExampleObject{
			Summary:     nil,
			Value:       &resultV,
			Description: nil,
			Name:        nil,
		}

		pairingResult := meta_schema.ExamplePairingObjectResult{
			ExampleObject:   result,
			ReferenceObject: nil,
		}
		ex := meta_schema.ExamplePairingOrReference{
			ExamplePairingObject: &meta_schema.ExamplePairingObject{
				Name:        nil,
				Description: nil,
				Params:      &pairingParams,
				Result:      &pairingResult,
			},
		}

		return &meta_schema.MethodObjectExamples{ex}, nil
	}

	appReflector.FnSchemaExamples = func(ty reflect.Type) (examples *meta_schema.Examples, err error) {
		v, ok := docgen.ExampleValues[ty]
		if !ok {
			return nil, nil
		}
		return &meta_schema.Examples{
			meta_schema.AlwaysTrue(v),
		}, nil
	}
	
	// Finally, register the configured reflector to the document.
	d.WithReflector(appReflector)
	return d
}
