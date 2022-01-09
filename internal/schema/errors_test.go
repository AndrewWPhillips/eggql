package schema_test

// errors_test.go has table-driven tests for error conditions in calls to schema.Build

import (
	"github.com/andrewwphillips/eggql/internal/schema"
	"strings"
	"testing"
)

type (
	Query           struct{ Message string }
	SingleInt       struct{ I int }
	QueryNoArgs     struct{ F func(int) bool } // missing metadata "params"
	QueryTooFewArgs struct {
		F func(int, int) bool `graphql:",params(a)"`
	}
	QueryTooManyArgs struct {
		F func(int, int) bool `graphql:"f,params(a,b,c)"`
	}
	QueryArgsNonFunc struct {
		B bool `graphql:"bbb, params( arg0, arg1 ) "` // only func resolver needs params
	}
	QueryReturn0 struct {
		F0 func()
	}
	QueryReturn2 struct {
		F2 func() (int, string) // if 2 values are returned then 2nd must be error (not string)
	}
	QueryReturn3 struct {
		F3 func() (int, error, string)
	}

	QueryObjectAndInput struct { // the same struct can't be used as Object type and Input
		A SingleInt
		B func(SingleInt) string `graphql:",params(i)"`
	}
	QueryBadOption struct {
		Fa func(int8) string `graphql:",parms(i)"` // parms should be params
	}
	QueryBadParam1 struct {
		Fb func(int8) string `graphql:",params(a b)"` // no comma
	}
	QueryBadParam2 struct {
		Fc func(int8) string `graphql:",params(a"` // no closing bracket
	}
	QueryBadParam3 struct {
		Fd func(int8) string `graphql:",params(a)b"`
	}
	QueryBadParam4 struct {
		Fe func(int8) string `graphql:",params((a)"`
	}
	QueryBadParam5 struct {
		Ff func(int8) string `graphql:",params(a))"`
	}
	QueryUnknownEnum struct {
		Fg func() int8 `graphql:":EnumUnknown"`
	}
	QueryEnumNotInt struct {
		Length float64 `graphql:"len:Unit"` // "Unit" is a known enum but can't be a float
	}
	QueryUnknownParam struct {
		F func(int) string `graphql:",params(i:Unknown)"`
	}
	QueryEnumParamNotInt struct {
		G func(bool) string `graphql:",params(i:Unit)"`
	}
	QueryBadName struct {
		S string `graphql:"@9"`
	}
	QueryBadDefaultEnum struct {
		E0 func(int) int `graphql:",params(unit:Unit=Inch)"` // Inch is not a valid enum value
	}
	QueryBadDefaultInt struct {
		E1 func(int) int `graphql:"e1,params(len=ten)"` // ten is not a valid Int
	}
	QueryBadDefaultFloat struct {
		E2 func(float64) int `graphql:"e2,params(f=x)"` // x is not a valid Float
	}
	QueryBadDefaultBoolean struct {
		E3 func(bool) int `graphql:"e3,params(b=1)"` // 1 is not a valid Boolean
	}
	QueryDupeField1 struct {
		M1 string `graphql:"m"`
		M2 string `graphql:"m"`
	}
	QueryDupeField2 struct {
		Dupe   func() int // generated name is "dupe"
		Field2 bool       `graphql:"dupe"`
	}
	Embedded   struct{ M string }
	QueryDupe1 struct {
		Embedded
		M string
	}
	QueryDupe2 struct {
		M string
		Embedded
	}
)

var (
	enums       = map[string][]string{"Unit": {"FOOT", "METER", "PARSEC"}}
	badName     = map[string][]string{"123": {"A"}}
	badValue    = map[string][]string{"Unit": {"456", "MILE"}}
	badValue2   = map[string][]string{"Unit": {"true", "false", "null"}}
	repeatValue = map[string][]string{"Unit": {"MILE", "FOOT", "MILE"}}
)

var errorData = map[string]struct {
	data    interface{}
	enums   map[string][]string
	problem string
}{
	"NonStruct":       {1, nil, "must be struct"},
	"BadType":         {struct{ C complex128 }{}, nil, "unhandled type"},
	"DupeQuery":       {struct{ Q Query }{}, nil, "same name"}, // two different types with same name "Query"
	"NoArgs":          {QueryNoArgs{}, nil, "no params"},
	"TooFewArgs":      {QueryTooFewArgs{}, nil, "argument count"},
	"TooManyArgs":     {QueryTooManyArgs{}, nil, "argument count"},
	"ArgsNonFunc":     {QueryArgsNonFunc{}, nil, "arguments cannot be supplied"},
	"Return0":         {QueryReturn0{}, nil, "must return a value"},
	"Return2":         {QueryReturn2{}, nil, "must be error type"},
	"Return3":         {QueryReturn3{}, nil, "returns too many values"},
	"ObjectInput":     {QueryObjectAndInput{}, nil, "same name"},
	"UnknownOption":   {QueryBadOption{}, nil, "unknown option"},
	"BadParam1":       {QueryBadParam1{}, nil, "not a valid name"},
	"BadParam2":       {QueryBadParam2{}, nil, "unmatched left bracket"},
	"BadParam3":       {QueryBadParam3{}, nil, "not in brackets"},
	"BadParam4":       {QueryBadParam4{}, nil, "unmatched left bracket"},
	"BadParam5":       {QueryBadParam5{}, nil, "unmatched right bracket"},
	"EnumName":        {Query{}, badName, "valid name"},
	"EnumValue":       {Query{}, badValue, "enum value"},
	"EnumValue2":      {Query{}, badValue2, "enum value"},
	"EnumRepeat":      {Query{}, repeatValue, "repeated enum value"},
	"UnknownEnum":     {QueryUnknownEnum{}, nil, "not found"},
	"EnumNotInt":      {QueryEnumNotInt{}, enums, "enum type must be an integer"},
	"UnknownParam":    {QueryUnknownParam{}, nil, "not found"},
	"EnumParamBad":    {QueryEnumParamNotInt{}, enums, "must be an integer"},
	"BadName":         {QueryBadName{}, nil, "not a valid name"},
	"BadDefaultEnum":  {QueryBadDefaultEnum{}, enums, "not found in enum"},
	"BadDefaultInt":   {QueryBadDefaultInt{}, nil, "default value"},
	"BadDefaultFloat": {QueryBadDefaultFloat{}, nil, "default value"},
	"BadDefaultBool":  {QueryBadDefaultBoolean{}, nil, "default value"},
	"DupeField1":      {QueryDupeField1{}, nil, "same name"},
	"DupeField2":      {QueryDupeField2{}, nil, "same name"},
	"DupeEmbedded1":   {QueryDupe1{}, nil, "same name"},
	"DupeEmbedded2":   {QueryDupe2{}, nil, "same name"},

	// TODO: test defaults errors: input, list
}

func TestSchemaErrors(t *testing.T) {
	for name, data := range errorData {
		out, err := schema.Build(data.enums, data.data)
		Assertf(t, out == "", "TestSchemaErrors: %12s: expected empty result, got %q", name, out)
		ok := err != nil
		if ok {
			// we got an error (good), but we should still make sure it's the right one
			ok = strings.Contains(err.Error(), data.problem)
		}
		Assertf(t, ok, "TestSchemaErrors: %12s: expected an error, got %v", name, err)
	}
}
