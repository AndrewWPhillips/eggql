package handler

// result.go is used to generate the query output string

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/andrewwphillips/eggql/internal/field"
	"github.com/dolmen-go/jsonmap"
	"github.com/vektah/gqlparser/v2/ast"
)

const (
	AllowIntrospection       = true
	AllowConcurrentQueries   = true
	ALlowNilResolverFunction = true // return NULL for an unimplemented (nil) resolver function

	TypeNameQuery = "__typename" // Name of "introspection" query that can be performed at any level
)

type (
	// gqlOperation controls an operation (query/mutation) of a GraphQL request
	gqlOperation struct {
		isMutation   bool
		variables    map[string]interface{}
		enums        map[string][]string       // forward lookup enum value (string) from int (slice index)
		enumsReverse map[string]map[string]int // allows reverse lookup int from enum value (map key = string)
	}

	// gqlValue contains the result of a query or queries, or an error, plus the name
	gqlValue struct {
		name  string      // name/alias of the entry/resolver
		value interface{} // scalar, nested result (jsonmap.Ordered), list ([]interface{})
		err   error       // non-nil if something went wrong whence the contents of value should be ignored
	}

	// idField stores name and type of fabricated id field (if required) for maps/slices/arrays
	idField struct {
		name  string
		value reflect.Value
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
//   idField = name/type of fabricated "id" field (see "field_id" option for lists of objects)
func (op *gqlOperation) GetSelections(ctx context.Context, set ast.SelectionSet, v reflect.Value, vIntro reflect.Value,
	introOp *gqlOperation, id *idField,
) (jsonmap.Ordered, error) {
	// Get the struct that contains the resolvers that we can use
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem() // follow indirection
	}
	for vIntro.IsValid() && vIntro.Type().Kind() == reflect.Ptr {
		vIntro = vIntro.Elem() // follow indirection
	}

	resultChans := make([]<-chan gqlValue, 0, len(set)) // TODO: allow extra cap. for fragment sets > 1 in length

	// resolve each (sub)query
	for _, s := range set {
		switch astType := s.(type) {
		case *ast.Field:
			// Find and execute the "resolver" in the struct (or recursively in embedded structs)
			if id != nil && astType.Name == id.name {
				ch := make(chan gqlValue, 1)
				ch <- gqlValue{name: id.name, value: id.value.Interface()}
				close(ch)
				resultChans = append(resultChans, ch)
			} else if vIntro.IsValid() && strings.HasPrefix(astType.Name, "__") {
				resultChans = append(resultChans, introOp.FindSelection(ctx, astType, vIntro))
			} else {
				resultChans = append(resultChans, op.FindSelection(ctx, astType, v))
			}

		case *ast.InlineFragment:
			if v.Type().Name() != astType.TypeCondition {
				continue
			}
			resultChans = append(resultChans, op.FindFragments(ctx, astType.SelectionSet, v))

		case *ast.FragmentSpread:
			resultChans = append(resultChans, op.FindFragments(ctx, astType.Definition.SelectionSet, v))
		}
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
					return jsonmap.Ordered{}, v.err
				}
				r.Data[v.name] = v.value
				r.Order = append(r.Order, v.name)
				if len(r.Order) != len(r.Data) {
					panic("map and slice in the jsonmap.Ordered should be the same size (map element replaced?)")
				}
			case <-ctx.Done():
				return jsonmap.Ordered{}, ctx.Err()
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
		// param. 'v' validation - note that this is a bug that should have been caught during schema building
		panic("FindSelection: search of query field in non-struct")
	}

	//r := make(chan gqlValue) //
	if astField.Name == TypeNameQuery {
		// Special "introspection" field
		r := make(chan gqlValue, 1)
		//r <- gqlValue{name: astField.Alias, value: v.Type().Name()}
		r <- gqlValue{name: astField.Alias, value: astField.ObjectDefinition.Name}
		close(r)
		return r
	}

	var i int
	// Check all the (exported) fields of the struct for a match to astField.Name
	// TODO: use map lookup O(1) [instead of O(n) linear search] to help perf. eg. for a large # of root query fields
	for i = 0; i < v.Type().NumField(); i++ {
		tField := v.Type().Field(i)
		vField := v.Field(i)
		fieldInfo, err := field.Get(&tField)
		if err != nil {
			// This condition should never occur - should have been caught during schema building
			panic(err)
		}
		if tField.Name == "_" || fieldInfo == nil {
			continue // unexported field
		}
		if fieldInfo.Embedded && fieldInfo.Empty {
			continue // union (no point scanning unexported fields)
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
			// end of chan (no match found) so continue below
		}
		if fieldInfo.Name == astField.Name {
			// resolver found so run it (concurrently)
			if op.isMutation || !AllowConcurrentQueries { // Mutations are run sequentially
				ch := make(chan gqlValue, 1)
				op.wrapResolve(ctx, astField, vField, reflect.Value{}, fieldInfo, ch)
				return ch
			} else {
				ch := make(chan gqlValue)
				// Calling wrapResolve as a go routine allows resolvers to run in parallel
				go op.wrapResolve(ctx, astField, vField, reflect.Value{}, fieldInfo, ch)
				return ch
			}
		}
	}
	// No matching field so close chan without writing
	r := make(chan gqlValue)
	close(r)
	return r
}

// wrapResolve calls resolve putting the return value on a chan and converting any panic to an error
func (op *gqlOperation) wrapResolve(
	ctx context.Context, astField *ast.Field, v, vID reflect.Value, fieldInfo *field.Info, ch chan<- gqlValue,
) {
	defer func() {
		// Convert any panics in resolvers into an (internal) error
		if recoverValue := recover(); recoverValue != nil {
			ch <- gqlValue{err: fmt.Errorf("Internal error: panic %v", recoverValue)}
		}
		close(ch)
	}()
	if value := op.resolve(ctx, astField, v, vID, fieldInfo); value != nil {
		ch <- *value
	}
}

func (op *gqlOperation) FindFragments(ctx context.Context, set ast.SelectionSet, v reflect.Value) <-chan gqlValue {
	result, err := op.GetSelections(ctx, set, v, reflect.Value{}, nil, nil)

	var ch chan gqlValue
	if err != nil {
		ch = make(chan gqlValue, 1)
		ch <- gqlValue{err: err}
	} else {
		if len(result.Order) != len(result.Data) {
			panic("slice and map must have the same number of elts")
		}
		ch = make(chan gqlValue, len(result.Order))
		for _, v := range result.Order {
			ch <- gqlValue{name: v, value: result.Data[v]}
		}
	}
	close(ch)
	return ch
}

// resolve calls a resolver given a query to obtain the results of the query (incl. listed and nested queries)
// Resolvers are often dynamic (where the resolver is a Go function) in which case the function is called to get the value.
// Returns a pointer to a value (or error) or nil if nothing results (eg if excluded by directive)
// Parameters:
//   ctx = a Go context that could expire at any time
//   astField = a query or sub-query - a field of a GraphQL object
//   v = value of the resolver (field of Go struct)
//   vID = value of "id" (only supplied if an element of a list)
//   fieldInfo = metadata for the resolver (eg parameter name) obtained from the struct field tag
func (op *gqlOperation) resolve(ctx context.Context, astField *ast.Field, v, vID reflect.Value, fieldInfo *field.Info,
) *gqlValue {
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
	if !v.IsValid() {
		return &gqlValue{name: astField.Alias}
	}
	for v.Type().Kind() == reflect.Ptr || v.Type().Kind() == reflect.Interface {
		if v.IsNil() {
			return &gqlValue{name: astField.Alias, value: v.Interface()}
		}
		v = v.Elem() // follow indirection
	}

	// For "subscript" option if v is a map/slice convert it to an element using the "subscript" to index into the container
	if fieldInfo.Subscript != "" {
		if len(astField.Arguments) != 1 || astField.Arguments[0].Name != fieldInfo.Subscript {
			return &gqlValue{err: fmt.Errorf("subscript resolver %q must supply an argument called %q", fieldInfo.Name, fieldInfo.Subscript)}
		}
		arg, err := op.getValue(fieldInfo.ElementType, fieldInfo.Subscript, "", astField.Arguments[0].Value.Raw)
		if err != nil {
			return &gqlValue{err: err}
		}
		switch v.Type().Kind() {
		case reflect.Map:
			v = v.MapIndex(arg)
			if !v.IsValid() {
				return &gqlValue{err: fmt.Errorf("index '%s' (value %q) is not valid for field %s", fieldInfo.Subscript, arg.Interface(), fieldInfo.Name)}
			}

		case reflect.Slice, reflect.Array:
			idx, ok := arg.Interface().(int)
			if !ok {
				//return &gqlValue{err: fmt.Errorf("subscript %q for resolver %q must be an integer to index a list", fieldInfo.Subscript, fieldInfo.Name)}
				panic(fmt.Sprintf("subscript %q for resolver %q must be an integer to index a list", fieldInfo.Subscript, fieldInfo.Name))
			}
			idx -= fieldInfo.BaseIndex
			if idx < 0 || idx >= v.Len() {
				return &gqlValue{err: fmt.Errorf(`%s (with %s of %d) not found`, fieldInfo.Name, fieldInfo.Subscript, idx+fieldInfo.BaseIndex)}
			}
			v = v.Index(idx)
		}
	}

	// It's a custom scalar if there exists a method (on ptr to type) with signature: func (*T) UnmarshalEGGQL(string) error
	// Note: we check for ptr (not value) receiver as "unmarshaling" modifies though we are marshaling here
	t := v.Type()
	pt := reflect.TypeOf(reflect.New(t).Interface())
	if pt.Implements(field.UnmarshalerType) {
		var valueString string
		var err error

		if t.Implements(reflect.TypeOf((*field.Marshaler)(nil)).Elem()) {
			// Call the Marshal method, ie: func (T) MarshalEGGQL() (string, error)
			valueString, err = v.Interface().(field.Marshaler).MarshalEGGQL()
			if err != nil {
				return &gqlValue{err: fmt.Errorf("%w marshaling custom scalar %q", err, t.Name())}
			}
		} else if pt.Implements(reflect.TypeOf((*field.Marshaler)(nil)).Elem()) {
			// In case Marshal method uses ptr receiver (value receiver preferred) ie: func (*T) MarshalEGGQL() (string, error)
			tmp := reflect.New(t) // we have to make an addressable copy of v so we can call with ptr receiver
			tmp.Elem().Set(v)
			valueString, err = tmp.Interface().(field.Marshaler).MarshalEGGQL()
			if err != nil {
				return &gqlValue{err: fmt.Errorf("%w marshalling pointer to custom scalar %q", err, t.Name())}
			}
		} else if t.Implements(reflect.TypeOf((*fmt.Stringer)(nil)).Elem()) {
			// func (T) String() string - method is present
			valueString = v.Interface().(fmt.Stringer).String()
		} else if pt.Implements(reflect.TypeOf((*fmt.Stringer)(nil)).Elem()) {
			// func (*T) String() string - method is present
			tmp := reflect.New(t) // we have to make an addressable copy of v so we can call with ptr receiver
			tmp.Elem().Set(v)
			valueString = tmp.Interface().(fmt.Stringer).String()
		} else {
			valueString = fmt.Sprintf("%v", v.Interface())
		}
		return &gqlValue{name: astField.Alias, value: valueString}
	}

	switch t.Kind() {
	case reflect.Struct:
		// Check if we have to fabricate an "id" field
		var id *idField
		if fieldInfo.FieldID != "" {
			id = &idField{name: fieldInfo.FieldID, value: vID}
			if fieldInfo.BaseIndex > 0 {
				tmp := vID.Interface().(int)
				id.value = reflect.ValueOf(tmp + fieldInfo.BaseIndex)
			}
		}
		// Look up all sub-queries in this object
		if result, err := op.GetSelections(ctx, astField.SelectionSet, v, reflect.Value{}, nil, id); err != nil {
			return &gqlValue{err: err}
		} else {
			return &gqlValue{name: astField.Alias, value: result}
		}

	case reflect.Map:
		var results []interface{}
		if v.IsNil() {
			if !fieldInfo.Nullable {
				return &gqlValue{err: fmt.Errorf("returning null when list %q is not nullable", astField.Alias)}
			}
			// else return nil (for null list)
		} else {
			// resolve for all values in the map
			results = make([]interface{}, 0, v.Len()) // to distinguish empty slice from nil slice
			for it := v.MapRange(); it.Next(); {
				if value := op.resolve(ctx, astField, it.Value(), it.Key(), fieldInfo); value != nil {
					results = append(results, value.value)
				}
			}
		}
		return &gqlValue{name: astField.Alias, value: results}

	case reflect.Slice, reflect.Array:
		var results []interface{}
		if t.Kind() == reflect.Slice && v.IsNil() {
			if !fieldInfo.Nullable {
				return &gqlValue{err: fmt.Errorf("returning null when list %q is not nullable", astField.Alias)}
			}
			// else return nil (for null list)
		} else {
			// resolve for all values in the list
			results = make([]interface{}, 0, v.Len()) // to distinguish empty slice from nil slice
			for i := 0; i < v.Len(); i++ {
				if value := op.resolve(ctx, astField, v.Index(i), reflect.ValueOf(i), fieldInfo); value != nil {
					if value.err != nil {
						return value
					}
					results = append(results, value.value)
				}
			}
		}
		return &gqlValue{name: astField.Alias, value: results}
	}
	if fieldInfo.GQLTypeName == "ID" {
		return &gqlValue{name: astField.Alias, value: v.Interface()}
	}
	// If enum or enum list get the integer index and look up the enum value
	if enumName := fieldInfo.GQLTypeName; enumName != "" {
		if enumName[len(enumName)-1] == '!' {
			enumName = enumName[:len(enumName)-1]
		}
		if len(enumName) > 2 && enumName[0] == '[' && enumName[len(enumName)-1] == ']' {
			enumName = enumName[1 : len(enumName)-1]
			if enumName[len(enumName)-1] == '!' {
				enumName = enumName[:len(enumName)-1]
			}
		}
		// Check that the enum exists
		if _, ok := op.enums[enumName]; !ok {
			return &gqlValue{err: fmt.Errorf("enum %q not found for field %q", enumName, fieldInfo.Name)}
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

	// Just return the scalar value (Int, String, Boolean, or Float)
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
