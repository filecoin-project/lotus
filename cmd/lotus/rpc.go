package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/jsonschema"
	go_openrpc_reflect "github.com/etclabscore/go-openrpc-reflect"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	meta_schema "github.com/open-rpc/meta-schema"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"contrib.go.opencensus.io/exporter/prometheus"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
)

var log = logging.Logger("main")

type RPCDiscovery struct {
	doc *go_openrpc_reflect.Document
}

func (d *RPCDiscovery) Discover() (*meta_schema.OpenrpcDocument, error) {
	return d.doc.Discover()
}

func serveRPC(a api.FullNode, stop node.StopFunc, addr multiaddr.Multiaddr, shutdownCh <-chan struct{}) error {
	rpcServer := jsonrpc.NewServer()
	fullAPI := apistruct.PermissionedFullAPI(metrics.MetricedFullAPI(a))
	rpcServer.Register("Filecoin", fullAPI)

	doc := newOpenRPCDocument()

	doc.RegisterReceiverName("Filecoin", fullAPI)

	rpcServer.Register("rpc", &RPCDiscovery{doc})

	ah := &auth.Handler{
		Verify: a.AuthVerify,
		Next:   rpcServer.ServeHTTP,
	}

	http.Handle("/rpc/v0", ah)

	importAH := &auth.Handler{
		Verify: a.AuthVerify,
		Next:   handleImport(a.(*impl.FullNodeAPI)),
	}

	http.Handle("/rest/v0/import", importAH)

	exporter, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "lotus",
	})
	if err != nil {
		log.Fatalf("could not create the prometheus stats exporter: %v", err)
	}

	http.Handle("/debug/metrics", exporter)

	lst, err := manet.Listen(addr)
	if err != nil {
		return xerrors.Errorf("could not listen: %w", err)
	}

	srv := &http.Server{
		Handler: http.DefaultServeMux,
		BaseContext: func(listener net.Listener) context.Context {
			ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-daemon"))
			return ctx
		},
	}

	sigCh := make(chan os.Signal, 2)
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			log.Warnw("received shutdown", "signal", sig)
		case <-shutdownCh:
			log.Warn("received shutdown")
		}

		log.Warn("Shutting down...")
		if err := srv.Shutdown(context.TODO()); err != nil {
			log.Errorf("shutting down RPC server failed: %s", err)
		}
		if err := stop(context.TODO()); err != nil {
			log.Errorf("graceful shutting down failed: %s", err)
		}
		log.Warn("Graceful shutdown successful")
		_ = log.Sync() //nolint:errcheck
		close(shutdownDone)
	}()
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	err = srv.Serve(manet.NetListener(lst))
	if err == http.ErrServerClosed {
		<-shutdownDone
		return nil
	}
	return err
}

func handleImport(a *impl.FullNodeAPI) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			w.WriteHeader(404)
			return
		}
		if !auth.HasPerm(r.Context(), nil, apistruct.PermWrite) {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
			return
		}

		c, err := a.ClientImportLocal(r.Context(), r.Body)
		if err != nil {
			w.WriteHeader(500)
			_ = json.NewEncoder(w).Encode(struct{ Error string }{err.Error()})
			return
		}
		w.WriteHeader(200)
		err = json.NewEncoder(w).Encode(struct{ Cid cid.Cid }{c})
		if err != nil {
			log.Errorf("/rest/v0/import: Writing response failed: %+v", err)
			return
		}
	}
}


var _ api.Common = &common.CommonAPI{}

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
