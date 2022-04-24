package schema

// validate.go has functions to help check that schema values are valid

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/andrewwphillips/eggql/internal/field"
)

var nameRegex = regexp.MustCompile(`^[_a-zA-Z][_a-zA-Z0-9]*$`)

// validGraphQLName checks that a string contains a valid GraphQL identifier like a variable,
// argument, or resolver name or an enum name or value.
func validGraphQLName(s string) bool {
	if strings.HasPrefix(s, "__") {
		return false // reserved names
	}
	return nameRegex.MatchString(s)
}

// validLiteral checks that a string is a valid constant for a type - eg only true/false are allowed for Boolean.
//   This is important to check for errors when building the schema rather than panic/client error when a query is run.
func (s schema) validLiteral(typeName string, enums map[string][]string, t reflect.Type, literal string) bool {
	// Get "unmodified" type name - without non-nullable (!) and list ([]) modifiers
	if len(typeName) > 1 && typeName[len(typeName)-1] == '!' {
		typeName = typeName[:len(typeName)-1] // remove non-nullability
	}

	// if it's a list check the elements
	if len(typeName) > 2 && typeName[0] == '[' && typeName[len(typeName)-1] == ']' {
		typeName = typeName[1 : len(typeName)-1]
		t = t.Elem() // TODO check that t is slice/array/map
		if len(typeName) > 1 && typeName[len(typeName)-1] == '!' {
			typeName = typeName[:len(typeName)-1] // remove non-nullability
		}

		// Get the list without square brackets
		if len(literal) < 2 || literal[0] != '[' || literal[len(literal)-1] != ']' {
			return false
		}
		literal = literal[1 : len(literal)-1]

		if literal == "" {
			return true // empty list is valid (we need this because strings.Split given an empty list still returns 1 string)
		}
		// Check that all the values in the list are valid
		valid := true
		for _, dv := range strings.Split(literal, ",") {
			if !s.validLiteral(typeName, enums, t, strings.Trim(dv, " ")) {
				valid = false
				break
			}
		}
		return valid
	}

	// Check for custom scalar
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaler)(nil)).Elem()) {
		if typeName != t.Name() {
			panic("Wrong type")
		}
		return reflect.New(t).Interface().(field.Unmarshaler).UnmarshalEGGQL(literal) == nil

	}

	// if it's an object check each of the fields
	if t.Kind() == reflect.Struct {
		if typeName != t.Name() {
			panic("Wrong type")
		}
		if len(literal) < 2 || literal[0] != '{' || literal[len(literal)-1] != '}' {
			return false
		}
		literal = literal[1 : len(literal)-1]

		// Split the into fields on comma TODO: handle commas inside strings
		for _, f := range strings.Split(literal, ",") {
			// split name:value on colon
			parts := strings.Split(f, ":")
			if len(parts) != 2 || !validGraphQLName(parts[0]) {
				return false
			}

			// Find the matching field in the struct (t)
			var fieldType reflect.Type
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.Name != "_" && f.PkgPath != "" {
					continue // ignore unexported fields
				}
				// make GraphQL name from the Go field name
				first, n := utf8.DecodeRuneInString(f.Name)
				name := string(unicode.ToLower(first)) + f.Name[n:]
				if name == parts[0] {
					fieldType = f.Type
					break
				}
			}
			if fieldType == nil {
				return false // field not found
			}
			fieldTypeName, _, err := s.getTypeName(fieldType)
			if err != nil || !s.validLiteral(fieldTypeName, enums, fieldType, parts[1]) {
				return false
			}
		}
	}

	switch typeName {
	case "Boolean":
		return literal == "true" || literal == "false"
	case "Int":
		_, err := strconv.Atoi(literal)
		return err == nil
	case "Float":
		_, err := strconv.ParseFloat(literal, 64)
		// TODO: check if GraphQL Float allows nan, inf, etc
		return err == nil
	case "String":
		return len(literal) > 1 && literal[0] == '"' && literal[len(literal)-1] == '"'
	case "ID":
		// ID literal can be a string or an integer
		if len(literal) > 1 && literal[0] == '"' && literal[len(literal)-1] == '"' {
			return true // string
		}
		_, err := strconv.Atoi(literal) // check if it's a valid int
		return err == nil
	}

	// For an enum type check that the literal is one of the enum values
	if values, ok := enums[typeName]; ok {
		// Check that the literal is in the list of enum values
		found := false
		for _, v := range values {
			if literal == v {
				found = true
				break
			}
		}
		return found
	}

	return true
}

// validateEnums checks that the enum names are OK and returns the enums without trailing descriptions
// If there is a problem then the 2nd return value (of type error) it not nil.
// If 2nd return value is nil, the 1st return value is the enums map names fixed - ie, anything from the
// 1st hash (#) onwards is removed from all enums names and all values.
func validateEnums(enums map[string][]string) (r map[string][]string, err error) {
	r = make(map[string][]string, len(enums))
	for name, list := range enums {
		name = strings.Split(name, "#")[0] // remove trailing description (if any)
		if !validGraphQLName(name) {
			err = fmt.Errorf("Enum %q is not a valid name", name)
			return
		}
		if len(list) == 0 {
			err = fmt.Errorf("Enum %q has no values", name)
			return
		}
		r[name] = make([]string, 0, len(list))

		inUse := make(map[string]struct{}, len(list)) // for repeated value check
		for _, v := range list {
			v = strings.Split(v, "#")[0]
			if v == "true" || v == "false" || v == "null" { // reserved names
				err = fmt.Errorf("%q is not an allowed enum value (enum %s)", v, name)
				return
			}
			if !validGraphQLName(v) {
				err = fmt.Errorf("%q is not a valid enum value (enum %s)", v, name)
				return
			}
			if _, ok := inUse[v]; ok {
				// We can't allow an enum to have multiple values with the same name
				err = fmt.Errorf("%q is a repeated enum value (enum %s)", v, name)
				return
			}
			inUse[v] = struct{}{}
			r[name] = append(r[name], v)
		}
	}
	return
}
