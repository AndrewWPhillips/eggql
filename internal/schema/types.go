package schema

import (
	"errors"
	"github.com/andrewwphillips/eggql/internal/field"
	"reflect"
)

// getTypeName returns the GraphQL type name corresponding to a Go type.
// The return value is just the type name for scalars and structs, but for slices/arrays
// it's the element type in square brackets, and for functions the (1st) return type.
// For types that cannot be used or are not handled (yet) it returns an error - eg. a
// func that does not return a single value (or a value and an error).
// For anonymous types (no name) or where the type is unknown it returns "" (no error).
func (s schema) getTypeName(t reflect.Type) (string, error) {
	//b1 := t.Implements(reflect.TypeOf((*field.Unmarshaller)(nil)).Elem())
	//b2 := reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaller)(nil)).Elem())
	//fmt.Printf("%s: %v %v %v %v\n", t.Name(), b1, b2)

	// Assume it's a custom scalar if there is a func (*T) UnmarshalEGGQL(string) error
	// Note that reflect.TypeOf(reflect.New(t).Interface()) is used to get the type of ptr to t.
	// (UnmarshalEGGQL must have a pointer (not value) receiver since the new value is saved.)
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaller)(nil)).Elem()) {
		*s.scalars = append(*s.scalars, t.Name())
		return t.Name(), nil
	}

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
		return s.getTypeName(t.Elem())
	case reflect.Map, reflect.Array, reflect.Slice:
		elemType, err := s.getTypeName(t.Elem())
		if err != nil {
			return "", err
		}
		if elemType == "" {
			return "", errors.New("bad element type for slice/array/map " + t.Name())
		}
		return "[" + elemType + "]", nil
	case reflect.Func:
		// For functions the (1st) return type is the type of the resolver
		if t.NumOut() == 0 { // help caller fix their defect by returning an error instead of panicking
			return "", errors.New("resolver functions must return a value: " + t.Name())
		}
		return s.getTypeName(t.Out(0))
	case reflect.Interface:
		return "", nil // functions returning an GraphQL "interface" type return a Go interface{} but we can't tell the type here
	default:
		return "", errors.New("unhandled type " + t.Name())
	}
}
