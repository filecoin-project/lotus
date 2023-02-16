package main

import (
	"fmt"
	"os"
	"strings"
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

func splitOverride(override string) (string, string) {
	x := strings.Split(override, "=")
	return x[0], x[1]
}

func main() {
	metadata, err := build.ReadEmbeddedBuiltinActorsMetadata()
	if err != nil {
		panic(err)
	}

	var metadataSpecificVersion []*build.BuiltinActorsMetadata

	// see ./build/actors/pack.sh
	// expected args are git bundle tag then number of per network overrides
	// overrides are in the format network_name=override

	if len(os.Args) > 1 {
		version := os.Args[1]
		overrides := map[string]string{}
		for _, override := range os.Args[3:] {
			k, v := splitOverride(override)
			overrides[k] = v
		}
		for _, m := range metadata {
			if strings.HasPrefix(version, fmt.Sprintf("v%d", m.Version)) {
				// correct version
				override, ok := overrides[m.Network]
				if ok {
					m.BundleGitTag = override
				} else {
					m.BundleGitTag = os.Args[2]
				}
				fmt.Println("hi")
				metadataSpecificVersion = append(metadataSpecificVersion, m)
			}
		}
		metadata = metadataSpecificVersion

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
