package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/iancoleman/strcase"
	"github.com/jinzhu/inflection"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

var (
	gotypeRe     = regexp.MustCompile(`^GoType: ?(.*)$`)
	spanColumnRe = regexp.MustCompile(`^SpannerColumn: ?(.*)$`)
)

func main() {
	var (
		s string
		t string
	)
	flag.StringVar(&s, "s", "", "graphql sdl file path")
	flag.StringVar(&t, "t", "", "template file path")
	flag.Parse()
	if s == "" || t == "" {
		flag.Usage()
		return
	}
	sb, err := read(s)
	if err != nil {
		log.Fatal(err)
	}
	body, err := loadGQL(sb)
	if err != nil {
		log.Fatal(err)
	}

	tb, err := read(t)
	if err != nil {
		log.Fatal(err)
	}
	funcMap := sprig.GenericFuncMap()
	// sprig camelcase (xstrings.ToCamelCase) is not valid
	funcMap["snakeToUpperCamel"] = func(a string) string { return strcase.ToCamel(strings.ToLower(a)) }
	funcMap["lowerCamel"] = func(a string) string { return strcase.ToLowerCamel(a) }
	funcMap["joinstr"] = func(a, b string) string { return a + b }
	// TODO for cloud spanner type...
	funcMap["GoType"] = func(f *ast.FieldDefinition, replaceObjectType bool) string {
		switch f.Type.NamedType {
		case "ID", "String", "Int", "Float", "Boolean":
			return addPtPrefixIfNull(f.Type) + goSingleType(f.Type, body, replaceObjectType)
		case "": //list
			return "[]" + addPtPrefixIfNull(f.Type.Elem) + goSingleType(f.Type.Elem, body, replaceObjectType)
		default: // custom scalar, other object
			return addPtPrefixIfNull(f.Type) + goSingleType(f.Type, body, replaceObjectType)
		}
		return ""
	}
	funcMap["SpannerGoType"] = func(f *ast.FieldDefinition, replaceObjectType bool) string {
		return spannerGoSingleType(f, body, replaceObjectType)
	}
	funcMap["exists"] = func(d *ast.Definition, name string) bool {
		for _, it := range d.Fields {
			if strcase.ToCamel(it.Name) == name {
				return true
			}
		}
		return false

	}
	funcMap["foundPK"] = func(objName string, fields ast.FieldList) *ast.FieldDefinition {
		for _, f := range fields {
			desc := f.Description
			if strings.Contains(desc, "SpannerPK") || strcase.ToCamel(f.Name) == "Id" || strcase.ToCamel(f.Name) == strcase.ToCamel(objName+"Id") {
				return f
			}

		}
		return nil
	}
	funcMap["isObject"] = func(f *ast.FieldDefinition) bool {
		return isObject(f, body)
	}

	funcMap["convertName"] = func(f *ast.FieldDefinition) string {
		desc := f.Description
		match := spanColumnRe.FindStringSubmatch(desc)
		if match != nil && len(match) > 1 {
			return match[1]
		}
		return f.Name
	}

	funcMap["ConvertObjectFieldName"] = func(f *ast.FieldDefinition) string {
		if !isObject(f, body) {
			return f.Name
		}
		name := f.Name
		desc := f.Description
		match := spanColumnRe.FindStringSubmatch(desc)
		if match != nil && len(match) > 1 {
			name = match[1]
		}
		namedType := f.Type.NamedType
		isArray := false
		if f.Type.NamedType == "" {
			isArray = true
			namedType = f.Type.Elem.NamedType
		}
		if def, ok := body.Types[namedType]; ok {
			if def.Kind == "OBJECT" {
				if isArray {
					return inflection.Plural(inflection.Singular(name) + "Id")
				}
				return name + "Id"
			} else {
				return name
			}
		}
		return name
	}
	tpl := template.Must(template.New(t).Funcs(template.FuncMap(funcMap)).Parse(string(tb)))
	if err := tpl.Execute(os.Stdout, *body); err != nil {
		log.Fatal(err)
	}
}
func read(file string) ([]byte, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func addPtPrefixIfNull(t *ast.Type) string {
	if t.NonNull {
		return ""
	}
	return "*"
}

func goSingleType(t *ast.Type, body *ast.Schema, replace bool) string {
	switch t.NamedType {
	case "ID", "String":
		return "string"
	case "Int":
		return "int64"
	case "Float":
		return "float64"
	case "Boolean":
		return "bool"
	default:
		if def, ok := body.Types[t.NamedType]; ok {
			match := gotypeRe.FindStringSubmatch(def.Description)
			if match != nil && len(match) > 1 {
				return match[1]
			}
			if !replace {
				return t.NamedType
			}
			for _, f := range def.Fields {
				desc := f.Description
				if strings.Contains(desc, "SpannerPK") || strcase.ToCamel(f.Name) == "Id" || strcase.ToCamel(f.Name) == strcase.ToCamel(t.NamedType+"Id") {
					return goSingleType(f.Type, body, replace)
				}

			}
			return "string"
		}
	}
	log.Fatalf("not found type %s", t.NamedType)
	return ""
}
func spannerGoSingleType(f *ast.FieldDefinition, body *ast.Schema, replaceObjectType bool) string {
	switch f.Type.NamedType {
	case "ID", "String":
		if f.Type.NonNull {
			return goSingleType(f.Type, body, replaceObjectType)
		}
		return "spanner.NullString"
	case "Int":
		if f.Type.NonNull {
			return goSingleType(f.Type, body, replaceObjectType)
		}
		return "spanner.NullInt64"

	case "Float":
		if f.Type.NonNull {
			return goSingleType(f.Type, body, replaceObjectType)
		}
		return "spanner.NullFloat64"
	case "Boolean":
		if f.Type.NonNull {
			return goSingleType(f.Type, body, replaceObjectType)
		}
		return "spanner.NullBool"
	case "": //list
		return "[]" + addPtPrefixIfNull(f.Type.Elem) + goSingleType(f.Type.Elem, body, replaceObjectType)
	default: // custom scalar, other object
		if def, ok := body.Types[f.Type.NamedType]; ok {
			if def.Kind == "ENUM" {
				if f.Type.NonNull {
					return addPtPrefixIfNull(f.Type) + goSingleType(f.Type, body, replaceObjectType)
				}
				return "spanner.NullString"
			}
			if def.Kind == "OBJECT" {
				if f.Type.NonNull || !replaceObjectType {
					return addPtPrefixIfNull(f.Type) + goSingleType(f.Type, body, replaceObjectType)
				}
				for _, df := range def.Fields {
					desc := df.Description
					if strings.Contains(desc, "SpannerPK") || strcase.ToCamel(df.Name) == "Id" || strcase.ToCamel(df.Name) == strcase.ToCamel(f.Type.NamedType+"Id") {
						return spannerGoSingleType(f, body, false)
					}

				}
				return "spanner.NullString"
			}
		}
		return addPtPrefixIfNull(f.Type) + goSingleType(f.Type, body, replaceObjectType)
	}
	return ""
}

func isObject(f *ast.FieldDefinition, body *ast.Schema) bool {
	namedType := f.Type.NamedType
	if f.Type.NamedType == "" {
		namedType = f.Type.Elem.NamedType
	}
	if def, ok := body.Types[namedType]; ok {
		if def.Kind == "OBJECT" {
			return true
		}
	}
	return false
}
func loadGQL(b []byte) (*ast.Schema, error) {
	astDoc, err := gqlparser.LoadSchema(&ast.Source{
		Input: string(b),
	})
	if err != nil {
		return nil, err
	}
	return astDoc, nil
}
