package handler

// introspection.go implements the introspection type which handles the GraphQL __schema and __type queries

import (
	"github.com/vektah/gqlparser/v2/ast"
)

type (
	// introspectionSchema just embeds the gqlparser schema so that we can add methods to it
	introspectionSchema struct{ *ast.Schema }

	// introspectionObject represents a type definition
	introspectionObject struct {
		*ast.Definition
		parent introspectionSchema
	}
	// introspectionField represents a field of an object/input type
	introspectionField struct {
		*ast.FieldDefinition
		parent introspectionObject
	}
	// introspectionArgument represents an argument to a field
	introspectionArgument struct {
		*ast.ArgumentDefinition
		parent introspectionField
	}
	// introspectionEnumValue represents a value of an enum
	introspectionEnumValue struct {
		*ast.EnumValueDefinition
		parent introspectionObject
	}
	// introspectionType can represent any type of the schema including list/nullable versions of other types
	introspectionType struct {
		*ast.Type
		parent introspectionSchema
	}

	// introspectionQuery represents the GraphQL root query object used for introspection queries
	// It only has "__schema" and the "__type(name)" fields.  Note that the other introspection
	// query (__typename) can be included at any level of a query and is not handled here.
	// (Of course, there is no root mutation/subscription as the schema cannot change.)
	introspectionQuery struct {
		iss       introspectionSchema
		GetSchema func() gqlSchema      `egg:"__schema"`
		GetType   func(string) *gqlType `egg:"__type(name)"`
	}

	// gqlSchema represents the GraphQL "__Schema" type returned by "__schema" query
	gqlSchema struct {
		Description      string
		Types            func() []gqlType
		QueryType        func() *gqlType
		MutationType     func() *gqlType
		SubscriptionType func() *gqlType
		Directives       func() []gqlDirective
	}

	// gqlType represents the GraphQL "__Type" type used in lots of places in introspection
	gqlType struct {
		Kind              int `egg:"kind:__TypeKind"`
		Name, Description string
		Fields            func(bool) []gqlField `egg:"(includeDeprecated=false),nullable"`
		Interfaces        func() []gqlType
		PossibleTypes     func() []gqlType
		EnumValues        func(bool) []gqlEnumValue `egg:"(includeDeprecated=false),nullable"`
		InputFields       func() []gqlInputValue
		OfType            *gqlType // nil unless kind is "LIST" or "NON_NULL"
		SpecifiedByUrl    string
	}

	// gqlField represents the GraphQL "__Field" type
	gqlField struct {
		Name, Description string
		// Remove deprecation from arguments - not (yet?) supported by vektah/gqlparser
		//Args func(bool) []gqlInputValue `egg:"(includeDeprecated=false)"`
		Args              func() []gqlInputValue
		Type              func() gqlType
		IsDeprecated      func() bool
		DeprecationReason func() string
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
		IsDeprecated      func() bool
		DeprecationReason func() string
	}

	// gqlDirective represents the GraphQL "__Directive" type
	gqlDirective struct {
		Name, Description string
		Locations         []int           `egg:":[__DirectiveLocation!]!"`
		Args              []gqlInputValue `egg:":[__InputValue!]!"`
		IsRepeatable      bool
	}
)

// IntroEnums stores the name and values (text) of the __TypeKind and __DirectiveLocation enums
var IntroEnums = map[string][]string{
	"__TypeKind": {"SCALAR", "OBJECT", "INTERFACE", "UNION", "ENUM", "INPUT_OBJECT", "LIST", "NON_NULL"},

	"__DirectiveLocation": {
		"QUERY", "MUTATION", "SUBSCRIPTION", "FIELD", "FRAGMENT_DEFINITION", "FRAGMENT_SPREAD", "INLINE_FRAGMENT",
		"SCHEMA",
		"SCALAR", "OBJECT", "FIELD_DEFINITION", "ARGUMENT_DEFINITION", "INTERFACE", "UNION", "ENUM", "ENUM_VALUE",
		"INPUT_OBJECT", "INPUT_FIELD_DEFINITION",
	},
}

// IntroEnumsReverse stores the same enums as IntroEnum, as maps for reverse lookup of int values
// Each enum is a map keyed by the enum value (string) giving the underlying (int) value
var IntroEnumsReverse = map[string]map[string]int{
	"__TypeKind": {
		"SCALAR":       0,
		"OBJECT":       1,
		"INTERFACE":    2,
		"UNION":        3,
		"ENUM":         4,
		"INPUT_OBJECT": 5,
		"LIST":         6,
		"NON_NULL":     7,
	},
	"__DirectiveLocation": {
		"QUERY":                  0,
		"MUTATION":               1,
		"SUBSCRIPTION":           2,
		"FIELD":                  3,
		"FRAGMENT_DEFINITION":    4,
		"FRAGMENT_SPREAD":        5,
		"INLINE_FRAGMENT":        6,
		"SCHEMA":                 7,
		"SCALAR":                 8,
		"OBJECT":                 9,
		"FIELD_DEFINITION":       10,
		"ARGUMENT_DEFINITION":    11,
		"INTERFACE":              12,
		"UNION":                  13,
		"ENUM":                   14,
		"ENUM_VALUE":             15,
		"INPUT_OBJECT":           16,
		"INPUT_FIELD_DEFINITION": 17,
	},
}

func init() {
	// validate that IntroEnums and IntroEnumsReverse are consistent
	if len(IntroEnums) != len(IntroEnumsReverse) {
		panic("different number of enums")
	}
	for name, list := range IntroEnums {
		m, ok := IntroEnumsReverse[name]
		if !ok || len(list) != len(m) {
			panic("IntroEnums inconsistency detected with " + name)
		}
		for i, v := range list {
			value, ok2 := m[v]
			if !ok2 || value != i {
				panic("IntroEnums inconsistency detected in " + name + " value " + v)
			}
		}
	}
}

func NewIntrospectionData(astSchema *ast.Schema) interface{} {
	pi := &introspectionQuery{iss: introspectionSchema{astSchema}}
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

// getTypeKind returns the enum __TypeKind value (int) corresp. to a string
func getTypeKind(kind ast.DefinitionKind) int {
	return IntroEnumsReverse["__TypeKind"][string(kind)]
}

func (iso introspectionObject) getFields(includeDeprecated bool) []gqlField {
	if iso.Fields == nil {
		return nil
	}
	r := make([]gqlField, 0, len(iso.Fields))
fieldLoop:
	for _, field := range iso.Fields {
		if !includeDeprecated {
			// skip deprecated fields
			for _, directive := range field.Directives {
				if directive.Name == "deprecated" {
					continue fieldLoop
				}
			}
		}
		isf := introspectionField{field, iso}
		r = append(r, gqlField{
			Name:              isf.Name,
			Description:       isf.Description,
			Args:              isf.getArgs,
			Type:              isf.getType,
			IsDeprecated:      isf.getIsDeprecated,
			DeprecationReason: isf.getDeprecationReason,
		})
	}
	return r
}

func (iso introspectionObject) getEnumValues(includeDeprecated bool) []gqlEnumValue {
	r := make([]gqlEnumValue, 0, len(iso.EnumValues))
valueLoop:
	for _, v := range iso.EnumValues {
		if !includeDeprecated {
			// skip deprecated values
			for _, directive := range v.Directives {
				if directive.Name == "deprecated" {
					continue valueLoop
				}
			}
		}
		isv := introspectionEnumValue{v, iso}
		r = append(r, gqlEnumValue{
			Name:              isv.Name,
			Description:       isv.Description,
			IsDeprecated:      isv.getIsDeprecated,
			DeprecationReason: isv.getDeprecationReason,
		})
	}
	return r
}

func (iso introspectionObject) getInterfaces() []gqlType {
	r := make([]gqlType, 0, len(iso.Interfaces))
	for _, name := range iso.Interfaces {
		r = append(r, *iso.parent.getType(name))
	}
	return r
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

// getType gets the type associated with a GraphQL field
func (isf introspectionField) getType() gqlType {
	return *introspectionType{isf.Type, isf.parent.parent}.getType()
}

// getIsDeprecated gets whether a field is deprecated
func (isf introspectionField) getIsDeprecated() bool {
	for _, directive := range isf.Directives {
		if directive.Name == "deprecated" {
			return true
		}
	}
	return false
}

// getDeprecationReason gets the reason a field is deprecated
func (isf introspectionField) getDeprecationReason() string {
	for _, directive := range isf.Directives {
		if directive.Name == "deprecated" {
			if len(directive.Arguments) > 0 {
				return directive.Arguments[0].Value.Raw
			}
			break
		}
	}
	return ""
}

// getType gets the type associated with a GraphQL field's argument
func (isa introspectionArgument) getType() gqlType {
	return *introspectionType{isa.Type, isa.parent.parent.parent}.getType()
}

// getIsdeprecated gets whether an enum value is deprecated
func (isv introspectionEnumValue) getIsDeprecated() bool {
	for _, directive := range isv.Directives {
		if directive.Name == "deprecated" {
			return true
		}
	}
	return false
}

// getDeprecationReason gets the reason an enum value is deprecated
func (isv introspectionEnumValue) getDeprecationReason() string {
	for _, directive := range isv.Directives {
		if directive.Name == "deprecated" {
			if len(directive.Arguments) > 0 {
				return directive.Arguments[0].Value.Raw
			}
			break
		}
	}
	return ""
}

// getType returns type info for any type including lists/non_null types (whence OfType contains the underlying type)
func (ist introspectionType) getType() (r *gqlType) {
	if ist.Elem != nil {
		r = &gqlType{
			Kind:   IntroEnumsReverse["__TypeKind"]["LIST"],
			OfType: introspectionType{ist.Elem, ist.parent}.getType(),
		}
	} else {
		r = ist.parent.getType(ist.NamedType)
	}

	if ist.NonNull {
		r = &gqlType{
			Kind:   IntroEnumsReverse["__TypeKind"]["NON_NULL"],
			OfType: r,
		}
	}
	return
}
