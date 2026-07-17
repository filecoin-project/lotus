package docgenopenrpc

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type recursiveSchema struct {
	Children []recursiveSchema
}
type sourceReceiver struct{}

func (*sourceReceiver) Example(_ context.Context, input string) (int, error) {
	return len(input), nil
}

func TestParseMethodSignature(t *testing.T) {
	method, ok := reflect.TypeFor[*sourceReceiver]().MethodByName("Example")
	if !ok {
		t.Fatal("Example method not found")
	}
	signature, err := parseMethodSignature(method, make(map[string]parsedSource))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(signature.params, []string{"context.Context", "string"}) {
		t.Fatalf("unexpected parameters: %v", signature.params)
	}
	if !reflect.DeepEqual(signature.results, []string{"int", "error"}) {
		t.Fatalf("unexpected results: %v", signature.results)
	}
	if !strings.Contains(signature.description, "return len(input), nil") {
		t.Fatalf("method body missing from description: %q", signature.description)
	}
	const urlPrefix = "https://github.com/filecoin-project/lotus/blob/master/api/docgen-openrpc/openrpc_test.go#L"
	if signature.externalDocs == nil || !strings.HasPrefix(signature.externalDocs.URL, urlPrefix) {
		t.Fatalf("unexpected external documentation: %#v", signature.externalDocs)
	}
}

func TestReflectSchemaBoundsRecursiveTypes(t *testing.T) {
	schema := reflectSchema(reflect.TypeFor[recursiveSchema]())
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document["definitions"]; ok {
		t.Fatal("schema contains descriptor-local definitions")
	}
	if _, ok := document["$defs"]; ok {
		t.Fatal("schema contains descriptor-local $defs")
	}
	if refs := schemaReferences(document); len(refs) != 0 {
		t.Fatalf("schema contains descriptor-local references: %v", refs)
	}

	properties := document["properties"].(map[string]any)
	children := properties["Children"].(map[string]any)
	items := children["items"].(map[string]any)
	if allowed, ok := items["additionalProperties"].(bool); !ok || !allowed {
		t.Fatalf("recursive field was not bounded: %v", items)
	}
}

func schemaReferences(value any) []string {
	var refs []string
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				refs = append(refs, ref)
			}
			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(value)
	return refs
}
