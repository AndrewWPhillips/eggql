package schema_test

import (
	"context"
	"github.com/andrewwphillips/eggql/internal/schema"
	"strconv"
	"strings"
	"testing"
	"unicode"
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

	QueryNullable struct {
		I int `graphql:",nullable"` // specify that field can be null
	}
	QuerySlice     struct{ Slice []int }
	QueryIntFunc   struct{ F func() int }
	QueryBoolFunc  struct{ F func() bool }
	QueryErrorFunc struct{ F func() (int, error) }
	QueryFuncParam struct {
		F func(float64) int `graphql:",params(q)"`
	}
	QueryFuncParam2 struct {
		F func(string, int) bool `graphql:",params( p1, p2 )"`
	}
	QueryFuncDefault struct {
		F func(string, int) bool `graphql:",params(p1,p2=42)"`
	}
	QueryFuncDefault2 struct {
		F func(string, float64) bool `graphql:",params(p1=\"a b\",p2=3.14)"`
	}
	QueryContextFunc struct {
		F func(context.Context) (int, error)
	}
	QueryCustomName struct {
		M string `graphql:"message"` // specify GraphQL query name
	}
	QueryUnexported struct {
		m1 string `graphql:"message"` // unexported field - tag should be ignored
		M2 string `graphql:"message"`
	}

	InputInt        struct{ I int }
	QueryInputParam struct {
		F func(InputInt) int `graphql:",params(in)"`
	}
	QueryInputAnon struct {
		F func(struct{ J int }) bool `graphql:",params(anon)"`
	}
	QueryRecurse struct {
		P *QueryRecurse // recursive data structure: P is (ptr to) type of enclosed struct
	}

	IInt struct{ I int }
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
	IRecurseList2 struct {
		List *[]IRecurseList2
	}
	QueryIRecurseList2 struct {
		IRecurseList2
	}
)

var testData = map[string]struct {
	data     interface{}
	expected string
}{
	"List":      {QueryList{}, "schema{ query:QueryList } type QueryList{ list:[Int]! }"},
	"List2":     {QueryList2{}, "schema{query:QueryList2} type QueryList2{list:[QueryString]!} type QueryString{m:String!}"},
	"Empty":     {QueryEmpty{}, "schema{ query:QueryEmpty } type QueryEmpty{}"},
	"String":    {QueryString{}, "schema{ query:QueryString } type QueryString{ m:String! }"},
	"Int":       {QueryInt{}, "schema{ query:QueryInt } type QueryInt{ i:Int! }"},
	"IntString": {QueryIntString{}, "schema{ query:QueryIntString } type QueryIntString{ i:Int! s:String! }"},
	"Bool":      {QueryBool{}, "schema{ query:QueryBool } type QueryBool{ b:Boolean! }"},
	"Float":     {QueryFloat{}, "schema{ query:QueryFloat } type QueryFloat{ f:Float! }"},
	"Nested": {QueryNested{}, "schema{ query:QueryNested }" +
		"type QueryNested{ str:QueryString! } type QueryString{ m:String! }"},
	"TypeReuse": {QueryTypeReuse{}, "schema{ query:QueryTypeReuse }" +
		"type QueryString{ m:String! } type QueryTypeReuse{ q1:QueryString! q2:QueryString! }"},
	"QueryPtr": {QueryPtr{}, "schema{ query:QueryPtr }" +
		"type QueryInt{ i:Int! } type QueryPtr{ ptr:QueryInt! }"},
	"AnonNested": {QueryAnonNested{}, "schema{ query:QueryAnonNested }" +
		"type Anon{ b:Int! } type QueryAnonNested{ anon:Anon! }"},
	"Nullable": {QueryNullable{}, "schema{ query:QueryNullable } type QueryNullable{ i:Int }"},
	//"Slice":       {QuerySlice{}, "schema{ query:QuerySlice } type QuerySlice{ slice:[Int!]!}"}, // TODO make non-ptr non-nullable!
	"Slice":     {QuerySlice{}, "schema{ query:QuerySlice } type QuerySlice{ slice:[Int]! }"},
	"Int Func":  {QueryIntFunc{}, "schema{ query:QueryIntFunc } type QueryIntFunc{ f:Int! }"},
	"BoolFunc":  {QueryBoolFunc{}, "schema{ query:QueryBoolFunc } type QueryBoolFunc{ f:Boolean! }"},
	"ErrorFunc": {QueryErrorFunc{}, "schema{ query:QueryErrorFunc } type QueryErrorFunc{ f:Int! }"},
	"FuncParam": {QueryFuncParam{}, "schema{ query:QueryFuncParam } type QueryFuncParam{ f(q:Float!):Int! }"},
	"FuncParam2": {QueryFuncParam2{}, "schema{ query:QueryFuncParam2 }" +
		"type QueryFuncParam2{ f(p1:String!,p2:Int!):Boolean! }"},
	"FuncDefault": {QueryFuncDefault{}, "schema{ query:QueryFuncDefault }" +
		"type QueryFuncDefault{ f(p1:String!,p2:Int!=42):Boolean! }"},
	"FuncDefault2": {QueryFuncDefault2{}, "schema{ query:QueryFuncDefault2 }" +
		" type QueryFuncDefault2{ f(p1:String!=\"a b\",p2:Float!=3.14):Boolean! }"},
	"ContextFunc": {QueryContextFunc{}, "schema{ query:QueryContextFunc } type QueryContextFunc{ f:Int! }"},
	"CustomName":  {QueryCustomName{}, "schema{ query:QueryCustomName } type QueryCustomName{ message:String! }"},
	"Unexported":  {QueryUnexported{}, "schema{ query:QueryUnexported } type QueryUnexported{ message:String! }"},
	"InputParam": {QueryInputParam{}, "schema{ query:QueryInputParam }" +
		"input InputInt{ i:Int! } type QueryInputParam{ f(in: InputInt!): Int! }"},
	"InputAnon": {QueryInputAnon{}, "schema{ query: QueryInputAnon }" +
		"input Anon{ j:Int! } type QueryInputAnon{ f(anon: Anon!): Boolean! }"},
	"Recurse": {QueryRecurse{}, "schema{ query:QueryRecurse } type QueryRecurse{ p:QueryRecurse }"},
	"Interface": {QueryInterface{},
		"schema{query:QueryInterface} interface IInt{i:Int!} type M1 implements IInt{i:Int! s:String!} type M2 implements IInt{b:Boolean! i:Int!} type QueryInterface{a:M1! b:M2!}"},
	"IfaceRecurse": {QueryIfaceRecurse{},
		"schema{query:QueryIfaceRecurse} interface IRecurse{b:QueryIfaceRecurse} type QueryIfaceRecurse implements IRecurse{b:QueryIfaceRecurse}"},
	"IRecurseList": {QueryIRecurseList{},
		"schema{query:QueryIRecurseList} interface IRecurseList{list:[QueryIRecurseList]} type QueryIRecurseList implements IRecurseList{list:[QueryIRecurseList]}"},
	"IRecurseList2": {QueryIRecurseList2{},
		"schema{query:QueryIRecurseList2} interface IRecurseList2{list:[IRecurseList2]} type QueryIRecurseList2 implements IRecurseList2{list:[IRecurseList2]}"},
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
// by a message (with params - printf style).  This allows the result of a test run to be quickly scanned to
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
