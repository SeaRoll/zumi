// A program that generates OpenAPI documentation for a Go server.
// It reads all `server.AddHandler` calls in the codebase and generates
// OpenAPI documentation based on the registered handlers.
//
// The format is as follows it scans through:
// server.AddHandler("GET ...", func(w, r) { ... })
// It looks at the usage of `server.ParseRequest` to determine the request structure
// and the response structure is based on how the `WriteJSON` and `WriteError`
// functions are used in the handler.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"net/http"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

const serverPackagePath = "github.com/SeaRoll/zumi/server"

var (
	output      = flag.String("output", "openapi.yaml", "Output file for the OpenAPI specification")
	title       = flag.String("title", "API Documentation", "Title of the OpenAPI spec")
	version     = flag.String("version", "1.0.0", "Version of the API")
	description = flag.String("description", "API generated from server.AddHandler calls", "Description of the API")
)

func main() {
	flag.Parse()
	pattern := "./..."

	log.Printf("Generating OpenAPI spec for package(s) matching: %s\n", pattern)
	yamlBytes, err := Generate(pattern)
	if err != nil {
		log.Fatalf("Error generating OpenAPI spec: %v", err)
	}
	err = os.WriteFile(*output, yamlBytes, 0644)
	if err != nil {
		log.Fatalf("Error writing to output file %s: %v", *output, err)
	}
	log.Printf("Successfully wrote OpenAPI spec to %s\n", *output)
}

// Custom parsed types for OpenAPI schema representation.
var customTypeSchemas = map[string]*openapi3.Schema{
	"UUID":       {Type: &openapi3.Types{"string"}, Format: "uuid", Description: "UUID formatted string"},
	"Time":       {Type: &openapi3.Types{"string"}, Format: "date-time", Description: "RFC3339 formatted date-time string"},
	"RawMessage": {Type: &openapi3.Types{"object"}, Description: "Represents any valid JSON object."},
}

// Maps HTTP status constants to their integer values.
var httpStatusMap = map[string]int{
	"StatusOK":                  http.StatusOK,
	"StatusCreated":             http.StatusCreated,
	"StatusAccepted":            http.StatusAccepted,
	"StatusNoContent":           http.StatusNoContent,
	"StatusBadRequest":          http.StatusBadRequest,
	"StatusUnauthorized":        http.StatusUnauthorized,
	"StatusForbidden":           http.StatusForbidden,
	"StatusNotFound":            http.StatusNotFound,
	"StatusInternalServerError": http.StatusInternalServerError,
	"StatusServiceUnavailable":  http.StatusServiceUnavailable,
}

// schemaGenerator holds the state for the generation process.
type schemaGenerator struct {
	typesInfo      *types.Info
	openAPISpec    *openapi3.T
	generatedTypes map[string]*openapi3.SchemaRef
	handlersFound  int
	usedTags       map[string]bool
	fset           *token.FileSet // FileSet to get position info
}

// Generate scans package(s) and creates an OpenAPI spec.
func Generate(pattern string) ([]byte, error) {

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Fset:  fset,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("packages contain errors")
	}

	gen := &schemaGenerator{
		openAPISpec: &openapi3.T{
			OpenAPI: "3.0.0",
			Info: &openapi3.Info{
				Title:       *title,
				Version:     *version,
				Description: *description,
			},
			Paths: openapi3.NewPaths(),
			Components: &openapi3.Components{
				Schemas: make(map[string]*openapi3.SchemaRef),
			},
		},
		generatedTypes: make(map[string]*openapi3.SchemaRef),
		usedTags:       make(map[string]bool),
		fset:           fset, // Store the FileSet in our generator
	}

	// Add a generic error response schema.
	gen.openAPISpec.Components.Schemas["ErrorResponse"] = &openapi3.SchemaRef{
		Value: openapi3.NewObjectSchema().WithProperties(map[string]*openapi3.Schema{
			"error": {Type: &openapi3.Types{"string"}, Description: "Error message"},
		}),
	}

	for _, pkg := range pkgs {
		gen.typesInfo = pkg.TypesInfo
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// The check is now a method that uses type info for robust resolution.
				if gen.isServerAddHandlerCall(call) {
					log.Printf("Found server.AddHandler call in %s", pkg.Fset.File(file.Pos()).Name())
					gen.processAddHandler(call, file.Comments)
					gen.handlersFound++
				}
				return true
			})
		}
	}

	if gen.handlersFound == 0 {
		log.Println("Warning: No 'server.AddHandler' calls were found.")
	}

	// Add used tags to the spec.
	var sortedTags []string
	for tagName := range gen.usedTags {
		sortedTags = append(sortedTags, tagName)
	}
	sort.Strings(sortedTags)

	for _, tagName := range sortedTags {
		gen.openAPISpec.Tags = append(gen.openAPISpec.Tags, &openapi3.Tag{
			Name:        tagName,
			Description: fmt.Sprintf("Operations related to %s", tagName),
		})
	}

	return yaml.Marshal(gen.openAPISpec)
}

// isServerAddHandlerCall uses type information to robustly check if a call expression
// is a call to the AddHandler function in the specified server package.
func (g *schemaGenerator) isServerAddHandlerCall(call *ast.CallExpr) bool {
	// Must have exactly 2 arguments: (route string, handler func).
	if len(call.Args) != 2 {
		return false
	}
	// The function being called must be a selector expression (e.g., pkg.Func).
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	// Use typesInfo to resolve the function object being called.
	obj, ok := g.typesInfo.Uses[sel.Sel]
	if !ok {
		return false
	}
	// Check if it's a function object.
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	// Check the function name and its package path. This is the robust part.
	return fn.Name() == "AddHandler" && fn.Pkg() != nil && fn.Pkg().Path() == serverPackagePath
}

// processAddHandler analyzes a `server.AddHandler` call expression.
func (g *schemaGenerator) processAddHandler(call *ast.CallExpr, comments []*ast.CommentGroup) {
	// First argument: method and path string.
	arg0, ok := call.Args[0].(*ast.BasicLit)
	if !ok || arg0.Kind != token.STRING {
		return
	}
	routeStr, _ := strconv.Unquote(arg0.Value)
	parts := strings.Fields(routeStr)
	if len(parts) != 2 {
		return
	}
	method, path := parts[0], parts[1]

	// Second argument: the handler function literal.
	handlerFunc, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return
	}

	// Find the comment group that immediately precedes the AddHandler call.
	description := ""
	callLine := g.fset.Position(call.Pos()).Line
	for _, cg := range comments {
		// Check if the comment group ends on the line just before the call starts.
		if g.fset.Position(cg.End()).Line == callLine-1 {
			description = cg.Text()
			break // Found the relevant comment, no need to check further.
		}
	}

	reqStruct, responses := g.findRequestAndResponseTypes(handlerFunc)

	tag := generateTagFromPath(path)
	op := &openapi3.Operation{
		Summary:     path,
		OperationID: generateOperationID(method, path),
		Description: description,
		Responses:   openapi3.NewResponses(),
		Tags:        []string{tag},
	}
	g.usedTags[tag] = true

	if reqStruct != nil {
		parameters, requestBody := g.extractRequestInfo(reqStruct)
		op.Parameters = parameters
		op.RequestBody = requestBody
	}

	errorResponseRef := openapi3.NewResponse().
		WithDescription("Error response").
		WithJSONSchemaRef(&openapi3.SchemaRef{
			Ref: "#/components/schemas/ErrorResponse",
		})

	for statusCode, respType := range responses {
		respDescription := http.StatusText(statusCode)
		if respDescription == "" {
			respDescription = "Response"
		}

		if statusCode >= 400 {
			op.AddResponse(statusCode, errorResponseRef)
			continue
		}

		response := openapi3.NewResponse().WithDescription(respDescription)
		if respType != nil {
			respSchemaRef := g.goTypeToSchemaRef(respType)
			response = response.WithJSONSchemaRef(respSchemaRef)
		}
		op.AddResponse(statusCode, response)
	}

	g.openAPISpec.AddOperation(path, method, op)
}

// findRequestAndResponseTypes inspects a function's body to find request and response types.
func (g *schemaGenerator) findRequestAndResponseTypes(fn *ast.FuncLit) (*types.Struct, map[int]types.Type) {
	var reqStruct *types.Struct
	responses := make(map[int]types.Type)

	if fn.Body == nil {
		return nil, nil
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.VAR && reqStruct == nil {
			for _, spec := range decl.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) > 0 && vs.Names[0].Name == "req" {
					if t, ok := g.typesInfo.TypeOf(vs.Type).Underlying().(*types.Struct); ok {
						reqStruct = t
					}
				}
			}
		}

		if call, ok := n.(*ast.CallExpr); ok {
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// We need to check if this is a call to our server package's functions
			// by resolving the type of the receiver (sel.X).
			if receiverIdent, ok := sel.X.(*ast.Ident); ok {
				if obj, ok := g.typesInfo.Uses[receiverIdent]; ok {
					if pkgName, ok := obj.(*types.PkgName); ok {
						if pkgName.Imported().Path() != serverPackagePath {
							return true // Not our server package, so we skip it.
						}
					}
				}
			}

			isWriteJSON := sel.Sel.Name == "WriteJSON"
			isWriteError := sel.Sel.Name == "WriteError"

			if (isWriteJSON || isWriteError) && len(call.Args) >= 2 {
				statusCode, resolved := g.resolveStatusCode(call.Args[1])
				if !resolved {
					return true
				}

				var respType types.Type
				if isWriteJSON && len(call.Args) > 2 {
					respType = g.typesInfo.TypeOf(call.Args[2])
				}
				responses[statusCode] = respType
			}
		}
		return true
	})
	return reqStruct, responses
}

// resolveStatusCode attempts to determine the integer status code from an AST expression.
func (g *schemaGenerator) resolveStatusCode(arg ast.Expr) (int, bool) {
	switch expr := arg.(type) {
	case *ast.BasicLit:
		if expr.Kind == token.INT {
			code, err := strconv.Atoi(expr.Value)
			if err == nil {
				return code, true
			}
		}
	case *ast.SelectorExpr:
		if ident, ok := expr.X.(*ast.Ident); ok && ident.Name == "http" {
			if code, ok := httpStatusMap[expr.Sel.Name]; ok {
				return code, true
			}
		}
	}
	return 0, false
}

// extractRequestInfo processes a request struct to find parameters and a request body.
func (g *schemaGenerator) extractRequestInfo(reqStruct *types.Struct) (openapi3.Parameters, *openapi3.RequestBodyRef) {
	params := openapi3.NewParameters()
	var requestBody *openapi3.RequestBodyRef

	var processRequestStruct func(s *types.Struct)
	processRequestStruct = func(s *types.Struct) {
		if s == nil {
			return
		}
		for i := 0; i < s.NumFields(); i++ {
			field := s.Field(i)
			st := reflect.StructTag(s.Tag(i))

			if field.Embedded() {
				var embeddedStruct *types.Struct
				if named, ok := field.Type().(*types.Named); ok {
					embeddedStruct, _ = named.Underlying().(*types.Struct)
				} else {
					embeddedStruct, _ = field.Type().Underlying().(*types.Struct)
				}
				processRequestStruct(embeddedStruct)
				continue
			}

			isRequired := false
			if validateTag := st.Get("validate"); validateTag != "" {
				if slices.Contains(strings.Split(validateTag, ","), "required") {
					isRequired = true
				}
			}

			_, isPointer := field.Type().(*types.Pointer)

			paramName, isParam := st.Lookup("path")
			queryName, isQuery := st.Lookup("query")
			headerName, isHeader := st.Lookup("header")
			_, isBody := st.Lookup("body")

			switch {
			case isParam:
				p := openapi3.NewPathParameter(paramName)
				p.Schema = g.goTypeToSchemaRef(field.Type())
				p.Required = true
				params = append(params, &openapi3.ParameterRef{Value: p})
			case isQuery:
				p := openapi3.NewQueryParameter(queryName)
				p.Schema = g.goTypeToSchemaRef(field.Type())
				p.Required = isRequired || !isPointer
				params = append(params, &openapi3.ParameterRef{Value: p})
			case isHeader:
				p := openapi3.NewHeaderParameter(headerName)
				p.Schema = g.goTypeToSchemaRef(field.Type())
				p.Required = isRequired || !isPointer
				params = append(params, &openapi3.ParameterRef{Value: p})
			case isBody:
				schemaRef := g.goTypeToSchemaRef(field.Type())
				reqBody := openapi3.NewRequestBody().WithJSONSchemaRef(schemaRef)
				reqBody.Required = isRequired
				requestBody = &openapi3.RequestBodyRef{Value: reqBody}
			}
		}
	}

	processRequestStruct(reqStruct)
	return params, requestBody
}

// goTypeToSchemaRef converts a Go type into an OpenAPI Schema Reference.
func (g *schemaGenerator) goTypeToSchemaRef(typ types.Type) *openapi3.SchemaRef {
	if typ == nil {
		return nil
	}
	if named, ok := typ.(*types.Named); ok {
		typeName := named.Obj().Name()
		if customSchema, exists := customTypeSchemas[typeName]; exists {
			return &openapi3.SchemaRef{Value: customSchema}
		}
		if ref, exists := g.generatedTypes[typeName]; exists {
			return ref
		}
		schemaRef := openapi3.NewSchemaRef("#/components/schemas/"+typeName, nil)
		g.generatedTypes[typeName] = schemaRef
		underlyingSchema := g.goTypeToSchemaRef(named.Underlying())
		g.openAPISpec.Components.Schemas[typeName] = underlyingSchema
		return schemaRef
	}

	if ptr, ok := typ.(*types.Pointer); ok {
		return g.goTypeToSchemaRef(ptr.Elem())
	}

	schema := openapi3.NewSchema()
	switch t := typ.Underlying().(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.String:
			schema.Type = &openapi3.Types{"string"}
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
			schema.Type = &openapi3.Types{"integer"}
		case types.Float32, types.Float64:
			schema.Type = &openapi3.Types{"number"}
		case types.Bool:
			schema.Type = &openapi3.Types{"boolean"}
		}
	case *types.Struct:
		schema.Type = &openapi3.Types{"object"}
		schema.Properties = make(map[string]*openapi3.SchemaRef)
		var requiredFields []string

		for i := 0; i < t.NumFields(); i++ {
			field := t.Field(i)
			if !field.Exported() {
				continue
			}
			jsonTag := reflect.StructTag(t.Tag(i)).Get("json")
			jsonName := strings.Split(jsonTag, ",")[0]
			if jsonName == "" || jsonName == "-" {
				continue
			}
			if _, isPointer := field.Type().(*types.Pointer); !isPointer {
				requiredFields = append(requiredFields, jsonName)
			}
			schema.Properties[jsonName] = g.goTypeToSchemaRef(field.Type())
		}
		if len(requiredFields) > 0 {
			schema.Required = requiredFields
		}
	case *types.Slice:
		schema.Type = &openapi3.Types{"array"}
		schema.Items = g.goTypeToSchemaRef(t.Elem())
	case *types.Map:
		schema.Type = &openapi3.Types{"object"}
		schema.AdditionalProperties = openapi3.AdditionalProperties{Schema: g.goTypeToSchemaRef(t.Elem())}
	}

	return &openapi3.SchemaRef{Value: schema}
}

// generateTagFromPath creates a tag from the first segment of a URL path.
func generateTagFromPath(path string) string {
	trimmedPath := strings.TrimPrefix(path, "/")
	parts := strings.Split(trimmedPath, "/")
	if len(parts) > 0 && parts[0] != "" {
		return strings.ToUpper(parts[0])
	}
	return "Default"
}

// generateOperationID creates a unique ID from the method and path.
func generateOperationID(method, path string) string {
	path = strings.ReplaceAll(path, "/", " ")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	return strings.ToLower(method) + "_" + strings.Join(strings.Fields(path), "_")
}
