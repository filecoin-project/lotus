package main

import (
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/filecoin-project/lotus/build"
)

var tmpl = template.Must(template.New("actor-metadata").Parse(`
// WARNING: This file has automatically been generated

package build

import (
	"github.com/ipfs/go-cid"
)

var EmbeddedBuiltinActorsMetadata = []*BuiltinActorsMetadata{
{{- range . }} {
	Network: {{printf "%q" .Network}},
	Version: {{.Version}},
	{{- if .BundleGitTag }}
		BundleGitTag: {{printf "%q" .BundleGitTag}},
	{{- end }}
	ManifestCid: cid.MustParse({{printf "%q" .ManifestCid}}),
	Actors: map[string]cid.Cid {
	{{- range $name, $cid := .Actors }}
		{{printf "%q" $name}}: cid.MustParse({{printf "%q" $cid}}),
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
	// read metadata from the embedded bundle, includes all info except git tags
	metadata, err := build.ReadEmbeddedBuiltinActorsMetadata()
	if err != nil {
		panic(err)
	}

	// IF args have been provided, extract git tag info from them, otherwise
	// rely on previously embedded metadata for git tags.
	if len(os.Args) > 1 {
		// see ./build/actors/pack.sh
		// (optional) expected args are:
		// $(GOCC) run ./gen/bundle $(VERSION) $(RELEASE) $(RELEASE_OVERRIDES)
		// overrides are in the format network_name=override
		gitTag := os.Args[2]
		packedActorsVersion, err := strconv.Atoi(os.Args[1][1:])
		if err != nil {
			panic(err)
		}

		overrides := map[string]string{}
		for _, override := range os.Args[3:] {
			k, v := splitOverride(override)
			overrides[k] = v
		}
		for _, m := range metadata {
			if int(m.Version) == packedActorsVersion {
				override, ok := overrides[m.Network]
				if ok {
					m.BundleGitTag = override
				} else {
					m.BundleGitTag = gitTag
				}
			} else {
				for _, v := range build.EmbeddedBuiltinActorsMetadata {
					// if we agree on the manifestCid for the previously embedded metadata, use the previously set tag
					if m.Version == v.Version && m.Network == v.Network && m.ManifestCid == v.ManifestCid {
						m.BundleGitTag = v.BundleGitTag
					}
				}
			}
		}
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
