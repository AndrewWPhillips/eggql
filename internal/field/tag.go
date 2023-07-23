package field

// tag.go handles extracting info from the "egg:" tag string (from struct field metadata)

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// AllowSubscript etc control which options are allowed TODO: use build tags instead?
	AllowSubscript  = true // "subscript" option generates a resolver to subscript into a list (array/slice/map)
	AllowFieldID    = true // "field_id" option generates an extra "id" field for queries on a list (array/slice/map)
	AllowComplexity = true // "complexity" option specifies how to estimate the complexity of a resalver
)

// GetInfoFromTag extracts GraphQL field name and type info from the field's tag (if any)
// If the tag just contains a dash (-) then nil is returned (no error).  If the tag string is empty
// (e.g. if no tag was supplied) then the returned Info is not nil but the Name field is empty.
func GetInfoFromTag(tag string) (*Info, error) {
	if tag == "-" {
		return nil, nil // this field is to be ignored
	}
	parts, description, err := SplitWithDesc(tag)
	if err != nil {
		return nil, fmt.Errorf("%w splitting tag %q", err, tag)
	}

	var fieldInfo *Info
	//fieldInfo := &Info{Description: description}
	for i, part := range parts {
		if i == 0 { // first string is the name
			fieldInfo, err = getMain(part)
			if err != nil {
				return nil, fmt.Errorf("%w resolver %q of tag %q", err, part, tag)
			}
			continue
		}
		if part == "" {
			continue // ignore empty sections
		}
		if part[0] == '@' {
			// anything starting with @ is assumed to be a directive & stored without validation TODO: validate that brackets match?
			fieldInfo.Directives = append(fieldInfo.Directives, part)
			continue
		}
		if subscript := getSubscript(part); subscript != "" {
			fieldInfo.Subscript = subscript
			continue
		}
		if fieldID := getFieldID(part); fieldID != "" {
			fieldInfo.FieldID = fieldID
			continue
		}
		if strings.Contains(part, "id") {
			// detect common mistake (id_field instead of field_id)
			return nil, fmt.Errorf(`unknown option %q, - did you mean "field_id"?`, part)
		}
		if baseIndex := getBaseIndex(part); baseIndex > 0 {
			fieldInfo.BaseIndex = baseIndex
			continue
		}
		if part == "nullable" {
			fieldInfo.Nullable = true
			continue
		}
		if part == "no_cache" || part == "nocache" {
			fieldInfo.NoCache = true
			continue
		}
		if strings.HasPrefix(part, "args") {
			return nil, errors.New(`args option is no longer supported - add arguments (in brackets) after resolver name`)
		}
		return nil, fmt.Errorf("unknown option %q in %q", part, tag)
	}

	// We can do a bit of validation here
	if fieldInfo.BaseIndex > 0 && fieldInfo.Subscript == "" && fieldInfo.FieldID == "" {
		return nil, fmt.Errorf(`you can't use "base" option without "subscript" or "field_id" (%s)`, tag)
	}

	fieldInfo.Description = description

	return fieldInfo, nil
}

// getMain handles the first part of the tag which may just be the resolver name (or even empty), but can
//
//	also include a type after a colon (:) and resolvers arguments (comma-separated and within brackets), where
//	each argument can have a name, type (after :) and default value (after =).  Note that many of these things
//	can be deduced (from the GO field name/type) and left out (except for the names of arguments).
func getMain(s string) (r *Info, err error) {
	r = &Info{}

	// First check if there is a resolver name (if not it is later derived from the field name)
	if s == "" || s[0] != ':' && s[0] != '(' {
		i := strings.IndexAny(s, ":(")
		if i == -1 {
			r.Name = s // empty string or just name
			return
		}
		r.Name = s[:i]
		s = s[i:]
	}

	// Next check if there's a trailing type (if not then it is derived from the field type)
	colon := -1
loop:
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ':':
			colon = i
			break loop
		case ')': // stop at end of args so we don't find colons inside the args
			break loop
		}
	}
	if colon > -1 {
		r.GQLTypeName = s[colon+1:]
		s = s[:colon]
	}

	// Finally, if there are brackets then get the resolver arguments
	list, err := getBracketedList(s, "")
	if err != nil {
		return nil, fmt.Errorf("%w getting resolver args", err)
	} else if list != nil {
		// use empty strings for default values for arg lists
		r.Args = make([]string, len(list))
		r.ArgTypes = make([]string, len(list))
		r.ArgDefaults = make([]string, len(list))
		r.ArgDescriptions = make([]string, len(list))
		for paramIndex, s := range list {
			// Strip description after hash (#)
			subParts := strings.SplitN(s, "#", 2)
			s = subParts[0]
			if len(subParts) > 1 {
				r.ArgDescriptions[paramIndex] = subParts[1]
			}
			// Strip of default value (if any) after equals sign (=)
			subParts = strings.Split(s, "=")
			s = subParts[0]
			if len(subParts) > 1 {
				r.ArgDefaults[paramIndex] = strings.Trim(subParts[1], " ")
			}
			// Strip of enum name after colon (:)
			subParts = strings.Split(s, ":")
			s = subParts[0]
			if len(subParts) > 1 {
				r.ArgTypes[paramIndex] = strings.Trim(subParts[1], " ")
			}

			r.Args[paramIndex] = strings.Trim(s, " ")
		}
	}
	return
}

// getSubscript checks for the subscript option string and if found returns the value (after
// the =) or "id" if no value is given
func getSubscript(s string) string {
	if !AllowSubscript {
		return ""
	}
	if s == "subscript" {
		return "id" // default field name if none given
	}
	if strings.HasPrefix(s, "subscript=") {
		return strings.TrimPrefix(s, "subscript=")
	}
	return ""
}

// getFieldID checks for the "field_id" option and if found returns the specified value (after
// the equals sign if present) or "id" if not present
func getFieldID(s string) string {
	if !AllowFieldID {
		return ""
	}
	if s == "field_id" {
		return "id"
	}
	if strings.HasPrefix(s, "field_id=") {
		return strings.TrimPrefix(s, "field_id=")
	}
	return ""
}

// getBaseIndex checks for the "base" option (only used if "subscript" or "field_id" is specified).
// It returns the integer value after the = or zero if not specified or there was an error
func getBaseIndex(s string) int {
	if !AllowFieldID {
		return 0
	}
	if strings.HasPrefix(s, "base=") {
		base, _ := strconv.Atoi(strings.TrimPrefix(s, "base="))
		return base
	}
	return 0
}

// getBracketedList gets a list of values from a string enclosed in brackets and preceded by a keyword
// This is used to extract info from the metadata (tag) of a struct field used
// for GraphQL resolvers, such as resolver arguments.
// Eg for getBracketedList("args(a,b=2)", "args") it will return the list
// of strings {"a", "b=2"}. It may return an error for badly formatted metadata.
// If the keyword does not match it returns nil (and no error).
func getBracketedList(s, keyword string) ([]string, error) {
	if !strings.HasPrefix(s, keyword+"(") {
		return nil, nil // keyword does not match
	}
	s = strings.TrimPrefix(s, keyword)

	// Get the bracket-enclosed string and split using commas
	last := len(s) - 1
	if last < 1 || s[0] != '(' || s[last] != ')' {
		return nil, errors.New("value(s) not in brackets for tag keyword " + keyword)
	}
	s = strings.Trim(s[1:last], " ")
	if s == "" {
		// Avoid behaviour of strings.Split on boundary condition (empty string)
		return []string{}, nil // empty parameter list
	}
	retval, err := SplitArgs(s)
	if err != nil {
		return nil, err
	}
	for i := range retval {
		retval[i] = strings.Trim(retval[i], " ")
	}
	return retval, nil
}
