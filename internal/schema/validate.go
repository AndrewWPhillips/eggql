package schema

// validate.go has functions to help check that schema values are valid

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
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
func validLiteral(kind reflect.Kind, s string) bool {
	switch kind {
	case reflect.Bool:
		return s == "true" || s == "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err := strconv.Atoi(s)
		return err == nil
	case reflect.Float32, reflect.Float64:
		_, err := strconv.ParseFloat(s, 64)
		// TODO: check if GraphQL Float allows nan, inf, etc
		return err == nil
	case reflect.String:
		// return true only is string is enclosed in quotes
		return len(s) > 1 && s[0] == '"' && s[len(s)-1] == '"'
	case reflect.Slice, reflect.Array:
		// TODO handle list
	default:
		// TODO handle any other type?
	}
	return true
}

// validateEnums just checks that each enum name and it's values are OK
func validateEnums(enums map[string][]string) error {
	for name, list := range enums {
		if !validGraphQLName(name) {
			return fmt.Errorf("Enum %q is not a valid name", name)
		}
		if len(list) == 0 {
			return fmt.Errorf("Enum %q has no values", name)
		}

		inUse := make(map[string]struct{}, len(list)) // for repeated value check
		for _, v := range list {
			if v == "true" || v == "false" || v == "null" { // reserved names
				return fmt.Errorf("%q is not an allowed enum value (enum %s)", v, name)
			}
			if !validGraphQLName(v) {
				return fmt.Errorf("%q is not a valid enum value (enum %s)", v, name)
			}
			if _, ok := inUse[v]; ok {
				// We can't allow an enum to have multiple values with the same name
				return fmt.Errorf("%q is a repeated enum value (enum %s)", v, name)
			}
			inUse[v] = struct{}{}
		}
	}
	return nil
}
