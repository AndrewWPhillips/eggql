package schema_test

import (
	"github.com/andrewwphillips/eggql/internal/schema"
	"strconv"
	"testing"
)

type (
	Cust1 int8
	Cust2 string
)

func (pi *Cust1) UnmarshalEGGQL(s string) error {
	return nil
}

func (ps *Cust2) UnmarshalEGGQL(in string) error {
	return nil
}

var scalarData = map[string]struct {
	data     interface{}
	expected string
}{
	// Just testing the GraphQL scalar declaration
	"return": {data: struct{ E Cust1 }{}, expected: "type Query{ e: Cust1! } scalar Cust1"},
	"list":   {data: struct{ E []Cust1 }{}, expected: "type Query{ e: [Cust1]! } scalar Cust1"},
	"arg": {
		data: struct {
			F func(Cust1) string `graphql:",args(i)"`
		}{},
		expected: "type Query{ f(i:Cust1!): String! } scalar Cust1",
	},
}

func TestScalarSchema(t *testing.T) {
	for name, data := range scalarData {
		exp := RemoveWhiteSpace(t, data.expected)
		out := RemoveWhiteSpace(t, schema.MustBuild(data.data))
		same := out == exp
		where := ""
		if !same {
			// Failing case - find the offset of the first different byte to help debug where the problem is
			for i := range out {
				if i >= len(exp) || out[i] != exp[i] {
					where = "\nwhere first difference is at character " + strconv.Itoa(i)
					break
				}
			}
		}

		Assertf(t, same, "TestScalarSchema: %10s: expected %q got %q%s", name, exp, out, where)
	}
}
