package schema_test

import (
	"github.com/andrewwphillips/eggql/internal/schema"
	"strconv"
	"testing"
)

type (
	// QueryUnit and the subsequent types here are used for the type of the data field of the
	// enumData map (below) used for table-driven tests of enums.
	QueryUnit struct {
		E int `graphql:":Unit"`
	}
	QueryListE struct {
		E []int `graphql:":[Unit]"`
	}
	QueryNamed struct {
		E int `graphql:"name:Unit"`
	}
	QueryParam struct {
		F func(int) string `graphql:",args(u:Unit)"`
	}
	QueryListParam struct {
		F func([]int) string `graphql:",args(u:[Unit])"`
	}
	QueryDefault struct {
		Height func(float64, int) string `graphql:",args(h,u:Unit=METER)"`
	}
	QueryListDefault struct {
		F func([]int) string `graphql:",args(u:[Unit]=[METER, FOOT, FOOT])"`
	}
	QueryDefaultEmpty struct {
		F func([]int) string `graphql:",args(u:[Unit]=[])"`
	}
)

var unitEnum = map[string][]string{"Unit": {"FOOT", "METER"}}

var enumData = map[string]struct {
	data     interface{}
	enums    map[string][]string
	expected string
}{
	"Unit": {data: QueryUnit{}, enums: unitEnum,
		expected: "schema{ query:QueryUnit } type QueryUnit{ e: Unit! } enum Unit { FOOT METER }"},
	"List": {data: QueryListE{}, enums: unitEnum,
		expected: "schema{ query:QueryListE } type QueryListE{ e: [Unit]! } enum Unit { FOOT METER }"},
	"Named": {data: QueryNamed{}, enums: unitEnum,
		expected: "schema{ query:QueryNamed } type QueryNamed{ name: Unit! } enum Unit { FOOT METER }"},
	"Param": {data: QueryParam{}, enums: unitEnum,
		expected: "schema{ query:QueryParam } type QueryParam{ f(u:Unit!): String! } enum Unit { FOOT METER }"},
	"ListParam": {data: QueryListParam{}, enums: unitEnum,
		expected: "schema{ query:QueryListParam } type QueryListParam{ f(u:[Unit]!): String! } enum Unit { FOOT METER }"},
	"Default": {data: QueryDefault{}, enums: unitEnum,
		expected: "schema{ query:QueryDefault } type QueryDefault{ height(h:Float!, u:Unit!=METER): String! } enum Unit { FOOT METER }"},
	"ListDefault": {data: QueryListDefault{}, enums: unitEnum,
		expected: "schema{ query:QueryListDefault } type QueryListDefault{ f(u:[Unit]!=[METER, FOOT, FOOT]): String! } enum Unit { FOOT METER }"},
	"DefaultEmpty": {data: QueryDefaultEmpty{}, enums: unitEnum,
		expected: "schema{ query:QueryDefaultEmpty } type QueryDefaultEmpty{ f(u:[Unit]!=[]): String! } enum Unit { FOOT METER }"},
}

func TestEnumSchema(t *testing.T) {
	for name, data := range enumData {
		exp := RemoveWhiteSpace(t, data.expected)
		out := RemoveWhiteSpace(t, schema.MustBuild(data.enums, data.data))
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

		Assertf(t, same, "TestEnumSchema: %10s: schema.Build expected %q got %q%s", name, exp, out, where)
	}
}
