package schema

// schemaTypes.go contains the schema Type which accumulates all the GraphQL type to be added to the schema

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/andrewwphillips/eggql/internal/field"
)

type (
	// schema stores all the types of the schema accumulated so far
	schema struct {
		declaration map[string]string       // stores the text declaration of all types generated
		description map[string]string       // corresponding description of the types
		idFieldName map[string]string       // if this object is stored in a list this is the name of a fabricated id field
		usedAs      map[reflect.Type]string // tracks which types (structs) we have seen and their GraphQL "type" (type/input/interface) - this is mainly to handle recursive data structures
		unions      map[string]union        // key is union name
		scalars     *[]string               // names of custom scalar types (implement MarshalEGGQL/UnmarshalEGGQL)
	}

	// objectField stores info on one field to be added to a GraphQL object
	objectField struct {
		name string
		typ  reflect.Type
	}

	// union contains details used to generate one GraphQL union
	union struct {
		desc    string
		objects map[string]struct{} // map keys = names of all GraphQL object types involved in the union
	}
)

// newSchemaTypes initialises an instance of the schemaTypes (by making the maps)
func newSchemaTypes() schema {
	return schema{
		declaration: make(map[string]string),
		description: make(map[string]string),
		idFieldName: make(map[string]string),
		usedAs:      make(map[reflect.Type]string),
		unions:      make(map[string]union),
		scalars:     &[]string{},
	}
}

// add creates a GraphQL object/input/interface as a string to be added to the schema and adds it
//   to the declaration map (using the type name as the key) avoiding adding the same type twice.
// Parameters:
//   name = preferred name for the type (if an empty string the Go type name is used)
//   t = the Go type used to generate the GraphQL type declaration
//   enums = enums map (just used to make sure an enum name is valid)
//   gqlType = "type" (for a GraphQL object), "input", "interface", etc
//   idField = info for "id" field to be added to an object (or nil if not in a list)
// Returns an error if the type could not be added - this may happen if the same struct is
//   used as an "input" type (ie resolver parameter) and as an "object" or "interface" type or
//   there is an error with the field declarations
func (s schema) add(name string, t reflect.Type, enums map[string][]string, gqlType string, idField *objectField,
) error {
	needName := name == ""
	if needName {
		name = t.Name()
	} else if name[len(name)-1] == '!' {
		name = name[:len(name)-1]
	}
	// TODO check if (effective type) of t can ever be func at this point - remove reflect.Func from loop/switch below?
	// follow indirection(s) and function return(s)
	for k := t.Kind(); k == reflect.Ptr || k == reflect.Func || k == reflect.Map || k == reflect.Slice || k == reflect.Array; k = t.Kind() {
		switch k {
		case reflect.Ptr:
			t = t.Elem() // follow indirection
		case reflect.Map, reflect.Slice, reflect.Array:
			if !needName {
				// Get the element type name from within the square brackets
				if len(name) < 2 || name[0] != '[' && name[len(name)-1] != ']' {
					panic("List type name should be in square brackets")
				}
				name = name[1 : len(name)-1] // remove sq. brackets
				if name[len(name)-1] == '!' {
					name = name[:len(name)-1]
				}
			}

			t = t.Elem() // element type
		case reflect.Func:
			if t.NumOut() == 0 {
				panic("Resolver func must have at least one return value")
			}
			t = t.Out(0) // get 1st return value (panics if nothing is returned)
		}
		if needName {
			name = t.Name()
		}
	}
	if t.Kind() != reflect.Struct {
		return nil // ignore it if not a struct (this is *not* an error situation)
	}

	var force bool // Do we need to force regeneration due to field_id not being present in the previous declaration

	// if idField has already been used for this struct then check it used the same name (otherwise generated schema is inconsistent)
	if idField != nil {
		if previous, ok := s.idFieldName[name]; ok {
			if previous != idField.name {
				// id_field used in previous declaration has a different name
				return fmt.Errorf("id_field on %q must have the same name (not %q and %q)", name, previous, idField.name)
			}
			// no need to force regeneration as it will be the same
		} else {
			force = true // force regeneration so we also get the fabricated "id" field
		}
		s.idFieldName[name] = idField.name
	}

	// Check if we have already seen this struct so we don't need to regenerate it
	if previousType, ok := s.usedAs[t]; ok {

		// Already seen but check that we are not using it in an incompatible way
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
		if !force {
			return nil // we already have the correct declaration
		}
		delete(s.declaration, name) // remove it, to be regenerated
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
	var idTypeName string
	if idField != nil {
		// check that none of the resolver names is the same as id field name
		if _, ok := resolvers[idField.name]; ok {
			return fmt.Errorf("field %q can't have the same name as 'field_id' of %q", idField.name, name)
		}

		idTypeName, _, err = s.getTypeName(idField.typ, false)
		if err != nil {
			return fmt.Errorf("%w getting type for ID field %q in %q", err, idField.name, name)
		}
		required += 4 + len(idField.name) + len(idTypeName)
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

	builder.WriteString(openString)
	if idField != nil {
		// Add fabricated ID field
		builder.WriteString("  ")
		builder.WriteString(idField.name)
		builder.WriteRune(':')
		builder.WriteString(idTypeName)
		builder.WriteRune('\n')
	}
	// Add resolvers in order of (sorted) keys
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
		log.Fatalln("string buffer size was incorrect", required, actual)
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
func (s schema) getResolvers(parentType string, t reflect.Type, enums map[string][]string, gqlType string,
) (r map[string]string, iface []string, desc string, err error) {
	r = make(map[string]string)

	// First get type info from all dummy fields - those with blank ID (_) as their name
	for i := 0; i < t.NumField(); i++ {
		tf := t.Field(i)
		if tf.Name == "_" {
			if tf.Type.Name() == "TagHolder" { // name must match the type declared in run.go
				// the field (otherwise not used) is just included to allow us to get the description from the field tag
				fieldInfo, err2 := field.Get(&tf)
				if err2 != nil {
					err = fmt.Errorf("%w getting decription from TagHolder", err2)
					return
				}
				desc = fieldInfo.Description
			} else {
				// This field is just included for its type so that eggql knows about it (this is used in implementing GraphQL interfaces)
				if err = s.add("", tf.Type, enums, gqlObjectTypeKeyword, nil); err != nil {
					return
				}
				// if GraphQL proposal to allow scalars to implement interfaces goes ahead we may need to call s.getTypeName(f.Type) here
			}
		}
	}
	for i := 0; i < t.NumField(); i++ {
		tf := t.Field(i)
		fieldInfo, err2 := field.Get(&tf)
		if err2 != nil {
			err = fmt.Errorf("%w getting field %q", err2, tf.Name)
			return
		}
		if tf.Name == "_" || fieldInfo == nil {
			continue // ignore unexported field
		}
		if fieldInfo.Name != "" && !validGraphQLName(fieldInfo.Name) {
			err = fmt.Errorf("%q is not a valid name", fieldInfo.Name)
			return
		}
		if fieldInfo.Embedded && fieldInfo.Empty {
			// Add parent type to union f.Name
			u := s.unions[tf.Name]
			if u.objects == nil {
				u.objects = make(map[string]struct{})
			}
			u.objects[parentType] = struct{}{} // add to the set of objects in the union

			// Check for any "description" tag field in the union
			for j := 0; j < tf.Type.NumField(); j++ {
				tf2 := tf.Type.Field(j)
				fieldInfo2, err3 := field.Get(&tf2) // just call this to get description for union
				if (u.desc != "" && u.desc != fieldInfo2.Description) || err3 != nil {
					// we should not get here - panic?
					return nil, nil, "", errors.New("Error in union description for " + tf2.Name)
				}
				u.desc = fieldInfo2.Description
			}
			s.unions[tf.Name] = u
			continue // embedding empty struct just signals a "union" so don't add a resolver for this
		} else if fieldInfo.Embedded {
			// Add struct to our collection as an "interface"
			if err2 = s.add(fieldInfo.GQLTypeName, tf.Type, enums, gqlInterfaceKeyword, nil); err2 != nil {
				err = fmt.Errorf("%w adding embedded (interface) type %q", err2, tf.Name)
				return
			}

			// Get the resolvers from the embedded struct (GraphQL "interface")
			resolvers, interfaces, _, err2 := s.getResolvers(parentType, tf.Type, enums, gqlType)
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
			iface = append(iface, tf.Name)
			continue // all resolvers for the "interface" have been added
		}

		// Get any description text to add to the schema
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
		} else if tf.Type.Kind() == reflect.Func {
			// Get resolver arguments (if any) from the "args" option - eg "(p1:String!, p2:Int!=42)"
			params, err2 = s.getParams(tf.Type, enums, fieldInfo)
			if err2 != nil {
				err = fmt.Errorf("%w getting args for %q", err2, fieldInfo.Name)
				return
			}
			if tf.Type.NumOut() == 0 {
				// should not get to here - panic?
				err = fmt.Errorf("resolver function %q does not return a value", fieldInfo.Name)
				return
			}
			effectiveType = tf.Type.Out(0)
			if fieldInfo.IsChan {
				effectiveType = effectiveType.Elem() // subscriptions are always channels
			}
		} else if tf.Type.Kind() == reflect.Chan {
			effectiveType = tf.Type.Elem()
		} else {
			effectiveType = tf.Type
		}

		var idField *objectField
		if fieldInfo.FieldID != "" {
			idField = &objectField{name: fieldInfo.FieldID, typ: fieldInfo.ElementType}
		}

		// Use resolver return type from the tag (if any) and assume it's not a scalar
		typeName, isScalar := fieldInfo.GQLTypeName, false
		if typeName != "" {
			// Ensure the name given is valid
			if isScalar, err2 = s.validateTypeName(typeName, enums, effectiveType); err2 != nil {
				var help string
				if strings.HasPrefix(fieldInfo.GQLTypeName, "[]") { // probably used []Type when [Type] was meant
					help = fmt.Sprintf("(did you mean %s)", "["+fieldInfo.GQLTypeName[2:]+"]")
				}
				err = fmt.Errorf("%w: resolver type (%s) of field %q: %s", err2, fieldInfo.GQLTypeName, fieldInfo.Name, help)
				return
			}
		}

		if typeName == "" {
			// Derive GraphQL type from the field type
			typeName, isScalar, err2 = s.getTypeName(effectiveType, fieldInfo.Nullable)
			if err2 != nil {
				err = fmt.Errorf("%w getting name for %q", err2, fieldInfo.Name)
				return
			}
		}

		if typeName == "" { // TODO: check if this is always correct thing to do
			typeName = tf.Name // use field name for anon structs
			if !fieldInfo.Nullable {
				typeName += "!"
			}
		}

		if _, ok := r[fieldInfo.Name]; ok {
			// We already have a field with this name - probably due to metadata (field tag) name
			// Note that this will be caught gqlparser.LoadSchema but we may as well signal it earlier
			err = fmt.Errorf("two fields with the same name %q", fieldInfo.Name)
			return
		}
		r[fieldInfo.Name] = resolverDesc + "  " + fieldInfo.Name + " " + params + ":" + typeName +
			" " + strings.Join(fieldInfo.Directives, " ") + "\n"

		if !isScalar {
			// Determine the "type" keyword for the nested object (type/input/interface).
			// Normally, it's the same as the parent (eg nested types in "input" are also "input") but an
			// object inside an "interface" is an object (ie GraphQL "type" keyword) not an "interface"
			nestedType := gqlType
			if nestedType == gqlInterfaceKeyword {
				nestedType = gqlObjectTypeKeyword // a field inside an embedded struct is not itself treated as an interface
			}
			if err = s.add(typeName, effectiveType, enums, nestedType, idField); err != nil {
				return
			}
		}
	}
	return
}

const paramStart, paramSep, paramEnd = "(", ", ", ")"

// getSubscript creates the arg list (just one arg) for "subscript" option on a slice/array/map
func (s schema) getSubscript(fieldInfo *field.Info) (string, error) {
	typeName, isScalar, err := s.getTypeName(fieldInfo.ElementType, false)
	if err != nil {
		return "", fmt.Errorf("%w getting subscript type for %q", err, fieldInfo.Name)
	}
	if !isScalar {
		// TODO check if this restriction is necessary
		return "", fmt.Errorf("you can't use an object type (%s) as a subscript", fieldInfo.Name)
	}
	return fmt.Sprintf("(%s: %s)", fieldInfo.Subscript, typeName), nil
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
		var err error

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
		if fieldInfo.ArgDescriptions[paramNum] != "" {
			builder.WriteString(`"""`)
			builder.WriteString(fieldInfo.ArgDescriptions[paramNum])
			builder.WriteString(`"""`)
		}
		builder.WriteString(fieldInfo.Args[paramNum])
		builder.WriteString(": ")

		effectiveType := t.In(i)
		// Get type name supplied in the tag (essential for ID, enums)
		typeName, isScalar := fieldInfo.ArgTypes[paramNum], false
		if typeName != "" {
			// Ensure the name given is valid TODO also need to return isScalar
			if isScalar, err = s.validateTypeName(typeName, enums, effectiveType); err != nil {
				return "", fmt.Errorf("type (%s) of arg %q not found: %w", typeName, fieldInfo.Args[paramNum], err)
			}
		}
		// No GraphQL type name supplied in the args so derive it from the Go function parameter's type
		if typeName == "" {
			if typeName, isScalar, err = s.getTypeName(effectiveType, false); err != nil {
				return "", fmt.Errorf("parameter %d (%s) of arg %q error: %w",
					i, effectiveType.Name(), fieldInfo.Args[paramNum], err)
			}
		}
		// If still not found (eg inline struct literal) use the field name to generate a type name
		if typeName == "" {
			// Create a type name for anon struct by upper-casing the 1st letter of the arg name
			first, n := utf8.DecodeRuneInString(fieldInfo.Args[paramNum])
			typeName = string(unicode.ToUpper(first)) + fieldInfo.Args[paramNum][n:]
			if effectiveType.Kind() != reflect.Ptr {
				typeName += "!"
			}
		}

		// Now check that the default for the arg is OK
		if fieldInfo.ArgDefaults[paramNum] != "" {
			// Check that the default value is a valid literal for the type
			if err = s.validLiteral(typeName, enums, effectiveType, fieldInfo.ArgDefaults[paramNum]); err != nil {
				return "", fmt.Errorf("%w: parameter %d (%s) of arg %q default value %q is not of the correct type (%s)",
					err, i, effectiveType.Name(), fieldInfo.Args[paramNum], fieldInfo.ArgDefaults[paramNum], typeName)
			}
		}
		builder.WriteString(typeName)

		// Do we also need to add = followed by the argument default value?
		if fieldInfo.ArgDefaults[paramNum] != "" {
			builder.WriteString(" = ")
			value := fieldInfo.ArgDefaults[paramNum]
			builder.WriteString(value)
		}
		if !isScalar {
			// If it's a struct we also need to add the "input" type to our collection
			if err := s.add(typeName, effectiveType, enums, gqlInputKeyword, nil); err != nil {
				return "", fmt.Errorf("%w adding INPUT type %q", err, typeName)
			}
		}

		sep = paramSep
		paramNum++
	}
	if paramNum < len(fieldInfo.Args) {
		return "", fmt.Errorf("not enough args (%d) expected %d", paramNum, len(fieldInfo.Args))
	}
	if sep != paramStart { // if we got any args
		builder.WriteString(paramEnd)
	}
	return builder.String(), nil
}
