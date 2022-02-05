// Package field is for analysing Go struct fields for use as GraphQL queries
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

// Info is used by Get to return info extracted from a struct field used as a GraphQL query resolver.  The info is based on the field's name, type and "graphql" (metadata) tag.
// Note that since Go has no native enums the GraphQL enum names are handled in metadata for
// both resolver return value and arguments:
//   return value - field name may be followed by "=<name>" where <name> is the enum name
//   argument - each argument name in a "params" section may be followed by "=<name>"
type Info struct {
	Name        string // field name for use in GraphQL queries - based on metadata (tag) or Go struct field name
	GQLTypeName string // name of enum - must be for field of int type (or function returning an int type)

	// The following are for function resolvers only
	Params     []string // name(s) of args to resolver function obtained from metadata
	Defaults   []string // corresp. default value(s) (as strings) where an empty string means there is no default
	Enums      []string // corresp. enum name if the parameter is of an enum type
	HasContext bool     // 1st function parameter is a context.Context (not a query argument)
	HasError   bool     // has 2 return values the 2nd of which is a Go error

	Embedded bool // embedded struct (which we use as a template for a GraphQL "interface")
	Nullable bool // pointer fields or those with the "nullable" tag are allowed to be null
}

var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Get checks if a field in a Go struct is exported and, if so, returns the GraphQL field info. incl. the
// GQL field name, derived from the Go field name (with 1st char lower-cased) or taken from the tag (metadata).
// It also returns other stuff like whether the result is nullable and GraphQL parameters (and default
// parameter values) if the resolver is a function.
// An error may be returned e.g. for malformed metadata, or a resolver function returning multiple values.
// If the field is not exported nil is returned, but no error.
func Get(f *reflect.StructField) (fieldInfo *Info, err error) {
	if f.PkgPath != "" {
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
		// make GraphQL name of Go field name with lower-case first letter
		first, n := utf8.DecodeRuneInString(f.Name)
		fieldInfo.Name = string(unicode.ToLower(first)) + f.Name[n:]
	}

	if f.Type.Kind() == reflect.Struct && f.Anonymous {
		// Embedded (anon) struct
		fieldInfo.Embedded = true
		return
	}

	// Get base type if it's a pointer
	t := f.Type
	for t.Kind() == reflect.Ptr {
		fieldInfo.Nullable = true // Pointer types can be null
		t = t.Elem()              // follow indirection
	}
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
				return nil, fmt.Errorf("no params found in graphql tag for %q but %d required", f.Name, t.NumIn()-firstIndex)
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

	if fieldInfo.GQLTypeName != "" {
		// For enums the resolver function must return a Go integral type or list thereof
		kind := t.Kind()
		if kind == reflect.Slice || kind == reflect.Array {
			kind = t.Elem().Kind()
		}
	}

	return
}

// GetTagInfo extracts GraphQL field name and type info from the field's tag (if any)
// If the tag just contains a dash (-) then nil is returned (no error).  If the tag string is empty
// (e.g. if no tag was supplied) then the returned Info is not nil but the Name field is empty.
func GetTagInfo(tag string) (*Info, error) {
	if tag == "-" {
		return nil, nil
	}
	parts, err := SplitNested(tag)
	if err != nil {
		return nil, fmt.Errorf("%w splitting tag %q", err, tag)
	}
	fieldInfo := &Info{}
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
		if part == "nullable" {
			fieldInfo.Nullable = true
			continue
		}
		if list, err := getBracketedList(part, "params"); err != nil {
			return nil, fmt.Errorf("%w getting params in %q", err, tag)
		} else if list != nil {
			fieldInfo.Params = make([]string, len(list))
			fieldInfo.Defaults = make([]string, len(list))
			fieldInfo.Enums = make([]string, len(list))
			for paramIndex, s := range list {
				// Strip of default value (if any) after equals sign (=)
				subParts := strings.Split(s, "=")
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
		// Removed: defaults are now part of params (after =)
		//if list, err := getBracketedList(part, "defaults"); err != nil {
		//	return nil, fmt.Errorf("%w getting defaults in %q", err, tag)
		//} else if list != nil {
		//	fieldInfo.Defaults = list
		//	continue
		//}
		return nil, fmt.Errorf("unknown option %q in GraphQL tag %q", part, tag)
	}
	return fieldInfo, nil
}
