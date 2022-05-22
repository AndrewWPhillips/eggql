package schema

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

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

	// Check if the type is a custom scalar
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(field.UnmarshalerType) {
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

	// Check if it's a known union
	if _, ok := s.unions[typeName]; ok {
		if t.Kind() != reflect.Struct && t.Kind() != reflect.Interface {
			// return false, fmt.Errorf("A union (%s) field must return an interface (not %v)", typeName, t.Kind())
			return false, fmt.Errorf("expecting resolver type %q but got %v", typeName, t.Kind())
		}
		return false, nil
	}

	// Check if it's an object type seen already
	if _, ok := s.declaration[typeName]; ok {
		if t.Kind() != reflect.Struct && t.Kind() != reflect.Interface {
			return false, fmt.Errorf("expecting resolver type %q but got %v", typeName, t.Kind())
		}
		if typeName != t.Name() && t.Name() != "" {
			return false, fmt.Errorf("Object field (%s) cannot have a resolver of type %q", t.Name(), typeName)
		}
		return false, nil
	}

	return false, fmt.Errorf("type %q is not known", typeName)
}

// getTypeName returns the GraphQL type name corresponding to a Go type.
// Parameters:
//  t = effective Go type of GraphQL field - for a function it uses the type of the 1st return value
//  nullable = true a value of the type can be assigned NULL
// Returns: name, isScalar, error
//  name = type name [in square brackets for a list(array/slice/map)], empty string if not know (eg anon struct)
//     if t is a function it uses the 1st return value
//  isScalar = true for Int/String/etc or custom scalars, or lists thereof (array/slice/map), false for struct or if not known
//  err = non-nil for type that can't be handled (eg func that returns nothing)
//     for anonymous structs it does not return an error but the returned name is an empty string
func (s schema) getTypeName(t reflect.Type, nullable bool) (name string, isScalar bool, err error) {
	defer func() {
		if err == nil && name != "" && !nullable {
			name += "!"
		}
	}()
	// Assume it's a custom scalar if there is a method with signature: func (*T) UnmarshalEGGQL(string) error
	// Note that reflect.TypeOf(reflect.New(t).Interface()) is used to get the type of ptr to t.
	// (UnmarshalEGGQL must have a pointer (not value) receiver since the new value is saved.)
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(field.UnmarshalerType) {
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
		name = t.Name()
		isScalar = true
		return
	}

	switch t.Kind() {
	case reflect.Bool:
		name = "Boolean"
		isScalar = true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		name = "Int"
		isScalar = true
	case reflect.Float32, reflect.Float64:
		name = "Float"
		isScalar = true
	case reflect.String:
		if t.Name() == "ID" && strings.Contains(t.PkgPath(), "eggql") {
			name = "ID"
		} else {
			name = "String"
		}
		isScalar = true

	case reflect.Struct:
		name = t.Name() // may be "" for anon struct

	case reflect.Ptr:
		name, isScalar, err = s.getTypeName(t.Elem(), false)
		nullable = true
		if name != "" && name[len(name)-1] == '!' {
			name = name[:len(name)-1] // remove non-nullability
		}
	case reflect.Map, reflect.Array, reflect.Slice:
		name, isScalar, err = s.getTypeName(t.Elem(), false)
		if err != nil {
			return
		}
		if name == "" {
			err = errors.New("element type unknown for slice/array/map " + t.Name())
			return
		}
		name = "[" + name + "]"
	case reflect.Interface:
		// Nothing needed here - return empty name and no error.  This is for GraphQL "interface" fields where
		// the Go func returns an interface{} but we don't know the type name, or whether it is a scalar or not
	default:
		err = errors.New("unhandled type " + t.Name())
	}
	return
}
