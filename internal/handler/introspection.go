package handler

// introspection.go implements the introspection type which handles the GraphQL __schema and __type queries

import (
	"github.com/vektah/gqlparser/v2/ast"
	"log"
)

type (
	// introspectionSchema just embeds the gqlparser schema so that we can add methods to it
	introspectionSchema struct{ *ast.Schema }
	// introspectionObject can represent a named type of the schema
	introspectionObject struct {
		*ast.Definition
		parent introspectionSchema
	}
	// introspectionField can represent a field of an object/input type
	introspectionField struct {
		*ast.FieldDefinition
		parent introspectionObject
	}
	// introspectionArgument represents an argument to a field
	introspectionArgument struct {
		*ast.ArgumentDefinition
		parent introspectionField
	}
	// introspectionType can represent any type of the schema including list/nullable versions of other types
	introspectionType struct {
		*ast.Type
		parent introspectionSchema
	}

	// introspection represents the GraphQL root query object for introspection
	// It only supports the "__schema" and the "__type(name)" queries.  Note that the other introspection
	// query (__typename) can be included at any level of a query and is not handled here.
	introspection struct {
		iss       introspectionSchema
		GetSchema func() gqlSchema      `graphql:"__schema"`
		GetType   func(string) *gqlType `graphql:"__type,args(name)"`
	}

	// gqlSchema represents the GraphQL "__schema" type
	gqlSchema struct {
		Description      string
		Types            func() []gqlType
		QueryType        func() *gqlType
		MutationType     func() *gqlType
		SubscriptionType func() *gqlType
		Directives       func() []gqlDirective
	}

	// gqlType represents the GraphQL "__Type" type
	gqlType struct {
		Kind              int `graphql:"kind:__TypeKind"`
		Name, Description string
		Fields            func(bool) []gqlField `graphql:",args(includeDeprecated=false)"`
		Interfaces        func() []gqlType
		PossibleTypes     func() []gqlType
		EnumValues        func(bool) []gqlEnumValue `graphql:",args(includeDeprecated=false)"`
		InputFields       func() []gqlInputValue
		OfType            *gqlType // nil unless kind is "LIST" or "NON_NULL"
		SpecifiedByUrl    string
	}

	// gqlField represents the GraphQL "__Field" type
	gqlField struct {
		Name, Description string
		// Remove deprecation from arguments - not (yet?) supported by vektah/gqlparser
		//Args              func(bool) []gqlInputValue `graphql:",args(includeDeprecated=false)"`
		Args              func() []gqlInputValue
		Type              func() gqlType
		IsDeprecated      bool
		DeprecationReason string
	}

	// gqlInputValue represents the GraphQL "__InputValue" type
	gqlInputValue struct {
		Name, Description string
		Type              func() gqlType
		DefaultValue      string
		// Remove deprecation - not (yet?) supported by vektah/gqlparser
		//IsDeprecated      bool
		//DeprecationReason string
	}

	// gqlEnumValue represents the GraphQL "__EnumValue" type
	gqlEnumValue struct {
		Name, Description string
		IsDeprecated      bool
		DeprecationReason string
	}

	// gqlDirective represents the GraphQL "__Directive" type
	gqlDirective struct {
		Name, Description string
		Locations         []int           `graphql:":[__DirectiveLocation!]!"`
		Args              []gqlInputValue `graphql:":[__InputValue!]!"`
		IsRepeatable      bool
	}
)

// IntrospectionEnums stores the name and values (text) of the __TypeKind and __DirectiveLocation enums
var IntrospectionEnums = map[string][]string{
	"__TypeKind": {"SCALAR", "OBJECT", "INTERFACE", "UNION", "ENUM", "INPUT_OBJECT", "LIST", "NON_NULL"},

	"__DirectiveLocation": {"QUERY", "MUTATION", "SUBSCRIPTION", "FIELD", "FRAGMENT_DEFINITION", "FRAGMENT_SPREAD", "INLINE_FRAGMENT", "SCHEMA",
		"SCALAR", "OBJECT", "FIELD_DEFINITION", "ARGUMENT_DEFINITION", "INTERFACE", "UNION", "ENUM", "ENUM_VALUE", "INPUT_OBJECT", "INPUT_FIELD_DEFINITION"},
}

func NewIntrospectionData(astSchema *ast.Schema) interface{} {
	pi := &introspection{iss: introspectionSchema{astSchema}}
	pi.GetSchema = pi.iss.getSchema
	pi.GetType = pi.iss.getType
	return pi
}

func (iss introspectionSchema) getSchema() gqlSchema {
	return gqlSchema{
		Description:      iss.Description,
		Types:            iss.getTypes,
		QueryType:        iss.getQueryType,
		MutationType:     iss.getMutationType,
		SubscriptionType: iss.getSubscriptionType,
		Directives:       nil, // TODO
	}
}

// getType looks up a type by name
func (iss introspectionSchema) getType(name string) *gqlType {
	// Check the global list of "named" types
	definition := iss.Types[name]
	if definition == nil {
		return nil
	}
	if name == "Nested" {
		log.Println("qqq remove this", definition.Kind) // TODO
	}

	r := introspectionObject{definition, iss}.getType()
	return &r
}

// getTypes gets a list of all (named) types in the schema
func (iss introspectionSchema) getTypes() []gqlType {
	r := make([]gqlType, 0, len(iss.Types))
	for _, definition := range iss.Types {
		r = append(r, introspectionObject{definition, iss}.getType())
	}
	return r
}

// getQueryType gets the schema query type
func (iss introspectionSchema) getQueryType() *gqlType {
	if iss.Query == nil {
		return nil
	}
	r := introspectionObject{iss.Query, iss}.getType()
	return &r
}

// getMutationType gets the mutation object's type
func (iss introspectionSchema) getMutationType() *gqlType {
	if iss.Mutation == nil {
		return nil
	}
	r := introspectionObject{iss.Mutation, iss}.getType()
	return &r
}

// getSubscriptionType gets the subscription type
func (iss introspectionSchema) getSubscriptionType() *gqlType {
	if iss.Subscription == nil {
		return nil
	}
	r := introspectionObject{iss.Subscription, iss}.getType()
	return &r
}

// getType gets the type info for a named GraphQL type
func (iso introspectionObject) getType() gqlType {
	return gqlType{
		Kind:          getTypeKind(iso.Kind),
		Name:          iso.Name,
		Description:   iso.Description,
		Fields:        iso.getFields, // TODO check this does not have input fields
		Interfaces:    iso.getInterfaces,
		PossibleTypes: nil, // TODO?
		EnumValues:    iso.getEnumValues,
		InputFields:   nil, // TODO
	}
}

func (iso introspectionObject) getFields(bool) []gqlField {
	if iso.Fields == nil {
		return nil
	}
	r := make([]gqlField, 0, len(iso.Fields))
	for _, field := range iso.Fields {
		isf := introspectionField{field, iso}
		r = append(r, gqlField{
			Name:        field.Name,
			Description: field.Description,
			Args:        isf.getArgs,
			Type:        isf.getType,
		})
	}
	return r
}

func (iso introspectionObject) getEnumValues(bool) []gqlEnumValue {
	r := make([]gqlEnumValue, 0, len(iso.EnumValues))
	for _, v := range iso.EnumValues {
		r = append(r, gqlEnumValue{
			Name:        v.Name,
			Description: v.Description,
		})
	}
	return r
}

// getType gets the type associated with a GraphQL field
func (isf introspectionField) getType() gqlType {
	return *introspectionType{isf.Type, isf.parent.parent}.getType()
}

// getArgs gets a list of arguments for a field
//func (isf introspectionField) getArgs(includeDeprecated bool) []gqlInputValue {
func (isf introspectionField) getArgs() []gqlInputValue {
	r := make([]gqlInputValue, 0, len(isf.Arguments))
	for _, arg := range isf.Arguments {
		isa := introspectionArgument{arg, isf}
		raw := ""
		if arg.DefaultValue != nil {
			raw = arg.DefaultValue.Raw
		}
		r = append(r, gqlInputValue{
			Name:         arg.Name,
			Description:  arg.Description,
			Type:         isa.getType,
			DefaultValue: raw,
		})
	}
	return r
}

// getType gets the type associated with a GraphQL field's argument
func (isa introspectionArgument) getType() gqlType {
	return *introspectionType{isa.Type, isa.parent.parent.parent}.getType()
}

func (iso introspectionObject) getInterfaces() []gqlType {
	r := make([]gqlType, 0, len(iso.Interfaces))
	for _, name := range iso.Interfaces {
		r = append(r, *iso.parent.getType(name))
	}
	return r
}

// getType returns type info for any type including lists/non_null types (whence OfType contains the underlying type)
func (ist introspectionType) getType() *gqlType {
	if ist.NamedType != "" {
		return ist.parent.getType(ist.NamedType)
	}
	if ist.NonNull {
		return &gqlType{
			Kind:   7, // TODO lookup "NON_NULL"
			OfType: introspectionType{ist.Elem, ist.parent}.getType(),
		}
	}
	if ist.Elem != nil { // LIST
		// recurse into recursive data structure
		return &gqlType{
			Kind:   6, // TODO lookup "LIST"
			OfType: introspectionType{ist.Elem, ist.parent}.getType(),
		}
	}
	panic("Unhandled type in introspectionType.getType()")
}

// getTypeKind returns the enum __TypeKind value (int) corresp. to a string
func getTypeKind(kind ast.DefinitionKind) int {
	for i, k := range IntrospectionEnums["__TypeKind"] { // TODO use reverse lookup map
		if k == string(kind) {
			return i
		}
	}
	panic("type kind not found" + string(kind))
	return -1
}
