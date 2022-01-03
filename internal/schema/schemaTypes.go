package schema

import (
	"fmt"
	"github.com/andrewwphillips/eggql/internal/field"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// schemaTypes stores all the types of the schema indexed by the type name
type schemaTypes map[string]string

// add creates a GraphQL object (type) declaration as a string to be added to the schema and
// adds it to the map (using the type name as the key) avoiding adding the same type twice
// Parameters:
//  name = preferred name for the type or an empty string to use the Go type name from t
//  t = the Go type used to generate the GraphQL type declaration
//  enums = enums map (just used to make sure an enum name is valid)
//  inputType = "type" for a GraphQL object or "input" for an input type
func (s schemaTypes) add(name string, t reflect.Type, enums map[string][]string, inputType string) error {
	// follow indirection(s) and function return(s)
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Func || k == reflect.Array || k == reflect.Slice; k = t.Kind() {
		switch k {
		case reflect.Ptr, reflect.Array, reflect.Slice:
			t = t.Elem() // follow indirection
		case reflect.Func:
			// TODO convert panic (function has no return type) to a returned error
			t = t.Out(0) // get 1st return value (panics if nothing is returned)
		}
	}
	if t.Kind() != reflect.Struct {
		return nil // ignore it if not a struct (this is *not* an error situation)
	}
	if name == "" {
		name = t.Name()
	}
	// Check that a different type with this name is not already in the schema
	if existing, ok := s[name]; ok {
		// We already have processed this type, but we need to check that it is not used to generate
		// both a GraphQL "object" type and an "input" type
		if !strings.HasPrefix(existing, inputType) {
			return fmt.Errorf("same name (%s) used for different types (object, input, interface)", name)
		}
		return nil // don't generate the same type again
	}

	// Get all the resolvers from the exported struct fields
	resolvers, interfaces, err := s.getResolvers(t, enums, inputType)
	if err != nil {
		return err // TODO add more info to error
	}

	// Work out how much string space we need for the resolvers etc
	// AND get a sorted list of resolver keys so resolvers are always written in the same order
	required := len(inputType) + 1 + len(name) + len(openString) + len(closeString)
	if len(interfaces) > 0 {
		required += len(implementsString)
		for _, iface := range resolvers {
			required += 1 + len(iface)
		}
	}
	keys := make([]string, 0, len(resolvers))
	for k, v := range resolvers {
		keys = append(keys, k)
		required += 5 + len(k) + len(v)
	}
	sort.Strings(keys)

	builder := &strings.Builder{}
	builder.Grow(required)

	builder.WriteString(inputType)
	builder.WriteRune(' ')
	builder.WriteString(name)

	// Add interfaces
	if len(interfaces) > 0 {
		builder.WriteString(implementsString)
		for _, iface := range interfaces {
			builder.WriteRune(' ')
			builder.WriteString(iface)
		}
	}

	// Add resolvers in order of (sorted) keys
	builder.WriteString(openString)
	for _, k := range keys {
		builder.WriteString("  ")
		builder.WriteString(k)
		builder.WriteRune(' ')
		builder.WriteString(resolvers[k])
	}
	builder.WriteString(closeString)

	// Check for use of the same name for different objects
	if existing, ok := s[name]; ok {
		if builder.String() != existing {
			// Somehow we have the different objects with the same name
			return fmt.Errorf("same name (%s) used for multiple objects", name)
		}
	}
	s[name] = builder.String()
	if required < len(s[name]) {
		panic("string buffer was not big enough (TODO: remove this)")
	}
	return nil
}

// getResolvers finds all the exported fields (including functions) of a struct and creates resolvers for them.  This
// includes any fields of an embedded (anon) struct which are added as resolvers and also remembered as "interface" names.
// Nested resolvers (named nested structs) are handled by a recursive call to s.add().
// Parameters:
//  t = the struct type containing the fields
//  enums = enums map (just used to make sure an enum name is valid)
//  inputType = "type" for a GraphQL object or "input" for an input type
// Returns:
//  map of resolvers: key is the resolver name; value is the rest of the GraphQL resolver declaration
//  slice of interfaces: names of the interfaces (from embedded anon struct) that the type implements
//  error: non-nil if something went wrong
func (s schemaTypes) getResolvers(t reflect.Type, enums map[string][]string, inputType string) (r map[string]string, iface []string, err error) {
	r = make(map[string]string)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fieldInfo, err2 := field.Get(&f)
		if err2 != nil {
			err = fmt.Errorf("%w getting field %q", err2, f.Name)
			return
		}
		if fieldInfo == nil {
			continue // ignore unexported field
		}
		if fieldInfo.Name != "" && !validGraphQLName(fieldInfo.Name) {
			err = fmt.Errorf("%q is not a valid name", fieldInfo.Name)
			return
		}
		if fieldInfo.Embedded {
			// Handled embedded struct as GraphQL "interface"
			resolvers, interfaces, err2 := s.getResolvers(f.Type, enums, inputType)
			if err2 != nil {
				return nil, nil, err2 // TODO: add more info to err
			}
			for k, v := range resolvers {
				if _, ok := r[k]; ok {
					// Interface filed has same name as normal (or other interface) field
					err = fmt.Errorf("two fields with the same name %q", k)
					return
				}
				r[k] = v
			}
			// TODO: do we need to check for duplicate interface names?
			iface = append(iface, interfaces...)
			iface = append(iface, f.Name)
			// Add the interface type to our collection
			if err = s.add(f.Name, f.Type, enums, gqlInterfaceType); err != nil {
				return // TODO: add more info to err
			}
			continue
		}

		// Get resolver arguments (if any) from the "params" option of the GraphQL tag
		params, err3 := s.getParams(f.Name, f.Type, enums, fieldInfo)
		if err3 != nil {
			err = fmt.Errorf("%w getting params for type %q", err3, fieldInfo.Name)
			return
		}
		// Get the resolver return type
		if fieldInfo.Enum != "" {
			if err = validateEnum(fieldInfo.Enum, enums); err != nil {
				err = fmt.Errorf("type of resolver %q was not found: %w", fieldInfo.Name, err)
				return
			}
		}
		typeName := fieldInfo.Enum
		if typeName == "" {
			typeName, err = getTypeName(f.Type)
			if err != nil {
				err = fmt.Errorf("%w getting name for %q", err, fieldInfo.Name)
				return
			}
		}
		if typeName == "" {
			typeName = f.Name // use field name for anon structs
		}

		endStr := "\n"
		if !fieldInfo.Nullable {
			endStr = "!\n" // add exclamation mark for required (non-nullable) field
		}
		if _, ok := r[fieldInfo.Name]; ok {
			// We already have a field with this name - probably to metadata (field tag) name
			// Note that this will be caught gqlparser.LoadSchema but we may as well signal it earlier
			err = fmt.Errorf("two fields with the same name %q", fieldInfo.Name)
			return
		}
		r[fieldInfo.Name] = params + ":" + typeName + endStr

		// Also add nested struct types (if any) to our collection
		if err = s.add(typeName, f.Type, enums, inputType); err != nil {
			return
		}
	}
	return
}

// validateEnum checks that a type name is a valid enum or enum list
// TODO: allow for and validate any type not just enums
func validateEnum(s string, enums map[string][]string) error {
	// if it's a list get the element type
	if len(s) > 2 && s[0] == '[' && s[len(s)-1] == ']' {
		s = s[1 : len(s)-1]
	}
	// Make sure it's a valid enum type
	if _, ok := enums[s]; !ok {
		return fmt.Errorf("enum %q was not found", s)
	}
	return nil
}

// getParams creates the list of GraphQL parameters for a resolver function
// For struct parameters it also adds the corresponding GraphQL "input" type.
func (s schemaTypes) getParams(name string, t reflect.Type, enums map[string][]string, fieldInfo *field.Info) (string, error) {
	const paramStart, paramSep, paramEnd = "(", ", ", ")"
	for t.Kind() == reflect.Ptr {
		t = t.Elem() // follow indirection
	}
	if t.Kind() != reflect.Func {
		return "", nil
	}

	builder := &strings.Builder{}
	sep := paramStart
	paramNum := 0
	for i := 0; i < t.NumIn(); i++ {
		// Skip 1st param if it's a context
		if i == 0 && fieldInfo.HasContext {
			// We allow for a first context parameter that is not a formal GraphQL parameter
			continue
		}
		if !validGraphQLName(fieldInfo.Params[paramNum]) {
			return "", fmt.Errorf("for %q parameter %d argument %q is not a valid name", name, i, fieldInfo.Params[paramNum])
		}
		builder.WriteString(sep)
		// the next line will panic if not enough arguments were given in "params" part of tag
		builder.WriteString(fieldInfo.Params[paramNum])
		builder.WriteString(": ")

		param := t.In(i)
		if fieldInfo.Enums[paramNum] != "" {
			// Enum or list of enum - check its Go type is integral, and it's a real supplied enum
			isList := false
			kind, enumName := param.Kind(), fieldInfo.Enums[paramNum]
			if kind == reflect.Slice || kind == reflect.Array {
				isList = true
				kind = param.Elem().Kind()
				if len(enumName) < 2 || enumName[0] != '[' || enumName[len(enumName)-1] != ']' {
					panic("Invalid enum list")
				}
				enumName = enumName[1 : len(enumName)-1]
			}
			if kind < reflect.Int || kind > reflect.Uintptr {
				return "", fmt.Errorf("for %q parameter %d (%s) must be an integer for enum %q", name, i, param.Name(), fieldInfo.Enums[paramNum])
			}
			values, ok := enums[enumName]
			if !ok {
				return "", fmt.Errorf("for %q parameter %d (%s) enum %q was not found", name, i, param.Name(), fieldInfo.Enums[paramNum])
			}
			// If there is a default value then check it's in the enum's value list
			if fieldInfo.Defaults[paramNum] != "" {
				ok := false
				if isList {
					// Get list as comma-separated sgtring without enclosing square brackets
					defaults := fieldInfo.Defaults[paramNum]
					if len(defaults) < 2 || defaults[0] != '[' || defaults[len(defaults)-1] != ']' {
						panic("Invalid enum default list:" + defaults)
					}
					defaults = defaults[1 : len(defaults)-1]

					// Split list at commas and check that each value is in the enum list
					ok = true
					for _, dv := range strings.Split(defaults, ",") {
						defaultValue := strings.Trim(dv, " ")
						if defaultValue == "" {
							continue
						}
						found := false
						for _, v := range values {
							if defaultValue == v {
								found = true
								break
							}
						}
						if !found {
							ok = false // this list element is not a valid enum value
							break
						}
					}
				} else {
					for _, v := range values {
						if fieldInfo.Defaults[paramNum] == v {
							ok = true
							break
						}
					}
				}
				if !ok {
					return "", fmt.Errorf("for %q parameter %d default value %q not found in enum %q", name, i, fieldInfo.Defaults[paramNum], fieldInfo.Enums[paramNum])
				}
			}
		} else {
			// For params that are not enum we just need to check any defaults are of the right type
			if fieldInfo.Defaults[paramNum] != "" {
				// Check that the default value is a valid literal for the type
				if !validLiteral(param.Kind(), fieldInfo.Defaults[paramNum]) {
					return "", fmt.Errorf("for %q parameter %d default value %q is not of the correct type", name, i, fieldInfo.Defaults[paramNum])
				}
			}
		}
		typeName := fieldInfo.Enums[paramNum]
		if typeName == "" {
			var err error
			typeName, err = getTypeName(param)
			if err != nil {
				return "", fmt.Errorf("for %q parameter %d (%s) error: %w", name, i, param.Name(), err)
			}
		}
		if typeName == "" {
			// Work out default type name for anon struct by upper-casing the 1st letter of the parameter name
			first, n := utf8.DecodeRuneInString(fieldInfo.Params[paramNum])
			typeName = string(unicode.ToUpper(first)) + fieldInfo.Params[paramNum][n:]
		}
		builder.WriteString(typeName)
		if param.Kind() != reflect.Ptr {
			builder.WriteRune('!')
		}

		// Do we also need to add = followed by the argument default value?
		if fieldInfo.Defaults[paramNum] != "" {
			builder.WriteString(" = ")
			value := fieldInfo.Defaults[paramNum]
			builder.WriteString(value)
		}
		// If it's a struct we also need to add the "input" type to our collection
		if err := s.add(typeName, param, enums, gqlInputType); err != nil {
			return "", fmt.Errorf("%w adding INPUT type %q", err, typeName)
		}

		sep = paramSep
		paramNum++
	}
	if paramNum < len(fieldInfo.Params) {
		return "", fmt.Errorf("expected %d parameters but function %s only has %d",
			len(fieldInfo.Params), name, paramNum)
	}
	if sep != paramStart { // if we got any params
		builder.WriteString(paramEnd)
	}
	return builder.String(), nil
}
