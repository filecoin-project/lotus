package main

import (
	"os"
	"text/template"

	"github.com/filecoin-project/lotus/build"
)

var tmpl *template.Template = template.Must(template.New("actor-metadata").Parse(`
// WARNING: This file has automatically been generated

package build

import (
	"github.com/ipfs/go-cid"
)

var EmbeddedBuiltinActorsMetadata []*BuiltinActorsMetadata = []*BuiltinActorsMetadata{
{{- range . }} {
	Network: {{printf "%q" .Network}},
	Version: {{.Version}},
	ManifestCid: MustParseCid({{printf "%q" .ManifestCid}}),
	Actors: map[string]cid.Cid {
	{{- range $name, $cid := .Actors }}
		{{printf "%q" $name}}: MustParseCid({{printf "%q" $cid}}),
	{{- end }}
	},
},
{{- end -}}
}
`))

func main() {
	metadata, err := build.ReadEmbeddedBuiltinActorsMetadata()
	if err != nil {
		panic(err)
	}

	fi, err := os.Create("./build/builtin_actors_gen.go")
	if err != nil {
		panic(err)
	}
	defer fi.Close() //nolint

	err = tmpl.Execute(fi, metadata)
	if err != nil {
		panic(err)
	}
}
