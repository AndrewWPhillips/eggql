// Package schema can be used to generate a GraphQL schema (as a string) from
// Go structure(s) representing the GraphQL query (and mutation and subscription)
// entry points.  This goes hand-in-hand with the "handler" which uses instantiations
// of those same structures to fulfill the query (mutation/subscription).
package schema

// schema.go contains the exported functions - Build and MustBuild

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// EntryPoint is an "enumeration" for the 3 different types of GraphQL entry point (query, mutation, subscription)
type EntryPoint int

const (
	Query EntryPoint = iota
	Mutation
	Subscription
)

const (
	openString       = " {\n"
	closeString      = "}\n"
	implementsString = " implements"

	gqlObjectType    = "type"
	gqlInputType     = "input"
	gqlEnumType      = "enum"
	gqlInterfaceType = "interface"
)

// MustBuild is the same as Build but panics on error
func MustBuild(qms ...interface{}) string {
	var enums map[string][]string
	if e, ok := qms[0].(map[string][]string); ok {
		enums = e
		qms = qms[1:]
	}
	s, err := Build(enums, qms...)
	if err != nil {
		panic(err)
	}
	return s
}

// Build generates a string containing a GraphQL schema from Go structs.
// It analyses a Go "query" struct (and optionally mutation and subscription) using
// any public fields to be used as queries.
func Build(enums map[string][]string, qms ...interface{}) (string, error) {
	if err := validateEnums(enums); err != nil {
		return "", err
	}
	builder := &strings.Builder{}   // where the (text) schema is generated
	schemaTypes := newSchemaTypes() // all generated GraphQL types

	// Create the "schema" clause of the schema document with query name etc
	builder.Grow(256) // Even simple schemas are at least this big
	builder.WriteString("schema ")
	builder.WriteString(openString)

	for i, v := range qms {
		if v == nil {
			continue // skip it
		}
		t := reflect.TypeOf(v)
		// If it's a pointer use the pointed to struct
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return "", errors.New("parameters to schema.Build must be structs")
		}

		// This switch means that query/mutation/subscription must be supplied in that order
		var entryPointName string
		switch EntryPoint(i) {
		case Query:
			builder.WriteString(" query: ")
			entryPointName = "Query"
		case Mutation:
			builder.WriteString(" mutation: ")
			entryPointName = "Mutation"
		case Subscription:
			return "", errors.New("Subscriptions are not yet supported")
			//builder.WriteString(" subscription: ") // TODO: implement subscriptions
			//entryPointName = "Subscription"
		default:
			return "", errors.New("More than 3 structs provided for schema (can only have query, mutation, subscription)")
		}
		typeName, _ := getTypeName(t)
		if typeName == "" {
			typeName = entryPointName // use default name for anon struct
		}
		// TODO omit "schema definition" if we're using Default Root Operation Type Names
		if err := schemaTypes.add(typeName, t, enums, gqlObjectType); err != nil {
			return "", fmt.Errorf("%w adding %q building schema for %s", err, typeName, entryPointName)
		}

		//builder.WriteString(" ")
		builder.WriteString(typeName)
		builder.WriteRune('\n')
	}
	builder.WriteString(closeString) // close schema clause

	// Work out space needed for the types and get a list of names to sort
	objectsLength := 0
	names := make([]string, 0, len(schemaTypes.declaration))
	for k, obj := range schemaTypes.declaration {
		objectsLength += len(obj) + 1
		names = append(names, k)
	}
	builder.Grow(objectsLength)

	// Add the GraphQL type to the schema
	sort.Strings(names) // we need to always output the types in the same order (eg for consistency in tests)
	for _, name := range names {
		builder.WriteString(schemaTypes.declaration[name])
	}

	// Work out how much space the enums will need and grow the string builder
	names = make([]string, 0, len(enums))
	objectsLength = 0
	for enumName, enumValues := range enums {
		names = append(names, enumName)
		objectsLength += 12 + len(enumName)
		for _, v := range enumValues {
			objectsLength += 2 + len(v)
		}
	}
	builder.Grow(objectsLength)

	// Add the enums to the schema
	sort.Strings(names) // this ensures we always output the enums in the same order
	for _, enumName := range names {
		builder.WriteString(gqlEnumType)
		builder.WriteRune(' ')
		builder.WriteString(enumName)
		builder.WriteString(openString)
		for _, v := range enums[enumName] {
			builder.WriteRune(' ')
			builder.WriteString(v)
			builder.WriteRune('\n')
		}
		builder.WriteString(closeString)
	}

	return builder.String(), nil
}
