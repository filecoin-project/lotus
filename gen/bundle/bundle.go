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
	{{if .BundleGitTag}} BundleGitTag: {{printf "%q" .BundleGitTag}}, {{end}}
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

	// TODO: Re-enable this when we can set the tag for ONLY the appropriate version
	// https://github.com/filecoin-project/lotus/issues/10185#issuecomment-1422864836
	//if len(os.Args) > 1 {
	//	for _, m := range metadata {
	//		m.BundleGitTag = os.Args[1]
	//	}
	//}

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
