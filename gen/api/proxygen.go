package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

type methodMeta struct {
	node  ast.Node
	ftype *ast.FuncType
}

type Visitor struct {
	Methods map[string]map[string]*methodMeta
	Include map[string][]string
}

func main() {
	var lets errgroup.Group
	lets.Go(generateApi)
	lets.Go(generateApiV2)
	lets.Go(generateApiV0)
	if err := lets.Wait(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("All API proxy files generated successfully.")
}

func generateApiV0() error {
	return generate("./api/v0api", "v0api", "v0api", "./api/v0api/proxy_gen.go")
}

func generateApi() error {
	return generate("./api", "api", "api", "./api/proxy_gen.go")
}

func generateApiV2() error {
	return generate("./api/v2api", "v2api", "v2api", "./api/v2api/proxy_gen.go")
}

func (v *Visitor) Visit(node ast.Node) ast.Visitor {
	st, ok := node.(*ast.TypeSpec)
	if !ok {
		return v
	}

	iface, ok := st.Type.(*ast.InterfaceType)
	if !ok {
		return v
	}
	if v.Methods[st.Name.Name] == nil {
		v.Methods[st.Name.Name] = map[string]*methodMeta{}
	}
	for _, m := range iface.Methods.List {
		switch ft := m.Type.(type) {
		case *ast.Ident:
			v.Include[st.Name.Name] = append(v.Include[st.Name.Name], ft.Name)
		case *ast.FuncType:
			v.Methods[st.Name.Name][m.Names[0].Name] = &methodMeta{
				node:  m,
				ftype: ft,
			}
		}
	}

	return v
}

func typeName(e ast.Expr, pkg string) (string, error) {
	switch t := e.(type) {
	case *ast.SelectorExpr:
		return t.X.(*ast.Ident).Name + "." + t.Sel.Name, nil
	case *ast.Ident:
		pstr := t.Name
		return pstr, nil
	case *ast.ArrayType:
		subt, err := typeName(t.Elt, pkg)
		if err != nil {
			return "", err
		}
		return "[]" + subt, nil
	case *ast.StarExpr:
		subt, err := typeName(t.X, pkg)
		if err != nil {
			return "", err
		}
		return "*" + subt, nil
	case *ast.MapType:
		k, err := typeName(t.Key, pkg)
		if err != nil {
			return "", err
		}
		v, err := typeName(t.Value, pkg)
		if err != nil {
			return "", err
		}
		return "map[" + k + "]" + v, nil
	case *ast.StructType:
		if len(t.Fields.List) != 0 {
			return "", xerrors.Errorf("can't struct")
		}
		return "struct{}", nil
	case *ast.InterfaceType:
		if len(t.Methods.List) != 0 {
			return "", xerrors.Errorf("can't interface")
		}
		return "interface{}", nil
	case *ast.ChanType:
		subt, err := typeName(t.Value, pkg)
		if err != nil {
			return "", err
		}
		if t.Dir == ast.SEND {
			subt = "->chan " + subt
		} else {
			subt = "<-chan " + subt
		}
		return subt, nil
	default:
		return "", xerrors.Errorf("unknown type")
	}
}

func generate(path, pkg, outpkg, outfile string) error {
	fset := token.NewFileSet()
	apiDir, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	outfile, err = filepath.Abs(outfile)
	if err != nil {
		return err
	}
	pkgs, err := parser.ParseDir(fset, apiDir, nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return err
	}

	ap := pkgs[pkg]

	v := &Visitor{make(map[string]map[string]*methodMeta), map[string][]string{}}
	ast.Walk(v, ap)

	type methodInfo struct {
		Num                                      string
		node                                     ast.Node
		Tags                                     map[string][]string
		NamedParams, ParamNames, Results, DefRes string
	}

	type strinfo struct {
		Num     string
		Methods map[string]*methodInfo
		Include []string
	}

	type meta struct {
		Infos   map[string]*strinfo
		Imports map[string]string
		OutPkg  string
	}

	m := &meta{
		OutPkg:  outpkg,
		Infos:   map[string]*strinfo{},
		Imports: map[string]string{},
	}

	for fn, f := range ap.Files {
		if strings.HasSuffix(fn, "gen.go") {
			continue
		}

		//fmt.Println("F:", fn)
		cmap := ast.NewCommentMap(fset, f, f.Comments)

		for _, im := range f.Imports {
			m.Imports[im.Path.Value] = im.Path.Value
			if im.Name != nil {
				m.Imports[im.Path.Value] = im.Name.Name + " " + m.Imports[im.Path.Value]
			}
		}

		for ifname, methods := range v.Methods {
			if _, ok := m.Infos[ifname]; !ok {
				m.Infos[ifname] = &strinfo{
					Num:     ifname,
					Methods: map[string]*methodInfo{},
					Include: v.Include[ifname],
				}
			}
			info := m.Infos[ifname]
			for mname, node := range methods {
				filteredComments := cmap.Filter(node.node).Comments()

				if _, ok := info.Methods[mname]; !ok {
					var params, pnames []string
					for _, param := range node.ftype.Params.List {
						pstr, err := typeName(param.Type, outpkg)
						if err != nil {
							return err
						}

						c := len(param.Names)
						if c == 0 {
							c = 1
						}

						for i := 0; i < c; i++ {
							pname := fmt.Sprintf("p%d", len(params))
							pnames = append(pnames, pname)
							params = append(params, pname+" "+pstr)
						}
					}

					results := []string{}
					for _, result := range node.ftype.Results.List {
						rs, err := typeName(result.Type, outpkg)
						if err != nil {
							return err
						}
						results = append(results, rs)
					}

					defRes := ""
					if len(results) > 1 {
						defRes = results[0]
						switch {
						case defRes[0] == '*' || defRes[0] == '<', defRes == "interface{}":
							defRes = "nil"
						case defRes == "bool":
							defRes = "false"
						case defRes == "string":
							defRes = `""`
						case defRes == "int", defRes == "int64", defRes == "uint64", defRes == "uint":
							defRes = "0"
						default:
							defRes = "*new(" + defRes + ")"
						}
						defRes += ", "
					}

					info.Methods[mname] = &methodInfo{
						Num:         mname,
						node:        node.node,
						Tags:        map[string][]string{},
						NamedParams: strings.Join(params, ", "),
						ParamNames:  strings.Join(pnames, ", "),
						Results:     strings.Join(results, ", "),
						DefRes:      defRes,
					}
				}

				// try to parse tag info
				if len(filteredComments) > 0 {
					tagstr := filteredComments[len(filteredComments)-1].List[0].Text
					tagstr = strings.TrimPrefix(tagstr, "//")
					tl := strings.Split(strings.TrimSpace(tagstr), " ")
					for _, ts := range tl {
						tf := strings.Split(ts, ":")
						if len(tf) != 2 {
							continue
						}
						if tf[0] != "perm" && tf[0] != "rpc_method" && tf[0] != "notify" { // todo: allow more tag types
							continue
						}
						info.Methods[mname].Tags[tf[0]] = tf
					}
				}
			}
		}
	}

	/*jb, err := json.MarshalIndent(Infos, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jb))*/

	w, err := os.OpenFile(outfile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}

	err = doTemplate(w, m, `// Code generated by github.com/filecoin-project/lotus/gen/api. DO NOT EDIT.

package {{.OutPkg}}

import (
{{range .Imports}}	{{.}}
{{end}}
)
`)
	if err != nil {
		return err
	}

	err = doTemplate(w, m, `

var ErrNotSupported = xerrors.New("method not supported")

{{range .Infos}}
type {{.Num}}Struct struct {
{{range .Include}}
	{{.}}Struct
{{end}}
	Internal {{.Num}}Methods
}

type {{.Num}}Methods struct {
{{range .Methods}}
	{{.Num}} func({{.NamedParams}}) ({{.Results}}) `+"`"+`{{$first := true}}{{range .Tags}}{{if $first}}{{$first = false}}{{else}} {{end}}{{index . 0}}:"{{index . 1}}"{{end}}`+"`"+`

{{end}}
	}

type {{.Num}}Stub struct {
{{range .Include}}
	{{.}}Stub
{{end}}
}
{{end}}

{{range .Infos}}
{{$name := .Num}}
{{range .Methods}}
func (s *{{$name}}Struct) {{.Num}}({{.NamedParams}}) ({{.Results}}) {
	if s.Internal.{{.Num}} == nil {
		return {{.DefRes}}ErrNotSupported
	}
	return s.Internal.{{.Num}}({{.ParamNames}})
}

func (s *{{$name}}Stub) {{.Num}}({{.NamedParams}}) ({{.Results}}) {
	return {{.DefRes}}ErrNotSupported
}
{{end}}
{{end}}

{{range .Infos}}var _ {{.Num}} = new({{.Num}}Struct)
{{end}}

`)
	return err
}

func doTemplate(w io.Writer, info interface{}, templ string) error {
	t := template.Must(template.New("").
		Funcs(template.FuncMap{}).Parse(templ))

	return t.Execute(w, info)
}
