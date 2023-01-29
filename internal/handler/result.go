package handler

// result.go is used to generate the query output string

import (
	"context"
	"fmt"
	"reflect"

	"github.com/andrewwphillips/eggql/internal/field"
	"github.com/dolmen-go/jsonmap"
	"github.com/vektah/gqlparser/v2/ast"
)

type (
	// gqlOperation controls an operation (query/mutation) of a GraphQL request
	gqlOperation struct {
		*Handler // required for resolver lookups, enums etc

		isMutation, isSubscription bool
		variables                  map[string]interface{} // varibale valid fr this op (extracted from the request)
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
//
//	entry for each selection, where the map "key" is the name of the entry/resolver and the value is:
//	a) scalar value (stored in an interface})
//	b) a nested jsonmap.Ordered if the resolver is a nested struct
//	c) a slice (ie []interface{}) if the resolver is a slice or array.
//
// Parameters:
//
//	ctx = a Go context that could expire at any time
//	set = list of selections from a GraphQL query to be resolved
//	data = slice of Go structs with the resolvers (usually has just one struct unless using schema stitching)
//	idField = name/type of fabricated "id" field (see "field_id" option for lists of objects)
func (op *gqlOperation) GetSelections(ctx context.Context, set ast.SelectionSet, data []interface{}, id *idField,
) (jsonmap.Ordered, error) {
	resultChans := make([]<-chan gqlValue, 0, len(set))
	for _, s := range set {
		// For each query we check all the data structs
	dataLoop:
		for _, d := range data {
			// Get the struct that contains the resolvers that we can use
			v := reflect.ValueOf(d)
			for v.Type().Kind() == reflect.Ptr {
				v = v.Elem() // follow indirection
			}

			switch astType := s.(type) {
			case *ast.Field:
				if id != nil && astType.Name == id.name {
					// Requesting generated ID field - return chan with the fabricated ID
					ch := make(chan gqlValue, 1)
					ch <- gqlValue{
						name: id.name, value: id.value.Interface(),
					}
					close(ch)
					resultChans = append(resultChans, ch)
					break dataLoop
				} else {
					// Find and execute the "resolver" in the struct (or recursively in embedded structs)
					if ch := op.FindSelection(ctx, astType, v); ch != nil {
						resultChans = append(resultChans, ch)
						break dataLoop // we got a result so stop looking
					}
				}

			case *ast.InlineFragment:
				if v.Type().Name() != astType.TypeCondition {
					continue dataLoop // TODO: decide whether to continue or break
				}
				resultChans = append(resultChans, op.FindFragments(ctx, astType.SelectionSet, v))

			case *ast.FragmentSpread:
				resultChans = append(resultChans, op.FindFragments(ctx, astType.Definition.SelectionSet, v))
			}
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
				if _, ok := r.Data[v.name]; !ok {
					r.Order = append(r.Order, v.name) // only append to order if not already in the map
				}
				r.Data[v.name] = v.value
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

// FindSelection returns resolved value in a chan (if found), or empty chan (if excluded), or nil (not found)
// Parameters:
//   - ctx: context that indicates if the request has been cancelled
//   - astField: contains the query name, arguments etc to be resolved
//   - v: struct which may contain the field required to resolve astField
//
// Returns:
//   - if found: closed chan containing a single value or error
//   - if found but excluded by directive: closed chan with no values
//   - if not found: nil
func (op *gqlOperation) FindSelection(ctx context.Context, astField *ast.Field, v reflect.Value) <-chan gqlValue {
	if v.Type().Kind() != reflect.Struct { // struct = 25
		// param. 'v' validation - note that this is a bug that should have been caught during schema building
		panic("FindSelection: search of query field in non-struct")
	}

	if !op.noIntrospection && astField.Name == "__typename" { // __typename is a special introspection field (see GraphQL spec)
		r := make(chan gqlValue, 1)
		r <- gqlValue{name: astField.Alias, value: astField.ObjectDefinition.Name}
		close(r)
		return r
	}

	// get the index of the resolver field then the type and value of that field
	resolverInfo, ok := op.resolverLookup[v.Type()][astField.Name]
	if !ok {
		// TODO: scan to double-check that we don't have a field with the correct name (= bug)
		// No matching field so close chan without writing
		return nil
	}
	tField := v.Type().Field(resolverInfo.Index)
	vField := v.Field(resolverInfo.Index)

	fieldInfo, _ := field.Get(&tField)
	// Recursively check fields of embedded struct
	if fieldInfo.Embedded {
		// if a field in the embedded struct matches a value is sent on the chan returned from FindSelection
		if ch := op.FindSelection(ctx, astField, vField); ch != nil {
			for v := range ch {
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
	}

	var cache ResolverCache
	if resolverInfo.Cache.Saved != nil {
		cache = resolverInfo.Cache
	}
	if op.isMutation || op.noConcurrency { // Mutations are run sequentially
		ch := make(chan gqlValue, 1)
		op.wrapResolve(ctx, astField, vField, reflect.Value{}, fieldInfo, cache, ch)
		return ch
	} else {
		ch := make(chan gqlValue)
		// Calling wrapResolve as a go routine allows resolvers to run in parallel
		go op.wrapResolve(ctx, astField, vField, reflect.Value{}, fieldInfo, cache, ch)
		return ch
	}
}

// wrapResolve calls resolve putting the return value on a chan and converting any panic to an error
func (op *gqlOperation) wrapResolve(
	ctx context.Context, astField *ast.Field, v, vID reflect.Value, fieldInfo *field.Info, cache ResolverCache,
	ch chan<- gqlValue,
) {
	defer func() {
		// Convert any panics in resolvers into an (internal) error
		if recoverValue := recover(); recoverValue != nil {
			ch <- gqlValue{err: fmt.Errorf("Internal error: panic %v", recoverValue)}
		}
		close(ch)
	}()
	if value := op.resolve(ctx, astField, v, vID, fieldInfo, cache); value != nil {
		ch <- *value
	}
}

func (op *gqlOperation) FindFragments(ctx context.Context, set ast.SelectionSet, v reflect.Value) <-chan gqlValue {
	result, err := op.GetSelections(ctx, set, []interface{}{v.Interface()}, nil)

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
//
//	ctx = a Go context that could expire at any time
//	astField = a query or sub-query - a field of a GraphQL object
//	v = value of the resolver (field of Go struct)
//	vID = value of "id" (only supplied if an element of a list)
//	fieldInfo = metadata for the resolver (eg parameter name) obtained from the struct field tag
func (op *gqlOperation) resolve(ctx context.Context, astField *ast.Field, v, vID reflect.Value, fieldInfo *field.Info,
	cache ResolverCache,
) (retval *gqlValue) {
	var key CacheKey
	if op.directiveBypass(astField) {
		return nil
	}

	// If this resolver has an active cache...
	if cache.Saved != nil {
		// Check if we have a cached value that we can return
		key = CacheKey{
			fieldValue: v,
		}
		cache.Mtx.Lock()
		result, ok := cache.Saved[key]
		cache.Mtx.Unlock()
		if ok {
			retval = &gqlValue{name: astField.Alias, value: result.Interface()}
			return
		}

		// If not in cache save any valid return in the cache
		defer func() {
			if retval.err == nil && retval.value != nil {
				cache.Mtx.Lock()
				cache.Saved[key] = reflect.ValueOf(retval.value)
				cache.Mtx.Unlock()
			}
		}()
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
		arg, err := op.getValue(fieldInfo.IndexType, fieldInfo.Subscript, "", astField.Arguments[0].Value.Raw)
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
			tmp := reflect.New(t) // we have to make an addressable copy of v, so that we can call with ptr receiver
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
		if result, err := op.GetSelections(ctx, astField.SelectionSet, []interface{}{v.Interface()}, id); err != nil {
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
				// TODO: allow list elements to be cached
				if value := op.resolve(ctx, astField, it.Value(), it.Key(), fieldInfo, ResolverCache{}); value != nil {
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
				// TODO: allow list elements to be cached
				if value := op.resolve(ctx, astField, v.Index(i), reflect.ValueOf(i), fieldInfo, ResolverCache{}); value != nil {
					if value.err != nil {
						return value
					}
					results = append(results, value.value)
				}
			}
		}
		return &gqlValue{name: astField.Alias, value: results}

	case reflect.Chan:
		return &gqlValue{name: astField.Alias, value: v.Interface()}
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
