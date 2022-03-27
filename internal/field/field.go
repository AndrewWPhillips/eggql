// Package field is for analysing Go struct fields for use as GraphQL query fields (resolvers)
package field

// field.go generates GraphQL resolver info from a Go struct field

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Info is returned Get() with info extracted from a struct field to be used as a GraphQL query resolver.
// The info is obtained from the field's name, type and "graphql" (metadata) tag.
// Note that since Go has no native enums the GraphQL enum names are handled in metadata for
// both resolver return value and arguments (see metadata examples).
type Info struct {
	Name        string // field name for use in GraphQL queries - based on metadata (tag) or Go struct field name
	GQLTypeName string // GraphQL type name (may be empty but is required for GraphQL enums)
	//Kind        reflect.Kind
	ResultType reflect.Type // Type (Go) used to generate the resolver (GraphQL) type = field type, func return type, or element type for array/slice

	// The following are for function resolvers only
	Params     []string // name(s) of args to resolver function obtained from metadata
	Enums      []string // corresp. enum name if the parameter is of an enum type
	Defaults   []string // corresp. default value(s) (as strings) where an empty string means there is no default
	DescArgs   []string // corresp. description of the argument
	HasContext bool     // 1st function parameter is a context.Context (not a query argument)
	HasError   bool     // has 2 return values the 2nd of which is a Go error

	Embedded bool // embedded struct (which we use as a template for a GraphQL "interface")
	Empty    bool // embedded struct has no fields (which we use for a GraphQL "union")
	Nullable bool // pointer fields or those with the "nullable" tag are allowed to be null

	// Subscript holds the result of the "subscript" option (for a slice/array/map)
	Subscript     string       // name resolver arg (default is "id")
	SubscriptType reflect.Type // arg type - int for slice/array, type of the key for maps
	// Description is text used as a GraphQL description for the field - taken from the tag string after any # character (outside brackets)
	Description string
}

// contextType is used to check if a resolver function takes a context.Context (1st) parameter
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

// errorType is used to check if a resolver function returns a (2nd) error return value
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Get checks if a field in a Go struct is exported and, if so, returns the GraphQL field info. incl. the
// GQL field name, derived from the Go field name (with 1st char lower-cased) or taken from the tag (metadata).
// It also returns other stuff like whether the result is nullable and GraphQL parameters (and default
// parameter values) if the resolver is a function.
// An error may be returned e.g. for malformed metadata, or a resolver function returning multiple values.
// If the field is not exported or the field name (1st tag value) is a dash (-) then nil is returned, but no error.
func Get(f *reflect.StructField) (fieldInfo *Info, err error) {
	if f.Name != "_" && f.PkgPath != "" {
		return // unexported field
	}

	if fieldInfo, err = GetTagInfo(f.Tag.Get("graphql")); err != nil {
		return nil, fmt.Errorf("%w getting tag info from field %q", err, f.Name)
	}
	if fieldInfo == nil {
		return // explicitly omitted field
	}

	// if no type name was provided in the tag generate a GraphQL name from the field name
	if fieldInfo.Name == "" {
		// make GraphQL name from Go field name (can't be empty string) with lower-case first letter
		first, n := utf8.DecodeRuneInString(f.Name)
		fieldInfo.Name = string(unicode.ToLower(first)) + f.Name[n:]
	}

	if f.Type.Kind() == reflect.Struct && f.Anonymous {
		// Embedded (anon) struct
		fieldInfo.Embedded = true
		// Determine if the struct is empty (no exported fields)
		fieldInfo.Empty = true
		for i := 0; i < f.Type.NumField(); i++ {
			first, _ := utf8.DecodeRuneInString(f.Type.Field(i).Name)
			if unicode.IsUpper(first) {
				fieldInfo.Empty = false
				break
			}
		}
		return
	}

	// Get base type if it's a pointer
	t := f.Type
	for t.Kind() == reflect.Ptr {
		fieldInfo.Nullable = true // Pointer types can be null
		t = t.Elem()              // follow indirection
	}

	// For a func we need to check for the correct number of args and use the func return type as the resolver type
	if t.Kind() == reflect.Func {
		firstIndex := 0
		// Check for first parameter of context.Context
		if t.NumIn() > firstIndex && t.In(firstIndex).Kind() == reflect.Interface && t.In(0).Implements(contextType) {
			// 1st param is a context so don't add it to the list of query arguments
			fieldInfo.HasContext = true
			firstIndex++
		}
		if t.NumIn()-firstIndex != len(fieldInfo.Params) {
			if len(fieldInfo.Params) == 0 {
				return nil, fmt.Errorf("no args found in graphql tag for %q but %d required", f.Name, t.NumIn()-firstIndex)
			}
			return nil, fmt.Errorf("function %q argument count should be %d but is %d",
				f.Name, len(fieldInfo.Params), t.NumIn()-firstIndex)
		}

		// Validate the resolver function return type(s)
		switch t.NumOut() {
		case 0:
			return nil, errors.New("resolver " + f.Name + " must return a value (or 2)")
		case 1:
			// nothing here
		case 2:
			t2 := t.Out(1)
			if t2.Kind() != reflect.Interface || !t2.Implements(errorType) {
				return nil, errors.New("resolver " + f.Name + " 2nd return must be error type")
			}
			fieldInfo.HasError = true
		default:
			return nil, errors.New("resolver " + f.Name + " returns too many values")
		}
		t = t.Out(0) // now use return type of func as resolver type
	} else {
		if fieldInfo.Params != nil {
			return nil, errors.New("arguments cannot be supplied for non-function resolver " + f.Name)
		}
	}

	fieldInfo.ResultType = t
	if fieldInfo.Subscript != "" {
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array && t.Kind() != reflect.Map {
			return nil, errors.New("cannot use subscript option since field " + f.Name + " is not a slice, array, or map")
		}
		// Note that "subscript" option can be used with a function but the function should have no parameters (except for
		// and optional context) since the GraphQL "arguments" are used to provide the subscripting value.
		// A subscript function can have a context (HasContext) and error return (HasError) but must return a slice/array/map.
		if len(fieldInfo.Params) > 0 {
			return nil, errors.New(`cannot use "args" option with "subscript" option with field ` + f.Name)
		}

		fieldInfo.ResultType = t.Elem()

		// Get the "subscript" type - int (slice/array) or scalar type for map key
		fieldInfo.SubscriptType = reflect.TypeOf(1)
		if t.Kind() == reflect.Map {
			fieldInfo.SubscriptType = t.Key()
			if (fieldInfo.SubscriptType.Kind() < reflect.Int || fieldInfo.SubscriptType.Kind() > reflect.Float64) &&
				fieldInfo.SubscriptType.Kind() != reflect.String {
				// TODO allow any comparable type for map subscripts?
				return nil, errors.New("map key for subscript option " + f.Name + " must be a scalar")
			}
		}
	} else if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		// a GraphQL list has the type of it's elements
		fieldInfo.ResultType = t.Elem()
	}

	return
}

// GetTagInfo extracts GraphQL field name and type info from the field's tag (if any)
// If the tag just contains a dash (-) then nil is returned (no error).  If the tag string is empty
// (e.g. if no tag was supplied) then the returned Info is not nil but the Name field is empty.
func GetTagInfo(tag string) (*Info, error) {
	if tag == "-" {
		return nil, nil // this field is to be ignored
	}
	parts, desc, err := SplitWithDesc(tag)
	if err != nil {
		return nil, fmt.Errorf("%w splitting tag %q", err, tag)
	}
	fieldInfo := &Info{Description: desc}
	for i, part := range parts {
		if i == 0 { // first string is the name
			// Check for enum by splitting on a colon (:)
			if subParts := strings.Split(part, ":"); len(subParts) > 1 {
				fieldInfo.Name = subParts[0]
				fieldInfo.GQLTypeName = subParts[1]
			} else {
				fieldInfo.Name = part
			}
			continue
		}
		if part == "" {
			continue // ignore empty sections
		}
		if strings.HasPrefix(part, "subscript") {
			subParts := strings.Split(part, "=")
			fieldInfo.Subscript = "id" // default argument name used to access slice or map elements
			if len(subParts) > 1 && subParts[1] != "" {
				fieldInfo.Subscript = subParts[1]
			}
			continue
		}
		if part == "nullable" {
			fieldInfo.Nullable = true
			continue
		}
		if list, err := getBracketedList(part, "args"); err != nil {
			return nil, fmt.Errorf("%w getting args in %q", err, tag)
		} else if list != nil {
			fieldInfo.Params = make([]string, len(list))
			fieldInfo.Enums = make([]string, len(list))
			fieldInfo.Defaults = make([]string, len(list))
			fieldInfo.DescArgs = make([]string, len(list))
			for paramIndex, s := range list {
				// Strip description after hash (#)
				subParts := strings.SplitN(s, "#", 2)
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.DescArgs[paramIndex] = subParts[1]
				}
				// Strip of default value (if any) after equals sign (=)
				subParts = strings.Split(s, "=")
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.Defaults[paramIndex] = strings.Trim(subParts[1], " ")
				}
				// Strip of enum name after colon (:)
				subParts = strings.Split(s, ":")
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.Enums[paramIndex] = strings.Trim(subParts[1], " ")
				}

				fieldInfo.Params[paramIndex] = strings.Trim(s, " ")
			}
			continue
		}
		return nil, fmt.Errorf("unknown option %q in GraphQL tag %q", part, tag)
	}
	return fieldInfo, nil
}
