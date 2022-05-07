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
	parts, desc, err := SplitWithDesc(tag)
	if err != nil {
		return nil, fmt.Errorf("%w splitting tag %q", err, tag)
	}
	fieldInfo := &Info{Description: desc}
	for i, part := range parts {
		if i == 0 { // first string is the name
			// Check for enum by splitting on a colon (:)
			if subParts := strings.Split(part, ":"); len(subParts) > 1 {
				fieldInfo.Name = subParts[0]
				fieldInfo.GQLTypeName = subParts[1]
			} else {
				fieldInfo.Name = part
			}
			continue
		}
		if part == "" {
			continue // ignore empty sections
		}
		if subscript := getSubscript(part); subscript != "" {
			fieldInfo.Subscript = subscript
			continue
		}
		if fieldID := getFieldID(part); fieldID != "" {
			fieldInfo.FieldID = fieldID
			continue
		}
		if baseIndex := getBaseIndex(part); baseIndex > 0 {
			fieldInfo.BaseIndex = baseIndex
			continue
		}
		if part == "nullable" {
			fieldInfo.Nullable = true
			continue
		}
		if list, err := getBracketedList(part, "args"); err != nil {
			return nil, fmt.Errorf("%w getting args in %q", err, tag)
		} else if list != nil {
			fieldInfo.Args = make([]string, len(list))
			fieldInfo.ArgTypes = make([]string, len(list))
			fieldInfo.ArgDefaults = make([]string, len(list))
			fieldInfo.ArgDescriptions = make([]string, len(list))
			for paramIndex, s := range list {
				// Strip description after hash (#)
				subParts := strings.SplitN(s, "#", 2)
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.ArgDescriptions[paramIndex] = subParts[1]
				}
				// Strip of default value (if any) after equals sign (=)
				subParts = strings.Split(s, "=")
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.ArgDefaults[paramIndex] = strings.Trim(subParts[1], " ")
				}
				// Strip of enum name after colon (:)
				subParts = strings.Split(s, ":")
				s = subParts[0]
				if len(subParts) > 1 {
					fieldInfo.ArgTypes[paramIndex] = strings.Trim(subParts[1], " ")
				}

				fieldInfo.Args[paramIndex] = strings.Trim(s, " ")
			}
			continue
		}
		return nil, fmt.Errorf("unknown option %q in %q", part, tag)
	}

	// We can do a bit of validation here
	if fieldInfo.BaseIndex > 0 && fieldInfo.Subscript == "" && fieldInfo.FieldID == "" {
		return nil, fmt.Errorf(`you can't use "base" option without "subscript" or "field_id" (%s)`, tag)
	}

	return fieldInfo, nil
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

// getSubscript checks for the "field_id" option and if found returns the value (after
// the =) or "id" if no value is given
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
