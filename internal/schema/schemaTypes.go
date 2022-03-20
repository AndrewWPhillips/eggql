package schema

// schemaTypes.go contains the schema Type which accumulates all the GraphQL type to be added to the schema

import (
	"errors"
	"fmt"
	"github.com/andrewwphillips/eggql/internal/field"
	"log"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// schema stores all the types of the schema accumulated so far
type schema struct {
	declaration map[string]string       // store the text declaration of all types generated
	usedAs      map[reflect.Type]string // tracks which types (structs) we have seen (mainly to handle recursive data structures)
	unions      map[string][]string     // key is union name, value are types involved in the union
}

// newSchemaTypes initialises an instance of the schemaTypes (by making the maps)
func newSchemaTypes() schema {
	return schema{
		declaration: make(map[string]string),
		usedAs:      make(map[reflect.Type]string),
		unions:      make(map[string][]string),
	}
}

// add creates a GraphQL object/input/interface declaration as a string to be added to the schema and
// adds it to the map (using the type name as the key) avoiding adding the same type twice
// Parameters:
//  name = preferred name for the type (if an empty string the Go type name is used)
//  t = the Go type used to generate the GraphQL type declaration
//  enums = enums map (just used to make sure an enum name is valid)
//  gqlType = "type" for a GraphQL object or "input", "interface", etc
// Returns an error if the type could not be added - currently this only happens if the same struct is
//  used as an "input" type (ie resolver parameter) and as an "object" or "interface" type
func (s schema) add(name string, t reflect.Type, enums map[string][]string, gqlType string) error {
	needName := name == ""
	if needName {
		name = t.Name()
	}
	// follow indirection(s) and function return(s)
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Func || k == reflect.Array || k == reflect.Slice; k = t.Kind() {
		switch k {
		case reflect.Ptr:
			t = t.Elem() // follow indirection
		case reflect.Array, reflect.Slice:
			if !needName {
				// Get the element type name from within the square brackets
				if len(name) < 2 || name[0] != '[' || name[len(name)-1] != ']' {
					panic("Type name for list should be in square brackets")
				}
				name = name[1 : len(name)-1]
			}

			t = t.Elem() // element type
		case reflect.Func:
			// TODO convert panic (function has no return type) to a returned error
			t = t.Out(0) // get 1st return value (panics if nothing is returned)
		}
		if needName {
			name = t.Name()
		}
	}
	if t.Kind() != reflect.Struct {
		return nil // ignore it if not a struct (this is *not* an error situation)
	}

	// Check if we have already seen this struct
	if previousType, ok := s.usedAs[t]; ok {
		if previousType == gqlObjectType && gqlType == gqlInterfaceKeyword {
			// switch type of declaration from "type" to "interface"
			s.usedAs[t] = gqlInterfaceKeyword
			if decl, ok := s.declaration[name]; ok {
				s.declaration[name] = gqlInterfaceKeyword + strings.TrimPrefix(decl, gqlObjectType)
			}
		} else if previousType == gqlInterfaceKeyword && gqlType == gqlObjectType {
			// nothing required here
		} else if previousType != gqlType {
			return fmt.Errorf("can't use %q for different GraphQL types (%s and %s)", name, previousType, gqlType)
		}
		return nil // already done
	}
	s.usedAs[t] = gqlType

	// Get all the resolvers from the exported struct fields
	resolvers, interfaces, err := s.getResolvers(name, t, enums, gqlType)
	if err != nil {
		return err // TODO add more info to error
	}

	// Work out how much string space we need for the resolvers etc.
	// AND get a sorted list of resolver keys so resolvers are always written in the same order
	required := len(gqlType) + 1 + len(name) + len(openString) + len(closeString)
	if len(interfaces) > 0 {
		required += len(implementsString) + (len(interfaces)-1)*2 // keyword + separator ( &)
		for _, iface := range interfaces {
			required += 1 + len(iface) // name of interface + 1 space
		}
	}
	keys := make([]string, 0, len(resolvers))
	for k, v := range resolvers {
		keys = append(keys, k)
		required += 3 + len(k) + len(v)
	}
	sort.Strings(keys)

	builder := &strings.Builder{}
	builder.Grow(required)

	builder.WriteString(gqlType)
	builder.WriteRune(' ')
	builder.WriteString(name)

	// Add interfaces
	if len(interfaces) > 0 {
		sep := implementsString + " "
		for _, iface := range interfaces {
			builder.WriteString(sep)
			sep = " & "
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
	if existing, ok := s.declaration[name]; ok {
		if builder.String() != existing {
			// Somehow we have the different objects with the same name
			return fmt.Errorf("same name (%s) used for multiple objects", name)
		}
	}
	s.declaration[name] = builder.String()
	actual := len(s.declaration[name])
	if required != actual {
		log.Fatalln("string buffer size was incorrect (TODO: remove this)", required, actual)
	}
	return nil
}

// getResolvers finds all the exported fields (including functions) of a struct and creates resolvers for them.  This
// includes any fields of an embedded (anon) struct which are added as resolvers and also remembered as "interface" names.
// Nested resolvers (named nested structs) are handled by a recursive call to s.add().
// Parameters:
//  parentType = name of the struct type
//  t = the struct type containing the fields
//  enums = enums map (just used to make sure an enum name is valid)
//  inputType = "type" for a GraphQL object or "input" for an input type
// Returns:
//  map of resolvers: key is the resolver name; value is the rest of the GraphQL resolver declaration
//  names of GraphQL interface(s) that the type implements (using Go embedded structs)
//  error: non-nil if something went wrong
func (s schema) getResolvers(parentType string, t reflect.Type, enums map[string][]string, gqlType string) (r map[string]string, iface []string, err error) {
	r = make(map[string]string)

	// First get type info from all dummy fields - those with blank ID (_) as their name
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Name == "_" {
			// A struct with name "_" is just included for its type (for implementing GraphQL interfaces)
			if err = s.add("", f.Type, enums, gqlObjectType); err != nil {
				return
			}
		}
	}
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
		if _, isEnum := enums[fieldInfo.GQLTypeName]; isEnum {
			// For enums the resolver must have a Go integer type
			if fieldInfo.ResultType.Kind() < reflect.Int || fieldInfo.ResultType.Kind() > reflect.Uintptr {
				err = errors.New("resolver with enum type must be an integer field " + f.Name)
				return
			}
		}
		if fieldInfo.Embedded && fieldInfo.Empty {
			// Add parent type to "union"
			s.unions[f.Name] = append(s.unions[f.Name], parentType)
			continue // embedding empty struct just signals a "union" so don't add a resolver for this
		} else if fieldInfo.Embedded {
			// Add struct to our collection as an "interface"
			if err2 = s.add(fieldInfo.GQLTypeName, f.Type, enums, gqlInterfaceKeyword); err2 != nil {
				err = fmt.Errorf("%w adding embedded (interface) type %q", err2, f.Name)
				return
			}

			// Get the resolvers from the embedded struct (GraphQL "interface")
			resolvers, interfaces, err2 := s.getResolvers(parentType, f.Type, enums, gqlType)
			if err2 != nil {
				// We can't get here - getResolvers for this struct has already been called w/o error in above s.add() method call
				//err = fmt.Errorf("%w adding embedded resolvers for %q", err2, f.Name); return
				return nil, nil, err2
			}
			for k, v := range resolvers {
				if _, ok := r[k]; ok {
					// Interface field has the same name as normal (or other interface) field
					err = fmt.Errorf("two fields with the same name %q", k)
					return
				}
				r[k] = v
			}
			iface = append(iface, interfaces...)
			iface = append(iface, f.Name)
			continue // all resolvers for the "interface" have been added
		}

		// Use the specified resolver return type
		typeName := fieldInfo.GQLTypeName
		if typeName != "" {
			// Ensure the name given is valid
			if err2 = s.validateTypeName(fieldInfo.GQLTypeName, enums); err2 != nil {
				var help string
				if strings.HasPrefix(fieldInfo.GQLTypeName, "[]") { // probably used []Type when [Type] was meant
					help = fmt.Sprintf("(did you mean %s)", "["+fieldInfo.GQLTypeName[2:]+"]")
				}
				err = fmt.Errorf("resolver type (%s) of field %q not recognized: %w %s", fieldInfo.GQLTypeName, fieldInfo.Name, err2, help)
				return
			}
		}

		var params string
		var effectiveType reflect.Type
		if fieldInfo.Subscript != "" {
			// Get the resolver arg (subscript) - eg "(id:Int!)"
			params, err2 = s.getSubscript(fieldInfo)
			if err2 != nil {
				err = fmt.Errorf("%w subscript for %q", err2, fieldInfo.Name)
				return
			}
			effectiveType = fieldInfo.ResultType
		} else {
			// Get resolver arguments (if any) from the "args" option - eg "(p1:String!, p2:Int!=42)"
			params, err2 = s.getParams(f.Type, enums, fieldInfo)
			if err2 != nil {
				err = fmt.Errorf("%w getting args for %q", err2, fieldInfo.Name)
				return
			}
			effectiveType = f.Type
		}
		if typeName == "" {
			// Get resolver return type
			typeName, err2 = getTypeName(effectiveType)
			if err2 != nil {
				err = fmt.Errorf("%w getting name for %q", err2, fieldInfo.Name)
				return
			}
		}

		if typeName == "" { // TODO: check if this is always correct thing to do
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
		nestedType := gqlType
		if nestedType == gqlInterfaceKeyword {
			nestedType = gqlObjectType // a field inside an embedded struct is not itself treated as an interface
		}
		if err = s.add(typeName, effectiveType, enums, nestedType); err != nil {
			return
		}
	}
	return
}

// validateTypeName checks that a type name is a valid enum or object type
func (s schema) validateTypeName(typeName string, enums map[string][]string) error {
	// if it's a list get the element type
	if len(typeName) > 2 && typeName[0] == '[' && typeName[len(typeName)-1] == ']' {
		typeName = typeName[1 : len(typeName)-1]
	}
	// First check if it's an enum type
	if _, ok := enums[typeName]; ok {
		return nil
	}
	// Check if it's an object type seen already
	if _, ok := s.declaration[typeName]; ok {
		return nil
	}
	// Check if it's a union
	if _, ok := s.unions[typeName]; ok {
		return nil
	}
	// TODO check scalar types?

	return fmt.Errorf("type %q was not found", typeName)
}

const paramStart, paramSep, paramEnd = "(", ", ", ")"

// getSubscript creates the arg list (just one arg) for "subscript" option on a slice/array/map
func (s schema) getSubscript(fieldInfo *field.Info) (string, error) {
	typeName, err := getTypeName(fieldInfo.SubscriptType)
	if err != nil {
		return "", fmt.Errorf("%w getting subscript type for %q", err, fieldInfo.Name)
	}
	return fmt.Sprintf("(%s: %s!)", fieldInfo.Subscript, typeName), nil
}

// getParams creates the list of GraphQL arguments for a resolver function
// If any arg uses a Go struct then it also adds the corresponding GraphQL "input" type to the schemaTypes collection
func (s schema) getParams(t reflect.Type, enums map[string][]string, fieldInfo *field.Info) (string, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem() // follow indirection
	}
	if t.Kind() != reflect.Func {
		return "", nil
	}

	builder := &strings.Builder{}
	sep := paramStart
	paramNum := 0
	var contextSeen bool
	for i := 0; i < t.NumIn(); i++ {
		// Skip 1st param if it's a context
		if !contextSeen && fieldInfo.HasContext {
			// contextContext parameter is not a formal GraphQL parameter
			contextSeen = true
			continue
		}
		if !validGraphQLName(fieldInfo.Params[paramNum]) {
			return "", fmt.Errorf("parameter %d argument %q is not a valid name", i, fieldInfo.Params[paramNum])
		}
		builder.WriteString(sep)
		// the next line will panic if not enough arguments were given in "args" part of tag
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
				return "", fmt.Errorf("parameter %d (%s) must be an integer for enum %q", i, param.Name(), fieldInfo.Enums[paramNum])
			}
			values, ok := enums[enumName]
			if !ok {
				return "", fmt.Errorf("parameter %d (%s) enum %q was not found", i, param.Name(), fieldInfo.Enums[paramNum])
			}
			// If there is a default value then check it's in the enum's value list
			if fieldInfo.Defaults[paramNum] != "" {
				ok := false
				if isList {
					// Get list as comma-separated string without enclosing square brackets
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
					return "", fmt.Errorf("parameter %d default value %q not found in enum %q", i, fieldInfo.Defaults[paramNum], fieldInfo.Enums[paramNum])
				}
			}
		} else {
			// For args that are not enum we just need to check any defaults are of the right type
			if fieldInfo.Defaults[paramNum] != "" {
				// Check that the default value is a valid literal for the type
				if !validLiteral(param.Kind(), fieldInfo.Defaults[paramNum]) {
					return "", fmt.Errorf("parameter %d default value %q is not of the correct type", i, fieldInfo.Defaults[paramNum])
				}
			}
		}
		typeName := fieldInfo.Enums[paramNum]
		if typeName == "" {
			var err error
			typeName, err = getTypeName(param)
			if err != nil {
				return "", fmt.Errorf("parameter %d (%s) error: %w", i, param.Name(), err)
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
		return "", fmt.Errorf("not enough args (%d) expected %d", paramNum, len(fieldInfo.Params))
	}
	if sep != paramStart { // if we got any args
		builder.WriteString(paramEnd)
	}
	return builder.String(), nil
}
