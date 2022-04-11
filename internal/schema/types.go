package schema

import (
	"errors"
	"github.com/andrewwphillips/eggql/internal/field"
	"reflect"
)

// getTypeName returns the GraphQL type name corresponding to a Go type.
// Parameters:
//  t = Go type of GraphQL field - for a function it uses the type of the 1st return value
// Returns: name, isScalar, error
//  name = type name [in square brackets for a list(array/slice/map)], empty string if not know (eg anon struct)
//     if t is a function it uses the 1st return value
//  isScalar = true for Int/String/etc or custom scalars, or lists thereof (array/slice/map), false for struct or if not known
//  error = non-nil for type that can't be handled (eg func that returns nothing)
//     for anonymous structs it does not return an error but the returned name is an empty string
func (s schema) getTypeName(t reflect.Type) (string, bool, error) {
	//b1 := t.Implements(reflect.TypeOf((*field.Unmarshaler)(nil)).Elem())
	//b2 := reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaler)(nil)).Elem())
	//fmt.Printf("%s: %v %v %v %v\n", t.Name(), b1, b2)

	// Assume it's a custom scalar if there is a method with signature: func (*T) UnmarshalEGGQL(string) error
	// Note that reflect.TypeOf(reflect.New(t).Interface()) is used to get the type of ptr to t.
	// (UnmarshalEGGQL must have a pointer (not value) receiver since the new value is saved.)
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaler)(nil)).Elem()) {
		found := false
		for _, name := range *s.scalars {
			if name == t.Name() {
				found = true
				break
			}
		}
		if !found {
			*s.scalars = append(*s.scalars, t.Name())
		}
		return t.Name(), true, nil
	}

	switch t.Kind() {
	case reflect.Bool:
		return "Boolean", true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "Int", true, nil
	case reflect.Float32, reflect.Float64:
		return "Float", true, nil
	case reflect.String:
		return "String", true, nil

	case reflect.Struct:
		if tmp := t.Name(); tmp != "" {
			return tmp, false, nil // use struct's type name and indicate it's not a scalar
		}
		return "", false, nil // not an error but struct is anon
	case reflect.Ptr:
		return s.getTypeName(t.Elem())
	case reflect.Map, reflect.Array, reflect.Slice:
		elemType, isScalar, err := s.getTypeName(t.Elem())
		if err != nil {
			return "", false, err
		}
		if elemType == "" {
			return "", false, errors.New("bad element type for slice/array/map " + t.Name())
		}
		return "[" + elemType + "]", isScalar, nil
	case reflect.Func:
		// For functions the (1st) return type is the type of the resolver
		if t.NumOut() == 0 { // help caller fix their defect by returning an error instead of panicking
			return "", false, errors.New("resolver functions must return a value: " + t.Name())
		}
		return s.getTypeName(t.Out(0))
	case reflect.Interface:
		// Resolver functions returning a GraphQL "interface" type return a Go interface{} but we
		// don't know the type name, or whether it is a scalar or not
		return "", false, nil
	default:
		return "", false, errors.New("unhandled type " + t.Name())
	}
}
