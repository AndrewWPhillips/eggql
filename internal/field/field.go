// Package field is for analysing Go struct fields for use as GraphQL query fields (resolvers)
package field

// field.go generates GraphQL resolver info from a Go struct field using reflection

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"
)

// TagKey is tag string "key" for our app - used to find optiond in the field metadata (tag string)
const TagKey = "egg"

type (
	// ID indicates that a field is to be used as a GraphQL ID type
	ID string

	// Unmarshaler must be implemented by custom scalar types to decode a string into the type
	// It must be able to handle a string created with MarshalerEGGQL() (below) [or String() if there is no marshaler]
	Unmarshaler interface {
		UnmarshalEGGQL(string) error
	}
	// Marshaler may be implemented by custom scalar types to encode the type in a string
	// It should create a string compatible with Unmarshaller (above).
	// If not given then the fmt.Stringer is used (ie String() method is called), and
	//    if that's not implemented then fmt.Sprintf with %v is used
	Marshaler interface {
		MarshalEGGQL() (string, error)
	}
)

// UnmarshalerType is the dynamic type of the Unmarshaler interface
// It's used to check if a type has an UnmarshalEGGQL method, which indicates it is a custom scalar type.
// The way it is obtained is a little tricky - you first get the type of a ptr to an Unmarshaler (which
//   here is nil but that does not matter as we are only concerned with types not values) then get
//   the type of what it points to (using reflect.Type.Elem()).
var UnmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()

// Info is returned from Get() with info extracted from a struct field to be used as a GraphQL query resolver.
// The info is obtained from the field's name, type and field's tag string (using TagKey).
// Note that the GraphQL type is usually deduced but sometimes needs to be supplied (saved in GQLTypeName
// for the resolver return type and ArgTypes is for resolver arguments) - but this is currently only necessary
// for GraphQL ID type for enum names (can't be deduced since Go does not have an enum type).
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

	Directives []string // directives to apply to the field (eg "@deprecated")

	// Subscript holds the result of the "subscript" option (for a slice/array/map)
	Subscript string // name of resolver arg (default is "id")
	// FieldID holds the result of the "field_id" option (for a slice/array/map)
	FieldID string // name of id field (default is "id")
	// BaseIndex is the offset (from zero) for numeric IDs (slice/array only)
	// Eg if BaseIndex is 10 then ID 10 refers to element 0, ID 11 => element 1, etc
	// This is only used in conjunction with Subscript or FieldID options (on slices/arrays, but not maps)
	BaseIndex int
	// ElementType is the type of elements if the field is a map/slice/array - only used if FieldID or Subscript are not empty
	ElementType reflect.Type //  int for slice/array, type of the key for maps
	// Description is text used as a GraphQL description for the field - taken from the tag string after any # character (outside brackets)
	Description string // All text in the tag after the first hash (#) [unless the # is in brackets or in a string]
}

// contextType is used to check if a resolver function takes a context.Context (1st) parameter
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

// errorType is used to check if a resolver function returns a (2nd) error return value
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Get checks if a field (of a Go struct) is exported and, if so, returns the GraphQL field info. incl. the
//   field name, derived from the Go field name (with 1st char lower-cased) or taken from the tag (metadata).
//   It also returns other stuff like whether the result is nullable and GraphQL parameters (and default
//   parameter values) if the resolver is a function.
// Returns
// - ptr to field.Info, or nil if the field is not used (ie: not exported or metadata is just a dash (-))
//   A special case is a field name of underscore (_) which return field.Info but only with the Description field set
// - error for different reasons such as:
//   - malformed metadata such as an unknown option (not one of args, nullable, subscript, field_id, base)
//   - type of the field is invalid (eg resolver function with no return value)
//   - inconsistency between the type and metadata (eg function parameters do not match the "args" option)
func Get(f *reflect.StructField) (fieldInfo *Info, err error) {
	if f.Name != "_" && f.PkgPath != "" {
		return // ignore unexported field unless it's underscore (_)
	}

	// tag is the metadata associated with our "key".  Note that "tag" often refers to the complete metadata string
	// attached to a struct field, but in this case "tag" is just the string for our "key".
	// Note that even if tag is empty field info is still generated (using reflection) eg: from the field name and type.
	tag := f.Tag.Get(TagKey)
	if tag == "" {
		// Note the tag key was changed from "graphql" to "egg" to avoid any possibility of conflict with thunder package
		tag = f.Tag.Get("graphql") // TODO: remove later, leave in for backward compatibility for now
	}

	if fieldInfo, err = GetInfoFromTag(tag); err != nil {
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

	// Now we use the field type for info, validation and (directly or indirectly) the resolver return type
	t := f.Type

	// check for embedded struct (used to signal a GraphQL interface) and *empty* embedded struct (union)
	if t.Kind() == reflect.Struct && f.Anonymous {
		// Embedded (anon) struct
		fieldInfo.Embedded = true
		// Determine if the struct is empty (no exported fields)
		fieldInfo.Empty = true
		for i := 0; i < t.NumField(); i++ {
			first, _ := utf8.DecodeRuneInString(t.Field(i).Name)
			if unicode.IsUpper(first) { // still considered to be empty if field is not exported
				fieldInfo.Empty = false
				break
			}
		}
		return
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

	// Validation of "subscript", "field_id", "base" etc
	if fieldInfo.FieldID != "" && fieldInfo.Subscript != "" {
		return nil, errors.New(`cannot use "field_id" and "subscript" options together in field ` + f.Name)
	}

	if fieldInfo.Subscript != "" {
		if t.Kind() != reflect.Map && t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return nil, errors.New("cannot use subscript option since field " + f.Name + " is not a slice, array, or map")
		}
		// Note that "subscript" option can be used with a function but the function should have no parameters (except for
		// and optional context) since the GraphQL "arguments" are used to provide the subscripting value.
		// A subscript function can have a context (HasContext) and error return (HasError) but must return a slice/array/map.
		if len(fieldInfo.Args) > 0 {
			return nil, errors.New(`cannot use "subscript" option if args specified for field ` + f.Name)
		}
	}

	if fieldInfo.FieldID != "" {
		if t.Kind() != reflect.Map && t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return nil, errors.New("cannot use field_id option since field " + f.Name + " is not a slice, array, or map")
		}
	}
	if fieldInfo.BaseIndex > 0 {
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return nil, errors.New(`cannot use "base" option since field ` + f.Name + " is not a slice or array")
		}
		if fieldInfo.FieldID == "" && fieldInfo.Subscript == "" {
			return nil, errors.New(`cannot use "base" without "subscript" or "field_id" in field ` + f.Name)
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
