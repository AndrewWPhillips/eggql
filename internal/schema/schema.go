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
	closeString      = "}\n\n"
	implementsString = " implements"

	gqlObjectTypeKeyword = "type"
	gqlInputKeyword      = "input"
	gqlEnumKeyword       = "enum"
	gqlScalarKeyword     = "scalar"
	gqlInterfaceKeyword  = "interface"
	gqlUnionKeyword      = "union"
)

// MustBuild calls Build but panics on error
// It takes an (optional) map of enums followed by (up to) 3 root objects (query, mutation, subscription)
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

// Build generates a string containing a GraphQL schema.
//   - rawEnums: map of enums where each map entry is a slice of strings - each string (incl.
//     map keys) is a name opt. followed by hash (#) and a description.
//     It can be nil if no enums are supplied.
//   - 2nd, 3rd and 4th parameters represent the root query, mutation and subscription and
//     *must* be structs (or nil). Each struct is scanned for exported fields to be used to
//     generate the query fields.
//     Any of the 3 can be nil if not implemented, but you must supply at least one.
func Build(rawEnums map[string][]string, qms ...interface{}) (string, error) {
	enums, err := validateEnums(rawEnums)
	if err != nil {
		return "", err
	}

	var entry [3]string             // the names of the 3 root entry points
	schemaTypes := newSchemaTypes() // all generated GraphQL types

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

		// Note: we call getTypeName just for the root type name.  We don't care about the other return
		// values since we know it's a struct (2nd return value will always be false, 3rd will be nil).
		// We pass nullable = true as we don't want the trailing exclamation mark (!)
		entry[i], _, err = schemaTypes.getTypeName(t, true)
		if err != nil {
			panic("type of root object (struct) should always be valid")
		}

		if entry[i] == "" { // if given an unnamed struct we use the default name
			switch EntryPoint(i) {
			case Query:
				entry[i] = "Query"
			case Mutation:
				entry[i] = "Mutation"
			case Subscription:
				entry[i] = "Subscription"
			default:
				return "", errors.New("More than 3 structs provided (at most should have: query, mutation, subscription)")
			}
		}

		// *** Add root type and (recursively) any contained types ***
		if err := schemaTypes.add(entry[i], t, enums, gqlObjectTypeKeyword, nil); err != nil {
			return "", fmt.Errorf("%w adding entry point %d %q", err, i, entry[i])
		}
	}

	// Build the schema from the found types (and supplied enums) and return it as text
	return schemaTypes.build(rawEnums, entry)
}

// build creates the full schema text from the type declarations and unions members + enum param
// - rawEnums: each map key is the enum name and the corresp. slice contains the enum values (starting at zero)
// - entry: contains the name of the 3 root object types (if empty string then that object type is not used)
// returns: schema as a string or an error
func (s schema) build(rawEnums map[string][]string, entry [3]string) (string, error) {
	builder := &strings.Builder{} // where the (text) schema is generated
	builder.Grow(256)             // Even simple schemas are at least this big

	// First write schema definition if using any non-std entry names
	if entry[0] != "" && entry[0] != "Query" ||
		entry[1] != "" && entry[1] != "Mutation" ||
		entry[2] != "" && entry[2] != "Subscription" {
		// then
		builder.WriteString("schema ")
		builder.WriteString(openString)
		for i := range entry {
			if entry[i] != "" {
				switch EntryPoint(i) {
				case Query:
					builder.WriteString(" query: ")
				case Mutation:
					builder.WriteString(" mutation: ")
				case Subscription:
					builder.WriteString(" subscription: ")
				}
				builder.WriteString(entry[i])
				builder.WriteRune('\n')
			}
		}
		builder.WriteString(closeString) // close schema clause
	}

	// Now write all the schema types. NOTE: where values are stored in maps (objects, unions and
	// enums) we get a slice of the keys and sort them so that we can write them in the same order
	// each time.  This is nec. to ensure consistent schema text for checking automated test results.

	// *** Objects - work out space needed for the objects and get a list of names to sort
	names := make([]string, 0, len(s.declaration))
	objectsLength := 0
	for k, obj := range s.declaration {
		if s.description[k] != "" {
			objectsLength += 7 + len(s.description[k])
		}
		objectsLength += len(obj) + 1
		names = append(names, k)
	}
	builder.Grow(objectsLength)

	sort.Strings(names)
	for _, name := range names { // append each "type" to the schema
		if s.description[name] != "" {
			builder.WriteString(`"""`)
			builder.WriteString(s.description[name])
			builder.WriteString(`"""`)
			builder.WriteRune('\n')
		}
		builder.WriteString(s.declaration[name])
	}

	// *** Unions - work out order of unions and length
	names = make([]string, 0, len(s.unions))
	objectsLength = 0
	for unionName, unionValues := range s.unions {
		names = append(names, unionName)
		objectsLength += len(gqlUnionKeyword) + 1 + len(unionName) // union <name>
		if unionValues.desc != "" {
			objectsLength += 7 + len(unionValues.desc) // six quotes + newline
		}
		for v := range unionValues.objects {
			objectsLength += 3 + len(v) // enum value + 2 spaces and either '=' or '|'
		}
		objectsLength += 1 // eoln
	}
	builder.Grow(objectsLength)

	sort.Strings(names)
	for _, unionName := range names { // append all the unions to the schema
		if s.unions[unionName].desc != "" {
			builder.WriteString(`"""`)
			builder.WriteString(s.unions[unionName].desc)
			builder.WriteString(`"""`)
			builder.WriteRune('\n')

		}
		builder.WriteString(gqlUnionKeyword)
		builder.WriteRune(' ')
		builder.WriteString(unionName)
		// Get sorted list of union names so the order is consistent
		unionNames := make([]string, 0, len(s.unions[unionName].objects))
		for n := range s.unions[unionName].objects {
			unionNames = append(unionNames, n)
		}
		sort.Strings(unionNames)

		sep := " = "
		for _, n := range unionNames {
			builder.WriteString(sep)
			builder.WriteString(n)
			sep = " | "
		}
		builder.WriteString("\n\n")
	}

	// *** Enums - calc. space for enum strings (to grow the string builder) and make list of enums to sort
	names = make([]string, 0, len(rawEnums))
	objectsLength = 0
	for enumName, enumValues := range rawEnums {
		names = append(names, enumName)
		objectsLength += 12 + len(enumName)
		if strings.Contains(enumName, "#") {
			objectsLength += 3 // 2 quote characters + a newline (we've already including the length of desc string above)
		}
		for _, v := range enumValues {
			objectsLength += 2 + len(v)
			if strings.Contains(v, "#") {
				objectsLength += 4 // space, 2 quotes, newline (we've already added desc string length)
			}
		}
	}
	builder.Grow(objectsLength)

	sort.Strings(names) // this ensures we always output the enums in the same order
	var parts []string
	for _, enumName := range names { // add all the enums
		parts = strings.SplitN(enumName, "#", 2)
		if len(parts) > 1 && parts[1] != "" {
			builder.WriteRune('"')
			builder.WriteString(parts[1])
			builder.WriteString("\"\n")
		}
		builder.WriteString(gqlEnumKeyword)
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

	// *** Custom scalars
	objectsLength = 0
	for _, name := range *s.scalars {
		objectsLength += 8 + len(name)
	}
	builder.Grow(objectsLength)

	for _, name := range *s.scalars {
		builder.WriteString(gqlScalarKeyword)
		builder.WriteRune(' ')
		builder.WriteString(name)
		builder.WriteRune('\n')
	}

	return builder.String(), nil
}
