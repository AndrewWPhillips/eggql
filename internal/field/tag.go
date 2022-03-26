package field

// tag.go handles extracting info from the "GraphQL" tag of s Go struct field

import (
	"errors"
	"fmt"
	"strings"
)

// SplitNested splits a string (similarly to strings.Split) but on commas and skipping "nested structures" - ie anything
// inside round brackets, square brackets or braces. For example "a,b(c,d),e"  => []string{ "a", "b(c,d)", "e" }
// The first encountered hash (#) (outside of brackets) designates that the rest of the string is a description (2nd return value)
// If there is a problem with the input string such as unmatched brackets then it returns an error.
func SplitNested(s string) ([]string, string, error) {
	// First count the number of commas that aren't within brackets
	var count, round, square, brace int
	var inString bool
	hash := -1
loop:
	for i, c := range s {
		if inString {
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '(':
			round++
		case '[':
			square++
		case '{':
			brace++
		case ')':
			round--
			if round < 0 {
				return nil, "", fmt.Errorf("unmatched right bracket ')` in %q", s)
			}
		case ']':
			square--
			if square < 0 {
				return nil, "", fmt.Errorf("unmatched right square bracket ']' in %q", s)
			}
		case '}':
			brace--
			if brace < 0 {
				return nil, "", fmt.Errorf("unmatched right brace '}' in %q", s)
			}
		case ',':
			if round == 0 && square == 0 && brace == 0 { // only count "top-level" commas
				count++
			}
		case '#':
			if hash == -1 && round == 0 && square == 0 && brace == 0 { // ignore # in brackets
				hash = i
				break loop
			}
		}
	}
	if inString {
		return nil, "", fmt.Errorf("unmatched quote (unterminated string) in %q", s)
	}
	if round > 0 {
		return nil, "", fmt.Errorf("unmatched left bracket '(' in %q", s)
	}
	if square > 0 {
		return nil, "", fmt.Errorf("unmatched left square bracket '[' in %q", s)
	}
	if brace > 0 {
		return nil, "", fmt.Errorf("unmatched left brace '{' in %q", s)
	}

	desc := ""
	if hash > -1 {
		desc = s[hash+1:]
		s = s[:hash]
	}

	retval := make([]string, 0, count+1)

	for sepNum := 0; sepNum < count; sepNum++ {
		// Find the next comma (that's outside brackets)
		end := -1
	findComma:
		for i, c := range s {
			if inString {
				if c == '"' {
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '(':
				round++
			case '[':
				square++
			case '{':
				brace++
			case ')':
				round--
			case ']':
				square--
			case '}':
				brace--
			case ',':
				if round == 0 && square == 0 && brace == 0 {
					end = i
					break findComma
				}
			}
		}
		if end == -1 {
			return nil, "", fmt.Errorf("comma not found in %q", s)
		}
		retval = append(retval, strings.Trim(s[:end], " "))
		s = s[end+1:]
	}
	// Add last (or only) segment
	retval = append(retval, strings.Trim(s, " "))

	return retval, desc, nil
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
	retval, _, err := SplitNested(s)
	if err != nil {
		return nil, err
	}
	for i := range retval {
		retval[i] = strings.Trim(retval[i], " ")
	}
	return retval, nil
}
