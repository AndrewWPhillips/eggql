package handler

// result.go is used to generate the query output string

import (
	"context"
	"fmt"
	"github.com/andrewwphillips/eggql/internal/field"
	"github.com/dolmen-go/jsonmap"
	"github.com/vektah/gqlparser/ast"
	"reflect"
)

type (
	// gqlOperation performs an operation (query) of a GraphQL request
	gqlOperation struct {
		isMutation bool
		variables  map[string]interface{}
		enums      map[string][]string
	}
)

// GetSelections resolves the selections in a query by finding and evaluating the corresponding resolver(s)
// Returns a jsonmap.Ordered (a map of values and a slice that remembers the order they were added) that contains an
//     entry for each selection, where the map "key" is the name of the entry/resolver and the value is:
//     a) scalar value (stored in an interface})
//     b) a nested jsonmap.Ordered if the resolver is a nested struct
//     c) a slice (ie []interface{}) if the resolver is a slice or array.
// Parameters:
//   ctx = a Go context that could expire at any time
//   set = list of selections from a GraphQL query to be resolved
//   v = value of Go struct whose (exported) fields are the resolvers
func (op *gqlOperation) GetSelections(ctx context.Context, set ast.SelectionSet, v reflect.Value) (jsonmap.Ordered, error) {
	// Get the struct that contains the resolvers that we can use
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem() // follow indirection
	}
	if v.Type().Kind() != reflect.Struct { // struct = 25
		// bug since we should have checked this when building the scehma
		panic("We can only search for a query field within a struct") // TODO return error?
	}

	result := jsonmap.Ordered{
		Data:  make(map[string]interface{}),
		Order: make([]string, 0, len(set)),
	}

	// resolve each (sub)query
	for _, s := range set {
		// TODO: check if ctx has expired here
		switch astType := s.(type) {
		case *ast.Field:
			// Find and execute the "resolver" in the struct (or recursively in embedded structs)
			value, err := op.FindSelection(ctx, t, v, astType)
			if err != nil {
				return result, err
			}
			if value != nil {
				key := astType.Alias // name used as map key
				if _, ok := result.Data[key]; ok {
					return result, fmt.Errorf("resolver %q in %s has duplicate name", key, astType.Name)
				}
				result.Data[key] = value
				result.Order = append(result.Order, key)
			}
			// else TODO return error if not found (note: nil is currently returned when a field is excluded by directive)
		case *ast.FragmentSpread:
			fragments, err := op.GetSelections(ctx, astType.Definition.SelectionSet, q)
			if err != nil {
				return result, err
			}
			// Add the entries found in the fragment (in the order they were found)
			for _, key := range fragments.Order {
				// check if a selection with this name is already present
				if _, ok := result.Data[key]; ok {
					return result, fmt.Errorf("resolver %q in fragment %s has duplicate name", key, astType.Name)
				}
				result.Data[key] = fragments.Data[key]
			}
			result.Order = append(result.Order, fragments.Order...)
		}
	}
	return result, nil
}

// FindSelection scans a struct for a match (exported field with name matching the ast.Field)
// It probably should never return an error unless there is a bug since schema validation should avoid any problems.
// It may return nil (even when error is nil) if
//  a) no matching field was found (which may occur for embedded structs since the field may be matched in the main struct)
//  b) the field was excluded based on a directive
func (op *gqlOperation) FindSelection(ctx context.Context, astField *ast.Field, v reflect.Value) (interface{}, error) {
	var i int
	// Check all the (exported) fields of the struct for a match to astField.Name
	for i = 0; i < v.Type().NumField(); i++ {
		// TODO: check if ctx has expired here
		tField := v.Type().Field(i)
		vField := v.Field(i)
		fieldInfo, err := field.Get(&tField)
		if err != nil {
			panic(err) // TODO: return an error (no panics)
		}
		if fieldInfo == nil {
			continue // unexported field
		}
		// Recursively check fields of embedded struct
		if fieldInfo.Embedded {
			if value, err := op.FindSelection(ctx, astField, vField); err != nil {
				return nil, err
			} else if value != nil {
				return value, nil // found it
			}
		}
		if fieldInfo.Name == astField.Name {
			// resolver found so use it
			if value, err := op.resolve(ctx, astField, vField, fieldInfo); err != nil {
				return nil, err
			} else {
				return value, nil
			}
		}
	}
	return nil, nil // indicate that astField.Name was not found
}

// resolve calls a resolver given a query to obtain the results of the query (incl. listed and nested queries)
// Resolvers are often dynamic (where the resolver is a Go function) in which case the function is called to get the value.
// Returns a value (or an error) where the value (returned in an interface{} type) is:
//   * a scalar - integer, float, boolean, string
//   * a nested query - returned as a jsonmap.Ordered
//   * a list - returned as a []interface{}).
//   * nil - if no value is to be provided (eg due to "skip" directive on the field)
// Parameters:
//   ctx = a Go context that could expire at any time
//   astField = a query or sub-query - a field of a GraphQL object
//   v = value of the resolver (field of Go struct)
//   fieldInfo = metadata for the resolver (eg parameter name) obtained from the struct field tag
func (op *gqlOperation) resolve(ctx context.Context, astField *ast.Field, v reflect.Value, fieldInfo *field.Info) (interface{}, error) {
	if op.directiveBypass(astField) {
		// TODO return a special value so that scan of fields stops
		return nil, nil
	}
	if v.Type().Kind() == reflect.Func {
		var err error
		// For function fields, we have to call it to get the resolver value to use
		if v, err = op.fromFunc(ctx, astField, v, fieldInfo); err != nil {
			return nil, err
		}
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem() // follow indirection
		v = v.Elem()
	}

	switch v.Type().Kind() {
	case reflect.Struct:
		return op.GetSelections(ctx, astField.SelectionSet, v.Interface()) // returns map of interface{} as an interface{}

	case reflect.Slice, reflect.Array:
		var results []interface{}
		for i := 0; i < v.Len(); i++ {
			// TODO: check if ctx has expired here
			if value, err2 := op.resolve(ctx, astField, v.Index(i), fieldInfo); err2 != nil {
				return nil, err2
			} else if value != nil {
				results = append(results, value)
			}
		}
		return results, nil // returns slice of interface{} as an interface{}
	}
	// If enum or enum list get the integer index and look up the enum value
	if enumName := fieldInfo.Enum; enumName != "" {
		if len(enumName) > 2 && enumName[0] == '[' && enumName[len(enumName)-1] == ']' {
			enumName = enumName[1 : len(enumName)-1]
		}
		idx := -1
		switch value := v.Interface().(type) {
		case int:
			idx = value
		case int8:
			idx = int(value)
		case int16:
			idx = int(value)
		case int32:
			idx = int(value)
		case int64:
			idx = int(value)
		case uint8:
			idx = int(value)
		case uint16:
			idx = int(value)
		case uint32:
			idx = int(value)
		case uint64:
			idx = int(value)
		default:
			return nil, fmt.Errorf("invalid return type %d for enum (should be an integer type)", v.Kind())
		}
		return op.enums[enumName][idx], nil
	}

	return v.Interface(), nil
}

// directiveBypass handles field directives - just standard "skip" and "include" for now
// Returns: true if a directive indicates the field is not to be processed
func (op *gqlOperation) directiveBypass(astField *ast.Field) bool {
	for _, d := range astField.Directives {
		if d.Name != "skip" && d.Name != "include" {
			continue // panic("Unexpected directive")
		}
		reverse := d.Name == "skip"
		for _, arg := range d.Arguments {
			if arg.Name == "if" {
				if rawValue, err := arg.Value.Value(op.variables); err != nil {
					panic(err)
				} else if b, ok := rawValue.(bool); ok {
					return b == reverse
				}
			}
		}
	}
	return false
}
