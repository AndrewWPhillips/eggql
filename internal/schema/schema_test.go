package schema_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/andrewwphillips/eggql"
	"github.com/andrewwphillips/eggql/internal/schema"
)

type (
	// QueryString and the subsequent types here are used for the type of the data field of the
	// testData map (below) used for table-driven tests.  Note that these types are only used for
	// their type information (include metadata) not for any instantiated values of fields.
	QueryEmpty     struct{}
	QueryString    struct{ M string }
	QueryInt       struct{ I int }
	QueryIntString struct {
		I int
		S string
	}
	QueryBool       struct{ B bool }
	QueryFloat      struct{ F float64 }
	QueryNested     struct{ Str QueryString }
	QueryTypeReuse  struct{ Q1, Q2 QueryString }
	QueryPtr        struct{ Ptr QueryInt }
	QueryList       struct{ List []int }
	QueryList2      struct{ List []QueryString }
	QueryAnonNested struct{ Anon struct{ B byte } } // anon type - should use field name as "type" name

	QuerySlice     struct{ Slice []int }
	QueryMap       struct{ Map map[string]int }
	QueryIntFunc   struct{ F func() int }
	QueryBoolFunc  struct{ F func() bool }
	QueryErrorFunc struct{ F func() (int, error) }
	QueryFuncParam struct {
		F func(float64) int `egg:",args(q)"`
	}
	QueryFuncParam2 struct {
		F func(string, int) bool `egg:",args( p1, p2 )"`
	}
	QueryFuncDefault struct {
		F func(string, int) bool `egg:",args(p1,p2=42)"`
	}
	QueryFuncDefault2 struct {
		F func(string, float64) bool `egg:",args(p1=\"a b\",p2=3.14)"`
	}
	QueryContextFunc struct {
		F func(context.Context) (int, error)
	}
	QueryCustomName struct {
		M string `egg:"message"` // specify GraphQL query name
	}
	QueryUnexported struct {
		m1 string `egg:"message"` // unexported field - tag should be ignored
		M2 string `egg:"message"`
	}

	InputInt        struct{ I int }
	QueryInputParam struct {
		F func(InputInt) int `egg:",args(in)"`
	}
	QueryInputAnon struct {
		F func(struct{ J int }) bool `egg:",args(anon)"`
	}
	QueryRecurse struct {
		P *QueryRecurse // recursive data structure: P is (ptr to) type of enclosed struct
	}

	IInt struct{ I int } // embed for interface
	M1   struct {
		IInt
		S string
	}
	M2 struct {
		IInt
		B bool
	}
	QueryInterface struct {
		A M1
		B M2
	}
	I2Int struct{ IInt } // for interface implements interface
	M3    struct {
		I2Int
		X, Y float64
	}
	QueryIfaceOfIface struct {
		Xy M3
	}

	IRecurse struct {
		B *QueryIfaceRecurse
	}
	QueryIfaceRecurse struct {
		IRecurse
	}
	IRecurseList struct {
		List *[]QueryIRecurseList
	}
	QueryIRecurseList struct {
		IRecurseList
	}

	// Person and Droid structs have an embedded Character -> in GraphQL schema Person and Droid implement the Character interface
	Person struct {
		Character   // a person is a character
		Personality string
	}
	Droid struct {
		Character       // a droid is a character
		PrimaryFunction string
	}
	Character struct {
		_       eggql.TagHolder `# star wars character`
		Name    string
		Friends []*Character
	}
	QueryInterface2 struct {
		_    *Person // this is the only way for the schema builder to know about the Person type
		Hero Character
	}
	QuerySubscriptSlice struct {
		Slice []string `egg:",subscript"`
	}
	QuerySubscriptArray struct {
		A [3]bool `egg:",subscript="`
	}
	QuerySubscriptMap struct {
		M map[string]float64 `egg:",subscript=s"`
	}

	U  struct{}
	U1 struct {
		U
		V int
	}
	U2 struct {
		U
		W string
	}
	QueryUnion struct {
		A U1
		B U2
	}
	QueryUnion2 struct {
		_ U1
		_ U2
		S []interface{} `egg:":[U]"`
	}
	QueryDescOnly struct {
		_ eggql.TagHolder `egg:"# no fields"`
	}
	QueryDescObject struct {
		Nested struct {
			_ eggql.TagHolder `egg:"# nested object"`
			I int
		}
	}
	IDesc struct {
		_ eggql.TagHolder `egg:"# interface"` // How we attach a description to an interface type
		I int
	}
	QueryDescInterface struct {
		IDesc
	}
	UDesc struct {
		_ eggql.TagHolder `egg:"# a union"` // How we attach a description to a union
	}
	UDesc1 struct {
		UDesc
	}
	UDesc2 struct {
		UDesc
	}
	QueryDescUnion struct {
		A UDesc1
		B UDesc2
	}
	QueryDescField struct {
		I int `egg:"# Test of # for description"`
	}
	QueryDescAll struct {
		_ eggql.TagHolder `egg:"#q (type)"`
		S func() string   `egg:"#s (#1)"`
		T []int           `egg:"#t (#2) "`
	}
	Cust1 int8 // custom scalar type (see UnmarshalEGGQL method below)
)

// UnmarshalEGGQL is just added as a method on Cust1 to indicate that it is a custom scalar
func (pi *Cust1) UnmarshalEGGQL(s string) error {
	return nil // nothing needed here as we are just testing schema generation
}

var testData = map[string]struct {
	data     interface{}
	expected string
}{
	"List": {QueryList{}, "schema{ query:QueryList } type QueryList{ list:[Int]! }"},
	"List2": {
		QueryList2{},
		"schema{query:QueryList2} type QueryList2{list:[QueryString]!} type QueryString{m:String!}",
	},
	"Empty":     {QueryEmpty{}, "schema{ query:QueryEmpty } type QueryEmpty{}"},
	"String":    {QueryString{}, "schema{ query:QueryString } type QueryString{ m:String! }"},
	"Int":       {QueryInt{}, "schema{ query:QueryInt } type QueryInt{ i:Int! }"},
	"IntString": {QueryIntString{}, "schema{ query:QueryIntString } type QueryIntString{ i:Int! s:String! }"},
	"Bool":      {QueryBool{}, "schema{ query:QueryBool } type QueryBool{ b:Boolean! }"},
	"Float":     {QueryFloat{}, "schema{ query:QueryFloat } type QueryFloat{ f:Float! }"},
	"Nested": {
		QueryNested{}, "schema{ query:QueryNested }" +
			"type QueryNested{ str:QueryString! } type QueryString{ m:String! }",
	},
	"TypeReuse": {
		QueryTypeReuse{}, "schema{ query:QueryTypeReuse }" +
			"type QueryString{ m:String! } type QueryTypeReuse{ q1:QueryString! q2:QueryString! }",
	},
	"QueryPtr": {
		QueryPtr{}, "schema{ query:QueryPtr }" +
			"type QueryInt{ i:Int! } type QueryPtr{ ptr:QueryInt! }",
	},
	"AnonNested": {
		QueryAnonNested{}, "schema{ query:QueryAnonNested }" +
			"type Anon{ b:Int! } type QueryAnonNested{ anon:Anon! }",
	},
	//"Slice":       {QuerySlice{}, "schema{ query:QuerySlice } type QuerySlice{ slice:[Int!]!}"}, // TODO make non-ptr non-nullable!
	"Slice":     {QuerySlice{}, "schema{ query:QuerySlice } type QuerySlice{ slice:[Int]! }"},
	"Map":       {QueryMap{}, "schema{ query:QueryMap } type QueryMap{ map:[Int]! }"},
	"Int Func":  {QueryIntFunc{}, "schema{ query:QueryIntFunc } type QueryIntFunc{ f:Int! }"},
	"BoolFunc":  {QueryBoolFunc{}, "schema{ query:QueryBoolFunc } type QueryBoolFunc{ f:Boolean! }"},
	"ErrorFunc": {QueryErrorFunc{}, "schema{ query:QueryErrorFunc } type QueryErrorFunc{ f:Int! }"},
	"FuncParam": {QueryFuncParam{}, "schema{ query:QueryFuncParam } type QueryFuncParam{ f(q:Float!):Int! }"},
	"FuncParam2": {
		QueryFuncParam2{}, "schema{ query:QueryFuncParam2 }" +
			"type QueryFuncParam2{ f(p1:String!,p2:Int!):Boolean! }",
	},
	"FuncDefault": {
		QueryFuncDefault{}, "schema{ query:QueryFuncDefault }" +
			"type QueryFuncDefault{ f(p1:String!,p2:Int!=42):Boolean! }",
	},
	"FuncDefault2": {
		QueryFuncDefault2{}, "schema{ query:QueryFuncDefault2 }" +
			" type QueryFuncDefault2{ f(p1:String!=\"a b\",p2:Float!=3.14):Boolean! }",
	},
	"ContextFunc": {QueryContextFunc{}, "schema{ query:QueryContextFunc } type QueryContextFunc{ f:Int! }"},
	"CustomName":  {QueryCustomName{}, "schema{ query:QueryCustomName } type QueryCustomName{ message:String! }"},
	"Unexported":  {QueryUnexported{}, "schema{ query:QueryUnexported } type QueryUnexported{ message:String! }"},
	"InputParam": {
		QueryInputParam{}, "schema{ query:QueryInputParam }" +
			"input InputInt{ i:Int! } type QueryInputParam{ f(in: InputInt!): Int! }",
	},
	"InputAnon": {
		QueryInputAnon{}, "schema{ query: QueryInputAnon }" +
			"input Anon{ j:Int! } type QueryInputAnon{ f(anon: Anon!): Boolean! }",
	},
	"Recurse": {QueryRecurse{}, "schema{ query:QueryRecurse } type QueryRecurse{ p:QueryRecurse }"},
	"Interface": {
		QueryInterface{},
		"schema{query:QueryInterface} interface IInt{i:Int!}" +
			"type M1 implements IInt{i:Int! s:String!} type M2 implements IInt{b:Boolean! i:Int!} type QueryInterface{a:M1! b:M2!}",
	},
	// Note allowing an interface to implement a (different) interface is a new feature of GraphQL (2020) but seems to work with eggql as is
	"IfaceOfIface": {
		QueryIfaceOfIface{},
		"schema{query:QueryIfaceOfIface} interface I2Int implements IInt {i:Int!} interface IInt {i:Int!}" +
			"type M3 implements IInt & I2Int {i:Int! x:Float! y:Float!} type QueryIfaceOfIface{xy:M3!}",
	},
	"IfaceRecurse": {
		QueryIfaceRecurse{},
		"schema{query:QueryIfaceRecurse} interface IRecurse{b:QueryIfaceRecurse} type QueryIfaceRecurse implements IRecurse{b:QueryIfaceRecurse}",
	},
	"IRecurseList": {
		QueryIRecurseList{},
		"schema{query:QueryIRecurseList} interface IRecurseList{list:[QueryIRecurseList]} " +
			"type QueryIRecurseList implements IRecurseList{list:[QueryIRecurseList]}",
	},
	"Interface2": {
		QueryInterface2{},
		"schema{query:QueryInterface2} interface Character {friends:[Character]! name:String!} type Person " +
			" implements Character{friends:[Character]! name:String! personality:String!} type QueryInterface2{hero:Character!}",
	},
	"SubscriptSlice": {
		QuerySubscriptSlice{},
		"schema{ query:QuerySubscriptSlice } type QuerySubscriptSlice{slice(id:Int!):String! }",
	},
	"SubscriptArray": {
		QuerySubscriptArray{},
		"schema{ query:QuerySubscriptArray } type QuerySubscriptArray{a(id:Int!):Boolean! }",
	},
	"SubscriptMap": {
		QuerySubscriptMap{},
		"schema{ query:QuerySubscriptMap } type QuerySubscriptMap{m(s:String!):Float! }",
	},
	"Union": {
		QueryUnion{},
		"schema{query:QueryUnion} type QueryUnion{a:U1! b:U2!} type U1{v:Int!} type U2{w:String!} union U = U1 | U2",
	},
	"Union2": {
		QueryUnion2{}, // TODO Null Prob? - should list be nullable if derived from slice, ie: s:[U] not s:[U]!
		"schema{query:QueryUnion2} type QueryUnion2{s:[U]!} type U1{v:Int!} type U2{w:String!} union U = U1 | U2",
	},
	"Desc0": {QueryDescOnly{}, `schema{query:QueryDescOnly} """ no fields""" type QueryDescOnly{}`},
	"DescObject": {
		QueryDescObject{},
		`schema{query:QueryDescObject} """ nested object""" type Nested{i:Int!} type QueryDescObject{nested:Nested!}`,
	},
	"DescInterface": {
		QueryDescInterface{},
		`schema{query:QueryDescInterface} """ interface""" interface IDesc {i:Int!} type QueryDescInterface implements IDesc {i:Int!} `,
	},
	"DescUnion": {
		QueryDescUnion{},
		`schema{query:QueryDescUnion}type QueryDescUnion{a:UDesc1! b:UDesc2!} type UDesc1{} type UDesc2{} """a union""" union UDesc=UDesc1|UDesc2`,
	},
	"DescField": {
		QueryDescField{},
		`schema{query:QueryDescField}type QueryDescField{""" Test of # for description""" i:Int!}`,
	},
	"DescOjectAndFields": {
		QueryDescAll{}, // TODO NULL prob? - last field's Ints should not be nullable t:[Int!] not t:[Int]!
		`schema{query:QueryDescAll} """q (type)""" type QueryDescAll{"""s (#1)""" s:String! """t (#2)""" t:[Int]!}`,
	},
	"DescArg": {
		struct {
			R1 func(int) string `egg:",args(p#arg 1)"`
		}{},
		`type Query{r1("""arg 1""" p:Int!):String!}`,
	},
	"DescArg2": {
		struct {
			R2 func(int, float64) string `egg:",args(intArg=1#int, floatArg=3.14#float)"`
		}{},
		`type Query{r2("""int""" intArg:Int!=1, """float""" floatArg:Float!=3.14):String!}`,
	},
	// custom scalar as field, as field list and as arg
	"CustomScalarReturn": {data: struct{ E Cust1 }{}, expected: "type Query{ e: Cust1! } scalar Cust1"},
	"CustomScalarList":   {data: struct{ E []Cust1 }{}, expected: "type Query{ e: [Cust1]! } scalar Cust1"},
	"CustomScalarArg": {
		data: struct {
			F func(Cust1) string `egg:",args(i)"`
		}{},
		expected: "type Query{ f(i:Cust1!): String! } scalar Cust1",
	},
}

func TestBuildSchema(t *testing.T) {
	for name, data := range testData {
		exp := RemoveWhiteSpace(t, data.expected)
		out := RemoveWhiteSpace(t, schema.MustBuild(data.data))
		same := out == exp
		where := ""
		if !same {
			// Failing case - find the offset of the first different byte to help debug where the problem is
			for i := range out {
				if i >= len(exp) || out[i] != exp[i] {
					where = "\nwhere first difference is at character " + strconv.Itoa(i) + " of " + strconv.Itoa(len(exp))
					break
				}
			}
		}

		Assertf(t, same, "TestBuildSchema: %12s: make schema expected %q got %q%s", name, exp, out, where)
	}
}

// Assertf writes a tick or cross (depending on the status of a value that is asserted during tests), followed
// by a message (with parameters - printf style).  This allows the result of a test run to be quickly scanned to
// see which tests passed and which failed.  Note that all messages are printed (to stderr) if any test fails or
// if the -v (verbose) test flag is used.  If all tests pass then no messages are printed (unless -v is used).
func Assertf(t *testing.T, succeeded bool, format string, args ...interface{}) {
	const (
		succeed = "\u2713" // tick
		failed  = "X"      //"\u2717" // cross
	)

	t.Helper()
	if !succeeded {
		t.Errorf("%s\t"+format, append([]interface{}{failed}, args...)...)
	} else {
		t.Logf("%s\t"+format, append([]interface{}{succeed}, args...)...)
	}
}

// RemoveWhiteSpace is used to compare GraphQL schemas (text) without having to worry about whitespace issues.
// It returns it's input string but with unnecessary whitespace removed.  If a whitespace sequence separates "words"
// (keywords, identifiers, numbers etc) it is replaced with a single space to avoid words being merged together.
func RemoveWhiteSpace(t *testing.T, s string) string {
	type JustSeen int8
	const (
		Normal JustSeen = iota
		AlNum
		Space
	)

	t.Helper()
	var b strings.Builder
	b.Grow(len(s))
	var last JustSeen
	for _, c := range s {
		if unicode.IsSpace(c) {
			if last == AlNum {
				last = Space
			}
			continue
		}

		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			if last == Space {
				// add one space for whitespace that had alphanumerics before and after
				b.WriteByte(' ')
			}
			last = AlNum
		} else {
			last = Normal
		}
		b.WriteRune(c)
	}
	return b.String()
}
