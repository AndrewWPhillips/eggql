package handler

// result.go is used to generate the query output string

import (
	"context"
	"fmt"
	"github.com/andrewwphillips/eggql/internal/field"
	"github.com/dolmen-go/jsonmap"
	"github.com/vektah/gqlparser/ast"
	"reflect"
	"strings"
)

const (
	AllowIntrospection = true
)

type (
	// gqlOperation controls an operation (query/mutation) of a GraphQL request
	gqlOperation struct {
		isMutation bool
		variables  map[string]interface{}
		enums      map[string][]string
	}

	// gqlValue contains the result of a query or queries, or an error, plus the name
	gqlValue struct {
		name  string      // name/alias of the entry/resolver
		value interface{} // scalar, nested result (jsonmap.Ordered), list ([]interface{})
		err   error       // non-nil if something went wrong whence the contents of value should be ignored
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
//   vIntro = Go struct with values for introspection (only supplied at root level)
//   introOp = gqlOperation struct to be used with vIntro (contains some required enums)
func (op *gqlOperation) GetSelections(ctx context.Context, set ast.SelectionSet, v, vIntro reflect.Value, introOp *gqlOperation) (jsonmap.Ordered, error) {
	// Get the struct that contains the resolvers that we can use
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem() // follow indirection
	}
	for vIntro.IsValid() && vIntro.Type().Kind() == reflect.Ptr {
		vIntro = vIntro.Elem() // follow indirection
	}

	resultChans := make([]<-chan gqlValue, 0, len(set)) // TODO: allow extra cap. for fragment sets > 1 in length

	var result jsonmap.Ordered
	var err error
	// resolve each (sub)query
	for _, s := range set {
		switch astType := s.(type) {
		case *ast.Field:
			// Find and execute the "resolver" in the struct (or recursively in embedded structs)
			if vIntro.IsValid() && strings.HasPrefix(astType.Name, "__") {
				resultChans = append(resultChans, introOp.FindSelection(ctx, astType, vIntro))
			} else {
				resultChans = append(resultChans, op.FindSelection(ctx, astType, v))
			}
			continue

		case *ast.InlineFragment:
			result, err = op.GetSelections(ctx, astType.SelectionSet, v, reflect.Value{}, nil)
		case *ast.FragmentSpread:
			result, err = op.GetSelections(ctx, astType.Definition.SelectionSet, v, reflect.Value{}, nil)
		}
		// This code (until end of loop) is shared code for fragments only
		var ch chan gqlValue
		if err != nil {
			ch = make(chan gqlValue, 1)
			ch <- gqlValue{err: err}
		} else {
			if len(result.Order) != len(result.Data) {
				panic("slice and map must have same number of elts")
			}
			ch = make(chan gqlValue, len(result.Order))
			for _, v := range result.Order {
				ch <- gqlValue{name: v, value: result.Data[v]}
			}
		}
		close(ch)
		resultChans = append(resultChans, ch)
	}

	// Now extract the values (will block until all channels have closed)
	r := jsonmap.Ordered{
		Data:  make(map[string]interface{}),
		Order: make([]string, 0, len(set)),
	}
	for _, ch := range resultChans {
	inner:
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					break inner
				}
				if v.err != nil {
					return r, v.err
				}
				r.Data[v.name] = v.value
				r.Order = append(r.Order, v.name)
			case <-ctx.Done():
				return r, ctx.Err()
			}
		}
	}
	return r, nil
}

// FindSelection scans v for a field that matches astField and resolves the value (if found)
// It returns a chan that will send (perhaps later) a single value (or error) and is then closed.
// If no match is found, or the matched field is excluded (by a directive) the chan is closed without any value sent.
func (op *gqlOperation) FindSelection(ctx context.Context, astField *ast.Field, v reflect.Value) <-chan gqlValue {
	if v.Type().Kind() != reflect.Struct { // struct = 25
		// param. 'v' validation - note that this is a bug which is precluded during building of the schema
		panic("FindSelection: search of query field in non-struct")
	}

	//r := make(chan gqlValue) //
	if astField.Name == "__typename" {
		// Special "introspection" field
		r := make(chan gqlValue, 1)
		r <- gqlValue{name: astField.Alias, value: astField.ObjectDefinition.Name}
		close(r)
		return r
	}

	var i int
	// Check all the (exported) fields of the struct for a match to astField.Name
	for i = 0; i < v.Type().NumField(); i++ {
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
			// if a field in the embedded struct matches a value is sent on the chan returned from FindSelection
			for v := range op.FindSelection(ctx, astField, vField) {
				// just send the 1st value from chan
				r := make(chan gqlValue, 1)
				// TODO: check if we should run this in a separate Go routine
				select {
				case r <- v:
					// nothing else needed here
				case <-ctx.Done():
					r <- gqlValue{err: ctx.Err()}
				}
				close(r)
				return r
			}
			// chan closed (no match found) so continue below
		}
		if fieldInfo.Name == astField.Name {
			// resolver found so run it (concurrently)
			if op.isMutation {
				// Mutations are run sequentially
				r := make(chan gqlValue, 1)
				if value := op.resolve(ctx, astField, vField, fieldInfo); value != nil {
					r <- *value
				}
				close(r)
				return r
			} else {
				// This go routine allows resolvers to run in parallel
				r := make(chan gqlValue)
				go func() {
					defer func() {
						// Convert any panics in resolvers into an (internal) error
						if recoverValue := recover(); recoverValue != nil {
							r <- gqlValue{err: fmt.Errorf("Internal error: panic %v", recoverValue)}
						}
						close(r)
					}()
					if value := op.resolve(ctx, astField, vField, fieldInfo); value != nil {
						r <- *value
					}
				}()
				return r
			}
		}
	}
	// No matching field so close chan without writing
	r := make(chan gqlValue)
	close(r)
	return r
}

// resolve calls a resolver given a query to obtain the results of the query (incl. listed and nested queries)
// Resolvers are often dynamic (where the resolver is a Go function) in which case the function is called to get the value.
// Returns a pointer to a value (or error) or nil if nothing results (eg if excluded by directive)
// Parameters:
//   ctx = a Go context that could expire at any time
//   astField = a query or sub-query - a field of a GraphQL object
//   v = value of the resolver (field of Go struct)
//   fieldInfo = metadata for the resolver (eg parameter name) obtained from the struct field tag
func (op *gqlOperation) resolve(ctx context.Context, astField *ast.Field, v reflect.Value, fieldInfo *field.Info) *gqlValue {
	if op.directiveBypass(astField) {
		return nil
	}

	if v.Type().Kind() == reflect.Func {
		var err error
		// For function fields, we have to call it to get the resolver value to use
		if v, err = op.fromFunc(ctx, astField, v, fieldInfo); err != nil {
			return &gqlValue{err: err}
		}
	}
	for v.Type().Kind() == reflect.Ptr || v.Type().Kind() == reflect.Interface {
		if v.IsNil() {
			return &gqlValue{name: astField.Alias, value: v.Interface()}
		}
		v = v.Elem() // follow indirection
	}

	if fieldInfo.Subscript != "" {
		if len(astField.Arguments) != 1 || astField.Arguments[0].Name != fieldInfo.Subscript {
			return &gqlValue{err: fmt.Errorf("subscript resolver %q must supply an argument called %q", fieldInfo.Name, fieldInfo.Subscript)}
		}
		arg, err := op.getValue(fieldInfo.SubscriptType, fieldInfo.Subscript, "qqq", astField.Arguments[0].Value.Raw)
		if err != nil {
			return &gqlValue{err: err}
		}
		switch v.Type().Kind() {
		case reflect.Map:
			v = v.MapIndex(arg)
			if !v.IsValid() {
				return &gqlValue{err: fmt.Errorf("index '%s' (value %v) is not valid for field %s", fieldInfo.Subscript, arg.Interface(), fieldInfo.Name)}
			}

		case reflect.Slice, reflect.Array:
			idx, ok := arg.Interface().(int)
			if !ok {
				//return &gqlValue{err: fmt.Errorf("subscript %q for resolver %q must be an integer to index a list", fieldInfo.Subscript, fieldInfo.Name)}
				panic(fmt.Sprintf("subscript %q for resolver %q must be an integer to index a list", fieldInfo.Subscript, fieldInfo.Name))
			}
			if idx < 0 || idx >= v.Len() {
				return &gqlValue{err: fmt.Errorf("index '%s' (value %d) is out of range for field %s", fieldInfo.Subscript, idx, fieldInfo.Name)}
			}
			v = v.Index(idx)
		}
	}

	switch v.Type().Kind() {
	case reflect.Struct:
		// Look up all sub-queries in this object
		if result, err := op.GetSelections(ctx, astField.SelectionSet, v, reflect.Value{}, nil); err != nil {
			return &gqlValue{err: err}
		} else {
			return &gqlValue{name: astField.Alias, value: result}
		}

	case reflect.Slice, reflect.Array:
		// resolve for all values in the list
		var results []interface{}
		if v.Type().Kind() == reflect.Array || !v.IsNil() {
			results = make([]interface{}, 0) // to distinguish empty slice from nil slice
			for i := 0; i < v.Len(); i++ {
				if value := op.resolve(ctx, astField, v.Index(i), fieldInfo); value != nil {
					results = append(results, value.value)
				}
			}
		}
		return &gqlValue{name: astField.Alias, value: results}
	}
	// If enum or enum list get the integer index and look up the enum value
	if enumName := fieldInfo.GQLTypeName; enumName != "" {
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
			return &gqlValue{err: fmt.Errorf("invalid return type %d for enum (should be an integer type)", v.Kind())}
		}
		return &gqlValue{name: astField.Alias, value: op.enums[enumName][idx]}
	}
	return &gqlValue{name: astField.Alias, value: v.Interface()}
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
