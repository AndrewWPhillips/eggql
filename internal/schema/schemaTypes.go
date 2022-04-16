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

type (
	// schema stores all the types of the schema accumulated so far
	schema struct {
		declaration map[string]string       // stores the text declaration of all types generated
		description map[string]string       // corresponding description of the types
		usedAs      map[reflect.Type]string // tracks which types (structs) we have seen (mainly to handle recursive data structures)
		unions      map[string]union        // key is union name
		scalars     *[]string               // names of custom scalar types (implement MarshalEGGQL/UnmarshalEGGQL)
	}

	// union contains details used to generate one GraphQL union
	union struct {
		desc    string
		objects []string // names of all GraphQL object types involved in the union
	}
)

// newSchemaTypes initialises an instance of the schemaTypes (by making the maps)
func newSchemaTypes() schema {
	return schema{
		declaration: make(map[string]string),
		description: make(map[string]string),
		usedAs:      make(map[reflect.Type]string),
		unions:      make(map[string]union),
		scalars:     &[]string{},
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
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Func || k == reflect.Map || k == reflect.Slice || k == reflect.Array; k = t.Kind() {
		switch k {
		case reflect.Ptr:
			t = t.Elem() // follow indirection
		case reflect.Map, reflect.Slice, reflect.Array:
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
		if previousType == gqlObjectTypeKeyword && gqlType == gqlInterfaceKeyword {
			// switch type of declaration from "type" to "interface"
			s.usedAs[t] = gqlInterfaceKeyword
			if decl, ok := s.declaration[name]; ok {
				s.declaration[name] = gqlInterfaceKeyword + strings.TrimPrefix(decl, gqlObjectTypeKeyword)
			}
		} else if previousType == gqlInterfaceKeyword && gqlType == gqlObjectTypeKeyword {
			// nothing required here
		} else if previousType != gqlType {
			return fmt.Errorf("can't use %q for different GraphQL types (%s and %s)", name, previousType, gqlType)
		}
		return nil // already done
	}
	s.usedAs[t] = gqlType

	// Get all the resolvers from the exported struct fields
	resolvers, interfaces, desc, err := s.getResolvers(name, t, enums, gqlType)
	if err != nil {
		return fmt.Errorf("%w getting resolvers for %q", err, name)
	}

	// Work out how much string space we need for the resolvers etc.
	required := len(gqlType) + 1 + len(name) + len(openString) + len(closeString)
	if len(interfaces) > 0 {
		required += len(implementsString) + (len(interfaces)-1)*2 // keyword + separator ( &)
		for _, iface := range interfaces {
			required += 1 + len(iface) // name of interface + 1 space
		}
	}
	// Get space for resolvers AND get a sorted list of resolver keys so resolvers are always written in the same order
	keys := make([]string, 0, len(resolvers))
	for k, v := range resolvers {
		keys = append(keys, k)
		required += len(v)
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
	s.description[name] = desc
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
//  text to be added (to the GraphQL schema) as a "description" of the type
//  error: non-nil if something went wrong
func (s schema) getResolvers(parentType string, t reflect.Type, enums map[string][]string, gqlType string) (r map[string]string, iface []string, desc string, err error) {
	r = make(map[string]string)

	// First get type info from all dummy fields - those with blank ID (_) as their name
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Name == "_" {
			if f.Type.Name() == "TagField" { // name must match the type declared in run.go
				// this field (zero size) is just included to allow us to get the description from the field tag
				fieldInfo, err2 := field.Get(&f)
				if err2 != nil {
					err = fmt.Errorf("%w getting decription from TagField", err2)
					return
				}
				desc = fieldInfo.Description
			} else {
				// This field is just included for its type so that eggql know about it (currently just used for implementing GraphQL interfaces)
				if err = s.add("", f.Type, enums, gqlObjectTypeKeyword); err != nil {
					return
				}
				// if proposal to allow scalars to implement interfaces goes ahead we may need to call s.getTypeName(f.Type) here
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
		if f.Name == "_" || fieldInfo == nil {
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
			// Add parent type to union f.Name
			u := s.unions[f.Name]
			u.objects = append(u.objects, parentType)
			// Check for any "description" tag field in the union
			for j := 0; j < f.Type.NumField(); j++ {
				f2 := f.Type.Field(j)
				fieldInfo2, err2 := field.Get(&f2) // just call this to get description for union
				if (u.desc != "" && u.desc != fieldInfo2.Description) || err2 != nil {
					return nil, nil, "", errors.New("Error in union description for " + f2.Name)
				}
				u.desc = fieldInfo2.Description
			}
			s.unions[f.Name] = u
			continue // embedding empty struct just signals a "union" so don't add a resolver for this
		} else if fieldInfo.Embedded {
			// Add struct to our collection as an "interface"
			if err2 = s.add(fieldInfo.GQLTypeName, f.Type, enums, gqlInterfaceKeyword); err2 != nil {
				err = fmt.Errorf("%w adding embedded (interface) type %q", err2, f.Name)
				return
			}

			// Get the resolvers from the embedded struct (GraphQL "interface")
			resolvers, interfaces, _, err2 := s.getResolvers(parentType, f.Type, enums, gqlType)
			if err2 != nil {
				// We should't ever get to here - getResolvers for this struct has already been called w/o error in above s.add() method call
				//err = fmt.Errorf("%w adding embedded resolvers for %q", err2, f.Name); return
				return nil, nil, "", err2
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

		// Use resolver return type from the tag (if any) and assume it's not a scalar
		typeName, isScalar := fieldInfo.GQLTypeName, false
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

		var resolverDesc string
		if fieldInfo.Description != "" {
			resolverDesc = `  """` + fieldInfo.Description + `"""` + "\n"
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
		// Get type name derived from Go type
		if typeName == "" {
			// Get resolver return type
			typeName, isScalar, err2 = s.getTypeName(effectiveType)
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
		r[fieldInfo.Name] = resolverDesc + "  " + fieldInfo.Name + " " + params + ":" + typeName + endStr

		if !isScalar {
			// We need to determine the type of the nested object (type/input/interface), normally it's the same as the
			// parent (eg nested types in "input" are also "input") but a type inside an "interface" is an object ("type")
			nestedType := gqlType
			if nestedType == gqlInterfaceKeyword {
				nestedType = gqlObjectTypeKeyword // a field inside an embedded struct is not itself treated as an interface
			}
			if err = s.add(typeName, effectiveType, enums, nestedType); err != nil {
				return
			}
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
	typeName, isScalar, err := s.getTypeName(fieldInfo.SubscriptType)
	if err != nil {
		return "", fmt.Errorf("%w getting subscript type for %q", err, fieldInfo.Name)
	}
	if !isScalar {
		// TODO check if this restriction is necessary
		return "", fmt.Errorf("you can't use an object type (%s) as a subscript", fieldInfo.Name)
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
		if !validGraphQLName(fieldInfo.Args[paramNum]) {
			return "", fmt.Errorf("parameter %d argument %q is not a valid name", i, fieldInfo.Args[paramNum])
		}
		builder.WriteString(sep)
		if fieldInfo.DescArgs[paramNum] != "" {
			builder.WriteString(`"""`)
			builder.WriteString(fieldInfo.DescArgs[paramNum])
			builder.WriteString(`"""`)
		}
		builder.WriteString(fieldInfo.Args[paramNum])
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
		// Get type name supplied in the tag (used for enums etc)
		typeName, isScalar := fieldInfo.Enums[paramNum], false

		// If not found use the Go type (eg Go int => Int! etc) of custom scalar type
		if typeName == "" {
			var err error
			typeName, isScalar, err = s.getTypeName(param)
			if err != nil {
				return "", fmt.Errorf("parameter %d (%s) error: %w", i, param.Name(), err)
			}
		}
		// If still not found (eg inline struct literal) use the field name to generate a type name
		if typeName == "" {
			// Work out default type name for anon struct by upper-casing the 1st letter of the parameter name
			first, n := utf8.DecodeRuneInString(fieldInfo.Args[paramNum])
			typeName = string(unicode.ToUpper(first)) + fieldInfo.Args[paramNum][n:]
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
		if !isScalar {
			// If it's a struct we also need to add the "input" type to our collection
			if err := s.add(typeName, param, enums, gqlInputKeyword); err != nil {
				return "", fmt.Errorf("%w adding INPUT type %q", err, typeName)
			}
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
