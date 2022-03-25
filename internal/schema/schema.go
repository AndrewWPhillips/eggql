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
	gqlInterfaceKeyword = "interface"
	gqlUnionKeyword     = "union"
)

// MustBuild is the same as Build but panics on error
func MustBuild(qms ...interface{}) string {
	enums, ok := qms[0].(map[string][]string) // check if enums given
	if ok {
		qms = qms[1:] // separate enums from the rest (query/mutation/subscription)
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
func Build(rawEnums map[string][]string, qms ...interface{}) (string, error) {
	enums, err := validateEnums(rawEnums)
	if err != nil {
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

		builder.WriteString(typeName)
		builder.WriteRune('\n')
	}
	builder.WriteString(closeString) // close schema clause

	// Work out space needed for the types and get a list of names to sort
	names := make([]string, 0, len(schemaTypes.declaration))
	objectsLength := 0
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

	// Unions - work out order and length
	names = make([]string, 0, len(schemaTypes.unions))
	objectsLength = 0
	for unionName, unionValues := range schemaTypes.unions {
		objectsLength += len(gqlUnionKeyword) + 1 + len(unionName) // union <name>
		names = append(names, unionName)
		for _, v := range unionValues {
			objectsLength += 3 + len(v)
		}
		objectsLength += 1 // eoln
	}
	builder.Grow(objectsLength)

	sort.Strings(names)
	for _, unionName := range names {
		builder.WriteString(gqlUnionKeyword)
		builder.WriteRune(' ')
		builder.WriteString(unionName)
		sep := " = "
		for _, v := range schemaTypes.unions[unionName] {
			builder.WriteString(sep)
			builder.WriteString(v)
			sep = " | "
		}
		builder.WriteRune('\n')
	}

	// calc. space for enum strings (to grow the string builder) and make list of enums to sort
	names = make([]string, 0, len(rawEnums))
	objectsLength = 0
	for enumName, enumValues := range rawEnums {
		names = append(names, enumName)
		objectsLength += 12 + len(enumName)
		if strings.Contains(enumName, "#") {
			objectsLength += 3
		}
		for _, v := range enumValues {
			objectsLength += 2 + len(v)
			if strings.Contains(v, "#") {
				objectsLength += 4
			}
		}
	}
	builder.Grow(objectsLength)
	sort.Strings(names) // this ensures we always output the enums in the same order

	// Add the enums to the schema
	var parts []string
	for _, enumName := range names {
		parts = strings.SplitN(enumName, "#", 2)
		if len(parts) > 1 && parts[1] != "" {
			builder.WriteRune('"')
			builder.WriteString(parts[1])
			builder.WriteString("\"\n")
		}
		builder.WriteString(gqlEnumType)
		builder.WriteRune(' ')
		builder.WriteString(parts[0])
		builder.WriteString(openString)
		for _, v := range rawEnums[enumName] {
			parts = strings.SplitN(v, "#", 2)
			if len(parts) > 1 && parts[1] != "" {
				builder.WriteString(" \"")
				builder.WriteString(parts[1])
				builder.WriteString("\"\n")
			}
			builder.WriteRune(' ')
			builder.WriteString(parts[0])
			builder.WriteRune('\n')
		}
		builder.WriteString(closeString)
	}

	return builder.String(), nil
}
