package handler

// lookup.go is used to build lookup tables for quick lookup of enums and resolvers

import (
	"reflect"
	"strings"

	"github.com/andrewwphillips/eggql/internal/field"
)

// makeEnumTables returns 2 maps that allows quick lookup of enums in both directions - ie allowing you to:
//  1. get an enum value's name (string) from its index (int)
//  2. get an enum value (int) from its name (string)
// Using the Episode and LengthUnit enums (see example/starwars/main.go) as an example:
//  - at the top level both returned maps have a key of the enum name
//     - both maps will have 2 elements with keys of "Episode" and "LengthUnit" (the enum type name)
//  - for the 1st return value each map element is a slice of strings
//     - eg []string{ "NEWHOPE", "EMPIRE", "JEDI" } and []string{"METER", "FOOT"}
//  - for the 2nd return value each map element is a map with all the enum values (keyed by name)
//     - eg map[string]int{"NEWHOPE": 0, "EMPIRE": 1, "JEDI": 2 } and map[string]int{"METER": 0, "FOOT": 1}
func makeEnumTables(enums map[string][]string) (map[string][]string, map[string]map[string]int) {
	byIndex := make(map[string][]string, len(enums))
	byName := make(map[string]map[string]int, len(enums))
	for enumName, list := range enums {
		enum := make([]string, 0, len(list))
		enumInt := make(map[string]int, len(list))
		for i, v := range list {
			v = strings.SplitN(v, "#", 2)[0] // remove description
			v = strings.SplitN(v, "@", 2)[0] // remove directive(s)
			v = strings.TrimRight(v, " ")    // remove trailing spaces
			enum = append(enum, v)
			enumInt[v] = i
		}
		name := strings.TrimRight(strings.SplitN(enumName, "#", 2)[0], " ")
		byIndex[name] = enum
		byName[name] = enumInt
	}
	return byIndex, byName
}

// makeResolverTables builds lookup tables for all query/mutation/subscription structs of a schema.
// This allows us to quickly find the index of a field (resolver) given the struct type and resolver name.
// At the top level we have a map indexed by all the struct's (its reflect.Type) used for the schema, then
// for each struct we have a map indexed by the resolver name and giving the index of the field in the struct.
func makeResolverTables(qms ...[]interface{}) ResolverLookupTables {
	lt := make(ResolverLookupTables)
	for _, q := range qms {
		if q == nil {
			continue
		}
		for _, v := range q {
			if v == nil {
				continue
			}
			lt.addLookup(reflect.TypeOf(v))
		}
	}
	return lt
}

func (lt ResolverLookupTables) addLookup(t reflect.Type) {
	// Get "base" type to see if it's a struct
	for k := t.Kind(); k == reflect.Ptr; k = t.Kind() {
		t = t.Elem()
	}
	if t.Kind() == reflect.Func {
		t = t.Out(0)
	}
	if k := t.Kind(); k == reflect.Map || k == reflect.Slice || k == reflect.Array || k == reflect.Chan {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	if _, ok := lt[t]; ok {
		return // already done (or being done if nil)
	}
	lt[t] = nil // Reserve this entry so we don't do it again

	r := make(map[string]int, t.NumField())
	// Find all the fields that are resolvers
	for i := 0; i < t.NumField(); i++ {
		tField := t.Field(i)
		fieldInfo, err := field.Get(&tField)
		if err != nil {
			continue // TODO: check error
		}
		if fieldInfo == nil {
			continue // ignore unexported field
		}
		if tField.Name == "_" {
			// ignored field may have been included for the type declaration
			lt.addLookup(fieldInfo.ResultType)
			continue
		}

		if fieldInfo.Embedded {
			if fieldInfo.Empty {
				continue // we don't need to look up anything in a union
			}
			// Embedding means all the fields are "promoted" to the parent struct
			for j := 0; j < fieldInfo.ResultType.NumField(); j++ {
				tf2 := fieldInfo.ResultType.Field(j)
				fieldInfo2, err2 := field.Get(&tf2)
				if err2 != nil {
					continue // TODO: check error
				}
				if tf2.Name == "_" || fieldInfo2 == nil {
					continue // ignore unexported field
				}
				r[fieldInfo2.Name] = i
				lt.addLookup(fieldInfo2.ResultType)
			}
		} else {
			r[fieldInfo.Name] = i
		}
		lt.addLookup(fieldInfo.ResultType)
	}
	lt[t] = r
}
