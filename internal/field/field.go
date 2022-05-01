// Package field is for analysing Go struct fields for use as GraphQL query fields (resolvers)
package field

// field.go generates GraphQL resolver info from a Go struct field using reflection

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// TagKey is tag string "key" for our app - used to find optiond in the field metadata (tag string)
	TagKey = "egg"

	AllowSubscript  = true // "subscript" option generates a resolver to subscript into a list (array/slice/map)
	AllowFieldID    = true // "field_id" option generates an extra "id" field for queries on a list (array/slice/map)
	AllowComplexity = true // "complexity" option specifies how to estimate the complexity of a resalver
)

type (
	// Unmarshaler must be implemented by custom scalar types to decode a string into the type
	Unmarshaler interface {
		UnmarshalEGGQL(string) error
	}
	// Marshaler may be implemented by custom scalar types to encode the type in a string
	// It should create a string compatible with Unmarshaller (above).
	// If not given then the Stringer interface is used - if that's not given then fmt.Sprintf with %v is used
	Marshaler interface {
		MarshalEGGQL() (string, error)
	}
)

// Info is returned Get() with info extracted from a struct field to be used as a GraphQL query resolver.
// The info is obtained from the field's name, type and field's tag string (using eggqlTagKey).
// Note that since Go has no native enums the GraphQL enum names are handled in metadata for
// both resolver return value and arguments (see metadata examples).
type Info struct {
	Name        string       // field name for use in GraphQL queries - based on metadata (tag) or Go struct field name
	GQLTypeName string       // GraphQL type name - usually empty but required if can't be deduced (eg enums)
	ResultType  reflect.Type // Type (Go) used to generate the resolver (GraphQL) type = field type, func return type, or element type for array/slice

	// The following are for function resolvers only
	Args            []string // name(s) of args to resolver function obtained from metadata
	ArgTypes        []string // corresp. type names - usually deduced from function parameter type but needed for ID and enums
	ArgDefaults     []string // corresp. default value(s) (as strings) where an empty string means there is no default
	ArgDescriptions []string // corresp. description of the argument
	HasContext      bool     // 1st function parameter is a context.Context (not a query argument)
	HasError        bool     // has 2 return values the 2nd of which is a Go error

	Embedded bool // embedded struct (which we use as a template for a GraphQL "interface")
	Empty    bool // embedded struct has no fields (which we use for a GraphQL "union")
	Nullable bool // pointers (plus slice/map if "nullable" option was specified)

	// FieldID holds the result of the "field_id" option (for a slice/array/map)
	FieldID string // name of id field (default is "id")
	// Subscript holds the result of the "subscript" option (for a slice/array/map)
	Subscript string // name of resolver arg (default is "id")
	// ElementType is the type of elements if the field is a map/slice/array - only used if FieldID or Subscript are not empty
	ElementType reflect.Type //  int for slice/array, type of the key for maps
	// Description is text used as a GraphQL description for the field - taken from the tag string after any # character (outside brackets)
	Description string // All text in the tag after the first hash (#) [unless the # is in brackets or in a string]
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
// If the field is not exported or the tag is a dash (-) then nil is returned (no error), unless the field
// *name* is an underscore (_) which returns an Info with the Description field filled in.
func Get(f *reflect.StructField) (fieldInfo *Info, err error) {
	if f.Name != "_" && f.PkgPath != "" {
		return // ignore unexported field unless it's underscore (_)
	}

	tag := f.Tag.Get(TagKey)
	if tag == "" {
		// Note the tag key was changed from "graphql" to "egg" to avoid any possibility of conflict with thunder package
		tag = f.Tag.Get("graphql") // TODO: remove later, leave in for backward compatibility for now
	}

	// Note that an empty/non-existent tag string does not mean the field is ignored by GetTagInfo() - field info is
	// still generated (using reflection) eg: from the field name and type.
	// However, a tag string with a single dash (-) means the field *is* ignored and GetTagInfo returns nil, nil.
	if fieldInfo, err = GetTagInfo(tag); err != nil {
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

	// Work with the field type (becomes the func return type if func field)
	t := f.Type

	// For a func we need to check for the correct number of args and use the func return type as the resolver type
	if t.Kind() == reflect.Func {
		firstIndex := 0
		// Check for first parameter of context.Context
		if t.NumIn() > firstIndex && t.In(firstIndex).Kind() == reflect.Interface && t.In(0).Implements(contextType) {
			// 1st param is a context so don't add it to the list of query arguments
			fieldInfo.HasContext = true
			firstIndex++
		}
		if t.NumIn()-firstIndex != len(fieldInfo.Args) {
			if len(fieldInfo.Args) == 0 {
				return nil, fmt.Errorf("no args found in %q metadata key for %q but %d required", TagKey, f.Name, t.NumIn()-firstIndex)
			}
			return nil, fmt.Errorf("function %q argument count should be %d but is %d",
				f.Name, len(fieldInfo.Args), t.NumIn()-firstIndex)
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
		if fieldInfo.Args != nil {
			return nil, errors.New("arguments cannot be supplied for non-function resolver " + f.Name)
		}
	}

	// Check that "nullable" flag was only used on slice/map
	if fieldInfo.Nullable && t.Kind() != reflect.Slice && t.Kind() != reflect.Map {
		return nil, errors.New("cannot use nullable option since field " + f.Name + " is not a slice, or map (try using a pointer)")
	}

	// Get "base type" if it's a pointer and remember that it's nullable
	for t.Kind() == reflect.Ptr {
		fieldInfo.Nullable = true // Pointer types can be null
		t = t.Elem()              // follow indirection
	}

	if fieldInfo.FieldID != "" && fieldInfo.Subscript != "" {
		return nil, errors.New(`cannot use "field_id"" and "subscript"" options together in field ` + f.Name)
	}
	if fieldInfo.FieldID != "" {
		if t.Kind() != reflect.Map && t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return nil, errors.New("cannot use field_id option since field " + f.Name + " is not a slice, array, or map")
		}
	}

	//fieldInfo.ResultType = t
	if fieldInfo.Subscript != "" {
		if t.Kind() != reflect.Map && t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return nil, errors.New("cannot use subscript option since field " + f.Name + " is not a slice, array, or map")
		}
		// Note that "subscript" option can be used with a function but the function should have no parameters (except for
		// and optional context) since the GraphQL "arguments" are used to provide the subscripting value.
		// A subscript function can have a context (HasContext) and error return (HasError) but must return a slice/array/map.
		if len(fieldInfo.Args) > 0 {
			return nil, errors.New(`cannot use "args" and "subscript" options together in field ` + f.Name)
		}
	}

	if fieldInfo.FieldID != "" || fieldInfo.Subscript != "" {
		// Get the "subscript" type - int (for slice/array) or scalar type for map key
		fieldInfo.ElementType = reflect.TypeOf(1)
		if t.Kind() == reflect.Map {
			fieldInfo.ElementType = t.Key()
			if (fieldInfo.ElementType.Kind() < reflect.Int || fieldInfo.ElementType.Kind() > reflect.Float64) &&
				fieldInfo.ElementType.Kind() != reflect.String {
				// for now we only allow string or int types for map subscripts
				return nil, errors.New("map key for subscript option " + f.Name + " must be an integer or string")
			}
		}
	}

	fieldInfo.ResultType = t
	if t.Kind() == reflect.Map || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
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
		if fieldID := getFieldID(part); fieldID != "" {
			fieldInfo.FieldID = fieldID
			continue
		}
		if subscript := getSubscript(part); subscript != "" {
			fieldInfo.Subscript = subscript
			continue
		}
		if part == "nullable" {
			fieldInfo.Nullable = true
			continue
		}
		if list, err := getBracketedList(part, "args"); err != nil {
			return nil, fmt.Errorf("%w getting args in %q", err, tag)
		} else if list != nil {
			fieldInfo.Args = make([]string, len(list))
			fieldInfo.ArgTypes = make([]string, len(list))
			fieldInfo.ArgDefaults = make([]string, len(list))
			fieldInfo.ArgDescriptions = make([]string, len(list))
			for paramIndex, s := range list {
				// Strip description after hash (#)
				subParts := strings.SplitN(s, "#", 2)
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.ArgDescriptions[paramIndex] = subParts[1]
				}
				// Strip of default value (if any) after equals sign (=)
				subParts = strings.Split(s, "=")
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.ArgDefaults[paramIndex] = strings.Trim(subParts[1], " ")
				}
				// Strip of enum name after colon (:)
				subParts = strings.Split(s, ":")
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.ArgTypes[paramIndex] = strings.Trim(subParts[1], " ")
				}

				fieldInfo.Args[paramIndex] = strings.Trim(s, " ")
			}
			continue
		}
		return nil, fmt.Errorf("unknown option %q in %q key (%s)", part, TagKey, tag)
	}
	return fieldInfo, nil
}

func getSubscript(s string) string {
	if !AllowSubscript {
		return ""
	}
	if s == "subscript" {
		return "id" // default field name if none given
	}
	if strings.HasPrefix(s, "subscript=") {
		return strings.TrimPrefix(s, "subscript=")
	}
	return ""
}

func getFieldID(s string) string {
	if !AllowFieldID {
		return ""
	}
	if s == "field_id" {
		return "id"
	}
	if strings.HasPrefix(s, "field_id=") {
		return strings.TrimPrefix(s, "field_id=")
	}
	return ""
}
