package schema

import (
	"errors"
	"reflect"
)

// getTypeName returns the GraphQL type name corresponding to a Go type.
// The return value is just the type name for scalars and structs, but for slices/arrays
// it's the element type in square brackets, and for functions the (1st) return type.
// For types that cannot be used or are not handled (yet) it returns an error - eg. a
// func that does not return a single value (or a value and an error).
// For anonymous types (no name) or where the type is unknown it returns "" (no error).
func getTypeName(t reflect.Type) (string, error) {
	switch t.Kind() {
	case reflect.Bool:
		return "Boolean", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "Int", nil
	case reflect.Float32, reflect.Float64:
		return "Float", nil
	case reflect.String:
		return "String", nil

	case reflect.Struct:
		if tmp := t.Name(); tmp != "" {
			return tmp, nil // use struct's type name
		}
		return "", nil // not an error but struct is anon
	case reflect.Ptr:
		return getTypeName(t.Elem())
	case reflect.Array, reflect.Slice: // TODO: check if we can also handle Map
		elemType, err := getTypeName(t.Elem())
		if err != nil {
			return "", err
		}
		return "[" + elemType + "]", nil
	case reflect.Func:
		// For functions the (1st) return type is the type of the resolver
		if t.NumOut() == 0 { // help caller fix their defect by returning an error instead of panicking
			return "", errors.New("resolver functions must return a value: " + t.Name())
		}
		return getTypeName(t.Out(0))
	case reflect.Interface:
		return "", nil // functions returning an GraphQL "interface" type return a Go interface{} but we can't tell the type here
	default:
		return "", errors.New("unhandled type " + t.Name())
	}
}
