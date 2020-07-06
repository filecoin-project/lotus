package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"net"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/alecthomas/jsonschema"
	go_openrpc_reflect "github.com/etclabscore/go-openrpc-reflect"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	meta_schema "github.com/open-rpc/meta-schema"
)

var comments, groupDocs = parseApiASTInfo()

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
		return go_openrpc_reflect.EthereumReflector.GetMethodName(moduleName, r, m, funcDecl)
	}

	appReflector.FnGetMethodSummary = func(r reflect.Value, m reflect.Method, funcDecl *ast.FuncDecl) (string, error) {
		if v, ok := comments[m.Name]; ok {
			return v, nil
		}
		return "", nil // noComment
	}

	// Finally, register the configured reflector to the document.
	d.WithReflector(appReflector)
	return d
}

func main() {
	commonPermStruct := &apistruct.CommonStruct{}
	fullStruct := &apistruct.FullNodeStruct{}

	doc := newOpenRPCDocument()

	doc.RegisterReceiverName("common", commonPermStruct)
	doc.RegisterReceiverName("full", fullStruct)

	out, err := doc.Discover()
	if err != nil {
		log.Fatalln(err)
	}

	jsonOut, err := json.MarshalIndent(out, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}
	err = ioutil.WriteFile("/tmp/lotus-openrpc-spec.json", jsonOut, os.ModePerm)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(string(jsonOut))

	// fmt.Println("---- (Comments?) Out:")
	// // out2, groupDocs := parseApiASTInfo()
	// out2, _ := parseApiASTInfo()
	// out2JSON, _ := json.MarshalIndent(out2, "", "    ")
	// fmt.Println(string(out2JSON))
	// fmt.Println("---- (Group Comments?):")
	// groupDocsJSON, _ := json.MarshalIndent(groupDocs, "", "    ")
	// fmt.Println(string(groupDocsJSON))
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

const noComment = "There are not yet any comments for this method."

func parseApiASTInfo() (map[string]string, map[string]string) { //nolint:golint
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
			out[mn] = noComment
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
				out[mn] = noComment
			}
		}
	}
	return out, groupDocs
}