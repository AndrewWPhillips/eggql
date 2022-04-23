package schema

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/andrewwphillips/eggql/internal/field"
)

// validateTypeName checks for a valid type name and that it matches the field type
// This is related to getTypeName (below) but checks rather than generates the type name
// Parameters
//   typeName = GraphQL type name to validate
//   enums == active enums in case the type is an enum
//   t = Go type to check that the GraphQL type is compatible with
// Returns isScalar, error
//   isScalar: true if it's a scalar including custom scalars and enums
//   error: non-nil if the type name is invalid or incompatible with the Go type (t)
func (s schema) validateTypeName(typeName string, enums map[string][]string, t reflect.Type) (bool, error) {
	// Get "unmodified" type - without non-nullable (!) and list modifiers
	if len(typeName) > 1 && typeName[len(typeName)-1] == '!' {
		typeName = typeName[:len(typeName)-1] // remove non-nullability
	} else if t.Kind() == reflect.Ptr {
		t = t.Elem() // nullable so get pointed to type
	}
	// if it's a list get the element type
	if len(typeName) > 2 && typeName[0] == '[' && typeName[len(typeName)-1] == ']' {
		typeName = typeName[1 : len(typeName)-1]
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array && t.Kind() != reflect.Map {
			return false, fmt.Errorf("A field with list resolver must have a slice/array/map type (not %v)", t.Kind())
		}
		t = t.Elem()

		if len(typeName) > 1 && typeName[len(typeName)-1] == '!' {
			typeName = typeName[:len(typeName)-1] // remove non-nullability
		} else if t.Kind() == reflect.Ptr {
			t = t.Elem() // nullable so get pointed to type
		}
	}

	// Check for custom scalar
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaler)(nil)).Elem()) {
		if typeName != t.Name() {
			return false, fmt.Errorf("Custom scalar field (%s) cannot have a resolver of type %q", t.Name(), typeName)
		}
		return true, nil
	}

	// Check for other scalar types
	switch typeName {
	case "Boolean":
		if t.Kind() != reflect.Bool {
			return false, fmt.Errorf("A Boolean GraphQL field must have a bool resolver (not %v)", t.Kind())
		}
		return true, nil
	case "Int":
		if t.Kind() < reflect.Int || t.Kind() > reflect.Uintptr {
			return false, fmt.Errorf("An Int GraphQL field must have an integer resolver (not %v)", t.Kind())
		}
		return true, nil
	case "Float":
		if t.Kind() < reflect.Float32 || t.Kind() > reflect.Float64 {
			return false, fmt.Errorf("A Float GraphQL field must have a floating point resolver (not %v)", t.Kind())
		}
		return true, nil
	case "String", "ID":
		if t.Kind() != reflect.String {
			return false, fmt.Errorf("A %q GraphQL field must have a string resolver (not %v)", typeName, t.Kind())
		}
		return true, nil
	}

	// Check if it's a known enum type
	if _, ok := enums[typeName]; ok {
		// For enums the resolver must have a Go integer type
		if t.Kind() < reflect.Int || t.Kind() > reflect.Uintptr {
			return false, fmt.Errorf("An Enum (%s) field must be an integer (not %v)", typeName, t.Kind())
		}
		return true, nil
	}

	// Check if it's an object type seen already
	if _, ok := s.declaration[typeName]; ok {
		if t.Kind() != reflect.Struct {
			return false, fmt.Errorf("An object (%s) field must have a struct resolver (not %v)", typeName, t.Kind())
		}
		if typeName != t.Name() {
			return false, fmt.Errorf("Object field (%s) cannot have a resolver of type %q", t.Name(), typeName)
		}
		return false, nil
	}

	// Check if it's a known union
	if _, ok := s.unions[typeName]; ok {
		if t.Kind() != reflect.Interface {
			return false, fmt.Errorf("A union (%s) field must return an interface (not %v)", typeName, t.Kind())
		}
		return false, nil
	}

	return false, fmt.Errorf("type %q is not known", typeName)
}

// getTypeName returns the GraphQL type name corresponding to a Go type.
// Parameters:
//  t = effective Go type of GraphQL field - for a function it uses the type of the 1st return value
// Returns: name, isScalar, error
//  name = type name [in square brackets for a list(array/slice/map)], empty string if not know (eg anon struct)
//     if t is a function it uses the 1st return value
//  isScalar = true for Int/String/etc or custom scalars, or lists thereof (array/slice/map), false for struct or if not known
//  error = non-nil for type that can't be handled (eg func that returns nothing)
//     for anonymous structs it does not return an error but the returned name is an empty string
func (s schema) getTypeName(t reflect.Type) (string, bool, error) {
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
	// Note we now get the "effective" field type so never a func - TODO check if we should allow for func effective type
	//case reflect.Func:
	//	// For functions the (1st) return type is the type of the resolver
	//	if t.NumOut() == 0 { // help caller fix their defect by returning an error instead of panicking
	//		return "", false, errors.New("resolver functions must return a value: " + t.Name())
	//	}
	//	return s.getTypeName(t.Out(0))
	case reflect.Interface:
		// Resolver functions returning a GraphQL "interface" type return a Go interface{} but we
		// don't know the type name, or whether it is a scalar or not
		return "", false, nil
	default:
		return "", false, errors.New("unhandled type " + t.Name())
	}
}
