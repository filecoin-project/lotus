package docgenopenrpc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-f3/gpbft"

	"github.com/filecoin-project/lotus/api/docgen"
	"github.com/filecoin-project/lotus/build"
)

const integerD = `{
          "title": "number",
          "type": "number",
          "description": "Number is a number"
        }`

const f3ECChain = `{
"items": {
    "additionalProperties": false,
    "properties": {
        "Commitments": {
            "items": {
                "description": "Number is a number",
                "title": "number",
                "type": "number"
            },
            "maxItems": 32,
            "minItems": 32,
            "type": "array"
        },
        "Epoch": {
            "title": "number",
            "type": "number"
        },
        "Key": {
            "media": {
                "binaryEncoding": "base64"
            },
            "type": "string"
        },
        "PowerTable": {
            "title": "Content Identifier",
            "type": "string"
        }
    },
    "type": "object"
},
"type": "array"
}`

const cidCidD = `{"title": "Content Identifier", "type": "string", "description": "Cid represents a self-describing content addressed identifier. It is formed by a Version, a Codec (which indicates a multicodec-packed content type) and a Multihash."}`

func Generate(out io.Writer, iface, pkg string, ainfo docgen.ApiASTInfo, outGzip bool) error {
	receiver, _, _ := docgen.GetAPIType(iface, pkg)
	methods, err := discoverMethods(receiver, ainfo.Comments)
	if err != nil {
		return err
	}
	doc := openRPCDocument{
		OpenRPC: "1.2.6",
		Info: openRPCInfo{
			Title:   "Lotus RPC API",
			Version: build.NodeBuildVersion,
		},
		Methods: methods,
	}

	var jsonOut []byte
	if outGzip {
		jsonOut, err = json.Marshal(doc)
	} else {
		jsonOut, err = json.MarshalIndent(doc, "", "    ")
	}
	if err != nil {
		return err
	}

	if !outGzip {
		_, err = out.Write(jsonOut)
		return err
	}

	writer := gzip.NewWriter(out)
	if _, err = writer.Write(jsonOut); err != nil {
		return err
	}
	return writer.Close()
}

func OpenRPCSchemaTypeMapper(ty reflect.Type) *jsonschema.Schema {
	if ty.Kind() == reflect.Ptr {
		ty = ty.Elem()
	}

	switch ty {
	case reflect.TypeFor[any]():
		return &jsonschema.Schema{
			Type:                 "object",
			AdditionalProperties: jsonschema.TrueSchema,
		}
	case reflect.TypeFor[cid.Cid]():
		return mustSchema(cidCidD)
	case reflect.TypeFor[gpbft.ECChain]():
		schema := mustSchema(f3ECChain)
		if schema.Items != nil && schema.Items.Properties != nil {
			if key, ok := schema.Items.Properties.Get("Key"); ok {
				key.Extras = map[string]any{
					"media": map[string]any{"binaryEncoding": "base64"},
				}
			}
		}
		return schema
	}

	switch ty.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return mustSchema(integerD)
	case reflect.Uintptr:
		return &jsonschema.Schema{Type: "number", Title: "uintptr-title"}
	default:
		return nil
	}
}

type openRPCDocument struct {
	OpenRPC string          `json:"openrpc"`
	Info    openRPCInfo     `json:"info"`
	Methods []openRPCMethod `json:"methods"`
}

type openRPCInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type openRPCMethod struct {
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Summary        string               `json:"summary"`
	ParamStructure string               `json:"paramStructure"`
	Params         []openRPCDescriptor  `json:"params"`
	Result         openRPCDescriptor    `json:"result"`
	ExternalDocs   *openRPCExternalDocs `json:"externalDocs,omitempty"`
	Deprecated     bool                 `json:"deprecated"`
}
type openRPCExternalDocs struct {
	Description string `json:"description"`
	URL         string `json:"url"`
}

type openRPCDescriptor struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Summary     *string            `json:"summary,omitempty"`
	Schema      *jsonschema.Schema `json:"schema"`
	Required    bool               `json:"required"`
	Deprecated  bool               `json:"deprecated"`
}

var (
	contextType  = reflect.TypeFor[context.Context]()
	errorType    = reflect.TypeFor[error]()
	emptySummary string
)

func discoverMethods(receiver any, comments map[string]string) ([]openRPCMethod, error) {
	receiverType := reflect.TypeOf(receiver)
	methods := make([]openRPCMethod, 0, receiverType.NumMethod())
	sourceFiles := make(map[string]parsedSource)
	for i := range receiverType.NumMethod() {
		method := receiverType.Method(i)
		if !isMethodEligible(method) {
			continue
		}

		signature, err := parseMethodSignature(method, sourceFiles)
		if err != nil {
			return nil, err
		}
		name := "Filecoin." + method.Name
		if method.Name == "ID" {
			name = "Filecoin_ID"
		}

		methods = append(methods, openRPCMethod{
			Name:           name,
			Description:    signature.description,
			Summary:        comments[method.Name],
			ParamStructure: "by-position",
			Params:         methodParams(method, signature.params),
			Result:         methodResult(method, signature.results),
			ExternalDocs:   signature.externalDocs,
		})
	}
	return methods, nil
}

func isMethodEligible(method reflect.Method) bool {
	if method.PkgPath != "" {
		return false
	}
	runtimeFunc := runtime.FuncForPC(method.Func.Pointer())
	runtimeFile, _ := runtimeFunc.FileLine(runtimeFunc.Entry())
	if strings.Contains(runtimeFile, "autogenerated") {
		return false
	}

	for i := range method.Type.NumOut() {
		if method.Type.Out(i).Kind() == reflect.Chan {
			return false
		}
	}

	switch method.Type.NumOut() {
	case 0, 1:
		return true
	case 2:
		return method.Type.Out(0) != errorType && method.Type.Out(1) == errorType
	default:
		return false
	}
}

func methodParams(method reflect.Method, descriptions []string) []openRPCDescriptor {
	params := make([]openRPCDescriptor, 0, method.Type.NumIn()-1)
	for i := 1; i < method.Type.NumIn(); i++ {
		ty := method.Type.In(i)
		if i == 1 && ty == contextType {
			continue
		}

		params = append(params, descriptor(
			fmt.Sprintf("p%d", i-1),
			descriptions[i-1],
			ty,
		))
	}
	return params
}

func methodResult(method reflect.Method, descriptions []string) openRPCDescriptor {
	if method.Type.NumOut() == 0 || method.Type.Out(0) == errorType {
		return openRPCDescriptor{
			Name:        "Null",
			Description: "Null",
			Schema:      &jsonschema.Schema{Type: "null"},
			Required:    true,
		}
	}
	return descriptor(descriptions[0], descriptions[0], method.Type.Out(0))
}

func descriptor(name, description string, ty reflect.Type) openRPCDescriptor {
	schema := reflectSchema(ty)
	schema.Examples = []any{docgen.ExampleValue("unknown", ty, ty)}
	return openRPCDescriptor{
		Name:        name,
		Description: description,
		Summary:     &emptySummary,
		Schema:      schema,
		Required:    true,
	}
}

type parsedSource struct {
	fileSet *token.FileSet
	file    *ast.File
}

type methodSignature struct {
	params       []string
	results      []string
	description  string
	externalDocs *openRPCExternalDocs
}

func parseMethodSignature(method reflect.Method, sourceFiles map[string]parsedSource) (methodSignature, error) {
	runtimeFunc := runtime.FuncForPC(method.Func.Pointer())
	runtimeFile, runtimeLine := runtimeFunc.FileLine(runtimeFunc.Entry())
	source, ok := sourceFiles[runtimeFile]
	if !ok {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, runtimeFile, nil, parser.ParseComments)
		if err != nil {
			return methodSignature{}, fmt.Errorf("parse method source %s: %w", runtimeFile, err)
		}
		source = parsedSource{fileSet: fileSet, file: file}
		sourceFiles[runtimeFile] = source
	}

	receiverType := method.Type.In(0)
	if receiverType.Kind() == reflect.Ptr {
		receiverType = receiverType.Elem()
	}
	for _, declaration := range source.file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != method.Name || receiverName(function) != receiverType.Name() {
			continue
		}
		var output bytes.Buffer
		if err := printer.Fprint(&output, token.NewFileSet(), function); err != nil {
			return methodSignature{}, fmt.Errorf("print method %s: %w", method.Name, err)
		}
		return methodSignature{
			params:       fieldTypeNames(source.fileSet, function.Type.Params),
			results:      fieldTypeNames(source.fileSet, function.Type.Results),
			description:  fmt.Sprintf("```go\n%s\n```", output.String()),
			externalDocs: methodExternalDocs(receiverType, runtimeFile, runtimeLine),
		}, nil
	}
	return methodSignature{}, fmt.Errorf("method declaration not found for %s", method.Name)
}

func methodExternalDocs(receiverType reflect.Type, runtimeFile string, runtimeLine int) *openRPCExternalDocs {
	packagePath := receiverType.PkgPath()
	parts := strings.Split(packagePath, "/")
	if len(parts) < 3 || parts[0] != "github.com" {
		return nil
	}
	repository := strings.Join(parts[:3], "/")
	packageDirectory := strings.Join(parts[3:], "/")
	if packageDirectory != "" {
		packageDirectory += "/"
	}
	return &openRPCExternalDocs{
		Description: "Github remote link",
		URL: fmt.Sprintf(
			"https://%s/blob/master/%s%s#L%d",
			repository,
			packageDirectory,
			filepath.Base(runtimeFile),
			runtimeLine,
		),
	}
}

func receiverName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return ""
	}
	receiver := function.Recv.List[0].Type
	if pointer, ok := receiver.(*ast.StarExpr); ok {
		receiver = pointer.X
	}
	if identifier, ok := receiver.(*ast.Ident); ok {
		return identifier.Name
	}
	return ""
}

func fieldTypeNames(fileSet *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	names := make([]string, 0, fields.NumFields())
	for _, field := range fields.List {
		var output bytes.Buffer
		if err := printer.Fprint(&output, fileSet, field.Type); err != nil {
			panic(err)
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			names = append(names, output.String())
		}
	}
	return names
}

func reflectSchema(ty reflect.Type) *jsonschema.Schema {
	schema := (&jsonschema.Reflector{
		Anonymous:                  true,
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: true,
		Mapper:                     OpenRPCSchemaTypeMapper,
	}).ReflectFromType(ty)
	schema.Version = ""
	normalizeSchema(schema)
	return schema
}

func normalizeSchema(root *jsonschema.Schema) {
	definitions := root.Definitions
	normalized := inlineSchemaReferences(root, definitions, make(map[string]bool))
	*root = *normalized
}

func inlineSchemaReferences(schema *jsonschema.Schema, definitions jsonschema.Definitions, resolving map[string]bool) *jsonschema.Schema {
	if schema == nil {
		return nil
	}
	if name, ok := strings.CutPrefix(schema.Ref, "#/$defs/"); ok {
		definition, found := definitions[name]
		if !found {
			return schema
		}
		if resolving[name] {
			return &jsonschema.Schema{AdditionalProperties: jsonschema.TrueSchema}
		}
		resolving[name] = true
		inlined := inlineSchemaReferences(definition, definitions, resolving)
		delete(resolving, name)
		return inlined
	}

	clone := *schema
	clone.Definitions = nil
	if schema.Properties != nil {
		clone.Properties = jsonschema.NewProperties()
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			clone.Properties.Set(pair.Key, inlineSchemaReferences(pair.Value, definitions, resolving))
		}
	}
	clone.AllOf = inlineSchemaSlice(schema.AllOf, definitions, resolving)
	clone.AnyOf = inlineSchemaSlice(schema.AnyOf, definitions, resolving)
	clone.OneOf = inlineSchemaSlice(schema.OneOf, definitions, resolving)
	clone.PrefixItems = inlineSchemaSlice(schema.PrefixItems, definitions, resolving)
	clone.DependentSchemas = inlineSchemaMap(schema.DependentSchemas, definitions, resolving)
	clone.PatternProperties = inlineSchemaMap(schema.PatternProperties, definitions, resolving)
	clone.Not = inlineSchemaReferences(schema.Not, definitions, resolving)
	clone.If = inlineSchemaReferences(schema.If, definitions, resolving)
	clone.Then = inlineSchemaReferences(schema.Then, definitions, resolving)
	clone.Else = inlineSchemaReferences(schema.Else, definitions, resolving)
	clone.Items = inlineSchemaReferences(schema.Items, definitions, resolving)
	clone.Contains = inlineSchemaReferences(schema.Contains, definitions, resolving)
	clone.AdditionalProperties = inlineSchemaReferences(schema.AdditionalProperties, definitions, resolving)
	clone.PropertyNames = inlineSchemaReferences(schema.PropertyNames, definitions, resolving)
	clone.ContentSchema = inlineSchemaReferences(schema.ContentSchema, definitions, resolving)
	return &clone
}

func inlineSchemaSlice(schemas []*jsonschema.Schema, definitions jsonschema.Definitions, resolving map[string]bool) []*jsonschema.Schema {
	if schemas == nil {
		return nil
	}
	inlined := make([]*jsonschema.Schema, len(schemas))
	for i, schema := range schemas {
		inlined[i] = inlineSchemaReferences(schema, definitions, resolving)
	}
	return inlined
}

func inlineSchemaMap(schemas map[string]*jsonschema.Schema, definitions jsonschema.Definitions, resolving map[string]bool) map[string]*jsonschema.Schema {
	if schemas == nil {
		return nil
	}
	inlined := make(map[string]*jsonschema.Schema, len(schemas))
	for name, schema := range schemas {
		inlined[name] = inlineSchemaReferences(schema, definitions, resolving)
	}
	return inlined
}

func mustSchema(input string) *jsonschema.Schema {
	var schema jsonschema.Schema
	if err := json.Unmarshal([]byte(input), &schema); err != nil {
		panic(err)
	}
	return &schema
}
