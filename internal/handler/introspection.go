package handler

// introspection.go implements the introspection type which handles the GraphQL __schema and __type queries

import (
	"github.com/vektah/gqlparser/v2/ast"
)

type (
	introspection struct {
		astSchema *ast.Schema

		Schema  gqlSchema             `graphql:"__schema"`
		GetType func(string) *gqlType `graphql:"__type,args(name)"`
	}

	gqlSchema struct {
		Types            []gqlType
		QueryType        *gqlType
		MutationType     *gqlType
		SubscriptionType *gqlType
		Directives       []gqlDirective
	}

	gqlType struct {
		Kind              int `graphql:"kind:__TypeKind"`
		Name, Description string
		Fields            []gqlField
		Interfaces        []gqlType
		PossibleTypes     []gqlType
		EnumValues        []gqlEnumValue
		InputFields       []gqlInputValue
		OfType            *gqlType
	}

	gqlField struct {
		Name, Description string
		Args              []gqlInputValue
		Type              gqlType
		IsDeprecated      bool
		DeprecationReason string
	}

	gqlInputValue struct {
		Name, Description string
		Type              gqlType
		DefaultValue      string
	}

	gqlEnumValue struct {
		Name, Description string
		IsDeprecated      bool
		DeprecationReason string
	}

	gqlDirective struct {
		Name, Description string
		Locations         []int `graphql:"locations:[__DirectiveLocation!]!"`
		Args              []gqlInputValue
	}
)

var IntrospectionEnums = map[string][]string{
	"__TypeKind": {"SCALAR", "OBJECT", "INTERFACE", "UNION", "ENUM", "INPUT_OBJECT", "LIST", "NON_NULL"},

	"__DirectiveLocation": {"QUERY", "MUTATION", "SUBSCRIPTION", "FIELD", "FRAGMENT_DEFINITION", "FRAGMENT_SPREAD", "INLINE_FRAGMENT", "SCHEMA",
		"SCALAR", "OBJECT", "FIELD_DEFINITION", "ARGUMENT_DEFINITION", "INTERFACE", "UNION", "ENUM", "ENUM_VALUE", "INPUT_OBJECT", "INPUT_FIELD_DEFINITION"},
}

func NewIntrospectionData(astSchema *ast.Schema) interface{} {
	i := &introspection{
		astSchema: astSchema,
		Schema: gqlSchema{
			Types: getTypes(astSchema.Types),
		},
	}
	if astSchema.Query != nil {
		i.Schema.QueryType = &gqlType{
			Kind:        getTypeKind(astSchema.Query.Kind),
			Name:        astSchema.Query.Name,
			Description: astSchema.Query.Description,
			Fields:      getFields(astSchema.Query.Fields),
		}
	}
	if astSchema.Mutation != nil {
		i.Schema.MutationType = &gqlType{
			Kind:        getTypeKind(astSchema.Mutation.Kind),
			Name:        astSchema.Mutation.Name,
			Description: astSchema.Mutation.Description,
			Fields:      getFields(astSchema.Mutation.Fields),
		}
	}
	// TODO subscription
	i.GetType = i.getType
	return i
}

func (pi *introspection) getType(name string) *gqlType {
	defn := pi.astSchema.Types[name]
	if defn == nil {
		return nil
	}
	return &gqlType{
		Kind:        getTypeKind(defn.Kind),
		Name:        name,
		Description: defn.Description,
		Fields:      getFields(defn.Fields),
		Interfaces:  getInterfaces(defn.Interfaces),
		EnumValues:  getEnumValues(defn.EnumValues),
	}
}

func getTypes(schemaTypes map[string]*ast.Definition) (r []gqlType) {
	for name, defn := range schemaTypes {
		r = append(r, gqlType{
			Kind:        getTypeKind(defn.Kind),
			Name:        name,
			Description: defn.Description,
			Fields:      getFields(defn.Fields),
			Interfaces:  getInterfaces(defn.Interfaces),
			EnumValues:  getEnumValues(defn.EnumValues),
		})
	}
	return
}

func getInterfaces(interfaces []string) (r []gqlType) {
	for _, name := range interfaces {
		r = append(r, gqlType{Name: name})
	}
	return
}

func getEnumValues(values ast.EnumValueList) (r []gqlEnumValue) {
	for _, v := range values {
		r = append(r, gqlEnumValue{
			Name:        v.Name,
			Description: v.Description,
		})
	}
	return
}

func getTypeKind(kind ast.DefinitionKind) int {
	for i, k := range IntrospectionEnums["__TypeKind"] {
		if k == string(kind) {
			return i
		}
	}
	panic("type kind not found" + string(kind))
	return -1
}

func getKindFromValueKind(valueKind ast.ValueKind) int {
	if valueKind > ast.Variable && valueKind < ast.EnumValue {
		return 0 // scalar
	}
	if valueKind == ast.EnumValue {
		return 4
	}
	if valueKind == ast.ListValue {
		return 6
	}
	if valueKind == ast.ObjectValue {
		return 1
	}
	return 1
}

func getType(dv *ast.Value, t *ast.Type) gqlType {
	var kind = 1
	var desc string
	var fields []gqlField
	if dv != nil {
		kind = getKindFromValueKind(dv.Kind)
		if dv.Definition != nil {
			desc = dv.Definition.Description
			fields = getFields(dv.Definition.Fields)
		}
	}
	return gqlType{
		Kind:        kind,
		Name:        t.Name(),
		Description: desc,
		Fields:      fields,
	}
}

func getArgs(arguments ast.ArgumentDefinitionList) (r []gqlInputValue) {
	for _, arg := range arguments {
		raw := ""
		if arg.DefaultValue != nil {
			raw = arg.DefaultValue.Raw
		}
		r = append(r, gqlInputValue{
			Name:         arg.Name,
			Description:  arg.Description,
			Type:         getType(arg.DefaultValue, arg.Type),
			DefaultValue: raw,
		})
	}
	return
}

func getFields(fields ast.FieldList) (r []gqlField) {
	for _, f := range fields {
		r = append(r, gqlField{
			Name:        f.Name,
			Description: f.Description,
			Args:        getArgs(f.Arguments),
			Type:        getType(f.DefaultValue, f.Type),
		})
	}
	return
}
