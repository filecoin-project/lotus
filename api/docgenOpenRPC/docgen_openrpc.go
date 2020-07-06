package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"

	"github.com/alecthomas/jsonschema"
	go_openrpc_reflect "github.com/etclabscore/go-openrpc-reflect"
	"github.com/ethereum/go-ethereum/params"
	"github.com/filecoin-project/lotus/api/apistruct"
	meta_schema "github.com/open-rpc/meta-schema"
)

// newOpenRPCDocument returns a Document configured with application-specific logic.
func newOpenRPCDocument() *go_openrpc_reflect.Document {
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
			title := "Core-Geth RPC API"
			info.Title = (*meta_schema.InfoObjectProperties)(&title)

			version := params.VersionWithMeta
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

	// Finally, register the configured reflector to the document.
	d.WithReflector(appReflector)
	return d
}



func main() {
	commonPermStruct := &apistruct.CommonStruct{}
	// fullStruct := &apistruct.FullNodeStruct{}

	doc := newOpenRPCDocument()

	doc.RegisterReceiverName("common", commonPermStruct)
	// doc.RegisterReceiverName("full", fullStruct)
	
	out, err := doc.Discover()
	if err != nil {
		log.Fatalln(err)
	}

	jsonOut, err := json.MarshalIndent(out, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(string(jsonOut))
}
