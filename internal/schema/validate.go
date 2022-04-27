package schema

// validate.go has functions to help check that schema values are valid

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

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
// Returns: nil if valid or an error explaining why it is invalid
func (s schema) validLiteral(typeName string, enums map[string][]string, t reflect.Type, literal string) error {
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
			return fmt.Errorf("default value %q for a list %q be enclosed in square brackets", literal, typeName)
		}
		literal = literal[1 : len(literal)-1]

		if literal == "" {
			return nil // empty list is valid (we need this because strings.Split given an empty list still returns 1 string)
		}
		// Check that all the values in the list are valid
		for _, dv := range strings.Split(literal, ",") {
			if err := s.validLiteral(typeName, enums, t, strings.Trim(dv, " ")); err != nil {
				return fmt.Errorf("%w: value in %q for list %q is not of correct type", err, literal, typeName)
			}
		}
		return nil
	}

	// Check for custom scalar
	if reflect.TypeOf(reflect.New(t).Interface()).Implements(reflect.TypeOf((*field.Unmarshaler)(nil)).Elem()) {
		if typeName != t.Name() {
			panic("Wrong type")
		}
		if reflect.New(t).Interface().(field.Unmarshaler).UnmarshalEGGQL(literal) != nil {
			return fmt.Errorf("default value %q is not valid for custom scalar %q", literal, typeName)
		}
		return nil
	}

	// if it's an object check each of the fields
	if t.Kind() == reflect.Struct {
		if typeName != t.Name() {
			panic("Wrong type")
		}
		if len(literal) < 2 || literal[0] != '{' || literal[len(literal)-1] != '}' {
			return fmt.Errorf("default value %q for object %q be enclosed in braces {}", literal, typeName)
		}
		literal = literal[1 : len(literal)-1]

		// Split the into fields on comma TODO: handle commas inside strings
		for _, f := range strings.Split(literal, ",") {
			// split name:value on colon
			parts := strings.Split(f, ":")
			if len(parts) != 2 || !validGraphQLName(parts[0]) {
				return fmt.Errorf("default value %q for object %q is malformed", literal, typeName)
			}

			// Find the matching field in the struct (t)
			var fieldType reflect.Type
			var fieldTypeName string
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				fieldInfo, err := field.Get(&f)
				if err != nil {
					return fmt.Errorf("%w getting default value of field %q in object %q", err, parts[0], typeName)
				}
				if f.Name != "_" && f.PkgPath != "" {
					continue // ignore unexported fields
				}
				if fieldInfo.Name != "" && !validGraphQLName(fieldInfo.Name) {
					return fmt.Errorf("%q is not a valid field name in object %q", fieldInfo.Name, typeName)
				}
				if parts[0] == fieldInfo.Name {
					fieldTypeName = fieldInfo.GQLTypeName
					fieldType = f.Type
					break
				}
			}
			if fieldType == nil {
				return fmt.Errorf("%q (in default value %q) is not a field of %q", parts[0], literal, typeName)
			}
			if fieldTypeName == "" {
				var err error
				fieldTypeName, _, err = s.getTypeName(fieldType)
				if err != nil {
					return fmt.Errorf("%w: value in %q for object %q has bad type", err, literal, typeName)
				}
			}
			if err := s.validLiteral(fieldTypeName, enums, fieldType, parts[1]); err != nil {
				return fmt.Errorf("%w: value in %q in object %q is not of correct type", err, literal, typeName)
			}
		}
		return nil // object fields were all OK
	}

	switch typeName {
	case "Boolean":
		if literal != "true" && literal != "false" {
			return fmt.Errorf("%q is not a valid Boolean (must be true or false) for %q", literal, typeName)
		}
		return nil
	case "Int":
		if _, err := strconv.Atoi(literal); err != nil {
			return fmt.Errorf("%w: %q is not a valid Int for %q", err, literal, typeName)
		}
		return nil
	case "Float":
		if _, err := strconv.ParseFloat(literal, 64); err != nil {
			return fmt.Errorf("%w: %q is not a valid Float for %q", err, literal, typeName)
		}
		// TODO: check if GraphQL Float allows nan, inf, etc
		return nil
	case "String":
		if len(literal) < 2 || literal[0] == '"' || literal[len(literal)-1] == '"' {
			return fmt.Errorf("<%s> is not a valid String (must be in double-quotes) for %q", literal, typeName)

		}
		return nil
	case "ID":
		// ID literal can be a string or an integer
		if len(literal) > 1 && literal[0] == '"' && literal[len(literal)-1] == '"' {
			return nil // string
		}
		if _, err := strconv.Atoi(literal); err != nil {
			return fmt.Errorf("%w: %q is not a valid ID (must be integer or string) for %q", err, literal, typeName)
		}
		return nil
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
		if !found {
			return fmt.Errorf("%q is not a valid enum value for for %q", literal, typeName)
		}
		return nil // good enum value
	}

	return nil // assume it's OK if we get here (TODO: check if we need to check more types)
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
