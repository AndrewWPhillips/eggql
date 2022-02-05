package handler

// call.go uses reflection to call a Go function that implements a GraphQL resolver

import (
	"context"
	"errors"
	"fmt"
	"github.com/andrewwphillips/eggql/internal/field"
	"github.com/vektah/gqlparser/ast"
	"reflect"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// fromFunc converts a Go function into the type/value of what it returns by calling it using reflection
// Parameters:
//   ctx - is a context.Context that may be cancelled at any time
//   field - is the GraphQL query object field
//   v - the reflection "value" of the Go function's return value
//   fieldInfo - contains the parameter names and defaults obtained from the Go field metadata
func (op *gqlOperation) fromFunc(ctx context.Context, astField *ast.Field, v reflect.Value, fieldInfo *field.Info) (vReturn reflect.Value, err error) {
	if v.IsNil() {
		err = fmt.Errorf("function for %q is not implemented (nil)", astField.Name)
		return
	}
	args := make([]reflect.Value, v.Type().NumIn()) // list of arguments for the function call
	baseArg := 0                                    // index of 1st query resolver argument (== 1 if function call needs ctx, else == 0)

	if fieldInfo.HasContext {
		args[baseArg] = reflect.ValueOf(ctx)
		baseArg++ // we're now expecting one less value in params/defaults lists
	}

	// Add supplied arguments
	for _, argument := range astField.Arguments {
		// Which arg # is it (GraphQL params are supplied by name not position)
		n := -1
		for paramNum, paramName := range fieldInfo.Params {
			if paramName == argument.Name {
				if n != -1 {
					// Note this is a BUG not an "error" as it should have been caught by validator
					panic("argument specified more than once: " + argument.Name + " in " + astField.Name)
				}
				n = paramNum
			}
		}
		if n == -1 {
			// Note this is a BUG not an "error" as it should have been caught by validator
			panic("unknown argument: " + argument.Name + " in " + astField.Name)
		}

		// rawValue stores the value of an argument the same way the JSON decoder does. Eg: a GraphQL "object" (to be
		// decoded into a Go struct) is stored as a map[string]interface{} where each map entry is a field of the object
		// and a GraphQL list is stored in a []interface{}. Obviously these can be nested, such as an object containing
		// another object or a list.
		var rawValue interface{}
		if rawValue, err = argument.Value.Value(op.variables); err != nil {
			return
		}

		// Now convert the "raw" value into the expected Go parameter type
		if args[baseArg+n], err = op.getValue(v.Type().In(baseArg+n), argument.Name, fieldInfo.Enums[n], rawValue); err != nil {
			return
		}
	}

	// For any arguments not supplied use the default
	for n, arg := range args {
		// if the argument has not yet been set
		if !arg.IsValid() {
			// Find the arg in the field definition and get the default value
			// (which should come from the text of fieldInfo.Defaults[n-baseArg])
			ok := false
			for _, defArg := range astField.Definition.Arguments {
				if defArg.Name == fieldInfo.Params[n-baseArg] {
					tmp, err := defArg.DefaultValue.Value(op.variables)
					if err != nil {
						panic(err)
					}
					args[n], err = op.getValue(v.Type().In(n), defArg.Name, fieldInfo.Enums[n-baseArg], tmp)
					if err != nil {
						panic(err)
					}
					ok = true
					break
				}
			}
			if !ok {
				panic("default not found for " + fieldInfo.Params[n-baseArg])
			}
		}
	}

	out := v.Call(args) // === the actual function call (using reflection) ===
	if len(out) < 1 || len(out) > 2 {
		// panic here as this should have already been validated in schema generation
		panic("a resolver should only return one or two values")
	}

	// Extract the error return value (if any)
	if fieldInfo.HasError {
		if len(out) != 2 {
			panic("resolver should have an error return")
		}
		//typ := out[1].Type()
		//if typ.Kind() != reflect.Interface || !typ.Implements(errorType) {
		//	panic("a resolver function's 2nd return value must be a Go error")
		//}
		if iface := out[1].Interface(); iface != nil {
			err = iface.(error) // return error from the call
		}
	}
	return out[0], err
}

// getValue returns a value (eg for a resolver argument) given an interface{} and an expected Go type
// Parameters:
//   t = expected type
//   name = corresponding name of the argument
//   value = what needs to be returned as a value of type t
func (op *gqlOperation) getValue(t reflect.Type, name string, enumName string, value interface{}) (reflect.Value, error) {
	// If it's an enum we need to convert the enum name (string) to int
	if enumName != "" && t.Kind() >= reflect.Int && t.Kind() <= reflect.Uint64 {
		toFind, ok := value.(string)
		if !ok {
			return reflect.Value{}, fmt.Errorf("getting enum (%s) for %q expected string", enumName, name)
		}
		for i, v := range op.enums[enumName] {
			// TODO: pre-create a map for lookup rather than doing linear search
			if v == toFind {
				value = i // value changes from string to int
				break
			}
		}
	}
	kind := reflect.TypeOf(value).Kind() // expected type of value to return
	if kind == t.Kind() && kind != reflect.Map && kind != reflect.Slice {
		return reflect.ValueOf(value), nil // no conversion necessary
	}
	// Try to convert the type of the variable to the expected type
	switch kind {
	case reflect.Map:
		// GraphQl "input" variables are decoded from JSON as a map[string]interface{} which we use to make
		// a Go struct where the string is a field name and the value in the interface is the field value.
		m, ok := value.(map[string]interface{})
		if !ok {
			return reflect.Value{}, fmt.Errorf("decoding %q - expected map[string] of interface{}", name)
		}
		return op.getStruct(t, name, m)
	case reflect.Slice:
		list, ok := value.([]interface{})
		if !ok {
			return reflect.Value{}, fmt.Errorf("decoding variable %q - expected slice of interface{}", name)
		}
		if len(enumName) > 2 && enumName[0] == '[' && enumName[len(enumName)-1] == ']' {
			enumName = enumName[1 : len(enumName)-1]
		}
		return op.getList(t, name, enumName, list)
	case reflect.String:
		return op.getString(t, value.(string))
	case reflect.Int:
		return op.getInt(t, int64(value.(int)))
	case reflect.Int8:
		return op.getInt(t, int64(value.(int8)))
	case reflect.Int16:
		return op.getInt(t, int64(value.(int16)))
	case reflect.Int32:
		return op.getInt(t, int64(value.(int32)))
	case reflect.Int64:
		return op.getInt(t, value.(int64))
	case reflect.Uint:
		return op.getInt(t, int64(value.(uint)))
	case reflect.Uint8:
		return op.getInt(t, int64(value.(uint8)))
	case reflect.Uint16:
		return op.getInt(t, int64(value.(uint16)))
	case reflect.Uint32:
		return op.getInt(t, int64(value.(uint32)))
	case reflect.Uint64:
		return op.getInt(t, int64(value.(uint64)))
	case reflect.Float32:
		return op.getFloat(t, float64(value.(float32)))
	case reflect.Float64:
		return op.getFloat(t, value.(float64))
	default:
		return reflect.Value{}, fmt.Errorf("variable %q is of unsupported type (kind %v)", name, kind.String())
	}
}

// getStruct converts a map (eg a from JSON decoder) to a struct including any nested structs, and slices
// Parameters
//  t = type of the struct that we need to fill in from the GraphQL object
//  name = name of the argument // TODO unnec. so remove
//  m = map key is field names of the object, map value is field values
func (op *gqlOperation) getStruct(t reflect.Type, name string, m map[string]interface{}) (reflect.Value, error) {
	if t.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("argument %q is not an GraphQL INPUT type", name)
	}

	// Create an instance of the struct and fill in the exported fields using m
	r := reflect.New(t).Elem()
	for idx := 0; idx < t.NumField(); idx++ {
		f := t.Field(idx)
		fieldInfo, err2 := field.Get(&f)
		if err2 != nil {
			return reflect.Value{}, fmt.Errorf("%w getting field %q", err2, f.Name)
		}
		if fieldInfo == nil {
			continue // ignore unexported field
		}
		// TODO check if we need to handle fieldInfo.Embedded - I don't think INPUT types can implement interfaces
		first, n := utf8.DecodeRuneInString(fieldInfo.Name)
		if first == utf8.RuneError {
			return reflect.Value{}, fmt.Errorf("field %q of variable %q is not valid non-empty UTF8 string", fieldInfo.Name, name)
		}
		goField := r.FieldByName(string(unicode.ToUpper(first)) + fieldInfo.Name[n:])
		if !goField.IsValid() {
			return reflect.Value{}, fmt.Errorf("field %q of %q is not a field name of the GraphQL INPUT type", fieldInfo.Name, name)
		}
		v, err := op.getValue(goField.Type(), fieldInfo.Name, fieldInfo.GQLTypeName, m[fieldInfo.Name])
		if err != nil {
			return reflect.Value{}, fmt.Errorf("converting field %q of %q: %w", fieldInfo.Name, name, err)
		}

		goField.Set(v)
	}
	return r, nil
}

// getList converts a list of values from a GraphQL variable or literal into a Go slice
// Parameters
//  t = type of the slice that we need to fill in from the GraphQL list
//  name = name of the argument
//  enumName = name of enum if list is a list of enums
//  list = slice of element from the GraphQL list
func (op *gqlOperation) getList(t reflect.Type, name string, enumName string, list []interface{}) (reflect.Value, error) {
	if t.Kind() != reflect.Slice { // TODO also handle arrays
		return reflect.Value{}, fmt.Errorf("argument %q is not a list", name)
	}

	// Create an instance of the struct and fill in the fields that we were given
	//	r := reflect.New(t).Elem()
	r := reflect.MakeSlice(t, len(list), len(list))
	for i, value := range list {
		goElement := r.Index(i)

		// Get the field value as the type of the Go struct's field
		v, err := op.getValue(goElement.Type(), fmt.Sprintf("%s[%d]", name, i), enumName, value)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("getting slice value %s[%d]: %w", name, i, err)
		}
		goElement.Set(v)
	}
	return r, nil
}

// getInt takes an integer and returns the value as the desired Go type (incl. ints, floats, bool, & string types).
// This is used to ensure a GraphQL variable is passed as the correct type as a parameter to a resolver function.
// Depending on the types and values the result might not be specified.  For example an int may overflow if converted
// to a smaller type.  Negative or large values may also be changed (wrap around) if converting between signed and
// unsigned integers.  Even a large int value may lose precision if converted to a float.
func (op *gqlOperation) getInt(t reflect.Type, i int64) (reflect.Value, error) {
	switch t.Kind() {
	case reflect.Bool:
		return reflect.ValueOf(i != 0), nil
	case reflect.Int:
		return reflect.ValueOf(int(i)), nil
	case reflect.Int8:
		return reflect.ValueOf(int8(i)), nil
	case reflect.Int16:
		return reflect.ValueOf(int16(i)), nil
	case reflect.Int32:
		return reflect.ValueOf(int32(i)), nil
	case reflect.Int64:
		return reflect.ValueOf(i), nil
	case reflect.Uint:
		return reflect.ValueOf(uint(i)), nil
	case reflect.Uint8:
		return reflect.ValueOf(uint8(i)), nil
	case reflect.Uint16:
		return reflect.ValueOf(uint16(i)), nil
	case reflect.Uint32:
		return reflect.ValueOf(uint32(i)), nil
	case reflect.Uint64:
		return reflect.ValueOf(uint64(i)), nil
	case reflect.Float32:
		return reflect.ValueOf(float32(i)), nil
	case reflect.Float64:
		return reflect.ValueOf(float64(i)), nil
	case reflect.String:
		return reflect.ValueOf(strconv.FormatInt(i, 10)), nil
	}
	return reflect.Value{}, errors.New("TODO")
}

// getFloat takes a float and returns the value as the desired Go type (incl. all int, float types + string).
// There may be overflow or loss of precision eg if a float64 is converted to a float32 or int.
func (op *gqlOperation) getFloat(t reflect.Type, f float64) (reflect.Value, error) {
	switch t.Kind() {
	case reflect.Float32:
		return reflect.ValueOf(float32(f)), nil
	case reflect.Float64:
		return reflect.ValueOf(f), nil
	case reflect.String:
		return reflect.ValueOf(strconv.FormatFloat(f, 'g', -1, 64)), nil
	default:
		return op.getInt(t, int64(f))
	}
}

// getString converts a string into the expected type of a resolver function's parameter
// Parameters:
//   t = the resolver argument's type
//   s = the argument value as a string
func (op *gqlOperation) getString(t reflect.Type, s string) (reflect.Value, error) {
	// Convert the default value (a string) to the type expected as function argument
	switch t.Kind() {
	case reflect.Bool:
		//boolValue := len(s) > 0 && (s[0] == 't' || s[0] == 'T' || s[0] == '1')
		//return reflect.ValueOf(boolValue), nil
		// The only GraphQL boolean literals are "true" and "false"
		switch s {
		case "false":
			return reflect.ValueOf(false), nil
		case "true":
			return reflect.ValueOf(true), nil
		}
		return reflect.Value{}, errors.New("Invalid boolean value: " + s)
	case reflect.Int:
		intValue, err := strconv.Atoi(s)
		return reflect.ValueOf(intValue), err
	case reflect.Int8:
		intValue, err := strconv.ParseInt(s, 10, 8)
		return reflect.ValueOf(int8(intValue)), err
	case reflect.Int16:
		intValue, err := strconv.ParseInt(s, 10, 16)
		return reflect.ValueOf(int16(intValue)), err
	case reflect.Int32:
		intValue, err := strconv.ParseInt(s, 10, 32)
		return reflect.ValueOf(int32(intValue)), err
	case reflect.Int64:
		intValue, err := strconv.ParseInt(s, 10, 64)
		return reflect.ValueOf(intValue), err
	case reflect.Uint:
		uintValue, err := strconv.ParseUint(s, 10, 0)
		return reflect.ValueOf(uint(uintValue)), err
	case reflect.Uint8:
		uintValue, err := strconv.ParseUint(s, 10, 8)
		return reflect.ValueOf(uint8(uintValue)), err
	case reflect.Uint16:
		uintValue, err := strconv.ParseUint(s, 10, 16)
		return reflect.ValueOf(uint16(uintValue)), err
	case reflect.Uint32:
		uintValue, err := strconv.ParseUint(s, 10, 32)
		return reflect.ValueOf(uint32(uintValue)), err
	case reflect.Uint64:
		uintValue, err := strconv.ParseUint(s, 10, 64)
		return reflect.ValueOf(uintValue), err
	case reflect.Float32:
		floatValue, err := strconv.ParseFloat(s, 32)
		return reflect.ValueOf(float32(floatValue)), err
	case reflect.Float64:
		floatValue, err := strconv.ParseFloat(s, 64)
		return reflect.ValueOf(floatValue), err
	case reflect.String:
		return reflect.ValueOf(s), nil
	}

	return reflect.Value{}, errors.New("unexpected type in getString") // TODO: check if we missed anything
}
