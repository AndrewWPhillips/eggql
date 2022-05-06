package field

// split.go has functions to split tag strings at a comma separator but allowing for brackets, quotes, etc

import (
	"fmt"
	"strings"
)

// SplitArgs splits a string on commas and returns the resulting slice of strings.
// It ignores commas within strings, round brackets, square brackets or braces, which
// allows for "nested" structures. For example "a,b(c,d),e"  => []string{ "a", "b(c,d)", "e" }
// An error is returned if there is a problem with the input string such as unmatched brackets.
func SplitArgs(s string) ([]string, error) {
	// First count the number of commas that aren't within brackets
	var count, round, square, brace int
	var inString bool

	for _, c := range s {
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
				return nil, fmt.Errorf("unmatched right bracket ')` in %q", s)
			}
		case ']':
			square--
			if square < 0 {
				return nil, fmt.Errorf("unmatched right square bracket ']' in %q", s)
			}
		case '}':
			brace--
			if brace < 0 {
				return nil, fmt.Errorf("unmatched right brace '}' in %q", s)
			}
		case ',':
			if round == 0 && square == 0 && brace == 0 { // only count "top-level" commas
				count++
			}
		}
	}
	if inString {
		return nil, fmt.Errorf("unmatched quote (unterminated string) in %q", s)
	}
	if round > 0 {
		return nil, fmt.Errorf("unmatched left bracket '(' in %q", s)
	}
	if square > 0 {
		return nil, fmt.Errorf("unmatched left square bracket '[' in %q", s)
	}
	if brace > 0 {
		return nil, fmt.Errorf("unmatched left brace '{' in %q", s)
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
			return nil, fmt.Errorf("comma not found in %q", s)
		}
		retval = append(retval, strings.Trim(s[:end], " "))
		s = s[end+1:]
	}
	// Add last (or only) segment
	retval = append(retval, strings.Trim(s, " "))

	return retval, nil
}

// TODO somehow reduce duplicate code in SplitArgs/SplitWithDesc

// SplitWithDesc is like SplitArgs but also allows a trailing "description" (anything after the first #).
// On success, it returns a list of strings, the description (if any) and a nil error.
// A non-nil error is returned if there is a problem with the input string such as unmatched brackets.
func SplitWithDesc(s string) ([]string, string, error) {
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
