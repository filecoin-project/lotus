package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/xerrors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

type methodMeta struct {
	node  ast.Node
	ftype *ast.FuncType
}

type Visitor struct {
	Methods map[string]map[string]*methodMeta
	Include map[string][]string
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
func main() {
	if err := runMain(); err != nil {
		fmt.Println("error: ", err)
	}
}

func typeName(e ast.Expr) (string, error) {
	switch t := e.(type) {
	case *ast.SelectorExpr:
		return t.X.(*ast.Ident).Name + "." + t.Sel.Name, nil
	case *ast.Ident:
		pstr := t.Name
		if !unicode.IsLower(rune(pstr[0])) {
			pstr = "api." + pstr // todo src pkg name
		}
		return pstr, nil
	case *ast.ArrayType:
		subt, err := typeName(t.Elt)
		if err != nil {
			return "", err
		}
		return "[]" + subt, nil
	case *ast.StarExpr:
		subt, err := typeName(t.X)
		if err != nil {
			return "", err
		}
		return "*" + subt, nil
	case *ast.MapType:
		k, err := typeName(t.Key)
		if err != nil {
			return "", err
		}
		v, err := typeName(t.Value)
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
		subt, err := typeName(t.Value)
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

func runMain() error {
	fset := token.NewFileSet()
	apiDir, err := filepath.Abs("./api")
	if err != nil {
		return err
	}
	pkgs, err := parser.ParseDir(fset, apiDir, nil, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return err
	}

	ap := pkgs["api"]

	v := &Visitor{make(map[string]map[string]*methodMeta), map[string][]string{}}
	ast.Walk(v, ap)

	type methodInfo struct {
		Name                             string
		node                             ast.Node
		Tags                             map[string][]string
		NamedParams, ParamNames, Results string
	}

	type strinfo struct {
		Name    string
		Methods map[string]*methodInfo
		Include []string
	}

	type meta struct {
		Infos   map[string]*strinfo
		Imports map[string]string
	}

	m := &meta{
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
		}

		for ifname, methods := range v.Methods {
			if _, ok := m.Infos[ifname]; !ok {
				m.Infos[ifname] = &strinfo{
					Name:    ifname,
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
						pstr, err := typeName(param.Type)
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

					var results []string
					for _, result := range node.ftype.Results.List {
						rs, err := typeName(result.Type)
						if err != nil {
							return err
						}
						results = append(results, rs)
					}

					info.Methods[mname] = &methodInfo{
						Name:        mname,
						node:        node.node,
						Tags:        map[string][]string{},
						NamedParams: strings.Join(params, ", "),
						ParamNames:  strings.Join(pnames, ", "),
						Results:     strings.Join(results, ", "),
					}
				}

				// try to parse tag info
				if len(filteredComments) > 0 {
					tagstr := filteredComments[len(filteredComments)-1].Text()
					tl := strings.Split(strings.TrimSpace(tagstr), " ")
					for _, ts := range tl {
						tf := strings.Split(ts, ":")
						if len(tf) != 2 {
							continue
						}
						if tf[0] != "perm" { // todo: allow more tag types
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

	w := os.Stdout

	err = doTemplate(w, m, `package apistruct
import (
{{range .Imports}}{{.}}
{{end}}
)
`)
	if err != nil {
		return err
	}

	err = doTemplate(w, m, `
{{range .Infos}}
type {{.Name}}Struct struct {
{{range .Include}}
	{{.}}Struct
{{end}}
	Internal struct {
{{range .Methods}}
		{{.Name}} func({{.NamedParams}}) ({{.Results}}) `+"`"+`{{range .Tags}}{{index . 0}}:"{{index . 1}}"{{end}}`+"`"+`
{{end}}
	}
}
{{end}}

{{range .Infos}}
{{$name := .Name}}
{{range .Methods}}
func (s *{{$name}}Struct) {{.Name}}({{.NamedParams}}) ({{.Results}}) {
	return s.Internal.{{.Name}}({{.ParamNames}})
}
{{end}}
{{end}}

{{range .Infos}}var _ api.{{.Name}} = new({{.Name}}Struct)
{{end}}

`)
	return err
}

func doTemplate(w io.Writer, info interface{}, templ string) error {
	t := template.Must(template.New("").
		Funcs(template.FuncMap{
			"ReadHeader": func(rdr string) string {
				return fmt.Sprintf(`cbg.CborReadHeaderBuf(%s, scratch)`, rdr)
			},
		}).Parse(templ))

	return t.Execute(w, info)
}
