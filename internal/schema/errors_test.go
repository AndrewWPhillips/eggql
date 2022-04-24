package schema_test

// errors_test.go has table-driven tests for error conditions in calls to schema.Build

import (
	"strconv"
	"strings"
	"testing"

	"github.com/andrewwphillips/eggql/internal/schema"
)

type (
	Query        struct{ Message string }
	SingleInt    struct{ I int }
	QueryBadName struct {
		S string `egg:"@9"`
	}

	QueryBadParam1 struct {
		Fb func(int8) string `egg:",args(a b)"` // no comma
	}
	QueryBadParam2 struct {
		Fc func(int8) string `egg:",args(a"` // no closing bracket
	}
	QueryBadParam3 struct {
		Fd func(int8) string `egg:",args(a)b"`
	}
	QueryBadParam4 struct {
		Fe func(int8) string `egg:",args((a)"`
	}
	QueryBadParam5 struct {
		Ff func(int8) string `egg:",args(a))"`
	}
	QueryUnknownEnum struct {
		Fg func() int8 `egg:":EnumUnknown"`
	}
	QueryEnumNotInt struct {
		Length float64 `egg:"len:Unit"` // "Unit" is a known enum but can't be a float
	}
	QueryUnknownParam struct {
		F func(int) string `egg:",args(i:Unknown)"`
	}
	QueryEnumParamNotInt struct {
		G func(bool) string `egg:",args(i:Unit)"`
	}
	QueryBadDefaultEnum struct {
		E0 func(int) int `egg:",args(unit:Unit=Inch)"` // Inch is not a valid enum value
	}
	QueryBadDefaultInt struct {
		E1 func(int) int `egg:"e1,args(len=ten)"` // ten is not a valid Int
	}
	QueryBadDefaultFloat struct {
		E2 func(float64) int `egg:"e2,args(f=x)"` // x is not a valid Float
	}
	QueryBadDefaultBoolean struct {
		E3 func(bool) int `egg:"e3,args(b=1)"` // 1 is not a valid Boolean
	}
	QueryDupeField1 struct {
		M1 string `egg:"m"`
		M2 string `egg:"m"`
	}
	QueryDupeField2 struct {
		Dupe   func() int // generated name is "dupe"
		Field2 bool       `egg:"dupe"`
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
	emptyEnum   = map[string][]string{"Unit": {"FOOT"}, "Empty": {}}
)

// CustScalarStruct implements UnmarshalEGGQL to signal it's a scalar type
type CustScalarStruct struct{}

// UnmarshalEGGQL is just added as a method on Cust1 to indicate that it is a custom scalar
func (*CustScalarStruct) UnmarshalEGGQL(s string) error {
	return nil // nothing needed here as we are just testing schema generation
}

// CustScalarInt os a custom scalar based on an integer type (scalar)
type CustScalarInt int64

// UnmarshalEGGQL is just added as a method on Cust1 to indicate that it is a custom scalar
func (*CustScalarInt) UnmarshalEGGQL(s string) error {
	_, err := strconv.Atoi(s)
	return err
}

var errorData = map[string]struct {
	data    interface{}
	enums   map[string][]string
	problem string
}{
	"NonStruct":    {1, nil, "must be struct"},
	"BadType":      {struct{ C complex128 }{}, nil, "unhandled type"},
	"BadType2":     {struct{ C []interface{} }{}, nil, "bad element type"},
	"BadSliceType": {struct{ C []complex128 }{}, nil, "unhandled type"},
	"DupeQuery":    {struct{ Q Query }{}, nil, "same name"}, // two different types with same name "Query"
	"NoArgs":       {struct{ F func(int) bool }{}, nil, "no args"},
	"TooFewArgs": {
		struct {
			F func(int, int) bool `egg:",args(a)"`
		}{}, nil, "argument count",
	},
	"TooManyArgs": {
		struct {
			F func(int, int) bool `egg:"f,args(a,b,c)"`
		}{}, nil, "argument count",
	},
	"ArgsNonFunc": {
		struct {
			B bool `egg:"bbb, args( arg0, arg1 ) "` // only func resolver needs args
		}{}, nil, "arguments cannot be supplied",
	},
	"Return0": {struct{ F0 func() }{}, nil, "must return a value"},
	"Return2": {struct{ F2 func() (int, string) }{}, nil, "must be error type"}, // 2nd return should be error
	"Return3": {struct{ F3 func() (int, error, string) }{}, nil, "returns too many values"},
	"ObjectInput": {
		struct { // the same struct can't be used as Object type and Input
			A SingleInt              // SingleInt is used as a (nested) object type
			B func(SingleInt) string `egg:",args(i)"`
		}{}, nil, "different GraphQL types",
	},
	"InterfaceInput": {
		struct {
			SingleInt                        // SingleInt is embedded to be used as an interface type
			B         func(SingleInt) string `egg:",args(i)"`
		}{}, nil, "different GraphQL types",
	},
	"DupeInterface": {
		struct {
			SingleInt
			Query `egg:":SingleInt"`
		}{}, nil, "same name",
	},
	"BadInterface": {struct{ QueryBadName }{}, nil, "not a valid name"},
	"BadReserved": {
		struct {
			Message string `egg:"__message"`
		}{}, nil, "not a valid name",
	},
	"UnknownOption": {
		struct {
			Fa func(int8) string `egg:",params(i)"` // params should be args
		}{}, nil, "unknown option",
	},
	"NoReturn":        {struct{ Fa func() }{}, nil, "must return a value"},
	"BadParam1":       {QueryBadParam1{}, nil, "not a valid name"},
	"BadParam2":       {QueryBadParam2{}, nil, "unmatched left bracket"},
	"BadParam3":       {QueryBadParam3{}, nil, "not in brackets"},
	"BadParam4":       {QueryBadParam4{}, nil, "unmatched left bracket"},
	"BadParam5":       {QueryBadParam5{}, nil, "unmatched right bracket"},
	"EnumName":        {Query{}, badName, "valid name"},
	"EnumValue":       {Query{}, badValue, "enum value"},
	"EnumValue2":      {Query{}, badValue2, "enum value"},
	"EnumRepeat":      {Query{}, repeatValue, "repeated enum value"},
	"EmptyEnum":       {Query{}, emptyEnum, "has no values"},
	"UnknownEnum":     {QueryUnknownEnum{}, nil, "not found"},
	"EnumNotInt":      {QueryEnumNotInt{}, enums, "must be an integer"},
	"UnknownParam":    {QueryUnknownParam{}, nil, "not found"},
	"EnumParamBad":    {QueryEnumParamNotInt{}, enums, "must be an integer"},
	"BadName":         {QueryBadName{}, nil, "not a valid name"},
	"BadDefaultEnum":  {QueryBadDefaultEnum{}, enums, "not of the correct type"},
	"BadDefaultInt":   {QueryBadDefaultInt{}, nil, "default value"},
	"BadDefaultFloat": {QueryBadDefaultFloat{}, nil, "default value"},
	"BadDefaultBool":  {QueryBadDefaultBoolean{}, nil, "default value"},
	"DupeField1":      {QueryDupeField1{}, nil, "same name"},
	"DupeField2":      {QueryDupeField2{}, nil, "same name"},
	"DupeEmbedded1":   {QueryDupe1{}, nil, "same name"},
	"DupeEmbedded2":   {QueryDupe2{}, nil, "same name"},
	"BadTypeName": {
		struct {
			V int `egg:":UnknownType"`
		}{}, nil, "resolver type (UnknownType)",
	},
	"SquareBrackets": {
		struct {
			V []int `egg:":[]Int"` // using Go slice syntax not GraphQL list syntax
		}{}, nil, "did you mean [Int]",
	},
	"TypeCustomScalar1": {
		struct {
			V CustScalarStruct `egg:":Int"`
		}{}, nil, "ustom scalar",
	},
	"TypeCustomScalar2": {
		struct {
			V CustScalarInt `egg:":CustScalarStruct"`
		}{}, nil, "ustom scalar",
	},
	"TypeCustomScalar3": {
		struct {
			V CustScalarInt `egg:":X"`
		}{}, nil, "ustom scalar",
	},
	"TypeInt": {
		struct {
			V complex64 `egg:":Int"`
		}{}, nil, "must have an integer",
	},
	"TypeFloat": {
		struct {
			V complex64 `egg:":Float!"`
		}{}, nil, "must have a float",
	},
	"TypeString": {
		struct {
			V complex64 `egg:":String!"`
		}{}, nil, "string resolver",
	},
	"TypeBool": {
		struct {
			V complex64 `egg:":Boolean!"`
		}{}, nil, "bool resolver",
	},
	"TypeID": {
		struct {
			V complex64 `egg:":ID!"`
		}{}, nil, "string resolver",
	},
	"TypeSlice": {
		struct {
			V int `egg:":[Int]"`
		}{}, nil, "list resolver",
	},
	"TypeSliceFunc": {
		struct {
			V func() []bool `egg:":[Int]"`
		}{}, nil, "must have an integer",
	},
	"TypeEnum": {
		struct {
			V complex64 `egg:":Unit"`
		}{}, enums, "must be an integer",
	},
	"TypeStruct1": {
		struct {
			_ SingleInt
			V Query `egg:":SingleInt"`
		}{}, nil, "cannot have a resolver of type",
	},
	"TypeStruct2": {
		struct {
			Embedded
			V SingleInt `egg:":Embedded"`
		}{}, nil, "cannot have a resolver of type",
	},
	"TypeStruct3": {
		struct {
			_ SingleInt
			V complex64 `egg:":SingleInt"`
		}{}, nil, "must have a struct resolver",
	},
	"SubscriptOption1": {
		struct {
			V complex64 `egg:",subscript"`
		}{}, nil, "cannot use subscript option",
	},
	"SubscriptOption2": {
		struct {
			V SingleInt `egg:",subscript"`
		}{}, nil, "cannot use subscript option",
	},
	"SubscriptOption3": {
		struct {
			V map[SingleInt]int `egg:",subscript"`
		}{}, nil, "map key for subscript option",
	},
	"SubscriptOption4": {
		struct {
			V map[bool]int `egg:",subscript"`
		}{}, nil, "map key for subscript option",
	},

	"ArgDefaultBool": {
		struct {
			F func(bool) int `egg:",args(b=f)"`
		}{}, nil, "not of the correct type (Boolean)",
	},
	"ArgDefaultString": {
		struct {
			F func(string) bool `egg:",args(s=4)"`
		}{}, nil, "not of the correct type (String)",
	},
	"ArgDefaultID": {
		struct {
			F func(string) bool `egg:",args(id:ID=false)"`
		}{}, nil, "not of the correct type (ID)",
	},
	"ArgDefaultID2": {
		struct {
			F func(string) bool `egg:",args(id:ID=1.2)"` // can be int but not float
		}{}, nil, "not of the correct type (ID)",
	},
	"ArgDefaultInt": {
		struct {
			F func(int) string `egg:",args(i=\"s\")"`
		}{}, nil, "not of the correct type (Int)",
	},
	"ArgDefaultInt2": {
		struct {
			F func(int) string `egg:",args(i=3.14)"`
		}{}, nil, "not of the correct type (Int)",
	},
	"ArgDefaultFloat": {
		struct {
			F func(float64) string `egg:",args(f=s)"`
		}{}, nil, "not of the correct type (Float)",
	},
	"ArgDefaultEnum": {
		struct {
			F func(int) string `egg:",args(e:Unit=1)"`
		}{}, enums, "not of the correct type (Unit)",
	},
	"ArgDefaultEnum2": {
		struct {
			F func(int) string `egg:",args(e:Unit=\"FOOT\")"` // enum value in quotes is seen as a string literal
		}{}, enums, "not of the correct type (Unit)",
	},
	"ArgCustomScalar": {
		struct {
			F func(CustScalarInt) int `egg:",args(c:CustScalarInt=invalid)"`
		}{}, nil, "not of the correct type (CustScalarInt)",
	},
	"ArgDefaultListBad": {
		struct {
			F func([]int) string `egg:",args(ii=[)"`
		}{}, nil, "unmatched left square bracket",
	},
	"ArgDefaultListBoolean": {
		struct {
			F func([]bool) string `egg:",args(bb=[false, true, 1])"` // 1 is not a Boolean literal
		}{}, nil, "not of the correct type ([Boolean",
	},
	"ArgDefaultListString": {
		struct {
			F func([]string) string `egg:",args(ss=[\"s\", t])"` // t is not in quotes
		}{}, nil, "not of the correct type ([String",
	},
	"ArgDefaultListInt": {
		struct {
			F func([]int) string `egg:",args(ii=[s])"`
		}{}, nil, "not of the correct type ([Int",
	},
	"ArgDefaultListID": {
		struct {
			F func([]string) string `egg:",args(ids:[ID]=[\"1\", 2, 3.14])"` // float (3.14) is not an ID literal
		}{}, nil, "not of the correct type ([ID",
	},
	"ArgDefaultListEnum": {
		struct {
			F func([]int) string `egg:",args(ii:[Unit]=[METER, FOOT, 2])"`
		}{}, enums, "not of the correct type ([Unit",
	},
	"ArgCustomScalarList": {
		struct {
			F func([]CustScalarInt) int `egg:",args(c:[CustScalarInt]=[1,2,false])"` // false does not decode with CustScalarInt.UnmarshalEGGQL()
		}{}, nil, "not of the correct type ([CustScalarInt])",
	},

	// TODO: test defaults errors: input
}

func TestSchemaErrors(t *testing.T) {
	for name, data := range errorData {
		out, err := schema.Build(data.enums, data.data)
		if out != "" {
			Assertf(t, out == "", "TestSchemaErrors: %12s: expected empty result, got %q", name, out)
		}
		ok := err != nil
		if ok {
			// we got an error (good), but we should still make sure it's the right one
			// Note that this is a bit fragile as it scans the error text - tests may fail if error messages are modified
			ok = data.problem != "" && strings.Contains(err.Error(), data.problem)
		}
		Assertf(t, ok, "TestSchemaErrors: %12s: expected an error, got: %v", name, err)
	}
}
